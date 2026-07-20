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
	"net/http"
	"sort"
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
	"kv-shepherd.io/shepherd/internal/jobs"
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
	preflightOnly      bool
	batchGuard         *domain.BatchApprovalDispatchGuard
}

const (
	powerOperationStart   = "start"
	powerOperationStop    = "stop"
	powerOperationRestart = "restart"
)

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

// AtomicBatchDecisionWriter owns the crash-safe parent/retry intent plus River
// dispatcher insertion. It is kept separate so direct-ticket writer fakes and
// extensions do not need batch support unless they execute batch decisions.
type AtomicBatchDecisionWriter interface {
	ClaimBatchApprovalAndEnqueue(context.Context, domain.BatchApprovalClaimInput) error
	RetryBatchApprovalAndEnqueue(context.Context, domain.BatchApprovalRetryInput) error
	ValidateBatchApprovalDispatchGraph(context.Context, string, string) (domain.BatchApprovalDispatchGuard, error)
}

// AtomicBatchChildDecisionWriter extends the direct-ticket writer with a
// parent-scoped guard. Every batch child mutation must validate the durable
// graph under the same transaction that commits state and River work.
type AtomicBatchChildDecisionWriter interface {
	ApproveBatchCreateAndEnqueue(
		ctx context.Context,
		guard domain.BatchApprovalDispatchGuard,
		ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
		templateSnapshot map[string]interface{},
		instanceSizeSnapshot map[string]interface{},
		placementEvaluation map[string]interface{},
		modifiedSpec map[string]interface{},
	) (vmID, vmName string, err error)
	ApproveBatchDeleteAndEnqueue(context.Context, domain.BatchApprovalDispatchGuard, string, string, string, string) error
	ApproveBatchModifyAndEnqueue(context.Context, domain.BatchApprovalDispatchGuard, string, string, string, map[string]interface{}) error
	ApproveBatchPowerAndEnqueue(context.Context, domain.BatchApprovalDispatchGuard, string, string, string, string) error
	FailBatchApprovalChildDispatch(context.Context, domain.BatchApprovalDispatchGuard, string, string, string, string) error
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

func withDecisionTx(ctx context.Context, client *ent.Client, fn func(txClient *ent.Client) error) error {
	return withDecisionEntTx(ctx, client, func(tx *ent.Tx) error {
		return fn(tx.Client())
	})
}

func withDecisionEntTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin ticket decision transaction: %w", err)
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("%w: rollback ticket decision transaction: %w", err, rerr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ticket decision transaction: %w", err)
	}
	return nil
}

func requirePendingDecisionEvent(ticketID string, event *ent.DomainEvent) error {
	if event == nil {
		return fmt.Errorf("ticket %s requires a domain event", ticketID)
	}
	if event.Status != domainevent.StatusPENDING {
		return fmt.Errorf("ticket %s domain event %s is not pending (current: %s)", ticketID, event.ID, event.Status)
	}
	return nil
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

	opts = normalizeExecutionOptions(opts)
	if validationErr := validateApprovalExecutionOptions(ticket, opts); validationErr != nil {
		return validationErr
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}
	if eventErr := requirePendingDecisionEvent(ticketID, event); eventErr != nil {
		return eventErr
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
		return g.approveModify(ctx, ticket, event, ticketID, approver, opts, nil)
	case entticket.OperationTypeDELETE:
		return g.approveDelete(ctx, ticket, ticketID, approver, nil)
	case entticket.OperationTypePOWER:
		return g.approvePower(ctx, ticket, event, ticketID, approver, nil)
	case entticket.OperationTypeVNC_ACCESS:
		return g.approveVNC(ctx, ticket, event, ticketID, approver)
	default:
		// CREATE is the default operation type.
		return g.approveCreate(ctx, ticket, ticketID, approver, opts)
	}
}

