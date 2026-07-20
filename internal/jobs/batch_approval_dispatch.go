package jobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const batchApprovalDispatchFinalizationTimeout = 15 * time.Second

const (
	batchApprovalDispatchWorkerRetryableFailure      = "BATCH_APPROVAL_DISPATCH_RETRYABLE_FAILURE"
	batchApprovalDispatchWorkerConsistencyFailure    = "BATCH_APPROVAL_DISPATCH_CONSISTENCY_VIOLATION"
	batchApprovalDispatchWorkerExhaustedFailure      = "BATCH_APPROVAL_DISPATCH_RETRIES_EXHAUSTED"
	batchApprovalDispatchFinalizerConsistencyFailure = "BATCH_APPROVAL_DISPATCH_FINALIZER_CONSISTENCY_VIOLATION"
)

// BatchApprovalDispatchJobKind is also the dedicated queue name. Keeping the
// dispatcher off vm_operations prevents an older rolling-deployment replica,
// which does not register this worker kind, from reserving the new jobs.
const BatchApprovalDispatchJobKind = "batch_approval_dispatch"

// BatchApprovalDispatchConsistencyError marks durable parent/child state that
// cannot be repaired without choosing a new business outcome. Workers cancel
// these jobs without running the generic failure finalizer, because that
// finalizer must never rewrite children underneath a successful or cancelled
// parent.
type BatchApprovalDispatchConsistencyError struct {
	BatchID           string
	ParentStatus      string
	ParentEventStatus string
	PendingChildren   int
	ActiveChildren    int
	Detail            string
	Cause             error
}

func (e *BatchApprovalDispatchConsistencyError) Error() string {
	if e == nil {
		return "batch approval dispatch consistency violation"
	}
	detail := strings.TrimSpace(e.Detail)
	if detail == "" {
		detail = "parent and child states cannot be safely reconciled"
	}
	return fmt.Sprintf(
		"batch approval dispatch %s is inconsistent (parent=%s, event=%s, pending=%d, active=%d): %s",
		e.BatchID,
		e.ParentStatus,
		e.ParentEventStatus,
		e.PendingChildren,
		e.ActiveChildren,
		detail,
	)
}

