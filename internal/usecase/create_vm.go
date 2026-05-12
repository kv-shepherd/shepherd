// Package usecase provides application use cases (Clean Architecture).
//
// UseCases are reusable across HTTP, CLI, gRPC, Cron.
// ADR-0012: Atomic transactions managed at UseCase level.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

// CreateVMInput represents the input for creating a VM.
type CreateVMInput struct {
	ServiceID      string   `json:"service_id"`
	TemplateID     string   `json:"template_id"`
	InstanceSizeID string   `json:"instance_size_id"`
	Namespace      string   `json:"namespace"`
	Reason         string   `json:"reason"`
	RequestedBy    string   `json:"requested_by"`
	TargetCPUCores *float64 `json:"target_cpu_cores,omitempty"`
	TargetMemoryGi *float64 `json:"target_memory_gi,omitempty"`
	TargetDiskGB   *int     `json:"target_disk_gb,omitempty"`
}

// CreateVMOutput represents the output of a VM creation request.
type CreateVMOutput struct {
	TicketID string `json:"ticket_id"`
	EventID  string `json:"event_id"`
	Status   string `json:"status"`
}

// CreateVMUseCase orchestrates VM creation.
// ADR-0012: Two-phase execution (DB write → K8s create).
type CreateVMUseCase struct {
	entClient       *ent.Client
	vmService       *service.VMService
	instanceSizeSvc *service.InstanceSizeService
	templateSvc     *service.TemplateService
	auditLogger     *audit.Logger
}

// NewCreateVMUseCase creates a new CreateVMUseCase.
func NewCreateVMUseCase(
	entClient *ent.Client,
	vmService *service.VMService,
	instanceSizeSvc *service.InstanceSizeService,
	templateSvc *service.TemplateService,
) *CreateVMUseCase {
	return &CreateVMUseCase{
		entClient:       entClient,
		vmService:       vmService,
		instanceSizeSvc: instanceSizeSvc,
		templateSvc:     templateSvc,
	}
}

// WithAuditLogger sets the audit logger (optional dependency).
func (uc *CreateVMUseCase) WithAuditLogger(al *audit.Logger) *CreateVMUseCase {
	uc.auditLogger = al
	return uc
}

