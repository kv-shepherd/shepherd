// Package usecase provides Clean Architecture use cases.
//
// ADR-0012: Demonstrates hybrid atomic transaction pattern.
// Uses pgx transaction with Ent, sqlc, and River atomically.
//
// ADR-0015 §3: No SystemID stored in VM or request.
// System is resolved via ServiceID → Service.Edges.System.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/usecase
//
// METHOD SELECTION GUIDE:
//
//	Scenario                              Method
//	─────────────────────────────────────────────────────────────
//	Operation requires approval           Execute()
//	(e.g., CreateVM with approval policy)   → Creates Event + Ticket
//	                                         → No River Job yet
//	                                         → Returns: PENDING_APPROVAL
//
//	Admin approves a pending request      ApproveAndEnqueue()
//	                                         → Updates Ticket status
//	                                         → Inserts River Job atomically
//	                                         → Returns: APPROVED
//
//	Operation auto-approved by policy     AutoApproveAndEnqueue()
//	(e.g., CreateVM for privileged user)    → Creates Event + Ticket + Job
//	                                         → All in single atomic TX
//	                                         → Returns: PROCESSING
package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// CreateVMAtomicUseCase handles VM creation with atomic transaction.
//
// This is the core example of ADR-0012 hybrid transaction pattern:
// 1. Start pgx transaction
// 2. Write DomainEvent via sqlc
// 3. Insert River Job via InsertTx
// 4. Single atomic commit
type CreateVMAtomicUseCase struct {
	pool        *pgxpool.Pool
	sqlcQueries *sqlc.Queries
	riverClient *river.Client[pgx.Tx]
}

// NewCreateVMAtomicUseCase creates a new use case instance.
func NewCreateVMAtomicUseCase(
	pool *pgxpool.Pool,
	sqlcQueries *sqlc.Queries,
	riverClient *river.Client[pgx.Tx],
) *CreateVMAtomicUseCase {
	return &CreateVMAtomicUseCase{
		pool:        pool,
		sqlcQueries: sqlcQueries,
		riverClient: riverClient,
	}
}

// CreateVMRequest contains the VM creation request data.
//
// NOTE (ADR-0015 §3): No SystemID field.
// System is resolved via ServiceID → Service.Edges.System.
//
// NOTE (ADR-0015 §4): No Name field.
// Name is platform-generated: {namespace}-{system}-{service}-{index}
//
// NOTE (ADR-0017): No ClusterID field.
// ClusterID is determined by admin during approval (Stage 5.B).
// User specifies WHAT they want, admin decides WHERE it runs.
type CreateVMRequest struct {
	ServiceID  string // Required: parent service
	TemplateID string // Required: template to use
	Namespace  string // Required: target K8s namespace (immutable after submission)
	// NOTE: ClusterID is NOT here - admin selects during approval (ADR-0017)
	CPU         int    // Optional: override template default
	MemoryMB    int    // Optional: override template default
	Reason      string // Required: business reason for request
	RequestedBy string // Required: user who submitted the request
}

// CreateVMResult contains the VM creation result.
type CreateVMResult struct {
	EventID  string
	TicketID string
}

