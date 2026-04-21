// Package ticketing implements core ticket decision handling.
//
// The approval workflow remains provider-owned. This package only owns the
// canonical ticket state transition and execution side-effects after a provider
// reaches a final decision.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/governance/ticketing
package ticketing

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	entbatchticket "kv-shepherd.io/shepherd/ent/batchticket"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/notification"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider/vmmutationplan"
	"kv-shepherd.io/shepherd/internal/service"
)

// ExecutionOptions carries the canonical execution inputs attached to an
// approved ticket decision (ADR-0017 + Stage 5.B).
type ExecutionOptions struct {
	ClusterID     string
	StorageClass  string
	DVAccessModes []string
	DVVolumeMode  string

	// Resource override fields (master-flow Stage 5.B).
	EnableOverride  bool
	CPURequest      float64
	CPULimit        float64
	MemoryRequestGi float64
	MemoryLimitGi   float64
	DiskGB          int
}

type approveCreateConfig struct {
	skipClonePreflight bool
}

// AtomicDecisionWriter defines ADR-0012 atomic write operations for final
// ticket decisions.
type AtomicDecisionWriter interface {
	ApproveCreateAndEnqueue(
		ctx context.Context,
		ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
		templateSnapshot map[string]interface{},
		instanceSizeSnapshot map[string]interface{},
		placementEvaluation map[string]interface{},
		modifiedSpec map[string]interface{},
	) (vmID, vmName string, err error)
	ApproveDeleteAndEnqueue(ctx context.Context, ticketID, eventID, approver, vmID string) error
	ApproveModifyAndEnqueue(ctx context.Context, ticketID, eventID, approver string, modifiedSpec map[string]interface{}) error
	ApprovePowerAndEnqueue(ctx context.Context, ticketID, eventID, approver, operation string) error
}

// Service orchestrates canonical ticket decisions.
type Service struct {
	client       *ent.Client
	auditLogger  *audit.Logger
	validator    *service.ApprovalValidator
	atomicWriter AtomicDecisionWriter
	notifier     *notification.Triggers // Optional: nil-safe for backward compatibility
	vmService    *service.VMService     // Optional: nil-safe; enables DryRun Pre-flight Gate (ADR-0006 Addendum)
}

// NewService creates a new ticket decision service.
func NewService(client *ent.Client, auditLogger *audit.Logger, atomicWriter AtomicDecisionWriter) *Service {
	return &Service{
		client:       client,
		auditLogger:  auditLogger,
		validator:    service.NewApprovalValidator(client),
		atomicWriter: atomicWriter,
	}
}

// SetNotifier configures the notification trigger service.
// This is a setter to avoid breaking the existing constructor signature.
func (g *Service) SetNotifier(notifier *notification.Triggers) {
	g.notifier = notifier
}

// SetVMService injects a VMService for DryRun Pre-flight Gate validation.
// Must be called after both Service and VMService are initialized.
// When vmService is nil (default), the DryRun gate is skipped (backward compatible).
// Returns g for method chaining.
func (g *Service) SetVMService(svc *service.VMService) *Service {
	g.vmService = svc
	if g.validator != nil {
		g.validator.SetVMService(svc)
	}
	return g
}