// approvePower handles approval of POWER tickets.
func (g *Service) approvePower(
	ctx context.Context,
	ticket *ent.Ticket,
	event *ent.DomainEvent,
	ticketID, approver string,
	batchGuard *domain.BatchApprovalDispatchGuard,
) error {
	if event == nil {
		return fmt.Errorf("power approval requires domain event")
	}

	var payload domain.VMPowerPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse power payload for ticket %s: %w", ticketID, err)
	}
	operation := strings.TrimSpace(strings.ToLower(payload.Operation))
	switch operation {
	case powerOperationStart, powerOperationStop, powerOperationRestart:
	default:
		return fmt.Errorf("ticket %s has unsupported power operation %q", ticketID, payload.Operation)
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	var writeErr error
	if batchGuard != nil {
		batchWriter, ok := g.atomicWriter.(AtomicBatchChildDecisionWriter)
		if !ok || batchWriter == nil {
			return fmt.Errorf("guarded batch child writer is not configured")
		}
		writeErr = batchWriter.ApproveBatchPowerAndEnqueue(ctx, *batchGuard, ticketID, ticket.EventID, approver, operation)
	} else {
		writeErr = g.atomicWriter.ApprovePowerAndEnqueue(ctx, ticketID, ticket.EventID, approver, operation)
	}
	if writeErr != nil {
		return fmt.Errorf("approve power ticket %s atomically: %w", ticketID, writeErr)
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
	opts = normalizeExecutionOptions(opts)
	if err := validateCreateApprovalClusterSelection(opts.ClusterID); err != nil {
		return err
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
	if validationErr := validateCreateHugepagesApprovalOverride(payload, instanceSizeEntity, opts); validationErr != nil {
		return validationErr
	}
	createOverride, overrideErr := buildCreateApprovalOverride(payload, instanceSizeEntity, opts)
	if overrideErr != nil {
		return apperrors.BadRequest(apperrors.CodeValidationFailed,
			fmt.Sprintf("build approval resource override for ticket %s: %v", ticketID, overrideErr))
	}
	effectiveDedicatedCPU := instanceSizeEntity.DedicatedCPU ||
		service.HasDedicatedCPUInSpecOverrides(instanceSizeEntity.SpecOverrides)
	if overcommitErr := service.ValidateOvercommit(
		createOverride.CPULimit,
		createOverride.CPURequest,
		createOverride.MemoryLimitGi,
		createOverride.MemoryRequestGi,
		effectiveDedicatedCPU,
	); overcommitErr != nil {
		return apperrors.BadRequest(apperrors.CodeValidationFailed,
			fmt.Sprintf("resource request validation for ticket %s: %v", ticketID, overcommitErr))
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
			Override:       createOverride,
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
				return selectedClusterPreflightUnavailable(evaluation.ReasonMessage)
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
				return approvalPreflightUnavailable(preflightErr)
			}
			if appErr, ok := apperrors.IsAppError(preflightErr); ok {
				return appErr
			}
			return apperrors.BadRequest(
				apperrors.CodeValidationFailed,
				fmt.Sprintf("source pvc preflight failed for ticket %s", ticketID),
			)
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
			return approvalPreflightUnavailable(validateErr)
		case validateErr != nil:
			if appErr, ok := apperrors.IsAppError(validateErr); ok {
				return appErr
			}
			return apperrors.BadRequest(
				apperrors.CodeValidationFailed,
				fmt.Sprintf("pre-flight dryrun gate failed for ticket %s", ticketID),
			)
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

	// Stage 5.B: If admin enabled resource override, require at least one value.
	if opts.EnableOverride {
		// Guard: at least one override field must be non-zero to avoid a no-op override.
		if opts.CPULimit == 0 && opts.CPURequest == 0 && opts.MemoryLimitGi == 0 && opts.MemoryRequestGi == 0 && opts.DiskGB == 0 {
			return fmt.Errorf("enable_override is true but all resource override values are zero for ticket %s", ticketID)
		}
	}

	if config.preflightOnly {
		return nil
	}

	templateSnapshot := buildTemplateSnapshot(templateEntity)
	instanceSizeSnapshot := buildInstanceSizeSnapshot(instanceSizeEntity)
	if placementEvaluation != nil {
		instanceSizeSnapshot = applyResolvedRootVolumeToInstanceSizeSnapshot(instanceSizeSnapshot, placementEvaluation)
	}
	modifiedSpec := cloneMap(ticket.ModifiedSpec)
	if applyErr := applyRequestedCreateTargets(modifiedSpec, payload, instanceSizeEntity); applyErr != nil {
		return apperrors.BadRequest(apperrors.CodeValidationFailed,
			fmt.Sprintf("apply requested create targets for ticket %s: %v", ticketID, applyErr))
	}

	// Merge admin resource overrides into modifiedSpec (Stage 5.B).
	if opts.EnableOverride {
		modifiedSpec["enable_override"] = true
		if opts.CPURequest > 0 {
			modifiedSpec["cpu_request"] = opts.CPURequest
		} else if opts.CPULimit > 0 && createOverride.CPURequest > 0 && createOverride.CPURequest != instanceSizeEntity.CPURequest {
			modifiedSpec["cpu_request"] = createOverride.CPURequest
		}
		if opts.CPULimit > 0 {
			modifiedSpec["cpu_limit"] = opts.CPULimit
		}
		if opts.MemoryRequestGi > 0 {
			modifiedSpec["memory_request_gi"] = opts.MemoryRequestGi
		} else if opts.MemoryLimitGi > 0 && service.InstanceSizeUsesHugepages(instanceSizeEntity) && createOverride.MemoryRequestGi > 0 {
			modifiedSpec["memory_request_gi"] = createOverride.MemoryRequestGi
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

	var vmID, vmName string
	if config.batchGuard != nil {
		batchWriter, ok := g.atomicWriter.(AtomicBatchChildDecisionWriter)
		if !ok || batchWriter == nil {
			return fmt.Errorf("guarded batch child writer is not configured")
		}
		vmID, vmName, err = batchWriter.ApproveBatchCreateAndEnqueue(
			ctx, *config.batchGuard, ticketID, ticket.EventID, approver, opts.ClusterID,
			resolvedOpts.StorageClass, payload.ServiceID, payload.Namespace, payload.RequesterID,
			templateSnapshot, instanceSizeSnapshot, placementEvaluation, modifiedSpec,
		)
	} else {
		vmID, vmName, err = g.atomicWriter.ApproveCreateAndEnqueue(
			ctx, ticketID, ticket.EventID, approver, opts.ClusterID, resolvedOpts.StorageClass,
			payload.ServiceID, payload.Namespace, payload.RequesterID, templateSnapshot,
			instanceSizeSnapshot, placementEvaluation, modifiedSpec,
		)
	}
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

func normalizeExecutionOptions(opts ExecutionOptions) ExecutionOptions {
	opts.ClusterID = strings.TrimSpace(opts.ClusterID)
	opts.StorageClass = strings.TrimSpace(opts.StorageClass)
	opts.DVVolumeMode = strings.TrimSpace(opts.DVVolumeMode)
	if len(opts.DVAccessModes) > 0 {
		normalized := make([]string, 0, len(opts.DVAccessModes))
		for _, value := range opts.DVAccessModes {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				normalized = append(normalized, trimmed)
			}
		}
		opts.DVAccessModes = normalized
	}
	return opts
}

func validateApprovalExecutionOptions(ticket *ent.Ticket, opts ExecutionOptions) error {
	if ticket == nil {
		return fmt.Errorf("ticket is required")
	}
	if ticket.OperationType != entticket.OperationTypeCREATE {
		return nil
	}
	return validateCreateApprovalClusterSelection(opts.ClusterID)
}

func validateCreateApprovalClusterSelection(clusterID string) error {
	if strings.TrimSpace(clusterID) != "" {
		return nil
	}
	return apperrors.BadRequest(
		apperrors.CodeValidationFailed,
		"selected cluster is required for create approval",
	).WithFieldErrors([]apperrors.FieldError{{
		Field:   "selected_cluster_id",
		Code:    "REQUIRED",
		Message: "selected cluster is required for create approval",
	}})
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
			return approvalPreflightUnavailable(err)
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
	batchGuard *domain.BatchApprovalDispatchGuard,
) error {
	prepared, err := g.prepareModifyApproval(ctx, event, ticketID, opts)
	if err != nil {
		return err
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	var writeErr error
	if batchGuard != nil {
		batchWriter, ok := g.atomicWriter.(AtomicBatchChildDecisionWriter)
		if !ok || batchWriter == nil {
			return fmt.Errorf("guarded batch child writer is not configured")
		}
		writeErr = batchWriter.ApproveBatchModifyAndEnqueue(ctx, *batchGuard, ticketID, ticket.EventID, approver, prepared.modifiedSpec)
	} else {
		writeErr = g.atomicWriter.ApproveModifyAndEnqueue(ctx, ticketID, ticket.EventID, approver, prepared.modifiedSpec)
	}
	if writeErr != nil {
		return fmt.Errorf("approve modify ticket %s atomically: %w", ticketID, writeErr)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "modify_approved", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, prepared.payload.Actor, approver)
	}

	logger.Info("MODIFY ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", prepared.payload.VMID),
		zap.String("vm_name", prepared.payload.VMName),
		zap.String("event_id", ticket.EventID),
	)

	return nil
}

type preparedModifyApproval struct {
	payload      domain.VMModifyPayload
	modifiedSpec map[string]interface{}
}

func (g *Service) prepareModifyApproval(
	ctx context.Context,
	event *ent.DomainEvent,
	ticketID string,
	opts ExecutionOptions,
) (*preparedModifyApproval, error) {
	if event == nil {
		return nil, fmt.Errorf("modify approval requires domain event")
	}

	var payload domain.VMModifyPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("parse modify event payload: %w", err)
	}

	if g.vmService == nil {
		return nil, apperrors.New(
			"VM_MODIFY_APPROVAL_PREFLIGHT_UNAVAILABLE",
			"modify approval requires live cluster mutation preflight",
			http.StatusServiceUnavailable,
		)
	}

	liveVM, err := g.vmService.GetVM(ctx, payload.ClusterID, payload.Namespace, payload.VMName)
	if err != nil {
		if isClusterRuntimeUnavailable(err) {
			return nil, approvalPreflightUnavailable(err)
		}
		return nil, fmt.Errorf("load live vm for modify approval %s: %w", ticketID, err)
	}
	if liveVM == nil {
		return nil, fmt.Errorf("live vm is unavailable for modify approval %s", ticketID)
	}
	usesHugepages, err := g.modifyPayloadUsesHugepages(ctx, &payload, liveVM)
	if err != nil {
		return nil, err
	}
	overrideCPURequest, overrideMemoryRequest, modifySpec, validationErr := buildModifyApprovalSpec(&payload, opts, usesHugepages)
	if validationErr != nil {
		return nil, validationErr
	}

	plan, err := vmmutationplan.PlanVMResourceUpdatePatch(payload.Namespace, liveVM, vmmutationplan.VMLiveUpdateTargets{
		CPUCores:        payload.TargetCPUCores,
		MemoryGi:        payload.TargetMemoryGi,
		DiskGB:          payload.TargetDiskGB,
		CPURequest:      overrideCPURequest,
		MemoryRequestGi: overrideMemoryRequest,
	})
	if err != nil {
		return nil, apperrors.BadRequest(
			"VM_MODIFY_APPROVAL_INVALID",
			"modify request cannot be executed with the current VM state",
		)
	}
	if err := g.vmService.DryRunVMMutation(ctx, payload.ClusterID, payload.Namespace, payload.VMName, plan.Mutation); err != nil {
		if isClusterRuntimeUnavailable(err) {
			return nil, approvalPreflightUnavailable(err)
		}
		return nil, apperrors.BadRequest(
			"VM_MODIFY_APPROVAL_INVALID",
			"modify request cannot be executed with the current VM state",
		)
	}
	modifySpec = withApprovedVMMutation(modifySpec, plan)
	return &preparedModifyApproval{
		payload:      payload,
		modifiedSpec: modifySpec,
	}, nil
}

func approvalPreflightUnavailable(_ error) *apperrors.AppError {
	return apperrors.New(
		apperrors.CodeClusterUnhealthy,
		"approval requires live cluster preflight",
		http.StatusServiceUnavailable,
	).WithParams(map[string]interface{}{
		"reason": "CLUSTER_RUNTIME_UNAVAILABLE",
	})
}

func selectedClusterPreflightUnavailable(reason string) *apperrors.AppError {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "approval requires live cluster preflight"
	}
	return apperrors.New(
		apperrors.CodeClusterUnhealthy,
		reason,
		http.StatusServiceUnavailable,
	).WithParams(map[string]interface{}{
		"reason": "CLUSTER_RUNTIME_UNAVAILABLE",
	})
}

func isNonConsumingApprovalError(err error) bool {
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		return false
	}
	return appErr.HTTPStatus == http.StatusBadRequest ||
		appErr.HTTPStatus == http.StatusServiceUnavailable ||
		appErr.Code == apperrors.CodeClusterUnhealthy ||
		appErr.Code == apperrors.CodeValidationFailed ||
		appErr.Code == "VM_MODIFY_APPROVAL_INVALID" ||
		appErr.Code == "VM_MODIFY_APPROVAL_PREFLIGHT_UNAVAILABLE"
}