func (e *BatchApprovalDispatchConsistencyError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// BatchApprovalDispatchArgs follows the ADR-0009 owning-table claim-check
// exception: approval inputs and child state remain durable on the parent
// ticket and its related events, while River carries only that stable key.
type BatchApprovalDispatchArgs struct {
	BatchID string `json:"batch_id" river:"unique"`
}

func (BatchApprovalDispatchArgs) Kind() string { return BatchApprovalDispatchJobKind }

func (BatchApprovalDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       BatchApprovalDispatchJobKind,
		MaxAttempts: 5,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
			// Completed/cancelled/discarded dispatchers must not block an
			// explicit retry. Only runnable work participates in uniqueness.
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRetryable,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// BatchApprovalDispatcher is implemented by the governance ticket service.
// Keeping this interface in jobs avoids coupling the worker package to its
// concrete orchestration implementation.
type BatchApprovalDispatcher interface {
	DispatchBatchApproval(context.Context, string) error
	FailPendingBatchApprovalDispatch(context.Context, string, error) error
}

type BatchApprovalDispatchWorker struct {
	river.WorkerDefaults[BatchApprovalDispatchArgs]
	dispatcher BatchApprovalDispatcher
}

func NewBatchApprovalDispatchWorker(dispatcher BatchApprovalDispatcher) *BatchApprovalDispatchWorker {
	if dispatcher == nil {
		panic("batch approval dispatcher is not configured")
	}
	return &BatchApprovalDispatchWorker{dispatcher: dispatcher}
}

func (w *BatchApprovalDispatchWorker) Work(
	ctx context.Context,
	job *river.Job[BatchApprovalDispatchArgs],
) error {
	if job == nil {
		return river.JobCancel(fmt.Errorf("batch approval dispatch job is nil"))
	}
	parentID := strings.TrimSpace(job.Args.BatchID)
	if parentID == "" {
		return river.JobCancel(fmt.Errorf("batch approval dispatch parent ticket id is required"))
	}
	if w == nil || w.dispatcher == nil {
		return fmt.Errorf("batch approval dispatcher is not configured")
	}

	err := w.dispatcher.DispatchBatchApproval(ctx, parentID)
	if err == nil {
		return nil
	}
	safeDispatchErr := safeBatchApprovalDispatchWorkerError(batchApprovalDispatchWorkerRetryableFailure, err)
	var consistencyErr *BatchApprovalDispatchConsistencyError
	if errors.As(err, &consistencyErr) {
		logger.Error("batch approval dispatcher found an unsafe durable state; cancelling without finalization",
			zap.String("batch_id", parentID),
			zap.String("parent_status", consistencyErr.ParentStatus),
			zap.String("parent_event_status", consistencyErr.ParentEventStatus),
			zap.Int("pending_children", consistencyErr.PendingChildren),
			zap.Int("active_children", consistencyErr.ActiveChildren),
			zap.String("failure_reason", "BATCH_APPROVAL_DISPATCH_CONSISTENCY_VIOLATION"),
			zap.String("error_type", fmt.Sprintf("%T", err)),
		)
		return river.JobCancel(safeBatchApprovalDispatchWorkerError(batchApprovalDispatchWorkerConsistencyFailure, err))
	}
	if job.Attempt < job.MaxAttempts {
		if ctx.Err() != nil {
			return errors.Join(safeDispatchErr, ctx.Err())
		}
		return safeDispatchErr
	}

	// River will not run this job again. Use a bounded context detached from the
	// attempt so every still-PENDING child is made terminal and the parent can
	// converge instead of remaining EXECUTING forever.
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), batchApprovalDispatchFinalizationTimeout)
	defer cancel()
	if finalizeErr := w.dispatcher.FailPendingBatchApprovalDispatch(finalizeCtx, parentID, err); finalizeErr != nil {
		if errors.As(finalizeErr, &consistencyErr) {
			logger.Error("batch approval dispatcher finalizer refused an unsafe durable state; cancelling",
				zap.String("batch_id", parentID),
				zap.String("parent_status", consistencyErr.ParentStatus),
				zap.String("parent_event_status", consistencyErr.ParentEventStatus),
				zap.Int("pending_children", consistencyErr.PendingChildren),
				zap.Int("active_children", consistencyErr.ActiveChildren),
				zap.String("failure_reason", "BATCH_APPROVAL_DISPATCH_FINALIZER_CONSISTENCY_VIOLATION"),
				zap.String("error_type", fmt.Sprintf("%T", finalizeErr)),
			)
			return river.JobCancel(errors.Join(
				safeBatchApprovalDispatchWorkerError(batchApprovalDispatchWorkerExhaustedFailure, err),
				safeBatchApprovalDispatchWorkerError(batchApprovalDispatchFinalizerConsistencyFailure, finalizeErr),
			))
		}
		// A finalizer can itself hit a transient database outage. Snoozing does
		// not consume another River attempt, so convergence keeps retrying until
		// every remaining child and the parent are durable.
		logger.Error("batch approval dispatcher finalization failed; snoozing for convergence",
			zap.String("batch_id", parentID),
			zap.Int("attempt", job.Attempt),
			zap.String("failure_reason", "BATCH_APPROVAL_DISPATCH_FINALIZER_TRANSIENT_FAILURE"),
			zap.String("dispatch_error_type", fmt.Sprintf("%T", err)),
			zap.String("finalization_error_type", fmt.Sprintf("%T", finalizeErr)),
		)
		return river.JobSnooze(5 * time.Second)
	}
	return river.JobCancel(safeBatchApprovalDispatchWorkerError(batchApprovalDispatchWorkerExhaustedFailure, err))
}

func safeBatchApprovalDispatchWorkerError(reason string, cause error) error {
	return fmt.Errorf("%s (error_type=%T)", strings.TrimSpace(reason), cause)
}