// Approve approves a pending ticket. Admin-determined fields set here (ADR-0017).
// ADR-0012: ticket/domain/vm writes and River enqueue are committed atomically.
//
// Branching logic by operation_type:
//   - CREATE: ticket APPROVED + VM record CREATING → enqueue VMCreateArgs
//   - DELETE: ticket APPROVED + VM status DELETING → enqueue VMDeleteArgs
func (g *Service) Approve(ctx context.Context, ticketID, approver string, opts ExecutionOptions) error {
	ticket, err := g.client.Ticket.Get(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != entticket.StatusPENDING {
		return fmt.Errorf("ticket %s is not pending (current: %s)", ticketID, ticket.Status)
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}
	isBatchParent, err := g.isBatchParentTicket(ctx, ticket, event)
	if err != nil {
		return fmt.Errorf("resolve batch parent ticket %s: %w", ticketID, err)
	}
	if isBatchParent {
		return g.approveBatchParent(ctx, ticket, event, approver, opts)
	}

	// Branch by operation_type (ADR-0015 §5.D).
	switch ticket.OperationType {
	case entticket.OperationTypeMODIFY:
		return g.approveModify(ctx, ticket, event, ticketID, approver, opts)
	case entticket.OperationTypeDELETE:
		return g.approveDelete(ctx, ticket, ticketID, approver)
	case entticket.OperationTypePOWER:
		return g.approvePower(ctx, ticket, event, ticketID, approver)
	case entticket.OperationTypeVNC_ACCESS:
		return g.approveVNC(ctx, ticket, event, ticketID, approver)
	default:
		// CREATE is the default operation type.
		return g.approveCreate(ctx, ticket, ticketID, approver, opts)
	}
}

// approvePower handles approval of POWER tickets.
func (g *Service) approvePower(ctx context.Context, ticket *ent.Ticket, event *ent.DomainEvent, ticketID, approver string) error {
	if event == nil {
		return fmt.Errorf("power approval requires domain event")
	}

	var payload domain.VMPowerPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse power payload for ticket %s: %w", ticketID, err)
	}
	operation := strings.TrimSpace(strings.ToLower(payload.Operation))
	switch operation {
	case "start", "stop", "restart":
	default:
		return fmt.Errorf("ticket %s has unsupported power operation %q", ticketID, payload.Operation)
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	if err := g.atomicWriter.ApprovePowerAndEnqueue(ctx, ticketID, ticket.EventID, approver, operation); err != nil {
		return fmt.Errorf("approve power ticket %s atomically: %w", ticketID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "power_approved", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.Actor, approver)
	}

	logger.Info("POWER ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
		zap.String("operation", operation),
		zap.String("event_id", ticket.EventID),
	)
	return nil
}

// approveCreate handles approval of CREATE tickets (original flow).
func (g *Service) approveCreate(ctx context.Context, ticket *ent.Ticket, ticketID, approver string, opts ExecutionOptions) error {
	return g.approveCreateWithConfig(ctx, ticket, ticketID, approver, opts, approveCreateConfig{})
}

func (g *Service) approveCreateWithConfig(
	ctx context.Context,
	ticket *ent.Ticket,
	ticketID, approver string,
	opts ExecutionOptions,
	config approveCreateConfig,
) error {
	if opts.ClusterID == "" {
		return fmt.Errorf("selected cluster is required for create approval")
	}
	resolvedOpts := opts

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}

	payload, err := parseVMCreatePayload(event.Payload)
	if err != nil {
		return fmt.Errorf("parse create payload for ticket %s: %w", ticketID, err)
	}
	effectiveTemplateID, effectiveInstanceSizeID := resolveEffectiveSelectionIDs(
		payload.TemplateID,
		payload.InstanceSizeID,
		ticket.ModifiedSpec,
	)
	if effectiveTemplateID == "" {
		return fmt.Errorf("effective template id is empty for ticket %s", ticketID)
	}
	if effectiveInstanceSizeID == "" {
		return fmt.Errorf("effective instance size id is empty for ticket %s", ticketID)
	}

	// Load template and instance size once — shared by placement validation, DryRun gate,
	// overcommit validation, and snapshot building below.
	templateEntity, err := g.client.Template.Get(ctx, effectiveTemplateID)
	if err != nil {
		return fmt.Errorf("get template %s for ticket %s: %w", effectiveTemplateID, ticketID, err)
	}
	instanceSizeEntity, err := g.client.InstanceSize.Get(ctx, effectiveInstanceSizeID)
	if err != nil {
		return fmt.Errorf("get instance size %s for ticket %s: %w", effectiveInstanceSizeID, ticketID, err)
	}

	var placementEvaluation map[string]interface{}
	if g.validator != nil {
		evaluation, evalErr := g.validator.EvaluateClusterPlacement(ctx, service.ApprovalValidationInput{
			ClusterID:      opts.ClusterID,
			TemplateID:     effectiveTemplateID,
			InstanceSizeID: effectiveInstanceSizeID,
			Namespace:      payload.Namespace,
			StorageClass:   opts.StorageClass,
			DVAccessModes:  opts.DVAccessModes,
			DVVolumeMode:   opts.DVVolumeMode,
			Override:       buildCreateApprovalOverride(payload, instanceSizeEntity, opts),
		})
		if evalErr != nil {
			return fmt.Errorf("approval validation failed for ticket %s: %w", ticketID, evalErr)
		}
		if evaluation != nil && evaluation.RootVolumeResolution != nil {
			if effectiveStorageClass := strings.TrimSpace(evaluation.RootVolumeResolution.EffectiveStorageClass); effectiveStorageClass != "" {
				resolvedOpts.StorageClass = effectiveStorageClass
			}
			resolvedOpts.DVAccessModes = cloneStringSlice(evaluation.RootVolumeResolution.EffectiveAccessModes)
			resolvedOpts.DVVolumeMode = strings.TrimSpace(evaluation.RootVolumeResolution.EffectiveVolumeMode)
		}
		placementEvaluation = buildPlacementEvaluationSnapshot(evaluation, opts, resolvedOpts)
		if evaluation != nil && !evaluation.Eligible {
			if isSelectedClusterRuntimeUnavailable(evaluation) {
				logApprovalPreflightDegraded(ticketID, opts.ClusterID, "cluster_placement", stderrors.New(evaluation.ReasonMessage))
			} else {
				if g.auditLogger != nil {
					_ = g.auditLogger.LogApprovalWithDetails(ctx, ticketID, "validation_failed", approver, map[string]interface{}{
						"placement_evaluation": placementEvaluation,
					})
				}
				return fmt.Errorf(
					"approval validation failed for ticket %s: %w",
					ticketID,
					apperrors.BadRequest(evaluation.ReasonCode, evaluation.ReasonMessage),
				)
			}
		}
	}

	targetDiskGB := instanceSizeEntity.DiskGB
	if opts.EnableOverride && opts.DiskGB > 0 {
		targetDiskGB = opts.DiskGB
	}
	effectiveSourceType := service.EffectiveTemplateSourceType(
		templateEntity.SourceType,
		templateEntity.ImageURL,
		templateEntity.PvcName,
	)
	if g.vmService != nil && effectiveSourceType == service.TemplateSourceCDIPVCClone && !config.skipClonePreflight {
		targetStorageClass := strings.TrimSpace(resolvedOpts.StorageClass)
		if targetStorageClass == "" {
			selectedCluster, clusterErr := g.client.Cluster.Get(ctx, opts.ClusterID)
			if clusterErr != nil {
				return fmt.Errorf("get cluster %s for ticket %s: %w", opts.ClusterID, ticketID, clusterErr)
			}
			targetStorageClass = strings.TrimSpace(selectedCluster.DefaultStorageClass)
		}

		if preflightErr := g.vmService.ValidatePVCCloneSource(
			ctx,
			opts.ClusterID,
			payload.Namespace,
			templateEntity.PvcNamespace,
			templateEntity.PvcName,
			targetDiskGB,
			targetStorageClass,
		); preflightErr != nil {
			if isClusterRuntimeUnavailable(preflightErr) {
				logApprovalPreflightDegraded(ticketID, opts.ClusterID, "clone_source_preflight", preflightErr)
			} else {
				return fmt.Errorf("source pvc preflight failed for ticket %s: %w", ticketID, preflightErr)
			}
		}
	}

	// ── DryRun Pre-flight Gate (ADR-0006 Addendum) ──────────────────────────────
	// At this point: opts.ClusterID is known (admin-selected), so we can invoke a
	// real K8s server-side DryRun. Per K8s best practices, DryRun traverses the
	// full admission chain (schema + mutating + validating webhooks) without
	// persisting. This is the most authoritative available pre-flight check.
	//
	// Gate failure → return synchronous error; do NOT enqueue River job.
	// Gate unavailable (vmService nil) → skip silently (backward compatible).
	if g.vmService != nil {
		dryRunSpec, buildSpecErr := g.buildDryRunSpec(payload, templateEntity, instanceSizeEntity, resolvedOpts)
		if buildSpecErr != nil {
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("build dryrun spec for ticket %s: %v", ticketID, buildSpecErr))
		}
		result, validateErr := g.vmService.ValidateAndPrepare(ctx, opts.ClusterID, payload.Namespace, dryRunSpec)
		switch {
		case validateErr != nil && isClusterRuntimeUnavailable(validateErr):
			logApprovalPreflightDegraded(ticketID, opts.ClusterID, "create_dryrun", validateErr)
		case validateErr != nil:
			return fmt.Errorf("pre-flight dryrun gate failed for ticket %s: %w", ticketID, validateErr)
		case !result.Valid:
			return apperrors.BadRequest(apperrors.CodeValidationFailed,
				fmt.Sprintf("vm spec rejected by cluster %s for ticket %s: %s",
					opts.ClusterID, ticketID, strings.Join(result.Errors, "; ")))
		default:
			logger.Info("pre-flight dryrun passed",
				zap.String("ticket_id", ticketID),
				zap.String("cluster_id", opts.ClusterID),
			)
		}
	}
	// ── End DryRun Pre-flight Gate ────────────────────────────────────────────────

	// Stage 5.B: If admin enabled resource override, validate overcommit constraints.
	if opts.EnableOverride {
		// Guard: at least one override field must be non-zero to avoid a no-op override.
		if opts.CPULimit == 0 && opts.CPURequest == 0 && opts.MemoryLimitGi == 0 && opts.MemoryRequestGi == 0 && opts.DiskGB == 0 {
			return fmt.Errorf("enable_override is true but all resource override values are zero for ticket %s", ticketID)
		}

		cpuCores := opts.CPULimit
		cpuRequest := opts.CPURequest
		memoryGi := opts.MemoryLimitGi
		memoryRequestGi := opts.MemoryRequestGi

		// Reuse instanceSizeEntity loaded above (no extra DB round-trip).
		if overcommitErr := service.ValidateOvercommit(cpuCores, cpuRequest, memoryGi, memoryRequestGi, instanceSizeEntity.DedicatedCPU); overcommitErr != nil {
			return fmt.Errorf("resource override validation for ticket %s: %w", ticketID, overcommitErr)
		}
	}

	templateSnapshot := buildTemplateSnapshot(templateEntity)
	instanceSizeSnapshot := buildInstanceSizeSnapshot(instanceSizeEntity)
	if placementEvaluation != nil {
		instanceSizeSnapshot = applyResolvedRootVolumeToInstanceSizeSnapshot(instanceSizeSnapshot, placementEvaluation)
	}
	modifiedSpec := cloneMap(ticket.ModifiedSpec)
	applyRequestedCreateTargets(modifiedSpec, payload, instanceSizeEntity)

	// Merge admin resource overrides into modifiedSpec (Stage 5.B).
	if opts.EnableOverride {
		modifiedSpec["enable_override"] = true
		if opts.CPURequest > 0 {
			modifiedSpec["cpu_request"] = opts.CPURequest
		}
		if opts.CPULimit > 0 {
			modifiedSpec["cpu_limit"] = opts.CPULimit
		}
		if opts.MemoryRequestGi > 0 {
			modifiedSpec["memory_request_gi"] = opts.MemoryRequestGi
		}
		if opts.MemoryLimitGi > 0 {
			modifiedSpec["memory_limit_gi"] = opts.MemoryLimitGi
		}
		if opts.DiskGB > 0 {
			modifiedSpec["disk_gb"] = opts.DiskGB
		}
	} else if opts.DiskGB > 0 {
		// Disk size can be adjusted even without full override.
		modifiedSpec["disk_gb"] = opts.DiskGB
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}

	vmID, vmName, err := g.atomicWriter.ApproveCreateAndEnqueue(
		ctx,
		ticketID,
		ticket.EventID,
		approver,
		opts.ClusterID,
		resolvedOpts.StorageClass,
		payload.ServiceID,
		payload.Namespace,
		payload.RequesterID,
		templateSnapshot,
		instanceSizeSnapshot,
		placementEvaluation,
		modifiedSpec,
	)
	if err != nil {
		return fmt.Errorf("approve create ticket %s atomically: %w", ticketID, err)
	}

	// Audit log (best-effort, outside transaction).
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApprovalWithDetails(ctx, ticketID, "approved", approver, map[string]interface{}{
			"placement_evaluation": placementEvaluation,
		})
	}

	// Notification trigger: APPROVAL_COMPLETED → notify requester (master-flow.md Stage 5.F).
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.RequesterID, approver)
	}

	logger.Info("CREATE ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", vmID),
		zap.String("vm_name", vmName),
		zap.String("event_id", ticket.EventID),
		zap.Bool("resource_override", opts.EnableOverride),
	)

	return nil
}