func buildModifyApprovalSpec(
	payload *domain.VMModifyPayload,
	opts ExecutionOptions,
	usesHugepages bool,
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
	if usesHugepages {
		if opts.EnableOverride && opts.MemoryRequestGi > 0 && !float64Equal(opts.MemoryRequestGi, targetMemoryLimitGi) {
			return nil, nil, nil, hugepagesMemoryRequestAlignmentError(opts.MemoryRequestGi, targetMemoryLimitGi)
		}
		if targetMemoryLimitGi > 0 {
			effectiveMemoryRequest = targetMemoryLimitGi
			overrideMemoryRequest = &effectiveMemoryRequest
			modifiedSpec["memory_request_gi"] = targetMemoryLimitGi
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

func (g *Service) modifyPayloadUsesHugepages(ctx context.Context, payload *domain.VMModifyPayload, liveVM *domain.VM) (bool, error) {
	if liveVM != nil && strings.TrimSpace(liveVM.Spec.HugepagesPageSize) != "" {
		return true, nil
	}
	if payload != nil && strings.TrimSpace(payload.HugepagesPageSize) != "" {
		return true, nil
	}
	if payload == nil || strings.TrimSpace(payload.InstanceSizeID) == "" {
		return false, nil
	}
	size, err := g.client.InstanceSize.Get(ctx, strings.TrimSpace(payload.InstanceSizeID))
	if err != nil {
		if ent.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get instance size %s for modify approval: %w", payload.InstanceSizeID, err)
	}
	return service.InstanceSizeUsesHugepages(size), nil
}

func validateCreateHugepagesApprovalOverride(payload *vmCreatePayload, size *ent.InstanceSize, opts ExecutionOptions) error {
	if !service.InstanceSizeUsesHugepages(size) || !opts.EnableOverride || opts.MemoryRequestGi <= 0 {
		return nil
	}
	resolved, err := resolveCreateRequestTargets(payload, size)
	if err != nil {
		return err
	}
	targetMemoryLimitGi := resolved.MemoryLimitGi
	if opts.MemoryLimitGi > 0 {
		targetMemoryLimitGi = opts.MemoryLimitGi
	}
	if targetMemoryLimitGi <= 0 || float64Equal(opts.MemoryRequestGi, targetMemoryLimitGi) {
		return nil
	}
	return hugepagesMemoryRequestAlignmentError(opts.MemoryRequestGi, targetMemoryLimitGi)
}

func hugepagesMemoryRequestAlignmentError(request, limit float64) error {
	return apperrors.BadRequest(
		"HUGEPAGES_MEMORY_REQUEST_ALIGNMENT_REQUIRED",
		"hugepages-backed memory requires memory request to equal memory limit",
	).WithParams(map[string]interface{}{
		"memory_request_gi": request,
		"memory_limit_gi":   limit,
	}).WithFieldErrors([]apperrors.FieldError{{
		Field:   "memory_request_gi",
		Code:    "HUGEPAGES_REQUIRES_LIMIT_ALIGNMENT",
		Message: "hugepages-backed memory requires memory request to equal memory limit",
	}})
}

func float64Equal(a, b float64) bool {
	if a > b {
		return a-b < 1e-9
	}
	return b-a < 1e-9
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
func (g *Service) approveDelete(
	ctx context.Context,
	ticket *ent.Ticket,
	ticketID, approver string,
	batchGuard *domain.BatchApprovalDispatchGuard,
) error {
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
	var writeErr error
	if batchGuard != nil {
		batchWriter, ok := g.atomicWriter.(AtomicBatchChildDecisionWriter)
		if !ok || batchWriter == nil {
			return fmt.Errorf("guarded batch child writer is not configured")
		}
		writeErr = batchWriter.ApproveBatchDeleteAndEnqueue(ctx, *batchGuard, ticketID, ticket.EventID, approver, payload.VMID)
	} else {
		writeErr = g.atomicWriter.ApproveDeleteAndEnqueue(ctx, ticketID, ticket.EventID, approver, payload.VMID)
	}
	if writeErr != nil {
		return fmt.Errorf("approve delete ticket %s atomically: %w", ticketID, writeErr)
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

	if err := withDecisionTx(ctx, g.client, func(txClient *ent.Client) error {
		if err := updatePendingTicketEventDecisionPair(
			ctx,
			txClient,
			ticketID,
			ticket.EventID,
			entticket.StatusSUCCESS,
			domainevent.StatusCOMPLETED,
			approver,
			"",
		); err != nil {
			return fmt.Errorf("approve vnc ticket %s: %w", ticketID, err)
		}
		return nil
	}); err != nil {
		return err
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
	if eventErr := requirePendingDecisionEvent(ticketID, event); eventErr != nil {
		return eventErr
	}
	isBatchParent, err := g.isBatchParentTicket(ctx, ticket, event)
	if err != nil {
		return fmt.Errorf("resolve batch parent ticket %s: %w", ticketID, err)
	}
	if isBatchParent {
		return g.rejectBatchParent(ctx, ticket, approver, reason)
	}

	if err := withDecisionTx(ctx, g.client, func(txClient *ent.Client) error {
		if err := updatePendingTicketEventDecisionPair(
			ctx,
			txClient,
			ticketID,
			ticket.EventID,
			entticket.StatusREJECTED,
			domainevent.StatusCANCELLED,
			approver,
			reason,
		); err != nil {
			return fmt.Errorf("reject ticket %s: %w", ticketID, err)
		}
		return nil
	}); err != nil {
		return err
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
	if eventErr := requirePendingDecisionEvent(ticketID, event); eventErr != nil {
		return eventErr
	}
	isBatchParent, err := g.isBatchParentTicket(ctx, ticket, event)
	if err != nil {
		return fmt.Errorf("resolve batch parent ticket %s: %w", ticketID, err)
	}
	if isBatchParent {
		return g.cancelBatchParent(ctx, ticket, requester)
	}

	if err := withDecisionTx(ctx, g.client, func(txClient *ent.Client) error {
		if err := updatePendingTicketEventDecisionPair(
			ctx,
			txClient,
			ticketID,
			ticket.EventID,
			entticket.StatusCANCELLED,
			domainevent.StatusCANCELLED,
			"",
			"",
		); err != nil {
			return fmt.Errorf("set ticket CANCELLED for canceled ticket %s: %w", ticketID, err)
		}
		return nil
	}); err != nil {
		return err
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

	// Preserve synchronous approval validation while keeping every durable
	// mutation behind the parent+River transaction below. The worker repeats
	// validation because cluster state may change after this request returns.
	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}

		var preflightErr error
		switch child.OperationType {
		case entticket.OperationTypeCREATE:
			preflightErr = g.preflightCreateCloneSource(ctx, child, opts)
			if preflightErr == nil {
				preflightErr = g.approveCreateWithConfig(ctx, child, child.ID, approver, opts, approveCreateConfig{
					skipClonePreflight: true,
					preflightOnly:      true,
				})
			}
		case entticket.OperationTypeMODIFY:
			childEvent := parentEvent
			if childEvent == nil || child.EventID != childEvent.ID {
				childEvent, preflightErr = g.client.DomainEvent.Get(ctx, child.EventID)
				if preflightErr != nil {
					break
				}
			}
			_, preflightErr = g.prepareModifyApproval(ctx, childEvent, child.ID, opts)
		case entticket.OperationTypeDELETE, entticket.OperationTypePOWER, entticket.OperationTypeVNC_ACCESS:
			// These operations do not have approval-time cluster dry-run checks.
		}
		if preflightErr != nil {
			if isNonConsumingApprovalError(preflightErr) {
				return preflightErr
			}
			// Unknown infrastructure failures must not be converted into a
			// consumed approval. River can retry them after the durable claim.
			logger.Warn("batch child preflight deferred to durable dispatcher",
				zap.String("parent_ticket_id", parent.ID),
				zap.String("child_ticket_id", child.ID),
				zap.String("failure_reason", "BATCH_APPROVAL_PREFLIGHT_DEFERRED"),
				zap.String("error_type", fmt.Sprintf("%T", preflightErr)),
			)
		}
	}

	batchWriter, ok := g.atomicWriter.(AtomicBatchDecisionWriter)
	if !ok || batchWriter == nil {
		return fmt.Errorf("atomic batch approval writer is not configured")
	}
	if err := batchWriter.ClaimBatchApprovalAndEnqueue(ctx, domain.BatchApprovalClaimInput{
		ParentTicketID: parent.ID,
		ParentEventID:  parent.EventID,
		Approver:       approver,
		Execution:      batchApprovalExecutionFromOptions(opts),
	}); err != nil {
		return fmt.Errorf("claim batch parent %s and enqueue dispatcher atomically: %w", parent.ID, err)
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_dispatch_scheduled", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, parent.ID, parent.Requester, approver)
	}
	logger.Info("batch parent approved and dispatcher enqueued",
		zap.String("ticket_id", parent.ID),
		zap.String("approver", approver),
		zap.Int("children_total", len(children)),
	)
	return nil
}

// DispatchBatchApproval is the idempotent River entry point. Each child writer
// uses its own ticket/event CAS plus River InsertTx; completed children are
// skipped on retries and only PENDING children are eligible for dispatch.
func (g *Service) DispatchBatchApproval(ctx context.Context, parentID string) error {
	parent, parentEvent, children, guard, err := g.loadBatchApprovalDispatchState(ctx, parentID)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return batchApprovalDispatchConsistencyError(parent, parentEvent, batchApprovalDispatchChildCounts{}, "parent has no child tickets")
	}

	counts := countBatchApprovalDispatchChildren(children)
	if counts.active > 0 &&
		(parent.Status == entticket.StatusFAILED || parentEvent.Status == domainevent.StatusFAILED) {
		// A stale parent aggregation can commit after an explicit retry reset its
		// children. The parent-row lock in this narrow reconciliation prevents a
		// second stale write and permits only FAILED -> EXECUTING / PROCESSING.
		if (parent.Status != entticket.StatusEXECUTING && parent.Status != entticket.StatusFAILED) ||
			(parentEvent.Status != domainevent.StatusPROCESSING && parentEvent.Status != domainevent.StatusFAILED) {
			return batchApprovalDispatchConsistencyError(parent, parentEvent, counts, "active children belong to a non-repairable parent state")
		}
		if reconcileErr := jobs.ReconcileFailedParentBatchStatus(ctx, g.client, parent.ID); reconcileErr != nil {
			return fmt.Errorf("reconcile failed batch dispatch parent %s: %w", parent.ID, reconcileErr)
		}
		parent, parentEvent, children, guard, err = g.loadBatchApprovalDispatchState(ctx, parent.ID)
		if err != nil {
			return err
		}
		counts = countBatchApprovalDispatchChildren(children)
	}

	if counts.active == 0 {
		return g.syncTerminalBatchApprovalDispatch(ctx, parent, parentEvent, counts)
	}
	if parent.Status != entticket.StatusEXECUTING || parentEvent.Status != domainevent.StatusPROCESSING {
		return batchApprovalDispatchConsistencyError(parent, parentEvent, counts, "active children require an EXECUTING/PROCESSING parent")
	}

	opts := batchApprovalOptionsFromExecution(guard.Execution)
	approver := strings.TrimSpace(guard.Approver)

	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}
		dispatchErr := g.dispatchBatchChild(ctx, child, approver, opts, guard)
		if dispatchErr == nil {
			continue
		}
		if isRetryableBatchDispatchError(dispatchErr) {
			return fmt.Errorf("dispatch batch child %s: %w", child.ID, dispatchErr)
		}
		if persistErr := g.markChildApprovalDispatchFailed(ctx, child, guard, approver, dispatchErr, false); persistErr != nil {
			return persistErr
		}
	}
	if err := jobs.SyncParentBatchStatus(ctx, g.client, parent.ID); err != nil {
		return fmt.Errorf("sync batch parent %s after dispatcher: %w", parent.ID, err)
	}
	return nil
}

func (g *Service) dispatchBatchChild(
	ctx context.Context,
	child *ent.Ticket,
	approver string,
	opts ExecutionOptions,
	guard domain.BatchApprovalDispatchGuard,
) error {
	if child == nil {
		return permanentBatchDispatchError{err: fmt.Errorf("batch child ticket is nil")}
	}
	switch child.OperationType {
	case entticket.OperationTypeCREATE:
		return g.approveCreateWithConfig(ctx, child, child.ID, approver, opts, approveCreateConfig{batchGuard: &guard})
	case entticket.OperationTypeMODIFY:
		childEvent, err := g.client.DomainEvent.Get(ctx, child.EventID)
		if err != nil {
			return fmt.Errorf("load modify child event %s: %w", child.EventID, err)
		}
		return g.approveModify(ctx, child, childEvent, child.ID, approver, opts, &guard)
	case entticket.OperationTypeDELETE:
		return g.approveDelete(ctx, child, child.ID, approver, &guard)
	case entticket.OperationTypePOWER:
		childEvent, err := g.client.DomainEvent.Get(ctx, child.EventID)
		if err != nil {
			return fmt.Errorf("load power child event %s: %w", child.EventID, err)
		}
		return g.approvePower(ctx, child, childEvent, child.ID, approver, &guard)
	case entticket.OperationTypeVNC_ACCESS:
		childEvent, err := g.client.DomainEvent.Get(ctx, child.EventID)
		if err != nil {
			return fmt.Errorf("load vnc child event %s: %w", child.EventID, err)
		}
		return g.approveVNC(ctx, child, childEvent, child.ID, approver)
	default:
		return permanentBatchDispatchError{err: fmt.Errorf(
			"unsupported ticket operation type %s for child ticket %s",
			child.OperationType,
			child.ID,
		)}
	}
}

// FailPendingBatchApprovalDispatch is called only after River exhausts the
// dispatcher. Partial progress is safe: a snoozed finalizer skips children it
// already made terminal and continues until parent synchronization succeeds.
func (g *Service) FailPendingBatchApprovalDispatch(ctx context.Context, parentID string, cause error) error {
	if cause == nil {
		cause = stderrors.New("batch approval dispatcher exhausted without a recorded cause")
	}
	parent, parentEvent, children, guard, err := g.loadBatchApprovalDispatchState(ctx, parentID)
	if err != nil {
		return err
	}
	if len(children) == 0 {
		return batchApprovalDispatchConsistencyError(parent, parentEvent, batchApprovalDispatchChildCounts{}, "parent has no child tickets")
	}
	counts := countBatchApprovalDispatchChildren(children)
	if counts.active == 0 {
		return g.syncTerminalBatchApprovalDispatch(ctx, parent, parentEvent, counts)
	}

	// Normalize the only repairable active-parent pair before mutating any child.
	// This guard ensures the finalizer cannot commit FAILED children underneath a
	// successful/cancelled parent and then discover that parent CAS is forbidden.
	if (parent.Status == entticket.StatusEXECUTING || parent.Status == entticket.StatusFAILED) &&
		(parentEvent.Status == domainevent.StatusPROCESSING || parentEvent.Status == domainevent.StatusFAILED) {
		if parent.Status != entticket.StatusEXECUTING || parentEvent.Status != domainevent.StatusPROCESSING {
			if reconcileErr := jobs.ReconcileFailedParentBatchStatus(ctx, g.client, parent.ID); reconcileErr != nil {
				return fmt.Errorf("reconcile exhausted batch parent %s: %w", parent.ID, reconcileErr)
			}
			parent, parentEvent, children, guard, err = g.loadBatchApprovalDispatchState(ctx, parent.ID)
			if err != nil {
				return err
			}
			counts = countBatchApprovalDispatchChildren(children)
		}
	}
	if counts.active == 0 {
		return g.syncTerminalBatchApprovalDispatch(ctx, parent, parentEvent, counts)
	}
	if parent.Status != entticket.StatusEXECUTING || parentEvent.Status != domainevent.StatusPROCESSING {
		return batchApprovalDispatchConsistencyError(parent, parentEvent, counts, "finalizer cannot safely rewrite pending children")
	}
	if counts.pending == 0 {
		return jobs.SyncParentBatchStatus(ctx, g.client, parent.ID)
	}

	for _, child := range children {
		if child.Status != entticket.StatusPENDING {
			continue
		}
		if err := g.markChildApprovalDispatchFailed(ctx, child, guard, guard.Approver, cause, true); err != nil {
			return err
		}
	}
	if err := jobs.SyncParentBatchStatus(ctx, g.client, parent.ID); err != nil {
		return fmt.Errorf("sync exhausted batch parent %s: %w", parent.ID, err)
	}
	return nil
}

type batchApprovalDispatchChildCounts struct {
	total     int
	success   int
	failed    int
	cancelled int
	pending   int
	active    int
}

func countBatchApprovalDispatchChildren(children []*ent.Ticket) batchApprovalDispatchChildCounts {
	var counts batchApprovalDispatchChildCounts
	for _, child := range children {
		if child == nil {
			continue
		}
		counts.total++
		if child.Status == entticket.StatusPENDING {
			counts.pending++
		}
		switch child.Status {
		case entticket.StatusSUCCESS:
			counts.success++
		case entticket.StatusFAILED, entticket.StatusREJECTED:
			counts.failed++
		case entticket.StatusCANCELLED:
			counts.cancelled++
		default:
			counts.active++
		}
	}
	return counts
}

func (g *Service) syncTerminalBatchApprovalDispatch(
	ctx context.Context,
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	counts batchApprovalDispatchChildCounts,
) error {
	expectedParent, expectedEvent, ok := terminalBatchApprovalDispatchOutcome(counts)
	if !ok {
		return batchApprovalDispatchConsistencyError(parent, parentEvent, counts, "child set is not terminal")
	}
	parentPairMatches := parent != nil && parentEvent != nil &&
		parent.Status == expectedParent && parentEvent.Status == expectedEvent
	activePairCanConverge := parent != nil && parentEvent != nil &&
		parent.Status == entticket.StatusEXECUTING && parentEvent.Status == domainevent.StatusPROCESSING
	if !parentPairMatches && !activePairCanConverge {
		return batchApprovalDispatchConsistencyError(
			parent,
			parentEvent,
			counts,
			fmt.Sprintf("terminal children require parent/event %s/%s", expectedParent, expectedEvent),
		)
	}
	// Even an already matching terminal pair is synchronized so projection
	// counters cannot remain stale after a duplicate dispatcher delivery.
	return jobs.SyncParentBatchStatus(ctx, g.client, parent.ID)
}

func terminalBatchApprovalDispatchOutcome(
	counts batchApprovalDispatchChildCounts,
) (ticketStatus entticket.Status, eventStatus domainevent.Status, terminal bool) {
	if counts.total == 0 || counts.active != 0 {
		return "", "", false
	}
	switch {
	case counts.success == counts.total:
		return entticket.StatusSUCCESS, domainevent.StatusCOMPLETED, true
	case counts.cancelled == counts.total:
		return entticket.StatusCANCELLED, domainevent.StatusCANCELLED, true
	default:
		return entticket.StatusFAILED, domainevent.StatusFAILED, true
	}
}

func (g *Service) loadBatchApprovalDispatchState(
	ctx context.Context,
	parentID string,
) (loadedParent *ent.Ticket, loadedParentEvent *ent.DomainEvent, loadedChildren []*ent.Ticket, guard domain.BatchApprovalDispatchGuard, loadErr error) {
	parentID = strings.TrimSpace(parentID)
	parent, parentEvent, children, err := g.loadBatchApprovalDispatchStateSnapshot(ctx, parentID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, nil, domain.BatchApprovalDispatchGuard{}, &jobs.BatchApprovalDispatchConsistencyError{
				BatchID: parentID,
				Detail:  "durable parent ticket or parent event is missing",
				Cause:   err,
			}
		}
		return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, err
	}
	batchWriter, ok := g.atomicWriter.(AtomicBatchDecisionWriter)
	if !ok || batchWriter == nil {
		return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, batchApprovalDispatchConsistencyError(
			parent,
			parentEvent,
			countBatchApprovalDispatchChildren(children),
			"exact batch dispatch graph validator is not configured",
		)
	}
	guard, err = batchWriter.ValidateBatchApprovalDispatchGraph(ctx, parent.ID, parent.EventID)
	if err != nil {
		var invalidGraph *domain.BatchApprovalDispatchGraphInvalidError
		if !stderrors.As(err, &invalidGraph) {
			return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, fmt.Errorf(
				"validate batch approval dispatch graph %s: %w",
				parent.ID,
				err,
			)
		}
		consistencyErr := batchApprovalDispatchConsistencyError(
			parent,
			parentEvent,
			countBatchApprovalDispatchChildren(children),
			"exact parent, projection, child, and payload graph validation failed",
		)
		var typed *jobs.BatchApprovalDispatchConsistencyError
		if stderrors.As(consistencyErr, &typed) {
			typed.Cause = err
		}
		return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, consistencyErr
	}

	// Decisions are made from a fresh post-validation snapshot. Every child
	// mutation also revalidates guard.GraphFingerprint under the parent lock in
	// its own write transaction, closing the remaining post-reload window.
	parent, parentEvent, children, err = g.loadBatchApprovalDispatchStateSnapshot(ctx, parent.ID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, nil, domain.BatchApprovalDispatchGuard{}, &jobs.BatchApprovalDispatchConsistencyError{
				BatchID: parentID,
				Detail:  "durable parent ticket or parent event disappeared after validation",
				Cause:   err,
			}
		}
		return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, err
	}
	if parent.ID != guard.ParentTicketID || parent.EventID != guard.ParentEventID {
		return parent, parentEvent, children, domain.BatchApprovalDispatchGuard{}, batchApprovalDispatchConsistencyError(
			parent,
			parentEvent,
			countBatchApprovalDispatchChildren(children),
			"parent identity changed after exact graph validation",
		)
	}
	return parent, parentEvent, children, guard, nil
}

func (g *Service) loadBatchApprovalDispatchStateSnapshot(
	ctx context.Context,
	parentID string,
) (loadedParent *ent.Ticket, loadedParentEvent *ent.DomainEvent, loadedChildren []*ent.Ticket, loadErr error) {
	parentID = strings.TrimSpace(parentID)
	parent, err := g.client.Ticket.Get(ctx, parentID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load batch dispatch parent %s: %w", parentID, err)
	}
	parentEvent, err := g.client.DomainEvent.Get(ctx, parent.EventID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load batch dispatch parent event %s: %w", parent.EventID, err)
	}
	children, err := g.client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parent.ID)).
		Order(ent.Asc(entticket.FieldCreatedAt), ent.Asc(entticket.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("list batch dispatch children %s: %w", parent.ID, err)
	}
	if !batchParentEventIdentityMatches(parent, parentEvent) {
		return parent, parentEvent, children, batchApprovalDispatchConsistencyError(
			parent,
			parentEvent,
			countBatchApprovalDispatchChildren(children),
			"parent ticket and domain event identity or operation do not match",
		)
	}
	childEventIDs := make([]string, 0, len(children))
	for _, child := range children {
		childEventIDs = append(childEventIDs, strings.TrimSpace(child.EventID))
	}
	childEvents, err := g.client.DomainEvent.Query().
		Where(domainevent.IDIn(childEventIDs...)).
		All(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load batch dispatch child events %s: %w", parent.ID, err)
	}
	childEventsByID := make(map[string]*ent.DomainEvent, len(childEvents))
	for _, childEvent := range childEvents {
		childEventsByID[childEvent.ID] = childEvent
	}
	for _, child := range children {
		if !batchChildEventIdentityMatches(parent, child, childEventsByID[strings.TrimSpace(child.EventID)]) {
			return parent, parentEvent, children, batchApprovalDispatchConsistencyError(
				parent,
				parentEvent,
				countBatchApprovalDispatchChildren(children),
				fmt.Sprintf("child ticket %s and domain event identity or payload do not match", child.ID),
			)
		}
	}
	return parent, parentEvent, children, nil
}

