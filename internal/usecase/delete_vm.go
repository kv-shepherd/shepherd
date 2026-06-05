// Package usecase — DeleteVMUseCase orchestrates the VM deletion approval flow.
//
// ADR-0015 §5.D: VM deletion requires a ticket.
// ADR-0012: Atomic transaction for DomainEvent + Ticket.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entservice "kv-shepherd.io/shepherd/ent/service"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/observability"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// DeleteVMInput represents the input for requesting VM deletion.
type DeleteVMInput struct {
	VMID        string `json:"vm_id"`
	Confirm     bool   `json:"confirm"`
	ConfirmName string `json:"confirm_name"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requested_by"`
}

// DeleteVMOutput represents the output of a VM deletion request.
type DeleteVMOutput struct {
	TicketID string `json:"ticket_id"`
	EventID  string `json:"event_id"`
	Status   string `json:"status"`
}

const VMDeleteInvalidStateCode = "VM_DELETE_INVALID_STATE"

var vmDeleteAllowedStates = []string{
	string(entvm.StatusSTOPPED),
	string(entvm.StatusFAILED),
	string(entvm.StatusNOT_FOUND),
	string(entvm.StatusUNKNOWN),
}

// DeleteVMUseCase orchestrates VM deletion through the approval flow.
// ADR-0015 §5.D: VM deletion requires a ticket with operation_type=DELETE.
// Flow: User confirms deletion → DomainEvent + Ticket created → Admin approves → River job executes K8s delete.
type DeleteVMUseCase struct {
	entClient   *ent.Client
	auditLogger *audit.Logger
}

// NewDeleteVMUseCase creates a new DeleteVMUseCase.
func NewDeleteVMUseCase(entClient *ent.Client) *DeleteVMUseCase {
	return &DeleteVMUseCase{entClient: entClient}
}

// WithAuditLogger sets the audit logger (optional dependency).
func (uc *DeleteVMUseCase) WithAuditLogger(al *audit.Logger) *DeleteVMUseCase {
	uc.auditLogger = al
	return uc
}