func (g *Service) preflightCreateCloneSource(ctx context.Context, ticket *ent.Ticket, opts ExecutionOptions) error {
	if g == nil || g.vmService == nil || ticket == nil || strings.TrimSpace(opts.ClusterID) == "" {
		return nil
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}

	payload, err := parseVMCreatePayload(event.Payload)
	if err != nil {
		return fmt.Errorf("parse create payload for ticket %s: %w", ticket.ID, err)
	}

	effectiveTemplateID, effectiveInstanceSizeID := resolveEffectiveSelectionIDs(
		payload.TemplateID,
		payload.InstanceSizeID,
		ticket.ModifiedSpec,
	)
	if effectiveTemplateID == "" {
		return fmt.Errorf("effective template id is empty for ticket %s", ticket.ID)
	}
	if effectiveInstanceSizeID == "" {
		return fmt.Errorf("effective instance size id is empty for ticket %s", ticket.ID)
	}

	templateEntity, err := g.client.Template.Get(ctx, effectiveTemplateID)
	if err != nil {
		return fmt.Errorf("get template %s for ticket %s: %w", effectiveTemplateID, ticket.ID, err)
	}
	if service.EffectiveTemplateSourceType(
		templateEntity.SourceType,
		templateEntity.ImageURL,
		templateEntity.PvcName,
	) != service.TemplateSourceCDIPVCClone {
		return nil
	}

	instanceSizeEntity, err := g.client.InstanceSize.Get(ctx, effectiveInstanceSizeID)
	if err != nil {
		return fmt.Errorf("get instance size %s for ticket %s: %w", effectiveInstanceSizeID, ticket.ID, err)
	}
	targetDiskGB := instanceSizeEntity.DiskGB
	if opts.EnableOverride && opts.DiskGB > 0 {
		targetDiskGB = opts.DiskGB
	}

	targetStorageClass := strings.TrimSpace(opts.StorageClass)
	if targetStorageClass == "" {
		selectedCluster, clusterErr := g.client.Cluster.Get(ctx, opts.ClusterID)
		if clusterErr != nil {
			return fmt.Errorf("get cluster %s for ticket %s: %w", opts.ClusterID, ticket.ID, clusterErr)
		}
		targetStorageClass = strings.TrimSpace(selectedCluster.DefaultStorageClass)
	}

	if err := g.vmService.ValidatePVCCloneSource(
		ctx,
		opts.ClusterID,
		payload.Namespace,
		templateEntity.PvcNamespace,
		templateEntity.PvcName,
		targetDiskGB,
		targetStorageClass,
	); err != nil {
		if isClusterRuntimeUnavailable(err) {
			logApprovalPreflightDegraded(ticket.ID, opts.ClusterID, "batch_clone_source_preflight", err)
			return nil
		}
		return fmt.Errorf("source pvc preflight failed for ticket %s: %w", ticket.ID, err)
	}

	return nil
}

func isSelectedClusterRuntimeUnavailable(evaluation *service.ClusterCompatibilityResult) bool {
	return evaluation != nil &&
		evaluation.Cluster != nil &&
		evaluation.Cluster.Status != entcluster.StatusHEALTHY
}

func isClusterRuntimeUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if appErr, ok := apperrors.IsAppError(err); ok {
		return appErr.HTTPStatus == 503 || appErr.Code == apperrors.CodeClusterUnhealthy
	}
	if stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if stderrors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsUnexpectedServerError(err) ||
		apierrors.IsTooManyRequests(err) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"not healthy",
		"apiserver unreachable",
		"kubeconfig is empty",
		"no configuration has been provided",
		"invalid configuration",
		"connection refused",
		"dial tcp",
		"i/o timeout",
		"tls handshake timeout",
		"no such host",
		"server misbehaving",
		"client.timeout exceeded",
		"context deadline exceeded",
		"x509:",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func logApprovalPreflightDegraded(ticketID, clusterID, stage string, err error) {
	logger.Warn("approval preflight degraded due to cluster runtime unavailability",
		zap.String("ticket_id", ticketID),
		zap.String("cluster_id", strings.TrimSpace(clusterID)),
		zap.String("stage", stage),
		zap.Error(err),
	)
}