func batchChildEventIdentityMatches(parent, child *ent.Ticket, event *ent.DomainEvent) bool {
	if parent == nil || child == nil || event == nil ||
		strings.TrimSpace(child.ParentTicketID) != strings.TrimSpace(parent.ID) ||
		child.OperationType != parent.OperationType ||
		strings.TrimSpace(child.Requester) == "" ||
		strings.TrimSpace(child.Requester) != strings.TrimSpace(parent.Requester) ||
		strings.TrimSpace(child.EventID) != strings.TrimSpace(event.ID) ||
		strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(child.Requester) ||
		strings.TrimSpace(event.AggregateType) != "vm" ||
		strings.TrimSpace(event.AggregateID) == "" {
		return false
	}
	aggregateID := strings.TrimSpace(event.AggregateID)
	switch child.OperationType {
	case entticket.OperationTypeCREATE:
		if event.EventType != string(domain.EventVMCreationRequested) {
			return false
		}
		var payload domain.VMCreationPayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.ServiceID) == aggregateID
	case entticket.OperationTypeMODIFY:
		if event.EventType != string(domain.EventVMModifyRequested) {
			return false
		}
		var payload domain.VMModifyPayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.VMID) == aggregateID
	case entticket.OperationTypeDELETE:
		if event.EventType != string(domain.EventVMDeletionRequested) {
			return false
		}
		var payload domain.VMDeletePayload
		return json.Unmarshal(event.Payload, &payload) == nil && strings.TrimSpace(payload.VMID) == aggregateID
	case entticket.OperationTypePOWER:
		var payload domain.VMPowerPayload
		if json.Unmarshal(event.Payload, &payload) != nil || strings.TrimSpace(payload.VMID) != aggregateID {
			return false
		}
		switch strings.ToLower(strings.TrimSpace(payload.Operation)) {
		case powerOperationStart:
			return event.EventType == string(domain.EventVMStartRequested)
		case powerOperationStop:
			return event.EventType == string(domain.EventVMStopRequested)
		case powerOperationRestart:
			return event.EventType == string(domain.EventVMRestartRequested)
		default:
			return false
		}
	default:
		return false
	}
}

