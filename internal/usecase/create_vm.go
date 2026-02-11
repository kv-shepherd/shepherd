// Package usecase provides application use cases (Clean Architecture).
//
// UseCases are reusable across HTTP, CLI, gRPC, Cron.
// ADR-0012: Atomic transactions managed at UseCase level.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

// CreateVMInput represents the input for creating a VM.
type CreateVMInput struct {
	ServiceID      string `json:"service_id"`
	TemplateID     string `json:"template_id"`
	InstanceSizeID string `json:"instance_size_id"`
	Namespace      string `json:"namespace"`
	Reason         string `json:"reason"`
	RequestedBy    string `json:"requested_by"`
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
	auditLogger     *audit.Logger
}

// NewCreateVMUseCase creates a new CreateVMUseCase.
func NewCreateVMUseCase(
	entClient *ent.Client,
	vmService *service.VMService,
	instanceSizeSvc *service.InstanceSizeService,
) *CreateVMUseCase {
	return &CreateVMUseCase{
		entClient:       entClient,
		vmService:       vmService,
		instanceSizeSvc: instanceSizeSvc,
	}
}

// WithAuditLogger sets the audit logger (optional dependency).
func (uc *CreateVMUseCase) WithAuditLogger(al *audit.Logger) *CreateVMUseCase {
	uc.auditLogger = al
	return uc
}

// Execute runs the VM creation use case.
// Phase 1: Creates DomainEvent + ApprovalTicket in atomic transaction.
// Phase 2: After approval, K8s create is executed by River worker.
// master-flow.md Stage 5.A: includes duplicate pending guard + audit log.
func (uc *CreateVMUseCase) Execute(ctx context.Context, input CreateVMInput) (*CreateVMOutput, error) {
	// Validate instance size exists
	_, err := uc.instanceSizeSvc.GetByID(ctx, input.InstanceSizeID)
	if err != nil {
		return nil, fmt.Errorf("invalid instance size: %w", err)
	}

	// Duplicate pending guard (master-flow.md Stage 5.A)
	existingCount, err := uc.entClient.ApprovalTicket.Query().
		Where(
			approvalticket.StatusEQ(approvalticket.StatusPENDING),
			approvalticket.RequesterEQ(input.RequestedBy),
		).Count(ctx)
	if err == nil && existingCount > 0 {
		return nil, apperrors.Conflict(apperrors.CodeDuplicateRequest,
			"a pending VM request already exists for this user")
	}

	// Create domain event payload
	payload := domain.VMCreationPayload{
		RequesterID:    input.RequestedBy,
		ServiceID:      input.ServiceID,
		TemplateID:     input.TemplateID,
		InstanceSizeID: input.InstanceSizeID,
		Namespace:      input.Namespace,
		Reason:         input.Reason,
	}

	payloadBytes, err := payload.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	// Atomic transaction: create DomainEvent + ApprovalTicket (ADR-0012)
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

		// Create approval ticket
		ticket, err := tx.ApprovalTicket.Create().
			SetID(generateID()).
			SetEventID(event.ID).
			SetRequester(input.RequestedBy).
			SetReason(input.Reason).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create approval ticket: %w", err)
		}
		ticketID = ticket.ID

		return nil
	})

	if txErr != nil {
		return nil, fmt.Errorf("create vm request: %w", txErr)
	}

	// Audit log (master-flow.md Stage 5.A)
	if uc.auditLogger != nil {
		_ = uc.auditLogger.LogAction(ctx, "vm.request", "approval_ticket", ticketID, input.RequestedBy, map[string]interface{}{
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
			return fmt.Errorf("%w: rolling back: %v", err, rerr)
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