func buildPlacementEvaluationSnapshot(
	evaluation *service.ClusterCompatibilityResult,
	requested ExecutionOptions,
	effective ExecutionOptions,
) map[string]interface{} {
	if evaluation == nil || evaluation.Cluster == nil {
		return nil
	}
	clusterEntity := evaluation.Cluster
	effectiveStorageClass := strings.TrimSpace(effective.StorageClass)
	if effectiveStorageClass == "" {
		effectiveStorageClass = strings.TrimSpace(clusterEntity.DefaultStorageClass)
	}

	snapshot := map[string]interface{}{
		"selected_cluster_id":          clusterEntity.ID,
		"selected_cluster_name":        clusterEntity.Name,
		"selected_cluster_environment": string(clusterEntity.Environment),
		"requested_storage_class":      strings.TrimSpace(requested.StorageClass),
		"effective_storage_class":      effectiveStorageClass,
		"eligible":                     evaluation.Eligible,
		"evaluated_at":                 time.Now().UTC().Format(time.RFC3339),
	}
	if len(requested.DVAccessModes) > 0 {
		snapshot["requested_dv_access_modes"] = cloneStringSlice(requested.DVAccessModes)
	}
	if strings.TrimSpace(requested.DVVolumeMode) != "" {
		snapshot["requested_dv_volume_mode"] = strings.TrimSpace(requested.DVVolumeMode)
	}
	if len(effective.DVAccessModes) > 0 {
		snapshot["effective_dv_access_modes"] = cloneStringSlice(effective.DVAccessModes)
	}
	if strings.TrimSpace(effective.DVVolumeMode) != "" {
		snapshot["effective_dv_volume_mode"] = strings.TrimSpace(effective.DVVolumeMode)
	}
	if evaluation.RootVolumeResolution != nil {
		snapshot["root_volume_resolution_state"] = evaluation.RootVolumeResolution.State
		if evaluation.RootVolumeResolution.Message != "" {
			snapshot["root_volume_resolution_message"] = evaluation.RootVolumeResolution.Message
		}
		if len(evaluation.RootVolumeResolution.ModeOptions) > 0 {
			snapshot["root_volume_mode_options"] = cloneClaimPropertySets(evaluation.RootVolumeResolution.ModeOptions)
		}
	}
	if evaluation.ReasonCode != "" {
		snapshot["reason_code"] = evaluation.ReasonCode
	}
	if evaluation.ReasonMessage != "" {
		snapshot["reason_message"] = evaluation.ReasonMessage
	}
	if evaluation.AdvisoryCode != "" {
		snapshot["advisory_code"] = evaluation.AdvisoryCode
	}
	if evaluation.AdvisoryMessage != "" {
		snapshot["advisory_message"] = evaluation.AdvisoryMessage
	}
	if requested.EnableOverride {
		override := map[string]interface{}{
			"enabled": true,
		}
		if requested.CPURequest > 0 {
			override["cpu_request"] = requested.CPURequest
		}
		if requested.CPULimit > 0 {
			override["cpu_limit"] = requested.CPULimit
		}
		if requested.MemoryRequestGi > 0 {
			override["memory_request_gi"] = requested.MemoryRequestGi
		}
		if requested.MemoryLimitGi > 0 {
			override["memory_limit_gi"] = requested.MemoryLimitGi
		}
		if requested.DiskGB > 0 {
			override["disk_gb"] = requested.DiskGB
		}
		snapshot["override"] = override
	}
	return snapshot
}

// approveModify handles approval of MODIFY tickets.
func (g *Service) approveModify(
	ctx context.Context,
	ticket *ent.Ticket,
	event *ent.DomainEvent,
	ticketID, approver string,
	opts ExecutionOptions,
) error {
	if event == nil {
		return fmt.Errorf("modify approval requires domain event")
	}

	var payload domain.VMModifyPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse modify event payload: %w", err)
	}
	overrideCPURequest, overrideMemoryRequest, modifySpec, validationErr := buildModifyApprovalSpec(&payload, opts)
	if validationErr != nil {
		return validationErr
	}

	canRunRuntimeChecks := g.vmService != nil
	if canRunRuntimeChecks {
		liveVM, err := g.vmService.GetVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
		if err != nil {
			if isClusterRuntimeUnavailable(err) {
				logApprovalPreflightDegraded(ticketID, payload.ClusterID, "modify_live_vm_lookup", err)
				canRunRuntimeChecks = false
			} else {
				return fmt.Errorf("load live vm for modify approval %s: %w", ticketID, err)
			}
		}
		if canRunRuntimeChecks && liveVM == nil {
			return fmt.Errorf("live vm is unavailable for modify approval %s", ticketID)
		}
		if canRunRuntimeChecks {
			plan, err := vmmutationplan.PlanVMResourceUpdatePatch(payload.Namespace, liveVM, vmmutationplan.VMLiveUpdateTargets{
				CPUCores:        payload.TargetCPUCores,
				MemoryGi:        payload.TargetMemoryGi,
				DiskGB:          payload.TargetDiskGB,
				CPURequest:      overrideCPURequest,
				MemoryRequestGi: overrideMemoryRequest,
			})
			if err != nil {
				return apperrors.BadRequest(
					"VM_MODIFY_APPROVAL_INVALID",
					fmt.Sprintf("modify request cannot be executed with the current VM state: %v", err),
				)
			}
			if err := g.vmService.DryRunVMMutation(ctx, payload.ClusterID, payload.Namespace, payload.VMName, plan.Mutation); err != nil {
				if isClusterRuntimeUnavailable(err) {
					logApprovalPreflightDegraded(ticketID, payload.ClusterID, "modify_dryrun", err)
				} else {
					return apperrors.BadRequest(
						"VM_MODIFY_APPROVAL_INVALID",
						fmt.Sprintf("modify request cannot be executed with the current VM state: %v", err),
					)
				}
			}
			modifySpec = withApprovedVMMutation(modifySpec, plan)
		}
	}
	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	if err := g.atomicWriter.ApproveModifyAndEnqueue(ctx, ticketID, ticket.EventID, approver, modifySpec); err != nil {
		return fmt.Errorf("approve modify ticket %s atomically: %w", ticketID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "modify_approved", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.Actor, approver)
	}

	logger.Info("MODIFY ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
		zap.String("event_id", ticket.EventID),
	)

	return nil
}

func buildModifyApprovalSpec(
	payload *domain.VMModifyPayload,
	opts ExecutionOptions,
) (overrideCPURequest, overrideMemoryRequest *float64, modifiedSpec map[string]interface{}, err error) {
	if payload == nil {
		return nil, nil, nil, fmt.Errorf("modify payload is required")
	}

	targetCPULimit := payload.CurrentCPUCores
	if payload.TargetCPUCores != nil && *payload.TargetCPUCores > 0 {
		targetCPULimit = *payload.TargetCPUCores
	}
	targetMemoryLimitGi := payload.CurrentMemoryGi
	if payload.TargetMemoryGi != nil && *payload.TargetMemoryGi > 0 {
		targetMemoryLimitGi = *payload.TargetMemoryGi
	}

	effectiveCPURequest := payload.CurrentCPURequest
	effectiveMemoryRequest := payload.CurrentMemoryRequestGi
	modifiedSpec = map[string]interface{}{}

	if opts.EnableOverride {
		modifiedSpec["enable_override"] = true
		if opts.CPURequest > 0 {
			effectiveCPURequest = opts.CPURequest
			overrideCPURequest = &effectiveCPURequest
			modifiedSpec["cpu_request"] = opts.CPURequest
		}
		if opts.MemoryRequestGi > 0 {
			effectiveMemoryRequest = opts.MemoryRequestGi
			overrideMemoryRequest = &effectiveMemoryRequest
			modifiedSpec["memory_request_gi"] = opts.MemoryRequestGi
		}
	}

	if payload.TargetCPUCores != nil && effectiveCPURequest > 0 && targetCPULimit > 0 && effectiveCPURequest > targetCPULimit {
		return nil, nil, nil, apperrors.BadRequest(
			"VM_MODIFY_REQUEST_REVIEW_REQUIRED",
			"cpu request must be reviewed before approving a lower CPU limit",
		).WithParams(map[string]interface{}{
			"current_cpu_request": payload.CurrentCPURequest,
			"target_cpu_limit":    targetCPULimit,
		}).WithFieldErrors([]apperrors.FieldError{{
			Field:   "cpu_request",
			Code:    "LIMIT_BELOW_REQUEST",
			Message: "cpu request cannot exceed the approved CPU limit",
		}})
	}
	if payload.TargetMemoryGi != nil && effectiveMemoryRequest > 0 && targetMemoryLimitGi > 0 && effectiveMemoryRequest > targetMemoryLimitGi {
		return nil, nil, nil, apperrors.BadRequest(
			"VM_MODIFY_REQUEST_REVIEW_REQUIRED",
			"memory request must be reviewed before approving a lower memory limit",
		).WithParams(map[string]interface{}{
			"current_memory_request_gi": payload.CurrentMemoryRequestGi,
			"target_memory_limit_gi":    targetMemoryLimitGi,
		}).WithFieldErrors([]apperrors.FieldError{{
			Field:   "memory_request_gi",
			Code:    "LIMIT_BELOW_REQUEST",
			Message: "memory request cannot exceed the approved memory limit",
		}})
	}

	if len(modifiedSpec) == 0 {
		return nil, nil, nil, nil
	}
	return overrideCPURequest, overrideMemoryRequest, modifiedSpec, nil
}