func batchApprovalDispatchConsistencyError(
	parent *ent.Ticket,
	parentEvent *ent.DomainEvent,
	counts batchApprovalDispatchChildCounts,
	detail string,
) error {
	err := &jobs.BatchApprovalDispatchConsistencyError{
		PendingChildren: counts.pending,
		ActiveChildren:  counts.active,
		Detail:          detail,
	}
	if parent != nil {
		err.BatchID = parent.ID
		err.ParentStatus = parent.Status.String()
	}
	if parentEvent != nil {
		err.ParentEventStatus = parentEvent.Status.String()
	}
	return err
}

type permanentBatchDispatchError struct{ err error }

func (e permanentBatchDispatchError) Error() string { return e.err.Error() }
func (e permanentBatchDispatchError) Unwrap() error { return e.err }

func isRetryableBatchDispatchError(err error) bool {
	if err == nil {
		return false
	}
	var permanent permanentBatchDispatchError
	if stderrors.As(err, &permanent) {
		return false
	}
	if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	appErr, ok := apperrors.IsAppError(err)
	if !ok {
		// Unknown errors include database/provider failures. Preserve PENDING and
		// let River retry rather than consuming a potentially transient failure.
		return true
	}
	return appErr.HTTPStatus != http.StatusBadRequest
}

