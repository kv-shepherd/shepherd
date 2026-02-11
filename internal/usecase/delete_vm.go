// Package usecase — DeleteVMUseCase orchestrates the VM deletion approval flow.
//
// ADR-0015 §5.D: VM deletion requires approval ticket.
// ADR-0012: Atomic transaction for DomainEvent + ApprovalTicket.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
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

	// Step 2: Confirmation gate (ADR-0015 §13 addendum).
	confirmed := false
	if input.Confirm {
		confirmed = true
	}
	if input.ConfirmName != "" {
		if input.ConfirmName == vm.Name {
			confirmed = true
		} else {
			return nil, apperrors.BadRequest("CONFIRMATION_NAME_MISMATCH",
				fmt.Sprintf("expected '%s', got '%s'", vm.Name, input.ConfirmName))
		}
	}
	if !confirmed {
		return nil, apperrors.BadRequest("DELETE_CONFIRMATION_REQUIRED",
			"deletion requires confirm=true or confirm_name matching VM name")
	}

	// Step 3: State guard — only STOPPED or FAILED VMs can be deleted.
	if vm.Status != entvm.StatusSTOPPED && vm.Status != entvm.StatusFAILED {
		return nil, apperrors.Conflict("INVALID_STATE_TRANSITION",
			fmt.Sprintf("cannot delete VM in %s state, must be STOPPED or FAILED", vm.Status))
	}

	// Step 4: Duplicate pending guard — prevent multiple delete requests for same VM.
	existingCount, err := uc.entClient.ApprovalTicket.Query().
		Where(
			approvalticket.StatusEQ(approvalticket.StatusPENDING),
			approvalticket.OperationTypeEQ(approvalticket.OperationTypeDELETE),
			approvalticket.RequesterEQ(input.RequestedBy),
		).Count(ctx)
	if err == nil && existingCount > 0 {
		return nil, apperrors.Conflict(apperrors.CodeDuplicateRequest,
			"a pending VM delete request already exists for this user")
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