// Execute performs the VM creation with atomic transaction.
//
// Key Pattern (ADR-0012):
// - DomainEvent write and River Job insert happen in SAME transaction
// - Single tx.Commit() ensures atomicity
// - No orphan events possible (unlike eventual consistency model)
func (uc *CreateVMAtomicUseCase) Execute(ctx context.Context, req CreateVMRequest) (*CreateVMResult, error) {
	// Generate IDs
	eventID := uuid.New().String()
	ticketID := uuid.New().String()

	// Create domain event payload
	// NOTE (ADR-0015 §3): No SystemID - resolved via ServiceID
	// NOTE (ADR-0015 §4): No Name - platform-generated after approval
	// NOTE (ADR-0017): No ClusterID - admin selects during approval
	payload := domain.VMCreationPayload{
		ServiceID:  req.ServiceID,
		TemplateID: req.TemplateID,
		Namespace:  req.Namespace,
		// ClusterID is NOT included - admin determines this during approval (ADR-0017)
		CPU:      req.CPU,
		MemoryMB: req.MemoryMB,
		Reason:   req.Reason,
	}

	// ========== Atomic Transaction ==========
	tx, err := uc.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // Safe: no-op if already committed

	// Step 1: Write DomainEvent via sqlc (within tx)
	sqlcTx := uc.sqlcQueries.WithTx(tx)
	// AggregateID uses ServiceID since VM Name is generated after approval
	err = sqlcTx.CreateDomainEvent(ctx, sqlc.CreateDomainEventParams{
		EventID:       eventID,
		EventType:     "VM_CREATION_REQUESTED",
		AggregateType: "VM",
		AggregateID:   req.ServiceID + "-" + eventID[:8], // Temporary ID, actual VM name assigned later
		Payload:       payload.ToJSON(),
		Status:        "PENDING",
		CreatedBy:     req.RequestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create domain event: %w", err)
	}

	// Step 2: Create ApprovalTicket (within same tx)
	err = sqlcTx.CreateApprovalTicket(ctx, sqlc.CreateApprovalTicketParams{
		TicketID:      ticketID,
		EventID:       eventID,
		RequestType:   "CREATE_VM",
		RequestReason: req.Reason,
		Status:        "PENDING_APPROVAL",
		CreatedBy:     req.RequestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create approval ticket: %w", err)
	}

	// Step 3: River Job insertion strategy (ADR-0006 + ADR-0012)
	//
	// IMPORTANT: This flow demonstrates the "Approval Required" path:
	// - DomainEvent + ApprovalTicket are created atomically
	// - River Job is NOT inserted here (per ADR-0006: "Don't insert River Job before approval")
	// - After admin approval, ApproveAndEnqueue() will insert the River Job atomically
	//
	// For "Auto-Approval" flow (no human approval needed):
	// - Use a separate method that creates Event + Job in single atomic transaction
	// - See AutoApproveAndEnqueue() for that pattern

	// Step 4: Atomic Commit
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &CreateVMResult{
		EventID:  eventID,
		TicketID: ticketID,
	}, nil
}

// ApproveAndEnqueue is called after admin approval.
// Inserts the River job to trigger actual VM creation.
func (uc *CreateVMAtomicUseCase) ApproveAndEnqueue(ctx context.Context, ticketID string, modifiedSpec *domain.ModifiedSpec) error {
	tx, err := uc.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sqlcTx := uc.sqlcQueries.WithTx(tx)

	// Get ticket and event
	ticket, err := sqlcTx.GetApprovalTicket(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket: %w", err)
	}

	// Update ticket status
	err = sqlcTx.UpdateApprovalTicketStatus(ctx, sqlc.UpdateApprovalTicketStatusParams{
		TicketID:     ticketID,
		Status:       "APPROVED",
		ModifiedSpec: modifiedSpec.ToJSON(),
	})
	if err != nil {
		return fmt.Errorf("update ticket: %w", err)
	}

	// Update event status
	err = sqlcTx.UpdateDomainEventStatus(ctx, sqlc.UpdateDomainEventStatusParams{
		EventID: ticket.EventID,
		Status:  "PROCESSING",
	})
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}

	// Insert River Job (atomic with above updates)
	_, err = uc.riverClient.InsertTx(ctx, tx, jobs.EventJobArgs{EventID: ticket.EventID}, nil)
	if err != nil {
		return fmt.Errorf("insert river job: %w", err)
	}

	// Atomic commit
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

// AutoApproveAndEnqueue demonstrates the "Auto-Approval" flow (ADR-0012).
// Used when the operation does not require human approval (e.g., system-level operations).
//
// Key difference from Execute():
// - Event + Ticket + River Job are ALL created in a SINGLE atomic transaction
// - This achieves true ACID atomicity as promised by ADR-0012
func (uc *CreateVMAtomicUseCase) AutoApproveAndEnqueue(ctx context.Context, req CreateVMRequest) (*CreateVMResult, error) {
	eventID := uuid.New().String()
	ticketID := uuid.New().String()

	// NOTE (ADR-0015 §3, §4): No SystemID, no Name in payload
	// NOTE (ADR-0017): No ClusterID - admin selects during approval
	payload := domain.VMCreationPayload{
		ServiceID:  req.ServiceID,
		TemplateID: req.TemplateID,
		Namespace:  req.Namespace,
		// ClusterID is NOT included - admin determines this during approval (ADR-0017)
		CPU:      req.CPU,
		MemoryMB: req.MemoryMB,
		Reason:   req.Reason,
	}

	// ========== Single Atomic Transaction (ADR-0012 True ACID) ==========
	tx, err := uc.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sqlcTx := uc.sqlcQueries.WithTx(tx)

	// Step 1: Create DomainEvent (status = PROCESSING for auto-approve)
	// AggregateID uses ServiceID since VM Name is generated after approval
	err = sqlcTx.CreateDomainEvent(ctx, sqlc.CreateDomainEventParams{
		EventID:       eventID,
		EventType:     "VM_CREATION_REQUESTED",
		AggregateType: "VM",
		AggregateID:   req.ServiceID + "-" + eventID[:8], // Temporary ID, actual VM name assigned later
		Payload:       payload.ToJSON(),
		Status:        "PROCESSING", // Skip PENDING for auto-approve
		CreatedBy:     req.RequestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create domain event: %w", err)
	}

	// Step 2: Create ApprovalTicket (status = APPROVED for auto-approve)
	err = sqlcTx.CreateApprovalTicket(ctx, sqlc.CreateApprovalTicketParams{
		TicketID:      ticketID,
		EventID:       eventID,
		RequestType:   "CREATE_VM",
		RequestReason: req.Reason,
		Status:        "APPROVED", // Auto-approved
		CreatedBy:     req.RequestedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("create approval ticket: %w", err)
	}

	// Step 3: Insert River Job (same transaction - ADR-0012 core pattern)
	_, err = uc.riverClient.InsertTx(ctx, tx, jobs.EventJobArgs{EventID: eventID}, nil)
	if err != nil {
		return nil, fmt.Errorf("insert river job: %w", err)
	}

	// Step 4: Single Atomic Commit - All three succeed or all fail
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return &CreateVMResult{
		EventID:  eventID,
		TicketID: ticketID,
	}, nil
}