func batchApprovalExecutionFromOptions(opts ExecutionOptions) domain.BatchApprovalExecutionOptions {
	opts = normalizeExecutionOptions(opts)
	return domain.BatchApprovalExecutionOptions{
		ClusterID:       opts.ClusterID,
		StorageClass:    opts.StorageClass,
		DVAccessModes:   cloneStringSlice(opts.DVAccessModes),
		DVVolumeMode:    opts.DVVolumeMode,
		EnableOverride:  opts.EnableOverride,
		CPURequest:      opts.CPURequest,
		CPULimit:        opts.CPULimit,
		MemoryRequestGi: opts.MemoryRequestGi,
		MemoryLimitGi:   opts.MemoryLimitGi,
		DiskGB:          opts.DiskGB,
	}
}

func batchApprovalOptionsFromExecution(persisted domain.BatchApprovalExecutionOptions) ExecutionOptions {
	return normalizeExecutionOptions(ExecutionOptions{
		ClusterID:       persisted.ClusterID,
		StorageClass:    persisted.StorageClass,
		DVAccessModes:   cloneStringSlice(persisted.DVAccessModes),
		DVVolumeMode:    persisted.DVVolumeMode,
		EnableOverride:  persisted.EnableOverride,
		CPURequest:      persisted.CPURequest,
		CPULimit:        persisted.CPULimit,
		MemoryRequestGi: persisted.MemoryRequestGi,
		MemoryLimitGi:   persisted.MemoryLimitGi,
		DiskGB:          persisted.DiskGB,
	})
}

// BatchApprovalExecutionFromTicket reloads the durable execution plan used by
// retry scheduling. Legacy parents without a snapshot retain their selected
// cluster/storage values as a compatibility fallback.
func BatchApprovalExecutionFromTicket(parent *ent.Ticket) (domain.BatchApprovalExecutionOptions, error) {
	if parent == nil {
		return domain.BatchApprovalExecutionOptions{}, fmt.Errorf("batch parent ticket is required")
	}
	persisted := domain.BatchApprovalExecutionOptions{
		ClusterID:    strings.TrimSpace(parent.SelectedClusterID),
		StorageClass: strings.TrimSpace(parent.SelectedStorageClass),
	}
	if raw, ok := parent.ModifiedSpec["batch_approval_execution"]; ok {
		encoded, err := json.Marshal(raw)
		if err != nil {
			return domain.BatchApprovalExecutionOptions{}, fmt.Errorf("marshal persisted execution plan: %w", err)
		}
		if err := json.Unmarshal(encoded, &persisted); err != nil {
			return domain.BatchApprovalExecutionOptions{}, fmt.Errorf("decode persisted execution plan: %w", err)
		}
	}
	persisted.ClusterID = strings.TrimSpace(persisted.ClusterID)
	persisted.StorageClass = strings.TrimSpace(persisted.StorageClass)
	persisted.DVVolumeMode = strings.TrimSpace(persisted.DVVolumeMode)
	persisted.DVAccessModes = cloneStringSlice(persisted.DVAccessModes)
	return persisted, nil
}