// Execute runs the VM creation use case.
// Phase 1: Creates DomainEvent + Ticket in atomic transaction.
// Phase 2: After approval, K8s create is executed by River worker.
// master-flow.md Stage 5.A: includes duplicate pending guard + audit log.
func (uc *CreateVMUseCase) Execute(ctx context.Context, input CreateVMInput) (*CreateVMOutput, error) {
	if uc.templateSvc == nil {
		return nil, fmt.Errorf("template service is not configured")
	}

	// Validate template exists
	tpl, err := uc.templateSvc.GetByID(ctx, input.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("invalid template: %w", err)
	}
	if !tpl.Enabled {
		return nil, apperrors.BadRequest("TEMPLATE_DISABLED", "selected template is disabled")
	}
	if !service.IsUserRequestableTemplateSource(tpl.SourceType, tpl.ImageURL, tpl.PvcName) {
		return nil, apperrors.BadRequest(
			"TEMPLATE_SOURCE_NOT_REQUESTABLE",
			"selected template boot source is not available in the standard VM request flow",
		)
	}

	// Validate instance size exists
	size, err := uc.instanceSizeSvc.GetByID(ctx, input.InstanceSizeID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance size: %w", err)
	}
	if !size.Enabled {
		return nil, apperrors.BadRequest("INSTANCE_SIZE_DISABLED", "selected instance size is disabled")
	}
	if !service.TemplateInstanceSizeCompatible(tpl.SystemLabels, size.SystemLabels) {
		return nil, apperrors.BadRequest(
			"TEMPLATE_INSTANCE_SIZE_LABEL_MISMATCH",
			"selected instance size is not compatible with selected template system labels",
		).WithParams(map[string]interface{}{
			"template_system_labels":      service.NormalizeSystemLabelsForRead(tpl.SystemLabels),
			"instance_size_system_labels": service.NormalizeSystemLabelsForRead(size.SystemLabels),
		})
	}
	requestedTargets := service.VMRequestTargets{
		TargetCPUCores: input.TargetCPUCores,
		TargetMemoryGi: input.TargetMemoryGi,
		TargetDiskGB:   input.TargetDiskGB,
	}
	if validateErr := service.ValidateVMRequestTargets(requestedTargets); validateErr != nil {
		return nil, apperrors.BadRequest("INVALID_RESOURCE_TARGET", validateErr.Error())
	}
	resolvedTargets := service.ResolveVMRequestTargets(
		size.CPUCores,
		size.CPURequest,
		size.MemoryGi,
		size.MemoryRequestGi,
		size.DiskGB,
		requestedTargets,
	)
	namespaceEnv, err := uc.resolveNamespaceEnvironment(ctx, input.Namespace)
	if err != nil {
		return nil, err
	}
	if !service.CatalogScopeMatchesEnvironment(string(tpl.CatalogScope), namespaceEnv) {
		return nil, apperrors.BadRequest(
			"CATALOG_SCOPE_MISMATCH",
			fmt.Sprintf("template catalog_scope %q does not match namespace environment %q", tpl.CatalogScope, namespaceEnv),
		)
	}
	if !service.CatalogScopeMatchesEnvironment(string(size.CatalogScope), namespaceEnv) {
		return nil, apperrors.BadRequest(
			"CATALOG_SCOPE_MISMATCH",
			fmt.Sprintf("instance size catalog_scope %q does not match namespace environment %q", size.CatalogScope, namespaceEnv),
		)
	}

	// Duplicate pending guard (master-flow.md Stage 5.A):
	// same resource + same operation must return existing ticket reference.
	existingTicket, err := uc.findPendingCreateDuplicate(ctx, input, resolvedTargets)
	if err != nil {
		return nil, fmt.Errorf("check duplicate create request: %w", err)
	}
	if existingTicket != nil {
		return nil, apperrors.Conflict(
			apperrors.CodeDuplicateRequest,
			"a pending VM create request already exists for this resource",
		).WithParams(map[string]interface{}{
			"existing_ticket_id": existingTicket.ID,
			"operation":          "CREATE_VM",
			"resource_id":        strings.TrimSpace(input.ServiceID),
			"namespace":          strings.TrimSpace(input.Namespace),
		})
	}

	// Create domain event payload
	payload := domain.VMCreationPayload{
		RequesterID:    input.RequestedBy,
		ServiceID:      input.ServiceID,
		TemplateID:     input.TemplateID,
		InstanceSizeID: input.InstanceSizeID,
		Namespace:      input.Namespace,
		Reason:         input.Reason,
		TargetCPUCores: resolvedTargets.CPULimit,
		TargetMemoryGi: resolvedTargets.MemoryLimitGi,
		TargetDiskGB:   resolvedTargets.DiskGB,
	}

	payloadBytes, err := payload.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	// Atomic transaction: create DomainEvent + Ticket (ADR-0012)
	var eventID, ticketID string
	txErr := withTx(ctx, uc.entClient, func(tx *ent.Tx) error {
		// Create domain event
		event, err := tx.DomainEvent.Create().
			SetID(generateID()).
			SetEventType(string(domain.EventVMCreationRequested)).
			SetAggregateType("vm").
			SetAggregateID(input.ServiceID).
			SetPayload(payloadBytes).
			SetCreatedBy(input.RequestedBy).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create domain event: %w", err)
		}
		eventID = event.ID

		// Create the request ticket.
		ticket, err := tx.Ticket.Create().
			SetID(generateID()).
			SetEventID(event.ID).
			SetOperationType(entticket.OperationTypeCREATE).
			SetRequester(input.RequestedBy).
			SetReason(input.Reason).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create ticket: %w", err)
		}
		ticketID = ticket.ID

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("create vm request: %w", txErr)
	}

	// Audit log (master-flow.md Stage 5.A)
	if uc.auditLogger != nil {
		_ = uc.auditLogger.LogAction(ctx, "vm.request", "ticket", ticketID, input.RequestedBy, map[string]interface{}{
			"service_id":       input.ServiceID,
			"template_id":      input.TemplateID,
			"instance_size_id": input.InstanceSizeID,
			"namespace":        input.Namespace,
		})
	}

	logger.Info("VM creation request submitted",
		zap.String("event_id", eventID),
		zap.String("ticket_id", ticketID),
		zap.String("requester", input.RequestedBy),
	)

	return &CreateVMOutput{
		TicketID: ticketID,
		EventID:  eventID,
		Status:   "PENDING",
	}, nil
}

