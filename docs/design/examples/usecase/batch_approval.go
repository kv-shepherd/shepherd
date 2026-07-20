//go:build ignore

// Package usecase documents the Stage 5.E batch-operation transaction
// boundaries. It is intentionally non-runnable: production types live in the
// handler, ticketing, jobs, and atomic-writer packages referenced by ADR-0015.
package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
)

const maxLogicalDispatchAttempts = 3

// BatchSubmitRequest keeps request_id optional and opaque. Idempotency is
// scoped by the authenticated actor and the exact requested operation, so
// POWER_START, POWER_STOP, and POWER_RESTART never share a replay scope.
type BatchSubmitRequest struct {
	Operation string
	Items     []BatchTargetItem
	RequestID string
	Reason    string
}

type BatchTargetItem struct {
	ResourceID string
	Payload    any
}

type BatchSubmitResult struct {
	BatchID           string
	Status            string
	StatusURL         string
	RetryAfterSeconds int
}

type BatchActionResult struct {
	BatchID           string
	AffectedCount     int
	AffectedTicketIDs []string
}

// BatchApprovalUseCase demonstrates the current durable boundaries. The
// submission repository owns one READ COMMITTED transaction for locks, replay,
// mutable quota reads, and persistence. The atomic decision writer owns River
// InsertTx for initial approval and explicit retry.
type BatchApprovalUseCase struct {
	submissions SubmissionRepository
	decisions   AtomicBatchDecisionWriter
}

func NewBatchApprovalUseCase(
	submissions SubmissionRepository,
	decisions AtomicBatchDecisionWriter,
) *BatchApprovalUseCase {
	return &BatchApprovalUseCase{submissions: submissions, decisions: decisions}
}

// Execute creates one parent and its children atomically. The callback order is
// part of the contract: global -> actor -> request -> replay -> current limits
// -> rows. A replay is resolved before mutable throttling and returns the
// earliest matching historical parent without rewriting legacy request IDs.
func (u *BatchApprovalUseCase) Execute(
	ctx context.Context,
	req BatchSubmitRequest,
	actor string,
) (*BatchSubmitResult, error) {
	if err := validateBatchRequest(req, actor); err != nil {
		return nil, err
	}

	return u.submissions.WithSubmissionTx(ctx, func(tx SubmissionTx) (*BatchSubmitResult, error) {
		if err := tx.LockGlobal(ctx); err != nil {
			return nil, err
		}
		if err := tx.LockActor(ctx, actor); err != nil {
			return nil, err
		}

		requestID := batchreplay.Normalize(req.RequestID)
		if requestID != "" {
			if err := tx.LockRequest(ctx, actor, req.Operation, requestID); err != nil {
				return nil, err
			}
			existing, found, err := tx.FindReplay(ctx, actor, req.Operation, requestID)
			if err != nil {
				// Malformed matching history fails closed; it is never skipped to
				// create a second parent with the same logical request.
				return nil, err
			}
			if found {
				return buildSubmitResult(existing.BatchID, existing.Status), nil
			}
		}

		if err := tx.CheckCurrentLimits(ctx, actor, len(req.Items), time.Now().UTC()); err != nil {
			return nil, err
		}
		batchID, err := tx.CreateParentAndChildren(ctx, actor, req)
		if err != nil {
			return nil, err
		}
		return buildSubmitResult(batchID, "PENDING_APPROVAL"), nil
	})
}

// ApproveAndSchedule atomically claims the pending parent/event, stores the
// normalized execution snapshot, refreshes the projection, and inserts one
// parent-keyed dispatcher on its dedicated River queue. Children are dispatched
// only after that transaction commits.
func (u *BatchApprovalUseCase) ApproveAndSchedule(
	ctx context.Context,
	input BatchApprovalClaimInput,
) error {
	return u.decisions.ClaimBatchApprovalAndEnqueue(ctx, input)
}