func (g *Service) rejectBatchParent(
	ctx context.Context,
	parent *ent.Ticket,
	approver,
	reason string,
) error {
	if err := withDecisionEntTx(ctx, g.client, func(tx *ent.Tx) error {
		children, err := jobs.ValidateParentBatchChildrenInTx(ctx, tx, parent)
		if err != nil {
			return fmt.Errorf("validate child tickets for batch reject %s: %w", parent.ID, err)
		}
		txClient := tx.Client()
		sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		for _, child := range children {
			if child.Status != entticket.StatusPENDING {
				continue
			}
			if err := updateBatchChildDecisionPair(
				ctx,
				txClient,
				child,
				entticket.StatusREJECTED,
				domainevent.StatusCANCELLED,
				approver,
				reason,
			); err != nil {
				return fmt.Errorf("reject batch child %s: %w", child.ID, err)
			}
		}
		if err := updatePendingTicketEventDecisionPair(
			ctx,
			txClient,
			parent.ID,
			parent.EventID,
			entticket.StatusREJECTED,
			domainevent.StatusCANCELLED,
			approver,
			reason,
		); err != nil {
			return fmt.Errorf("reject batch parent ticket %s: %w", parent.ID, err)
		}
		return g.syncBatchProjectionByParentWithClient(ctx, txClient, parent)
	}); err != nil {
		return err
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_rejected", approver)
	}
	if g.notifier != nil {
		g.notifier.OnTicketRejected(ctx, parent.ID, parent.Requester, approver, reason)
	}
	return nil
}

func (g *Service) cancelBatchParent(ctx context.Context, parent *ent.Ticket, requester string) error {
	if err := withDecisionEntTx(ctx, g.client, func(tx *ent.Tx) error {
		children, err := jobs.ValidateParentBatchChildrenInTx(ctx, tx, parent)
		if err != nil {
			return fmt.Errorf("validate child tickets for batch cancel %s: %w", parent.ID, err)
		}
		txClient := tx.Client()
		sort.Slice(children, func(i, j int) bool { return children[i].ID < children[j].ID })
		for _, child := range children {
			if child.Status != entticket.StatusPENDING {
				continue
			}
			if err := updateBatchChildDecisionPair(
				ctx,
				txClient,
				child,
				entticket.StatusCANCELLED,
				domainevent.StatusCANCELLED,
				"",
				"",
			); err != nil {
				return fmt.Errorf("cancel batch child %s: %w", child.ID, err)
			}
		}
		if err := updatePendingTicketEventDecisionPair(
			ctx,
			txClient,
			parent.ID,
			parent.EventID,
			entticket.StatusCANCELLED,
			domainevent.StatusCANCELLED,
			"",
			"",
		); err != nil {
			return fmt.Errorf("set batch parent CANCELLED for ticket %s: %w", parent.ID, err)
		}
		return g.syncBatchProjectionByParentWithClient(ctx, txClient, parent)
	}); err != nil {
		return err
	}

	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, parent.ID, "batch_cancelled", requester)
	}
	return nil
}

func updateBatchChildDecisionPair(
	ctx context.Context,
	txClient *ent.Client,
	child *ent.Ticket,
	ticketStatus entticket.Status,
	eventStatus domainevent.Status,
	approver string,
	reason string,
) error {
	if child == nil {
		return fmt.Errorf("batch child ticket is nil")
	}
	parentTicketID := strings.TrimSpace(child.ParentTicketID)
	if parentTicketID == "" {
		return fmt.Errorf("batch child ticket %s has no parent identity", child.ID)
	}
	return updatePendingTicketEventDecisionPairWithParent(
		ctx,
		txClient,
		child.ID,
		child.EventID,
		parentTicketID,
		ticketStatus,
		eventStatus,
		approver,
		reason,
	)
}

func updatePendingTicketEventDecisionPair(
	ctx context.Context,
	txClient *ent.Client,
	ticketID string,
	eventID string,
	ticketStatus entticket.Status,
	eventStatus domainevent.Status,
	approver string,
	reason string,
) error {
	return updatePendingTicketEventDecisionPairWithParent(
		ctx,
		txClient,
		ticketID,
		eventID,
		"",
		ticketStatus,
		eventStatus,
		approver,
		reason,
	)
}