// Execute runs the VM deletion request use case.
// Phase 1: Validates VM state and confirmation.
// Phase 2: Creates DomainEvent + Ticket (operation_type=DELETE) in atomic transaction.
// Phase 3: After admin approval, the core ticket service enqueues the River job for K8s deletion.
func (uc *DeleteVMUseCase) Execute(ctx context.Context, input DeleteVMInput) (output *DeleteVMOutput, err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.vm.request_delete",
		attribute.String("shepherd.business.operation", "vm.request_delete"),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	// Step 1: Fetch VM and validate state.
	vm, err := uc.entClient.VM.Get(ctx, input.VMID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, apperrors.NotFound(apperrors.CodeVMNotFound, fmt.Sprintf("VM %s not found", input.VMID))
		}
		return nil, fmt.Errorf("get VM %s: %w", input.VMID, err)
	}

	// Step 2: Resolve namespace environment and apply tiered confirmation policy (ADR-0015 §13).
	nsEnv, err := uc.resolveNamespaceEnvironment(ctx, vm.Namespace)
	if err != nil {
		return nil, err
	}
	if validationErr := validateDeleteConfirmationByEnvironment(vm.Name, nsEnv, input.Confirm, input.ConfirmName); validationErr != nil {
		return nil, validationErr
	}

	// Step 3: State guard — STOPPED, FAILED, NOT_FOUND, or UNKNOWN VMs can be deleted.
	// NOT_FOUND: K8s resource no longer exists — safe to clean up DB record.
	// UNKNOWN: cluster unreachable — allow deletion request (K8s cleanup may fail but DB will be cleaned).
	if !VMDeleteAllowedStatus(vm.Status) {
		return nil, apperrors.Conflict(
			VMDeleteInvalidStateCode,
			VMDeleteInvalidStateMessage(vm.Status),
		).WithParams(VMDeleteInvalidStateParams(vm.Status))
	}

	// Step 4: Duplicate pending guard — same resource + same operation.
	existingTicket, err := uc.findPendingDeleteDuplicate(ctx, input.VMID)
	if err != nil {
		return nil, fmt.Errorf("check duplicate delete request: %w", err)
	}
	if existingTicket != nil {
		return nil, apperrors.Conflict(
			apperrors.CodeDuplicateRequest,
			"a pending VM delete request already exists for this resource",
		).WithParams(map[string]interface{}{
			"existing_ticket_id": existingTicket.ID,
			"operation":          "DELETE_VM",
			"resource_id":        input.VMID,
		})
	}

	// Step 5: Build domain event payload.
	serviceID := ""
	serviceName := ""
	systemID := ""
	systemName := ""
	if svc, svcErr := uc.entClient.Service.Query().
		Where(entservice.HasVmsWith(entvm.IDEQ(vm.ID))).
		WithSystem().
		Only(ctx); svcErr == nil && svc != nil {
		serviceID = svc.ID
		serviceName = svc.Name
		if svc.Edges.System != nil {
			systemID = svc.Edges.System.ID
			systemName = svc.Edges.System.Name
		}
	}

	clusterName := ""
	clusterEnvironment := ""
	if strings.TrimSpace(vm.ClusterID) != "" {
		if cluster, clusterErr := uc.entClient.Cluster.Query().
			Where(entcluster.IDEQ(vm.ClusterID)).
			Only(ctx); clusterErr == nil && cluster != nil {
			clusterName = strings.TrimSpace(cluster.DisplayName)
			if clusterName == "" {
				clusterName = strings.TrimSpace(cluster.Name)
			}
			if clusterName == "" {
				clusterName = cluster.ID
			}
			clusterEnvironment = string(cluster.Environment)
		}
	}

	ownerDisplayName := ""
	ownerUsername := ""
	if strings.TrimSpace(vm.CreatedBy) != "" {
		if owner, ownerErr := uc.entClient.User.Get(ctx, vm.CreatedBy); ownerErr == nil && owner != nil {
			ownerDisplayName = strings.TrimSpace(owner.DisplayName)
			if ownerDisplayName == "" {
				ownerDisplayName = strings.TrimSpace(owner.Username)
			}
			if ownerDisplayName == "" {
				ownerDisplayName = vm.CreatedBy
			}
			ownerUsername = strings.TrimSpace(owner.Username)
			if ownerUsername == "" {
				ownerUsername = vm.CreatedBy
			}
		} else {
			ownerDisplayName = vm.CreatedBy
			ownerUsername = vm.CreatedBy
		}
	}

	payload := domain.VMDeletePayload{
		VMID:               input.VMID,
		VMName:             vm.Name,
		ClusterID:          vm.ClusterID,
		ClusterName:        clusterName,
		ClusterEnvironment: clusterEnvironment,
		Namespace:          vm.Namespace,
		SystemID:           systemID,
		SystemName:         systemName,
		ServiceID:          serviceID,
		ServiceName:        serviceName,
		OwnerID:            vm.CreatedBy,
		OwnerDisplayName:   ownerDisplayName,
		OwnerUsername:      ownerUsername,
		RequestVMStatus:    string(vm.Status),
		Actor:              input.RequestedBy,
	}
	if strings.TrimSpace(vm.TicketID) != "" {
		if ticket, ticketErr := uc.entClient.Ticket.Get(ctx, vm.TicketID); ticketErr == nil && strings.TrimSpace(ticket.EventID) != "" {
			if event, eventErr := uc.entClient.DomainEvent.Get(ctx, ticket.EventID); eventErr == nil {
				var createPayload domain.VMCreationPayload
				if payloadErr := json.Unmarshal(event.Payload, &createPayload); payloadErr == nil {
					applyDeleteCreationSnapshot(&payload, createPayload)
				}
			}
		}
	}
	payloadBytes, err := payload.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal delete payload: %w", err)
	}

	// Step 6: Atomic transaction — DomainEvent + Ticket (ADR-0012).
	reason := input.Reason
	if reason == "" {
		reason = fmt.Sprintf("Request to delete VM %s", vm.Name)
	}

	var eventID, ticketID string
	txErr := withTx(ctx, uc.entClient, func(tx *ent.Tx) error {
		// Create domain event.
		event, err := tx.DomainEvent.Create().
			SetID(generateID()).
			SetEventType(string(domain.EventVMDeletionRequested)).
			SetAggregateType("vm").
			SetAggregateID(input.VMID).
			SetPayload(payloadBytes).
			SetCreatedBy(input.RequestedBy).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create domain event: %w", err)
		}
		eventID = event.ID

		// Create the deletion ticket with operation_type=DELETE.
		ticket, err := tx.Ticket.Create().
			SetID(generateID()).
			SetEventID(event.ID).
			SetOperationType(entticket.OperationTypeDELETE).
			SetRequester(input.RequestedBy).
			SetReason(reason).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create ticket: %w", err)
		}
		ticketID = ticket.ID

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("create vm delete request: %w", txErr)
	}

	// Step 7: Audit log (best-effort, outside transaction).
	if uc.auditLogger != nil {
		_ = uc.auditLogger.LogAction(ctx, "vm.delete_requested", "ticket", ticketID, input.RequestedBy, map[string]interface{}{
			"vm_id":   input.VMID,
			"vm_name": vm.Name,
		})
	}

	logger.Info("VM deletion request submitted (pending approval)",
		zap.String("event_id", eventID),
		zap.String("ticket_id", ticketID),
		zap.String("vm_id", input.VMID),
		zap.String("requester", input.RequestedBy),
	)

	return &DeleteVMOutput{
		TicketID: ticketID,
		EventID:  eventID,
		Status:   "PENDING",
	}, nil
}

func VMDeleteAllowedStatus(status entvm.Status) bool {
	switch status {
	case entvm.StatusSTOPPED, entvm.StatusFAILED, entvm.StatusNOT_FOUND, entvm.StatusUNKNOWN:
		return true
	default:
		return false
	}
}