// RequeueFailed resets only execution-FAILED children below the logical attempt
// cap. REJECTED is an approval decision and requires a new batch. The failed
// child and its paired event's accepted-state reset, existing-approver
// parent/event reopen, projection refresh, and a new dispatcher InsertTx either
// all commit or all roll back.
func (u *BatchApprovalUseCase) RequeueFailed(
	ctx context.Context,
	input BatchApprovalRetryInput,
) (*BatchActionResult, error) {
	for _, child := range input.Children {
		if child.Status != "FAILED" || child.AttemptCount >= maxLogicalDispatchAttempts {
			return nil, ErrChildNotRetryable
		}
	}
	if strings.TrimSpace(input.OriginalApprover) == "" {
		return nil, ErrMissingApprovalProvenance
	}
	if err := u.decisions.RetryBatchApprovalAndEnqueue(ctx, input); err != nil {
		return nil, err
	}
	ids := childTicketIDs(input.Children)
	return &BatchActionResult{BatchID: input.ParentTicketID, AffectedCount: len(ids), AffectedTicketIDs: ids}, nil
}

// CancelPending follows a handler-side parent identity/state check with one
// parent-keyed mutation transaction. Exact PENDING child Ticket/Event changes
// and parent Ticket/Event/projection aggregation commit together; the parent
// row lock and expected-state writes make a concurrent parent change fail
// closed, so a sync failure cannot leave a half-cancelled batch.
func (u *BatchApprovalUseCase) CancelPending(
	ctx context.Context,
	input BatchCancelInput,
) (*BatchActionResult, error) {
	ids, err := u.decisions.CancelPendingAndSync(ctx, input)
	if err != nil {
		return nil, err
	}
	return &BatchActionResult{BatchID: input.ParentTicketID, AffectedCount: len(ids), AffectedTicketIDs: ids}, nil
}

type SubmissionRepository interface {
	WithSubmissionTx(context.Context, func(SubmissionTx) (*BatchSubmitResult, error)) (*BatchSubmitResult, error)
}

type SubmissionTx interface {
	LockGlobal(context.Context) error
	LockActor(context.Context, string) error
	LockRequest(context.Context, string, string, string) error
	FindReplay(context.Context, string, string, string) (ExistingBatch, bool, error)
	CheckCurrentLimits(context.Context, string, int, time.Time) error
	CreateParentAndChildren(context.Context, string, BatchSubmitRequest) (string, error)
}

type AtomicBatchDecisionWriter interface {
	ClaimBatchApprovalAndEnqueue(context.Context, BatchApprovalClaimInput) error
	RetryBatchApprovalAndEnqueue(context.Context, BatchApprovalRetryInput) error
	CancelPendingAndSync(context.Context, BatchCancelInput) ([]string, error)
}

type ExistingBatch struct {
	BatchID string
	Status  string
}

type BatchApprovalClaimInput struct {
	ParentTicketID string
	ParentEventID  string
	Approver       string
	Execution      any
}

type BatchApprovalRetryInput struct {
	ParentTicketID   string
	ParentEventID    string
	OriginalApprover string
	Children         []BatchRetryChild
	Execution        any
}

type BatchRetryChild struct {
	TicketID     string
	EventID      string
	Status       string
	AttemptCount int
}

type BatchCancelInput struct {
	ParentTicketID string
	ParentEventID  string
	Actor          string
	Children       []string
}

var (
	ErrInvalidBatchRequest       = errors.New("invalid batch request")
	ErrChildNotRetryable         = errors.New("batch child is not retryable")
	ErrMissingApprovalProvenance = errors.New("batch has no durable approval provenance")
)

func validateBatchRequest(req BatchSubmitRequest, actor string) error {
	if strings.TrimSpace(actor) == "" || strings.TrimSpace(req.Operation) == "" || len(req.Items) == 0 {
		return ErrInvalidBatchRequest
	}
	switch strings.ToUpper(strings.TrimSpace(req.Operation)) {
	case "CREATE", "MODIFY", "DELETE", "POWER_START", "POWER_STOP", "POWER_RESTART":
	default:
		return ErrInvalidBatchRequest
	}
	for _, item := range req.Items {
		if strings.TrimSpace(item.ResourceID) == "" {
			return ErrInvalidBatchRequest
		}
	}
	return nil
}

func buildSubmitResult(batchID, status string) *BatchSubmitResult {
	return &BatchSubmitResult{
		BatchID:           batchID,
		Status:            status,
		StatusURL:         "/api/v1/vms/batch/" + batchID,
		RetryAfterSeconds: 2,
	}
}

func childTicketIDs(children []BatchRetryChild) []string {
	ids := make([]string, 0, len(children))
	for _, child := range children {
		ids = append(ids, child.TicketID)
	}
	return ids
}