func updatePendingTicketEventDecisionPairWithParent(
	ctx context.Context,
	txClient *ent.Client,
	ticketID string,
	eventID string,
	parentTicketID string,
	ticketStatus entticket.Status,
	eventStatus domainevent.Status,
	approver string,
	reason string,
) error {
	if strings.TrimSpace(ticketID) == "" {
		return fmt.Errorf("ticket id is required")
	}
	if strings.TrimSpace(eventID) == "" {
		return fmt.Errorf("event id is required")
	}
	ticketUpdate := txClient.Ticket.Update().
		Where(
			entticket.ID(ticketID),
			entticket.EventID(eventID),
			entticket.StatusEQ(entticket.StatusPENDING),
		).
		SetStatus(ticketStatus)
	if parentTicketID != "" {
		ticketUpdate = ticketUpdate.Where(entticket.ParentTicketIDEQ(parentTicketID))
	} else {
		ticketUpdate = ticketUpdate.Where(entticket.ParentTicketIDIsNil())
	}
	if strings.TrimSpace(approver) != "" {
		ticketUpdate = ticketUpdate.SetApprover(approver)
	}
	if strings.TrimSpace(reason) != "" {
		ticketUpdate = ticketUpdate.SetRejectReason(reason)
	}
	affected, err := ticketUpdate.Save(ctx)
	if err != nil {
		return fmt.Errorf("update ticket decision: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("update ticket decision: expected 1 row, got %d", affected)
	}
	affected, err = txClient.DomainEvent.Update().
		Where(
			domainevent.ID(eventID),
			domainevent.StatusEQ(domainevent.StatusPENDING),
		).
		SetStatus(eventStatus).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("update event decision: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("update event decision: expected 1 row, got %d", affected)
	}
	return nil
}

func (g *Service) markChildApprovalDispatchFailed(
	ctx context.Context,
	child *ent.Ticket,
	guard domain.BatchApprovalDispatchGuard,
	approver string,
	cause error,
	exhausted bool,
) error {
	if child == nil {
		return nil
	}
	publicReason := classifyBatchApprovalDispatchFailure(cause, exhausted)
	logger.Warn("batch child dispatch failed",
		zap.String("parent_ticket_id", strings.TrimSpace(guard.ParentTicketID)),
		zap.String("child_ticket_id", child.ID),
		zap.String("failure_reason", publicReason),
		zap.String("error_type", fmt.Sprintf("%T", cause)),
	)
	batchWriter, ok := g.atomicWriter.(AtomicBatchChildDecisionWriter)
	if !ok || batchWriter == nil {
		return fmt.Errorf("guarded batch child writer is not configured")
	}
	if err := batchWriter.FailBatchApprovalChildDispatch(
		ctx,
		guard,
		child.ID,
		child.EventID,
		approver,
		publicReason,
	); err != nil {
		return fmt.Errorf("mark child ticket %s dispatch failure: %w", child.ID, err)
	}
	return nil
}

func classifyBatchApprovalDispatchFailure(cause error, exhausted bool) string {
	if exhausted {
		return domain.BatchApprovalDispatchFailureExhausted
	}
	var permanent permanentBatchDispatchError
	if stderrors.As(cause, &permanent) {
		return domain.BatchApprovalDispatchFailureUnsupported
	}
	return domain.BatchApprovalDispatchFailureValidation
}

func (g *Service) isBatchParentTicket(
	ctx context.Context,
	ticket *ent.Ticket,
	event *ent.DomainEvent,
) (bool, error) {
	if ticket == nil || event == nil {
		return false, nil
	}
	if !batchParentEventIdentityMatches(ticket, event) {
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

func batchParentEventIdentityMatches(parent *ent.Ticket, event *ent.DomainEvent) bool {
	if parent == nil || event == nil || strings.TrimSpace(parent.ParentTicketID) != "" {
		return false
	}
	if strings.TrimSpace(parent.EventID) != strings.TrimSpace(event.ID) ||
		strings.TrimSpace(event.AggregateType) != "batch" ||
		strings.TrimSpace(event.AggregateID) != strings.TrimSpace(parent.ID) ||
		strings.TrimSpace(parent.Requester) == "" ||
		strings.TrimSpace(event.CreatedBy) != strings.TrimSpace(parent.Requester) {
		return false
	}
	switch parent.OperationType {
	case entticket.OperationTypeCREATE:
		return event.EventType == string(domain.EventBatchCreateRequested)
	case entticket.OperationTypeMODIFY:
		return event.EventType == string(domain.EventBatchModifyRequested)
	case entticket.OperationTypeDELETE:
		return event.EventType == string(domain.EventBatchDeleteRequested)
	case entticket.OperationTypePOWER:
		return event.EventType == string(domain.EventBatchPowerRequested)
	default:
		return false
	}
}

func (g *Service) syncBatchProjectionByParentWithClient(ctx context.Context, client *ent.Client, parent *ent.Ticket) error {
	if client == nil || parent == nil || strings.TrimSpace(parent.ID) == "" {
		return fmt.Errorf("batch projection sync requires an exact parent ticket")
	}
	parentTicketID := strings.TrimSpace(parent.ID)
	children, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(parentTicketID)).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list child tickets for batch projection %s: %w", parentTicketID, err)
	}
	if len(children) == 0 {
		return fmt.Errorf("batch projection %s has no child tickets", parentTicketID)
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

	expectedBatchType, ok := batchProjectionTypeForOperation(parent.OperationType)
	if !ok {
		return fmt.Errorf("batch projection %s has unsupported parent operation %s", parentTicketID, parent.OperationType)
	}
	affected, err := client.BatchTicket.Update().
		Where(
			entbatchticket.ID(parentTicketID),
			entbatchticket.BatchTypeEQ(expectedBatchType),
			entbatchticket.CreatedByEQ(parent.Requester),
		).
		SetChildCount(len(children)).
		SetSuccessCount(successCount).
		SetFailedCount(failedCount).
		SetPendingCount(activeCount).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		logger.Warn("failed to sync batch projection from gateway",
			zap.String("parent_ticket_id", parentTicketID),
			zap.Error(err),
		)
		return fmt.Errorf("sync batch projection %s: %w", parentTicketID, err)
	}
	if affected != 1 {
		return fmt.Errorf("sync batch projection %s with exact identity: expected 1 row, got %d", parentTicketID, affected)
	}
	return nil
}

func batchProjectionTypeForOperation(operation entticket.OperationType) (entbatchticket.BatchType, bool) {
	switch operation {
	case entticket.OperationTypeCREATE:
		return entbatchticket.BatchTypeBATCH_CREATE, true
	case entticket.OperationTypeMODIFY:
		return entbatchticket.BatchTypeBATCH_MODIFY, true
	case entticket.OperationTypeDELETE:
		return entbatchticket.BatchTypeBATCH_DELETE, true
	case entticket.OperationTypePOWER:
		return entbatchticket.BatchTypeBATCH_POWER, true
	default:
		return "", false
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
		"updated_at":         size.UpdatedAt.UTC().Format(time.RFC3339Nano),
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
		"updated_at":    tpl.UpdatedAt.UTC().Format(time.RFC3339Nano),
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
	if payload == nil {
		return nil, fmt.Errorf("payload is required for dryrun")
	}
	if tmpl == nil {
		return nil, fmt.Errorf("template is required for dryrun")
	}
	if size == nil {
		return nil, fmt.Errorf("instance size is required for dryrun")
	}

	// Resolve boot image (ADR-0036 semantic fields).
	image, err := resolveTemplateImageForDryRun(tmpl)
	if err != nil {
		return nil, fmt.Errorf("resolve image for dryrun from template %s: %w", tmpl.ID, err)
	}
	requestedTargets, err := resolveCreateRequestTargets(payload, size)
	if err != nil {
		return nil, err
	}

	spec := &domain.VMSpec{
		Name:         "dryrun-" + payload.ServiceID, // unique per request, not persisted
		CPU:          requestedTargets.CPULimit,
		MemoryGi:     requestedTargets.MemoryLimitGi,
		DiskGB:       requestedTargets.DiskGB,
		Image:        image,
		StorageClass: strings.TrimSpace(opts.StorageClass),
		CloudInit:    tmpl.CloudInit,
		Labels: map[string]string{
			domain.ShepherdServiceIDLabel:  payload.ServiceID,
			domain.ShepherdTemplateIDLabel: tmpl.ID,
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
	baseCPULimit := spec.CPU
	baseCPURequest := spec.CPURequest
	effectiveDedicatedCPU := size.DedicatedCPU || service.HasDedicatedCPUInSpecOverrides(size.SpecOverrides)
	if opts.EnableOverride {
		if opts.CPULimit > 0 {
			spec.CPU = opts.CPULimit
		}
		if opts.MemoryLimitGi > 0 {
			spec.MemoryGi = opts.MemoryLimitGi
		}
		if opts.CPURequest > 0 {
			spec.CPURequest = opts.CPURequest
		} else if opts.CPULimit > 0 {
			if alignedRequest, adjusted := service.AlignCPULimitOnlyRequest(
				baseCPULimit,
				baseCPURequest,
				spec.CPU,
				effectiveDedicatedCPU,
			); adjusted {
				spec.CPURequest = alignedRequest
			}
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
	alignedMemoryRequest, _, err := service.AlignHugepagesMemoryRequestGi(size, spec.MemoryGi, spec.MemoryRequestGi)
	if err != nil {
		return nil, err
	}
	spec.MemoryRequestGi = alignedMemoryRequest

	return spec, nil
}

func resolveCreateRequestTargets(
	payload *vmCreatePayload,
	size *ent.InstanceSize,
) (service.ResolvedVMRequestTargets, error) {
	resolved := service.ResolveVMRequestTargets(
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
	alignedMemoryRequest, adjusted, err := service.AlignHugepagesMemoryRequestGi(size, resolved.MemoryLimitGi, resolved.MemoryRequestGi)
	if err != nil {
		return service.ResolvedVMRequestTargets{}, err
	}
	if adjusted {
		resolved.MemoryRequestGi = alignedMemoryRequest
		resolved.AdjustedMemoryGiReq = true
	}
	return resolved, nil
}

func buildCreateApprovalOverride(
	payload *vmCreatePayload,
	size *ent.InstanceSize,
	opts ExecutionOptions,
) (*service.ApprovalResourceOverride, error) {
	resolved, err := resolveCreateRequestTargets(payload, size)
	if err != nil {
		return nil, err
	}
	override := service.ApprovalResourceOverride{
		CPURequest:      resolved.CPURequest,
		CPULimit:        resolved.CPULimit,
		MemoryRequestGi: resolved.MemoryRequestGi,
		MemoryLimitGi:   resolved.MemoryLimitGi,
		DiskGB:          resolved.DiskGB,
	}
	if opts.EnableOverride {
		baseCPULimit := override.CPULimit
		baseCPURequest := override.CPURequest
		effectiveDedicatedCPU := size != nil && (size.DedicatedCPU || service.HasDedicatedCPUInSpecOverrides(size.SpecOverrides))
		if opts.CPULimit > 0 {
			override.CPULimit = opts.CPULimit
		}
		if opts.CPURequest > 0 {
			override.CPURequest = opts.CPURequest
		} else if opts.CPULimit > 0 {
			if alignedRequest, adjusted := service.AlignCPULimitOnlyRequest(
				baseCPULimit,
				baseCPURequest,
				override.CPULimit,
				effectiveDedicatedCPU,
			); adjusted {
				override.CPURequest = alignedRequest
			}
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
	alignedMemoryRequest, _, err := service.AlignHugepagesMemoryRequestGi(size, override.MemoryLimitGi, override.MemoryRequestGi)
	if err != nil {
		return nil, err
	}
	override.MemoryRequestGi = alignedMemoryRequest
	return &override, nil
}

func applyRequestedCreateTargets(
	modifiedSpec map[string]interface{},
	payload *vmCreatePayload,
	size *ent.InstanceSize,
) error {
	resolved, err := resolveCreateRequestTargets(payload, size)
	if err != nil {
		return err
	}
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
	return nil
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
