package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/attribute"

	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/observability"
	sqlcrepo "kv-shepherd.io/shepherd/internal/repository/sqlc"
)

const (
	batchPowerEventType               = "BATCH_POWER_REQUESTED"
	batchPowerType                    = "BATCH_POWER"
	batchPowerTicketOperation         = "POWER"
	batchPowerAggregateType           = "batch"
	batchPowerChildAggregateType      = "vm"
	batchPowerEventStatusPending      = "PENDING"
	batchPowerEventStatusProcessing   = "PROCESSING"
	batchPowerTicketStatusExecuting   = "EXECUTING"
	batchPowerBatchStatusInProgress   = "IN_PROGRESS"
	batchPowerBatchStatusPending      = "PENDING_APPROVAL"
	maxBatchPowerChildCountForSQLCInt = 1<<31 - 1
)

// BatchPowerSubmissionInput describes the durable parent/child rows for a
// batch power request. No River job is visible until this transaction commits.
type BatchPowerSubmissionInput struct {
	ParentID         string
	Actor            string
	RequestID        string
	Reason           string
	ParentPayload    []byte
	RequiresApproval bool
	Children         []BatchPowerChildInput
}

// BatchPowerChildInput is a single child VM operation in a batch power request.
type BatchPowerChildInput struct {
	EventType   string
	AggregateID string
	Payload     []byte
	Reason      string
}

// BatchPowerRetryInput describes failed power children that should be reset
// and re-enqueued atomically.
type BatchPowerRetryInput struct {
	ParentID string
	Children []BatchPowerRetryChildInput
}

// BatchPowerRetryChildInput identifies a persisted child ticket/event pair.
type BatchPowerRetryChildInput struct {
	TicketID string
	EventID  string
}