func (uc *CreateVMUseCase) findPendingCreateDuplicate(
	ctx context.Context,
	input CreateVMInput,
	resolvedTargets service.ResolvedVMRequestTargets,
) (*ent.Ticket, error) {
	events, err := uc.entClient.DomainEvent.Query().
		Where(
			domainevent.EventTypeEQ(string(domain.EventVMCreationRequested)),
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDEQ(strings.TrimSpace(input.ServiceID)),
			domainevent.StatusEQ(domainevent.StatusPENDING),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		var existing domain.VMCreationPayload
		if err := json.Unmarshal(event.Payload, &existing); err != nil {
			logger.Warn("skip malformed VM create payload while checking duplicates",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
			continue
		}
		if !sameCreateResource(existing, input, resolvedTargets) {
			continue
		}

		ticket, err := uc.entClient.Ticket.Query().
			Where(
				entticket.EventIDEQ(event.ID),
				entticket.OperationTypeEQ(entticket.OperationTypeCREATE),
				entticket.StatusEQ(entticket.StatusPENDING),
			).
			Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				continue
			}
			return nil, err
		}
		return ticket, nil
	}
	return nil, nil
}

func sameCreateResource(
	existing domain.VMCreationPayload,
	input CreateVMInput,
	resolvedTargets service.ResolvedVMRequestTargets,
) bool {
	sameCPU := compareCreateTargetFloat64(existing.TargetCPUCores, input.TargetCPUCores, resolvedTargets.CPULimit)
	sameMemory := compareCreateTargetFloat64(existing.TargetMemoryGi, input.TargetMemoryGi, resolvedTargets.MemoryLimitGi)
	sameDisk := compareCreateTargetInt(existing.TargetDiskGB, input.TargetDiskGB, resolvedTargets.DiskGB)
	return strings.TrimSpace(existing.ServiceID) == strings.TrimSpace(input.ServiceID) &&
		strings.TrimSpace(existing.TemplateID) == strings.TrimSpace(input.TemplateID) &&
		strings.TrimSpace(existing.InstanceSizeID) == strings.TrimSpace(input.InstanceSizeID) &&
		strings.TrimSpace(existing.Namespace) == strings.TrimSpace(input.Namespace) &&
		sameCPU &&
		sameMemory &&
		sameDisk
}

func compareCreateTargetFloat64(existing float64, requested *float64, effective float64) bool {
	if existing > 0 {
		return math.Abs(existing-effective) < 1e-9
	}
	return requested == nil
}

func compareCreateTargetInt(existing int, requested *int, effective int) bool {
	if existing > 0 {
		return existing == effective
	}
	return requested == nil
}

func (uc *CreateVMUseCase) resolveNamespaceEnvironment(
	ctx context.Context,
	namespace string,
) (namespaceregistry.Environment, error) {
	name := strings.TrimSpace(namespace)
	if name == "" {
		return "", apperrors.BadRequest("NAMESPACE_REQUIRED", "namespace is required")
	}

	ns, err := uc.entClient.NamespaceRegistry.Query().
		Where(
			namespaceregistry.NameEQ(name),
			namespaceregistry.EnabledEQ(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", apperrors.BadRequest(
				"NAMESPACE_NOT_FOUND",
				fmt.Sprintf("namespace %q is not registered or disabled", name),
			)
		}
		return "", fmt.Errorf("query namespace registry for %q: %w", name, err)
	}
	return ns.Environment, nil
}

// withTx executes a function within a transaction.
func withTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if v := recover(); v != nil {
			_ = tx.Rollback()
			panic(v)
		}
	}()
	if err := fn(tx); err != nil {
		if rerr := tx.Rollback(); rerr != nil {
			return fmt.Errorf("%w: rollback failed: %w", err, rerr)
		}
		return err
	}
	return tx.Commit()
}

// generateID generates a unique UUID v7 (time-ordered, K-sortable).
func generateID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to v4 if v7 fails (should never happen)
		return uuid.New().String()
	}
	return id.String()
}
