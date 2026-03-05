// Package usecase — DeleteVMUseCase orchestrates the VM deletion approval flow.
//
// ADR-0015 §5.D: VM deletion requires approval ticket.
// ADR-0012: Atomic transaction for DomainEvent + ApprovalTicket.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
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

// DeleteVMUseCase orchestrates VM deletion through the approval flow.
// ADR-0015 §5.D: VM deletion requires an approval ticket with operation_type=DELETE.
// Flow: User confirms deletion → DomainEvent + ApprovalTicket created → Admin approves → River job executes K8s delete.
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
// Phase 2: Creates DomainEvent + ApprovalTicket (operation_type=DELETE) in atomic transaction.
// Phase 3: After admin approval, Gateway enqueues River job for K8s deletion.
func (uc *DeleteVMUseCase) Execute(ctx context.Context, input DeleteVMInput) (*DeleteVMOutput, error) {
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
	if err := validateDeleteConfirmationByEnvironment(vm.Name, nsEnv, input.Confirm, input.ConfirmName); err != nil {
		return nil, err
	}

	// Step 3: State guard — only STOPPED or FAILED VMs can be deleted.
	if vm.Status != entvm.StatusSTOPPED && vm.Status != entvm.StatusFAILED {
		return nil, apperrors.Conflict("INVALID_STATE_TRANSITION",
			fmt.Sprintf("cannot delete VM in %s state, must be STOPPED or FAILED", vm.Status))
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
	payload := domain.VMDeletePayload{
		VMID:      input.VMID,
		VMName:    vm.Name,
		ClusterID: vm.ClusterID,
		Namespace: vm.Namespace,
		Actor:     input.RequestedBy,
	}
	payloadBytes, err := payload.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal delete payload: %w", err)
	}

	// Step 6: Atomic transaction — DomainEvent + ApprovalTicket (ADR-0012).
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

		// Create approval ticket with operation_type=DELETE.
		ticket, err := tx.ApprovalTicket.Create().
			SetID(generateID()).
			SetEventID(event.ID).
			SetOperationType(approvalticket.OperationTypeDELETE).
			SetRequester(input.RequestedBy).
			SetReason(reason).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create approval ticket: %w", err)
		}
		ticketID = ticket.ID

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("create vm delete request: %w", txErr)
	}

	// Step 7: Audit log (best-effort, outside transaction).
	if uc.auditLogger != nil {
		_ = uc.auditLogger.LogAction(ctx, "vm.delete_requested", "approval_ticket", ticketID, input.RequestedBy, map[string]interface{}{
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
) (*ent.ApprovalTicket, error) {
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

		ticket, err := uc.entClient.ApprovalTicket.Query().
			Where(
				approvalticket.EventIDEQ(event.ID),
				approvalticket.OperationTypeEQ(approvalticket.OperationTypeDELETE),
				approvalticket.StatusEQ(approvalticket.StatusPENDING),
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
