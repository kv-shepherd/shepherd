//go:build ignore

// Package usecase demonstrates the current ADR-0012 sqlc + pgx + River
// transaction boundary for an approval-gated VM create request.
//
// Production code lives in internal/usecase/approval_atomic.go. This example
// intentionally uses the generated sqlc method names so documentation cannot
// drift back to the retired approval-ticket repository API.
package usecase

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/repository/sqlc"
)

// CreateVMAtomicUseCase owns the PostgreSQL transaction used by this example.
type CreateVMAtomicUseCase struct {
	pool        *pgxpool.Pool
	queries     *sqlc.Queries
	riverClient *river.Client[pgx.Tx]
}

func NewCreateVMAtomicUseCase(
	pool *pgxpool.Pool,
	queries *sqlc.Queries,
	riverClient *river.Client[pgx.Tx],
) *CreateVMAtomicUseCase {
	return &CreateVMAtomicUseCase{pool: pool, queries: queries, riverClient: riverClient}
}

// SubmitCreateRequestInput contains already-validated immutable request data.
// Payload is the serialized domain.VMCreationPayload claim check.
type SubmitCreateRequestInput struct {
	EventID     string
	TicketID    string
	AggregateID string
	Payload     []byte
	Requester   string
	Reason      string
}

// SubmitCreateRequestResult distinguishes the raw Ticket state from the public
// API projection: tickets.status is PENDING; the API reports PENDING_APPROVAL.
type SubmitCreateRequestResult struct {
	EventID  string
	TicketID string
	Status   string
}

// SubmitCreateRequest atomically inserts DomainEvent + Ticket. It deliberately
// does not enqueue a River job before approval.
func (uc *CreateVMAtomicUseCase) SubmitCreateRequest(
	ctx context.Context,
	input SubmitCreateRequestInput,
) (*SubmitCreateRequestResult, error) {
	tx, err := uc.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin submit transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := uc.queries.WithTx(tx)
	if err := qtx.InsertDomainEvent(ctx, sqlc.InsertDomainEventParams{
		ID:            input.EventID,
		EventType:     "VM_CREATION_REQUESTED",
		AggregateType: "VM",
		AggregateID:   input.AggregateID,
		Payload:       input.Payload,
		Status:        "PENDING",
		CreatedBy:     input.Requester,
	}); err != nil {
		return nil, fmt.Errorf("insert domain event: %w", err)
	}

	if err := qtx.InsertTicket(ctx, sqlc.InsertTicketParams{
		ID:             input.TicketID,
		EventID:        input.EventID,
		OperationType:  "CREATE",
		Status:         "PENDING",
		Requester:      input.Requester,
		Reason:         optionalText(input.Reason),
		ParentTicketID: pgtype.Text{},
	}); err != nil {
		return nil, fmt.Errorf("insert ticket: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit submit transaction: %w", err)
	}
	return &SubmitCreateRequestResult{
		EventID:  input.EventID,
		TicketID: input.TicketID,
		Status:   "PENDING_APPROVAL",
	}, nil
}

// ApproveCreateInput contains the approval decision and immutable snapshots
// resolved before the transaction begins.
type ApproveCreateInput struct {
	TicketID             string
	EventID              string
	Approver             string
	SelectedClusterID    string
	SelectedStorageClass string
	TemplateSnapshot     []byte
	InstanceSizeSnapshot []byte
	PlacementEvaluation  []byte
	ModifiedSpec         []byte
}

// ApproveAndEnqueue atomically transitions Ticket/Event and inserts the River
// job. All validation and provider lookups must complete before this method.
func (uc *CreateVMAtomicUseCase) ApproveAndEnqueue(
	ctx context.Context,
	input ApproveCreateInput,
) error {
	tx, err := uc.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin approval transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := uc.queries.WithTx(tx)
	affected, err := qtx.ApproveCreateTicket(ctx, sqlc.ApproveCreateTicketParams{
		Approver:             requiredText(input.Approver),
		SelectedClusterID:    requiredText(input.SelectedClusterID),
		SelectedStorageClass: input.SelectedStorageClass,
		TemplateSnapshot:     input.TemplateSnapshot,
		InstanceSizeSnapshot: input.InstanceSizeSnapshot,
		PlacementEvaluation:  input.PlacementEvaluation,
		ModifiedSpec:         input.ModifiedSpec,
		ID:                   input.TicketID,
		EventID:              input.EventID,
	})
	if err != nil {
		return fmt.Errorf("approve create ticket: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("approve create ticket: expected one pending CREATE ticket, updated %d", affected)
	}

	affected, err = qtx.SetDomainEventStatus(ctx, sqlc.SetDomainEventStatusParams{
		ID:     input.EventID,
		Status: "PROCESSING",
	})
	if err != nil {
		return fmt.Errorf("set domain event processing: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("set domain event processing: expected one pending event, updated %d", affected)
	}

	if _, err := uc.riverClient.InsertTx(
		ctx,
		tx,
		jobs.VMCreateArgs{EventID: input.EventID},
		nil,
	); err != nil {
		return fmt.Errorf("insert River job: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit approval transaction: %w", err)
	}
	return nil
}

func optionalText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func requiredText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: true}
}