// CreateBatchPowerAndMaybeEnqueue atomically persists the batch parent, child
// tickets/events, and direct-execution River jobs. River InsertTx keeps job
// visibility tied to the same commit as the application rows.
func (w *ApprovalAtomicWriter) CreateBatchPowerAndMaybeEnqueue(ctx context.Context, input BatchPowerSubmissionInput) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.batch_power.submit",
		attribute.String("shepherd.business.operation", "batch_power.submit"),
		attribute.String("shepherd.batch.type", batchPowerType),
		attribute.Bool("shepherd.approval.required", input.RequiresApproval),
		attribute.Int("shepherd.batch.child_count", len(input.Children)),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if !input.RequiresApproval && w.riverClient == nil {
		return fmt.Errorf("approval atomic writer river client is not initialized")
	}
	if validationErr := validateBatchPowerSubmissionInput(input); validationErr != nil {
		return validationErr
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin batch power tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)

	parentEventID, err := newV7ID()
	if err != nil {
		return fmt.Errorf("generate parent event id: %w", err)
	}
	childCount, err := sqlcInt32Count(len(input.Children), "batch power child count")
	if err != nil {
		return err
	}

	parentEventStatus := batchPowerEventStatusProcessing
	parentTicketStatus := batchPowerTicketStatusExecuting
	batchStatus := batchPowerBatchStatusInProgress
	if input.RequiresApproval {
		parentEventStatus = batchPowerEventStatusPending
		parentTicketStatus = batchPowerEventStatusPending
		batchStatus = batchPowerBatchStatusPending
	}

	if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
		ID:            parentEventID,
		EventType:     batchPowerEventType,
		AggregateType: batchPowerAggregateType,
		AggregateID:   strings.TrimSpace(input.ParentID),
		Payload:       input.ParentPayload,
		Status:        parentEventStatus,
		CreatedBy:     strings.TrimSpace(input.Actor),
	}); err != nil {
		return fmt.Errorf("insert batch power parent event %s: %w", parentEventID, err)
	}

	if err := qtx.InsertTicket(ctx, sqlcrepo.InsertTicketParams{
		ID:             strings.TrimSpace(input.ParentID),
		EventID:        parentEventID,
		OperationType:  batchPowerTicketOperation,
		Status:         parentTicketStatus,
		Requester:      strings.TrimSpace(input.Actor),
		Reason:         textOrNull(input.Reason),
		ParentTicketID: pgtype.Text{},
	}); err != nil {
		return fmt.Errorf("insert batch power parent ticket %s: %w", input.ParentID, err)
	}

	if err := qtx.InsertBatchTicket(ctx, sqlcrepo.InsertBatchTicketParams{
		ID:           strings.TrimSpace(input.ParentID),
		BatchType:    batchPowerType,
		ChildCount:   childCount,
		PendingCount: childCount,
		Status:       batchStatus,
		CreatedBy:    strings.TrimSpace(input.Actor),
		RequestID:    textOrNull(input.RequestID),
		Reason:       textOrNull(input.Reason),
	}); err != nil {
		return fmt.Errorf("insert batch power projection %s: %w", input.ParentID, err)
	}

	childTicketStatus := batchPowerTicketStatusExecuting
	if input.RequiresApproval {
		childTicketStatus = batchPowerEventStatusPending
	}
	for idx, child := range input.Children {
		childEventID, err := newV7ID()
		if err != nil {
			return fmt.Errorf("generate child event id %d: %w", idx, err)
		}
		childTicketID, err := newV7ID()
		if err != nil {
			return fmt.Errorf("generate child ticket id %d: %w", idx, err)
		}

		if err := qtx.InsertDomainEvent(ctx, sqlcrepo.InsertDomainEventParams{
			ID:            childEventID,
			EventType:     strings.TrimSpace(child.EventType),
			AggregateType: batchPowerChildAggregateType,
			AggregateID:   strings.TrimSpace(child.AggregateID),
			Payload:       child.Payload,
			Status:        batchPowerEventStatusPending,
			CreatedBy:     strings.TrimSpace(input.Actor),
		}); err != nil {
			return fmt.Errorf("insert batch power child event %s: %w", childEventID, err)
		}
		if err := qtx.InsertTicket(ctx, sqlcrepo.InsertTicketParams{
			ID:             childTicketID,
			EventID:        childEventID,
			OperationType:  batchPowerTicketOperation,
			Status:         childTicketStatus,
			Requester:      strings.TrimSpace(input.Actor),
			Reason:         textOrNull(child.Reason),
			ParentTicketID: textOrNull(input.ParentID),
		}); err != nil {
			return fmt.Errorf("insert batch power child ticket %s: %w", childTicketID, err)
		}
		if !input.RequiresApproval {
			if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{
				EventID: childEventID,
			}, nil); err != nil {
				return fmt.Errorf("enqueue vm_power for child event %s: %w", childEventID, err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch power tx: %w", err)
	}
	return nil
}

func sqlcInt32Count(count int, label string) (int32, error) {
	if count < 0 || count > maxBatchPowerChildCountForSQLCInt {
		return 0, fmt.Errorf("%s exceeds int32 limit", label)
	}
	return int32(count), nil // #nosec G115 -- guarded by the int32 bounds check above.
}

func validateBatchPowerSubmissionInput(input BatchPowerSubmissionInput) error {
	if strings.TrimSpace(input.ParentID) == "" ||
		strings.TrimSpace(input.Actor) == "" ||
		len(input.ParentPayload) == 0 ||
		len(input.Children) == 0 {
		return fmt.Errorf("batch power input is incomplete")
	}
	for idx, child := range input.Children {
		if strings.TrimSpace(child.EventType) == "" ||
			strings.TrimSpace(child.AggregateID) == "" ||
			len(child.Payload) == 0 {
			return fmt.Errorf("batch power child %d is incomplete", idx)
		}
	}
	return nil
}

// RetryBatchPowerAndEnqueue resets failed power children and inserts their
// River jobs in one transaction.
func (w *ApprovalAtomicWriter) RetryBatchPowerAndEnqueue(ctx context.Context, input BatchPowerRetryInput) (err error) {
	ctx, span := observability.StartSpan(ctx,
		"business.batch_power.retry",
		attribute.String("shepherd.business.operation", "batch_power.retry"),
		attribute.String("shepherd.batch.type", batchPowerType),
		attribute.Int("shepherd.batch.child_count", len(input.Children)),
	)
	defer func() {
		observability.RecordSpanError(span, err)
		span.End()
	}()

	if w.pool == nil || w.riverClient == nil || w.queries == nil {
		return fmt.Errorf("approval atomic writer is not initialized")
	}
	if validationErr := validateBatchPowerRetryInput(input); validationErr != nil {
		return validationErr
	}

	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin batch power retry tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := w.queries.WithTx(tx)
	parentID := strings.TrimSpace(input.ParentID)
	for _, child := range input.Children {
		ticketID := strings.TrimSpace(child.TicketID)
		eventID := strings.TrimSpace(child.EventID)
		affected, err := qtx.ResetPowerRetryTicket(ctx, sqlcrepo.ResetPowerRetryTicketParams{
			ID:             ticketID,
			EventID:        eventID,
			ParentTicketID: textOrNull(parentID),
		})
		if err != nil {
			return fmt.Errorf("reset power retry ticket %s: %w", ticketID, err)
		}
		if affected == 0 {
			return fmt.Errorf("reset power retry ticket %s: not found, not retryable, or event mismatch", ticketID)
		}
		affected, err = qtx.ResetDomainEventForRetry(ctx, eventID)
		if err != nil {
			return fmt.Errorf("reset power retry event %s: %w", eventID, err)
		}
		if affected == 0 {
			return fmt.Errorf("reset power retry event %s: not found or not retryable", eventID)
		}
		if _, err := w.riverClient.InsertTx(ctx, tx, jobs.VMPowerArgs{
			EventID: eventID,
		}, nil); err != nil {
			return fmt.Errorf("enqueue vm_power retry for event %s: %w", eventID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit batch power retry tx: %w", err)
	}
	return nil
}

func validateBatchPowerRetryInput(input BatchPowerRetryInput) error {
	if strings.TrimSpace(input.ParentID) == "" || len(input.Children) == 0 {
		return fmt.Errorf("batch power retry input is incomplete")
	}
	for idx, child := range input.Children {
		if strings.TrimSpace(child.TicketID) == "" || strings.TrimSpace(child.EventID) == "" {
			return fmt.Errorf("batch power retry child %d is incomplete", idx)
		}
	}
	return nil
}

func textOrNull(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	return pgtype.Text{String: trimmed, Valid: trimmed != ""}
}

func newV7ID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}