func withApprovedVMMutation(modifiedSpec map[string]interface{}, plan *vmmutationplan.VMResourceUpdatePlan) map[string]interface{} {
	if plan == nil || plan.Mutation == nil {
		return modifiedSpec
	}
	if modifiedSpec == nil {
		modifiedSpec = make(map[string]interface{})
	}
	modifiedSpec["vm_mutation"] = plan.Mutation.Snapshot()
	modifiedSpec["apply_mode"] = plan.ApplyMode
	modifiedSpec["requires_restart"] = plan.RequiresRestart
	return modifiedSpec
}

// approveDelete handles approval of DELETE tickets.
// ADR-0012: decision write + domain state + River enqueue are one atomic commit.
func (g *Service) approveDelete(ctx context.Context, ticket *ent.Ticket, ticketID, approver string) error {
	// Parse the event payload to extract VM info for the delete job.
	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}

	var payload struct {
		VMID      string `json:"vm_id"`
		VMName    string `json:"vm_name"`
		ClusterID string `json:"cluster_id"`
		Namespace string `json:"namespace"`
		Actor     string `json:"actor"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse delete event payload: %w", err)
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	if err := g.atomicWriter.ApproveDeleteAndEnqueue(ctx, ticketID, ticket.EventID, approver, payload.VMID); err != nil {
		return fmt.Errorf("approve delete ticket %s atomically: %w", ticketID, err)
	}

	// Audit log (best-effort).
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "delete_approved", approver)
	}

	// Notification trigger: APPROVAL_COMPLETED for delete → notify requester.
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.Actor, approver)
	}

	logger.Info("DELETE ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
		zap.String("event_id", ticket.EventID),
	)

	return nil
}

// approveVNC handles approval of VNC access tickets.
func (g *Service) approveVNC(ctx context.Context, ticket *ent.Ticket, event *ent.DomainEvent, ticketID, approver string) error {
	if event == nil {
		return fmt.Errorf("vnc approval requires domain event")
	}
	if event.EventType != string(domain.EventVNCAccessRequested) {
		return fmt.Errorf("ticket %s is VNC_ACCESS but domain event type is %s", ticketID, event.EventType)
	}

	if _, err := g.client.Ticket.UpdateOneID(ticketID).
		SetStatus(entticket.StatusAPPROVED).
		SetApprover(approver).
		Save(ctx); err != nil {
		return fmt.Errorf("approve vnc ticket %s: %w", ticketID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(ticket.EventID).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(ctx); err != nil {
		return fmt.Errorf("set domain event COMPLETED for vnc ticket %s: %w", ticketID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "vnc_access_approved", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, ticket.Requester, approver)
	}

	logger.Info("VNC ticket approved",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("event_id", ticket.EventID),
	)
	return nil
}

// Reject rejects a pending ticket.
func (g *Service) Reject(ctx context.Context, ticketID, approver, reason string) error {
	ticket, err := g.client.Ticket.Get(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != entticket.StatusPENDING {
		return fmt.Errorf("ticket %s is not pending (current: %s)", ticketID, ticket.Status)
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}
	isBatchParent, err := g.isBatchParentTicket(ctx, ticket, event)
	if err != nil {
		return fmt.Errorf("resolve batch parent ticket %s: %w", ticketID, err)
	}
	if isBatchParent {
		return g.rejectBatchParent(ctx, ticket, approver, reason)
	}

	if _, err := g.client.Ticket.UpdateOneID(ticketID).
		SetStatus(entticket.StatusREJECTED).
		SetApprover(approver).
		SetRejectReason(reason).
		Save(ctx); err != nil {
		return fmt.Errorf("reject ticket %s: %w", ticketID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(ticket.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set domain event CANCELLED for rejected ticket %s: %w", ticketID, err)
	}

	// Audit log (master-flow.md Stage 5.B)
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "rejected", approver)
	}

	// Notification trigger: APPROVAL_REJECTED → notify requester (master-flow.md Stage 5.F).
	if g.notifier != nil {
		g.notifier.OnTicketRejected(ctx, ticketID, ticket.Requester, approver, reason)
	}

	logger.Info("Ticket rejected",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("reason", reason),
	)

	return nil
}

// Cancel allows a user to cancel their own pending request (ADR-0015 §10).
func (g *Service) Cancel(ctx context.Context, ticketID, requester string) error {
	ticket, err := g.client.Ticket.Get(ctx, ticketID)
	if err != nil {
		if ent.IsNotFound(err) {
			return apperrors.NotFound("TICKET_NOT_FOUND", fmt.Sprintf("ticket %s not found", ticketID))
		}
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != entticket.StatusPENDING {
		return apperrors.Conflict(
			"TICKET_NOT_PENDING",
			fmt.Sprintf("ticket %s is not pending (current: %s)", ticketID, ticket.Status),
		)
	}

	if ticket.Requester != requester {
		return apperrors.Forbidden(
			"TICKET_CANCEL_FORBIDDEN",
			fmt.Sprintf("only requester can cancel ticket %s", ticketID),
		)
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}
	isBatchParent, err := g.isBatchParentTicket(ctx, ticket, event)
	if err != nil {
		return fmt.Errorf("resolve batch parent ticket %s: %w", ticketID, err)
	}
	if isBatchParent {
		return g.cancelBatchParent(ctx, ticket, requester)
	}

	if _, err := g.client.Ticket.UpdateOneID(ticketID).
		SetStatus(entticket.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set ticket CANCELLED for canceled ticket %s: %w", ticketID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(ticket.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set domain event CANCELLED for canceled ticket %s: %w", ticketID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "cancelled", requester)
	}

	return nil
}

func (g *Service) approveBatchParent(
	ctx context.Context,
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	approver string, opts ExecutionOptions,
) error {
	children, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		Order(ent.Asc(entticket.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list child tickets for batch %s: %w", parent.ID, err)
	}
	if len(children) == 0 {
		return fmt.Errorf("batch parent %s has no child tickets", parent.ID)
	}

	var (
		successCount int
		failedCount  int
		firstErr     error
	)
	createReady := make(map[string]struct{}, len(children))
	for _, child := range children {
		if child.Status != entticket.StatusPENDING || child.OperationType != entticket.OperationTypeCREATE {
			continue
		}
		if preflightErr := g.preflightCreateCloneSource(ctx, child, opts); preflightErr != nil {
			failedCount++
			if firstErr == nil {
				firstErr = preflightErr
			}
			g.markChildApprovalDispatchFailed(ctx, child, approver, preflightErr)
			continue
		}
		createReady[child.ID] = struct{}{}
	}
	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}
		if child.OperationType == entticket.OperationTypeCREATE {
			if _, ok := createReady[child.ID]; !ok {
				continue
			}
		}

		var approveErr error
		switch child.OperationType {
		case entticket.OperationTypeMODIFY:
			childEvent := parentEvent
			if childEvent == nil || child.EventID != childEvent.ID {
				childEvent, approveErr = g.client.DomainEvent.Get(ctx, child.EventID)
				if approveErr != nil {
					failedCount++
					g.markChildApprovalDispatchFailed(ctx, child, approver, fmt.Errorf("load modify child event %s: %w", child.EventID, approveErr))
					continue
				}
			}
			approveErr = g.approveModify(ctx, child, childEvent, child.ID, approver, opts)
		case entticket.OperationTypeDELETE:
			approveErr = g.approveDelete(ctx, child, child.ID, approver)
		case entticket.OperationTypePOWER:
			childEvent := parentEvent
			if childEvent == nil || child.EventID != childEvent.ID {
				childEvent, approveErr = g.client.DomainEvent.Get(ctx, child.EventID)
				if approveErr != nil {
					failedCount++
					g.markChildApprovalDispatchFailed(ctx, child, approver, fmt.Errorf("load power child event %s: %w", child.EventID, approveErr))
					continue
				}
			}
			approveErr = g.approvePower(ctx, child, childEvent, child.ID, approver)
		default:
			approveErr = g.approveCreateWithConfig(ctx, child, child.ID, approver, opts, approveCreateConfig{
				skipClonePreflight: true,
			})
		}
		if approveErr != nil {
			failedCount++
			if firstErr == nil {
				firstErr = approveErr
			}
			g.markChildApprovalDispatchFailed(ctx, child, approver, approveErr)
			continue
		}
		successCount++
	}

	parentStatus := entticket.StatusFAILED
	parentEventStatus := domainevent.StatusFAILED
	if successCount > 0 {
		parentStatus = entticket.StatusEXECUTING
		parentEventStatus = domainevent.StatusPROCESSING
	}

	parentUpdater := g.client.Ticket.UpdateOneID(parent.ID).
		SetStatus(parentStatus).
		SetApprover(approver)
	if parent.OperationType == entticket.OperationTypeCREATE && strings.TrimSpace(opts.ClusterID) != "" {
		parentUpdater = parentUpdater.SetSelectedClusterID(opts.ClusterID)
	}
	if parent.OperationType == entticket.OperationTypeCREATE && strings.TrimSpace(opts.StorageClass) != "" {
		parentUpdater = parentUpdater.SetSelectedStorageClass(opts.StorageClass)
	}
	if failedCount > 0 {
		rejectReason := fmt.Sprintf("%d child approvals failed during dispatch", failedCount)
		if firstErr != nil {
			message := strings.TrimSpace(firstErr.Error())
			if message != "" {
				rejectReason = message
			}
		}
		parentUpdater = parentUpdater.SetRejectReason(rejectReason)
	}
	if _, err := parentUpdater.Save(ctx); err != nil {
		return fmt.Errorf("update batch parent ticket %s: %w", parent.ID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(parentEvent.ID).SetStatus(parentEventStatus).Save(ctx); err != nil {
		return fmt.Errorf("update batch parent event %s: %w", parentEvent.ID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_approved", approver)
	}
	if g.notifier != nil && successCount > 0 {
		g.notifier.OnTicketApproved(ctx, parent.ID, parent.Requester, approver)
	}
	g.syncBatchProjectionByParentID(ctx, parent.ID)

	if successCount == 0 {
		if firstErr != nil {
			return firstErr
		}
		return fmt.Errorf("batch parent %s approval dispatch failed for all children", parent.ID)
	}

	logger.Info("batch parent approved and dispatched",
		zap.String("ticket_id", parent.ID),
		zap.String("approver", approver),
		zap.Int("children_total", len(children)),
		zap.Int("children_dispatched", successCount),
		zap.Int("children_failed", failedCount),
	)
	return nil
}

func (g *Service) rejectBatchParent(
	ctx context.Context,
	parent *ent.Ticket,
	approver,
	reason string,
) error {
	if _, err := g.client.Ticket.UpdateOneID(parent.ID).
		SetStatus(entticket.StatusREJECTED).
		SetApprover(approver).
		SetRejectReason(reason).
		Save(ctx); err != nil {
		return fmt.Errorf("reject batch parent ticket %s: %w", parent.ID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(parent.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set batch parent event CANCELLED for ticket %s: %w", parent.ID, err)
	}

	children, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list child tickets for batch reject %s: %w", parent.ID, err)
	}
	childEventIDs := make([]string, 0, len(children))
	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}
		if _, err := g.client.Ticket.UpdateOneID(child.ID).
			SetStatus(entticket.StatusREJECTED).
			SetApprover(approver).
			SetRejectReason(reason).
			Save(ctx); err != nil {
			return fmt.Errorf("reject child ticket %s: %w", child.ID, err)
		}
		childEventIDs = append(childEventIDs, child.EventID)
	}
	if len(childEventIDs) > 0 {
		if _, err := g.client.DomainEvent.Update().
			Where(domainevent.IDIn(childEventIDs...)).
			SetStatus(domainevent.StatusCANCELLED).
			Save(ctx); err != nil {
			return fmt.Errorf("cancel child events for batch reject %s: %w", parent.ID, err)
		}
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_rejected", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketRejected(ctx, parent.ID, parent.Requester, approver, reason)
	}
	g.syncBatchProjectionByParentID(ctx, parent.ID)
	return nil
}

func (g *Service) cancelBatchParent(ctx context.Context, parent *ent.Ticket, requester string) error {
	if _, err := g.client.Ticket.UpdateOneID(parent.ID).
		SetStatus(entticket.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set batch parent CANCELLED for ticket %s: %w", parent.ID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(parent.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set batch parent event CANCELLED for ticket %s: %w", parent.ID, err)
	}

	children, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list child tickets for batch cancel %s: %w", parent.ID, err)
	}
	childEventIDs := make([]string, 0, len(children))
	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}
		if _, err := g.client.Ticket.UpdateOneID(child.ID).
			SetStatus(entticket.StatusCANCELLED).
			Save(ctx); err != nil {
			return fmt.Errorf("cancel child ticket %s: %w", child.ID, err)
		}
		childEventIDs = append(childEventIDs, child.EventID)
	}
	if len(childEventIDs) > 0 {
		if _, err := g.client.DomainEvent.Update().
			Where(domainevent.IDIn(childEventIDs...)).
			SetStatus(domainevent.StatusCANCELLED).
			Save(ctx); err != nil {
			return fmt.Errorf("cancel child events for batch cancel %s: %w", parent.ID, err)
		}
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_cancelled", requester)
	}
	g.syncBatchProjectionByParentID(ctx, parent.ID)
	return nil
}

func (g *Service) markChildApprovalDispatchFailed(
	ctx context.Context,
	child *ent.Ticket,
	approver string,
	cause error,
) {
	if child == nil {
		return
	}
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "child approval dispatch failed"
	}
	if len(message) > 512 {
		message = message[:512]
	}
	if _, err := g.client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetApprover(approver).
		SetRejectReason(message).
		Save(ctx); err != nil {
		logger.Warn("failed to mark child ticket dispatch failure",
			zap.String("ticket_id", child.ID),
			zap.Error(err),
		)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(ctx); err != nil {
		logger.Warn("failed to mark child event dispatch failure",
			zap.String("event_id", child.EventID),
			zap.Error(err),
		)
	}
}

func (g *Service) isBatchParentTicket(
	ctx context.Context,
	ticket *ent.Ticket,
	event *ent.DomainEvent,
) (bool, error) {
	if ticket == nil || event == nil {
		return false, nil
	}
	if !isBatchEventType(event.EventType) {
		return false, nil
	}
	hasChildren, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(ticket.ID)).
		Exist(ctx)
	if err != nil {
		return false, err
	}
	return hasChildren, nil
}

func isBatchEventType(eventType string) bool {
	switch eventType {
	case string(domain.EventBatchCreateRequested), string(domain.EventBatchModifyRequested), string(domain.EventBatchDeleteRequested), string(domain.EventBatchPowerRequested):
		return true
	default:
		return false
	}
}

func (g *Service) syncBatchProjectionByParentID(ctx context.Context, parentTicketID string) {
	children, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parentTicketID)).
		All(ctx)
	if err != nil || len(children) == 0 {
		return
	}

	var (
		successCount   int
		failedCount    int
		cancelledCount int
		activeCount    int
	)
	for _, child := range children {
		switch child.Status {
		case entticket.StatusSUCCESS:
			successCount++
		case entticket.StatusFAILED, entticket.StatusREJECTED:
			failedCount++
		case entticket.StatusCANCELLED:
			cancelledCount++
		default:
			activeCount++
		}
	}

	var status entbatchticket.Status
	switch {
	case activeCount > 0:
		status = entbatchticket.StatusIN_PROGRESS
	case successCount == len(children):
		status = entbatchticket.StatusCOMPLETED
	case cancelledCount == len(children):
		status = entbatchticket.StatusCANCELLED
	case successCount > 0 && (failedCount+cancelledCount) > 0:
		status = entbatchticket.StatusPARTIAL_SUCCESS
	default:
		status = entbatchticket.StatusFAILED
	}

	if _, err := g.client.BatchTicket.UpdateOneID(parentTicketID).
		SetChildCount(len(children)).
		SetSuccessCount(successCount).
		SetFailedCount(failedCount).
		SetPendingCount(activeCount).
		SetStatus(status).
		Save(ctx); err != nil && !ent.IsNotFound(err) {
		logger.Warn("failed to sync batch projection from gateway",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
	}
}

// ListPending returns pending tickets sorted by creation time (oldest first).
func (g *Service) ListPending(ctx context.Context) ([]*ent.Ticket, error) {
	return g.client.Ticket.Query().
		Where(entticket.StatusEQ(entticket.StatusPENDING)).
		Order(ent.Asc(entticket.FieldCreatedAt)).
		All(ctx)
}

type vmCreatePayload struct {
	ServiceID      string   `json:"service_id"`
	TemplateID     string   `json:"template_id"`
	Namespace      string   `json:"namespace"`
	RequesterID    string   `json:"requester_id"`
	InstanceSizeID string   `json:"instance_size_id"`
	TargetCPUCores *float64 `json:"target_cpu_cores,omitempty"`
	TargetMemoryGi *float64 `json:"target_memory_gi,omitempty"`
	TargetDiskGB   *int     `json:"target_disk_gb,omitempty"`
}

func parseVMCreatePayload(raw json.RawMessage) (*vmCreatePayload, error) {
	var payload vmCreatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.ServiceID == "" ||
		payload.TemplateID == "" ||
		payload.Namespace == "" ||
		payload.RequesterID == "" ||
		payload.InstanceSizeID == "" {
		return nil, fmt.Errorf("invalid create payload: missing required fields")
	}
	return &payload, nil
}

func resolveEffectiveSelectionIDs(
	templateID string,
	instanceSizeID string,
	modifiedSpec map[string]interface{},
) (effectiveTemplateID, effectiveInstanceSizeID string) {
	effectiveTemplateID = strings.TrimSpace(templateID)
	effectiveInstanceSizeID = strings.TrimSpace(instanceSizeID)

	if override := lookupStringValue(modifiedSpec, "template_id"); override != "" {
		effectiveTemplateID = override
	}
	if override := lookupStringValue(modifiedSpec, "instance_size_id"); override != "" {
		effectiveInstanceSizeID = override
	}
	return effectiveTemplateID, effectiveInstanceSizeID
}

func buildInstanceSizeSnapshot(size *ent.InstanceSize) map[string]interface{} {
	if size == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":                 size.ID,
		"name":               size.Name,
		"display_name":       size.DisplayName,
		"description":        size.Description,
		"cpu_cores":          size.CPUCores,
		"memory_gi":          size.MemoryGi,
		"disk_gb":            size.DiskGB,
		"cpu_request":        size.CPURequest,
		"memory_request_gi":  size.MemoryRequestGi,
		"dedicated_cpu":      size.DedicatedCPU,
		"requires_gpu":       size.RequiresGpu,
		"requires_sriov":     size.RequiresSriov,
		"requires_hugepages": size.RequiresHugepages,
		"hugepages_size":     size.HugepagesSize,
		"dv_access_modes":    cloneStringSlice(size.DvAccessModes),
		"dv_volume_mode":     strings.TrimSpace(size.DvVolumeMode),
		"spec_overrides":     cloneMap(size.SpecOverrides),
		"sort_order":         size.SortOrder,
		"enabled":            size.Enabled,
		"created_by":         size.CreatedBy,
	}
}

func applyResolvedRootVolumeToInstanceSizeSnapshot(
	snapshot map[string]interface{},
	placementEvaluation map[string]interface{},
) map[string]interface{} {
	if len(snapshot) == 0 {
		snapshot = map[string]interface{}{}
	}
	if len(placementEvaluation) == 0 {
		return snapshot
	}

	switch value := placementEvaluation["effective_dv_access_modes"].(type) {
	case []string:
		snapshot["dv_access_modes"] = cloneStringSlice(value)
	case []interface{}:
		items := make([]string, 0, len(value))
		for _, raw := range value {
			if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
				items = append(items, strings.TrimSpace(text))
			}
		}
		snapshot["dv_access_modes"] = cloneStringSlice(items)
	}
	if value, ok := placementEvaluation["effective_dv_volume_mode"].(string); ok {
		snapshot["dv_volume_mode"] = strings.TrimSpace(value)
	}
	return snapshot
}

func buildTemplateSnapshot(tpl *ent.Template) map[string]interface{} {
	if tpl == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":            tpl.ID,
		"name":          tpl.Name,
		"display_name":  tpl.DisplayName,
		"description":   tpl.Description,
		"source_type":   service.EffectiveTemplateSourceType(tpl.SourceType, tpl.ImageURL, tpl.PvcName),
		"image_url":     tpl.ImageURL,
		"pvc_name":      tpl.PvcName,
		"pvc_namespace": tpl.PvcNamespace,
		"cloud_init":    tpl.CloudInit,
		"os_family":     tpl.OsFamily,
		"os_version":    tpl.OsVersion,
		"enabled":       tpl.Enabled,
		"created_by":    tpl.CreatedBy,
	}
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}

func cloneClaimPropertySets(items []domain.StorageClaimPropertySet) []domain.StorageClaimPropertySet {
	if len(items) == 0 {
		return nil
	}
	out := make([]domain.StorageClaimPropertySet, len(items))
	for i := range items {
		out[i] = domain.StorageClaimPropertySet{
			AccessModes: cloneStringSlice(items[i].AccessModes),
			VolumeMode:  items[i].VolumeMode,
		}
	}
	return out
}

func lookupStringValue(values map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		str, ok := raw.(string)
		if !ok {
			continue
		}
		if trimmed := strings.TrimSpace(str); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// PriorityTier calculates urgency tier based on pending duration (ADR-0015 §11).
func PriorityTier(createdAt time.Time) string {
	days := int(time.Since(createdAt).Hours() / 24)
	switch {
	case days >= 7:
		return "urgent" // Red
	case days >= 4:
		return "warning" // Yellow
	default:
		return "normal" // Default
	}
}

// buildDryRunSpec constructs a domain.VMSpec for DryRun pre-flight validation.
//
// Uses the same image resolution strategy as VMCreateWorker (ADR-0036: semantic fields
// image_url / pvc_name rather than spec JSONB). Admin resource overrides are applied
// so the DryRun reflects the actual resource allocation that will be submitted.
//
// The "dryrun-" prefix on the VM name makes the intent clear in K8s audit logs
// without risking collisions (server-side DryRun does not persist).
func (g *Service) buildDryRunSpec(
	payload *vmCreatePayload,
	tmpl *ent.Template,
	size *ent.InstanceSize,
	opts ExecutionOptions,
) (*domain.VMSpec, error) {
	// Resolve boot image (ADR-0036 semantic fields).
	image, err := resolveTemplateImageForDryRun(tmpl)
	if err != nil {
		return nil, fmt.Errorf("resolve image for dryrun from template %s: %w", tmpl.ID, err)
	}
	requestedTargets := resolveCreateRequestTargets(payload, size)

	spec := &domain.VMSpec{
		Name:         "dryrun-" + payload.ServiceID, // unique per request, not persisted
		CPU:          requestedTargets.CPULimit,
		MemoryGi:     requestedTargets.MemoryLimitGi,
		DiskGB:       requestedTargets.DiskGB,
		Image:        image,
		StorageClass: strings.TrimSpace(opts.StorageClass),
		CloudInit:    tmpl.CloudInit,
		Labels: map[string]string{
			"shepherd.io/service-id":  payload.ServiceID,
			"shepherd.io/template-id": tmpl.ID,
		},
		SpecOverrides:   cloneMap(size.SpecOverrides),
		CPURequest:      requestedTargets.CPURequest,
		MemoryRequestGi: requestedTargets.MemoryRequestGi,
		DVAccessModes:   cloneStringSlice(size.DvAccessModes),
		DVVolumeMode:    strings.TrimSpace(size.DvVolumeMode),
	}
	if len(opts.DVAccessModes) > 0 {
		spec.DVAccessModes = cloneStringSlice(opts.DVAccessModes)
	}
	if strings.TrimSpace(opts.DVVolumeMode) != "" {
		spec.DVVolumeMode = strings.TrimSpace(opts.DVVolumeMode)
	}

	// Apply admin resource overrides — must match what the actual job will use.
	if opts.EnableOverride {
		if opts.CPULimit > 0 {
			spec.CPU = opts.CPULimit
		}
		if opts.MemoryLimitGi > 0 {
			spec.MemoryGi = opts.MemoryLimitGi
		}
		if opts.CPURequest > 0 {
			spec.CPURequest = opts.CPURequest
		}
		if opts.MemoryRequestGi > 0 {
			spec.MemoryRequestGi = opts.MemoryRequestGi
		}
		if opts.DiskGB > 0 {
			spec.DiskGB = opts.DiskGB
		}
	} else if opts.DiskGB > 0 {
		spec.DiskGB = opts.DiskGB
	}

	return spec, nil
}

func resolveCreateRequestTargets(
	payload *vmCreatePayload,
	size *ent.InstanceSize,
) service.ResolvedVMRequestTargets {
	return service.ResolveVMRequestTargets(
		size.CPUCores,
		size.CPURequest,
		size.MemoryGi,
		size.MemoryRequestGi,
		size.DiskGB,
		service.VMRequestTargets{
			TargetCPUCores: payload.TargetCPUCores,
			TargetMemoryGi: payload.TargetMemoryGi,
			TargetDiskGB:   payload.TargetDiskGB,
		},
	)
}

func buildCreateApprovalOverride(
	payload *vmCreatePayload,
	size *ent.InstanceSize,
	opts ExecutionOptions,
) *service.ApprovalResourceOverride {
	resolved := resolveCreateRequestTargets(payload, size)
	override := service.ApprovalResourceOverride{
		CPURequest:      resolved.CPURequest,
		CPULimit:        resolved.CPULimit,
		MemoryRequestGi: resolved.MemoryRequestGi,
		MemoryLimitGi:   resolved.MemoryLimitGi,
		DiskGB:          resolved.DiskGB,
	}
	if opts.EnableOverride {
		if opts.CPURequest > 0 {
			override.CPURequest = opts.CPURequest
		}
		if opts.CPULimit > 0 {
			override.CPULimit = opts.CPULimit
		}
		if opts.MemoryRequestGi > 0 {
			override.MemoryRequestGi = opts.MemoryRequestGi
		}
		if opts.MemoryLimitGi > 0 {
			override.MemoryLimitGi = opts.MemoryLimitGi
		}
		if opts.DiskGB > 0 {
			override.DiskGB = opts.DiskGB
		}
	} else if opts.DiskGB > 0 {
		override.DiskGB = opts.DiskGB
	}
	return &override
}

func applyRequestedCreateTargets(
	modifiedSpec map[string]interface{},
	payload *vmCreatePayload,
	size *ent.InstanceSize,
) {
	resolved := resolveCreateRequestTargets(payload, size)
	if payload.TargetCPUCores != nil {
		modifiedSpec["cpu_limit"] = resolved.CPULimit
	}
	if payload.TargetMemoryGi != nil {
		modifiedSpec["memory_limit_gi"] = resolved.MemoryLimitGi
	}
	if payload.TargetDiskGB != nil {
		modifiedSpec["disk_gb"] = resolved.DiskGB
	}
	if payload.TargetCPUCores != nil && resolved.AdjustedCPURequest {
		modifiedSpec["cpu_request"] = resolved.CPURequest
	}
	if payload.TargetMemoryGi != nil && resolved.AdjustedMemoryGiReq {
		modifiedSpec["memory_request_gi"] = resolved.MemoryRequestGi
	}
}

// resolveTemplateImageForDryRun extracts the boot image string from an Ent Template.
// Mirrors VMCreateWorker.extractTemplateImageFromEnt (ADR-0036 semantic template fields).
func resolveTemplateImageForDryRun(tpl *ent.Template) (string, error) {
	image, err := service.ResolveTemplateBootTransport(tpl.SourceType, tpl.ImageURL, tpl.PvcName, tpl.PvcNamespace)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", tpl.ID, err)
	}
	return image, nil
}