func VMDeleteAllowedStatesLabel() string {
	return strings.Join(vmDeleteAllowedStates, ", ")
}

func VMDeleteInvalidStateMessage(status entvm.Status) string {
	return fmt.Sprintf(
		"cannot delete VM in %s state, must be %s",
		status,
		VMDeleteAllowedStatesLabel(),
	)
}

func VMDeleteInvalidStateParams(status entvm.Status) map[string]interface{} {
	return map[string]interface{}{
		"current_state":  string(status),
		"allowed_states": VMDeleteAllowedStatesLabel(),
	}
}

func applyDeleteCreationSnapshot(payload *domain.VMDeletePayload, createPayload domain.VMCreationPayload) {
	if payload == nil {
		return
	}
	payload.TemplateID = strings.TrimSpace(createPayload.TemplateID)
	payload.TemplateName = strings.TrimSpace(createPayload.TemplateName)
	payload.InstanceSizeID = strings.TrimSpace(createPayload.InstanceSizeID)
	payload.InstanceSizeName = strings.TrimSpace(createPayload.InstanceSizeName)
	payload.CurrentCPUCores = createPayload.TargetCPUCores
	payload.CurrentMemoryGi = createPayload.TargetMemoryGi
	payload.CurrentDiskGB = createPayload.TargetDiskGB
	if payload.ServiceName == "" {
		payload.ServiceName = strings.TrimSpace(createPayload.ServiceName)
	}
	if payload.SystemID == "" {
		payload.SystemID = strings.TrimSpace(createPayload.SystemID)
	}
	if payload.SystemName == "" {
		payload.SystemName = strings.TrimSpace(createPayload.SystemName)
	}
	if payload.OwnerID == "" {
		payload.OwnerID = strings.TrimSpace(createPayload.OwnerID)
	}
	if payload.OwnerDisplayName == "" {
		payload.OwnerDisplayName = strings.TrimSpace(createPayload.OwnerDisplayName)
	}
	if payload.OwnerUsername == "" {
		payload.OwnerUsername = strings.TrimSpace(createPayload.OwnerUsername)
	}
}

func (uc *DeleteVMUseCase) resolveNamespaceEnvironment(ctx context.Context, namespace string) (namespaceregistry.Environment, error) {
	name := strings.TrimSpace(namespace)
	if name == "" {
		return "", apperrors.BadRequest("NAMESPACE_REQUIRED", "vm namespace is empty")
	}

	registry, err := uc.entClient.NamespaceRegistry.Query().
		Where(
			namespaceregistry.NameEQ(name),
			namespaceregistry.EnabledEQ(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", apperrors.BadRequest(
				"NAMESPACE_ENVIRONMENT_NOT_FOUND",
				fmt.Sprintf("namespace %q is not registered or disabled", name),
			)
		}
		return "", fmt.Errorf("query namespace registry for %q: %w", name, err)
	}
	return registry.Environment, nil
}

func validateDeleteConfirmationByEnvironment(
	vmName string,
	environment namespaceregistry.Environment,
	confirm bool,
	confirmName string,
) error {
	name := strings.TrimSpace(vmName)
	confirmName = strings.TrimSpace(confirmName)

	switch environment {
	case namespaceregistry.EnvironmentTest:
		if confirm {
			return nil
		}
		return apperrors.BadRequest(
			"DELETE_CONFIRMATION_REQUIRED",
			"test environment deletion requires confirm=true",
		)
	case namespaceregistry.EnvironmentProd:
		if confirmName == "" {
			return apperrors.BadRequest(
				"DELETE_CONFIRMATION_REQUIRED",
				"prod environment deletion requires confirm_name matching VM name",
			)
		}
		if confirmName != name {
			return apperrors.BadRequest(
				"CONFIRMATION_NAME_MISMATCH",
				fmt.Sprintf("expected '%s', got '%s'", name, confirmName),
			)
		}
		return nil
	default:
		return apperrors.BadRequest(
			"UNSUPPORTED_NAMESPACE_ENVIRONMENT",
			fmt.Sprintf("unsupported namespace environment: %s", environment),
		)
	}
}

func (uc *DeleteVMUseCase) findPendingDeleteDuplicate(
	ctx context.Context,
	vmID string,
) (*ent.Ticket, error) {
	events, err := uc.entClient.DomainEvent.Query().
		Where(
			domainevent.EventTypeEQ(string(domain.EventVMDeletionRequested)),
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDEQ(strings.TrimSpace(vmID)),
			domainevent.StatusEQ(domainevent.StatusPENDING),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		var payload domain.VMDeletePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			logger.Warn("skip malformed VM delete payload while checking duplicates",
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
			continue
		}
		if strings.TrimSpace(payload.VMID) != strings.TrimSpace(vmID) {
			continue
		}

		ticket, err := uc.entClient.Ticket.Query().
			Where(
				entticket.EventIDEQ(event.ID),
				entticket.OperationTypeEQ(entticket.OperationTypeDELETE),
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
