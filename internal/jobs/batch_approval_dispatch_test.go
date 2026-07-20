package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/internal/testutil"
)

type batchApprovalDispatchTestDispatcher struct {
	dispatch func(context.Context, string) error
	finalize func(context.Context, string, error) error
}

func (d *batchApprovalDispatchTestDispatcher) DispatchBatchApproval(ctx context.Context, parentID string) error {
	if d.dispatch == nil {
		return nil
	}
	return d.dispatch(ctx, parentID)
}

func (d *batchApprovalDispatchTestDispatcher) FailPendingBatchApprovalDispatch(
	ctx context.Context,
	parentID string,
	cause error,
) error {
	if d.finalize == nil {
		return nil
	}
	return d.finalize(ctx, parentID, cause)
}

func TestBatchApprovalDispatchArgsKindAndInsertOpts(t *testing.T) {
	t.Parallel()

	args := BatchApprovalDispatchArgs{BatchID: "parent-ticket"}
	require.Equal(t, "batch_approval_dispatch", args.Kind())

	opts := args.InsertOpts()
	require.Equal(t, BatchApprovalDispatchJobKind, opts.Queue)
	require.Equal(t, 5, opts.MaxAttempts)
	require.True(t, opts.UniqueOpts.ByArgs)
	require.True(t, opts.UniqueOpts.ByQueue)
	require.ElementsMatch(t, []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateRetryable,
		rivertype.JobStateRunning,
		rivertype.JobStateScheduled,
	}, opts.UniqueOpts.ByState)
	require.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCompleted)
	require.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateCancelled)
	require.NotContains(t, opts.UniqueOpts.ByState, rivertype.JobStateDiscarded)
}

func TestBatchApprovalDispatchArgsSameParentRemainsUniqueAcrossOtherInsertOptions(t *testing.T) {
	pool := testutil.OpenPGXPool(t, "badu")
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err)
	_, err = migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil)
	require.NoError(t, err)

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)

	args := BatchApprovalDispatchArgs{BatchID: "parent-" + uuid.NewString()}
	first, err := client.Insert(t.Context(), args, &river.InsertOpts{
		Priority:    1,
		ScheduledAt: time.Now().Add(time.Hour),
		Tags:        []string{"first"},
	})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.False(t, first.UniqueSkippedAsDuplicate)

	duplicate, err := client.Insert(t.Context(), args, &river.InsertOpts{
		Priority:    4,
		ScheduledAt: time.Now().Add(2 * time.Hour),
		Tags:        []string{"second"},
	})
	require.NoError(t, err)
	require.NotNil(t, duplicate)
	require.True(t, duplicate.UniqueSkippedAsDuplicate)
	require.Equal(t, first.Job.ID, duplicate.Job.ID)

	distinct, err := client.Insert(t.Context(), BatchApprovalDispatchArgs{
		BatchID: args.BatchID + "-other",
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, distinct)
	require.False(t, distinct.UniqueSkippedAsDuplicate)
	require.NotEqual(t, first.Job.ID, distinct.Job.ID)
}

func TestBatchApprovalDispatchWorkerConstructorRequiresDispatcher(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		NewBatchApprovalDispatchWorker(nil)
	})
}

func TestBatchApprovalDispatchWorkerRejectsInvalidWork(t *testing.T) {
	t.Parallel()

	dispatcher := &batchApprovalDispatchTestDispatcher{}
	worker := NewBatchApprovalDispatchWorker(dispatcher)

	t.Run("blank parent", func(t *testing.T) {
		err := worker.Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
			Args: BatchApprovalDispatchArgs{BatchID: " \t\n "},
		})
		var cancelErr *river.JobCancelError
		require.ErrorAs(t, err, &cancelErr)
		require.ErrorContains(t, err, "parent ticket id is required")
	})
}

func TestBatchApprovalDispatchWorkerDispatchesTrimmedParent(t *testing.T) {
	t.Parallel()

	dispatchCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(ctx context.Context, parentID string) error {
			dispatchCalls++
			require.Equal(t, "parent-ticket", parentID)
			require.NoError(t, ctx.Err())
			return nil
		},
		finalize: func(context.Context, string, error) error {
			t.Fatal("finalizer must not run after a successful dispatch")
			return nil
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		Args: BatchApprovalDispatchArgs{BatchID: "  parent-ticket  "},
	})
	require.NoError(t, err)
	require.Equal(t, 1, dispatchCalls)
}

func TestBatchApprovalDispatchWorkerReturnsRetryableDispatchErrorBeforeFinalAttempt(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("dispatch unavailable")
	dispatchCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error {
			dispatchCalls++
			return dispatchErr
		},
		finalize: func(context.Context, string, error) error {
			t.Fatal("finalizer must not run before the final attempt")
			return nil
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2, MaxAttempts: 3},
		Args:   BatchApprovalDispatchArgs{BatchID: "parent-ticket"},
	})
	require.NotErrorIs(t, err, dispatchErr)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerRetryableFailure)
	require.NotContains(t, err.Error(), dispatchErr.Error())
	var cancelErr *river.JobCancelError
	require.NotErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.NotErrorAs(t, err, &snoozeErr)
	require.Equal(t, 1, dispatchCalls)
}

func TestBatchApprovalDispatchWorkerJoinsCancellationBeforeFinalAttempt(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("dispatch interrupted")
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(ctx context.Context, parentID string) error {
			require.Equal(t, "parent-ticket", parentID)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			return dispatchErr
		},
		finalize: func(context.Context, string, error) error {
			t.Fatal("finalizer must not run before the final attempt")
			return nil
		},
	}
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(cancelledCtx, &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 2},
		Args:   BatchApprovalDispatchArgs{BatchID: "parent-ticket"},
	})
	require.NotErrorIs(t, err, dispatchErr)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerRetryableFailure)
	require.NotContains(t, err.Error(), dispatchErr.Error())
	require.ErrorIs(t, err, context.Canceled)
	var cancelErr *river.JobCancelError
	require.NotErrorAs(t, err, &cancelErr)
}

func TestBatchApprovalDispatchWorkerFinalAttemptUsesDetachedBoundedFinalizer(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("dispatch exhausted")
	dispatchCalls := 0
	finalizeCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(ctx context.Context, parentID string) error {
			dispatchCalls++
			require.Equal(t, "parent-ticket", parentID)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			return dispatchErr
		},
		finalize: func(ctx context.Context, parentID string, cause error) error {
			finalizeCalls++
			require.Equal(t, "parent-ticket", parentID)
			require.ErrorIs(t, cause, dispatchErr)
			require.NoError(t, ctx.Err(), "finalizer context must be detached from attempt cancellation")
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "finalizer context must have a deadline")
			remaining := time.Until(deadline)
			require.Positive(t, remaining)
			require.LessOrEqual(t, remaining, 20*time.Second)
			return nil
		},
	}
	cancelledCtx, cancel := context.WithCancel(t.Context())
	cancel()

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(cancelledCtx, &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: " parent-ticket "},
	})
	require.NotErrorIs(t, err, dispatchErr)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerExhaustedFailure)
	require.NotContains(t, err.Error(), dispatchErr.Error())
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	var snoozeErr *river.JobSnoozeError
	require.NotErrorAs(t, err, &snoozeErr)
	require.Equal(t, 1, dispatchCalls)
	require.Equal(t, 1, finalizeCalls)
}

func TestBatchApprovalDispatchWorkerSnoozesWhenFinalizerFails(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("dispatch exhausted")
	finalizeErr := errors.New("finalizer unavailable")
	finalizeCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error {
			return dispatchErr
		},
		finalize: func(ctx context.Context, parentID string, cause error) error {
			finalizeCalls++
			require.NoError(t, ctx.Err())
			require.Equal(t, "parent-ticket", parentID)
			require.ErrorIs(t, cause, dispatchErr)
			return finalizeErr
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: "parent-ticket"},
	})
	var snoozeErr *river.JobSnoozeError
	require.ErrorAs(t, err, &snoozeErr)
	require.Positive(t, snoozeErr.Duration)
	var cancelErr *river.JobCancelError
	require.NotErrorAs(t, err, &cancelErr)
	require.Equal(t, 1, finalizeCalls)
}

func TestBatchApprovalDispatchWorkerCancelsConsistencyViolationWithoutFinalizing(t *testing.T) {
	t.Parallel()

	consistencyErr := &BatchApprovalDispatchConsistencyError{
		BatchID:           "parent-ticket",
		ParentStatus:      "SUCCESS",
		ParentEventStatus: "COMPLETED",
		PendingChildren:   1,
		ActiveChildren:    1,
		Detail:            "terminal parent still has an active child",
	}
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error {
			return consistencyErr
		},
		finalize: func(context.Context, string, error) error {
			t.Fatal("consistency violations must not run the failure finalizer")
			return nil
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: "parent-ticket"},
	})
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.NotErrorIs(t, err, consistencyErr)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerConsistencyFailure)
	var snoozeErr *river.JobSnoozeError
	require.NotErrorAs(t, err, &snoozeErr)
}

func TestBatchApprovalDispatchWorkerFinalAttemptMissingOwnerCancelsWithoutSnooze(t *testing.T) {
	t.Parallel()

	missingOwner := &BatchApprovalDispatchConsistencyError{
		BatchID: "missing-parent-ticket",
		Detail:  "durable parent ticket or parent event is missing",
	}
	finalizeCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error {
			return missingOwner
		},
		finalize: func(context.Context, string, error) error {
			finalizeCalls++
			return errors.New("must not run")
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: "missing-parent-ticket"},
	})
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.NotErrorIs(t, err, missingOwner)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerConsistencyFailure)
	var snoozeErr *river.JobSnoozeError
	require.NotErrorAs(t, err, &snoozeErr)
	require.Zero(t, finalizeCalls)
}

func TestBatchApprovalDispatchWorkerCancelsConsistencyViolationFromFinalizerWithoutSnoozing(t *testing.T) {
	t.Parallel()

	dispatchErr := errors.New("dispatch exhausted")
	consistencyErr := &BatchApprovalDispatchConsistencyError{
		BatchID:           "parent-ticket",
		ParentStatus:      "CANCELLED",
		ParentEventStatus: "CANCELLED",
		PendingChildren:   1,
		ActiveChildren:    1,
		Detail:            "finalizer cannot rewrite a cancelled batch",
	}
	finalizeCalls := 0
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error {
			return dispatchErr
		},
		finalize: func(ctx context.Context, parentID string, cause error) error {
			finalizeCalls++
			require.NoError(t, ctx.Err())
			require.Equal(t, "parent-ticket", parentID)
			require.ErrorIs(t, cause, dispatchErr)
			return consistencyErr
		},
	}

	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 5, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: "parent-ticket"},
	})
	var cancelErr *river.JobCancelError
	require.ErrorAs(t, err, &cancelErr)
	require.NotErrorIs(t, err, dispatchErr)
	require.NotErrorIs(t, err, consistencyErr)
	require.ErrorContains(t, err, batchApprovalDispatchWorkerExhaustedFailure)
	require.ErrorContains(t, err, batchApprovalDispatchFinalizerConsistencyFailure)
	var snoozeErr *river.JobSnoozeError
	require.NotErrorAs(t, err, &snoozeErr)
	require.Equal(t, 1, finalizeCalls)
}

func TestBatchApprovalDispatchWorkerDoesNotReturnUntrustedErrorText(t *testing.T) {
	t.Parallel()

	const sentinel = "postgres://svc:super-secret@example.com Bearer token-secret-value"
	dispatchErr := errors.New(sentinel)
	dispatcher := &batchApprovalDispatchTestDispatcher{
		dispatch: func(context.Context, string) error { return dispatchErr },
		finalize: func(context.Context, string, error) error {
			t.Fatal("finalizer must not run before the final attempt")
			return nil
		},
	}
	err := NewBatchApprovalDispatchWorker(dispatcher).Work(t.Context(), &river.Job[BatchApprovalDispatchArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
		Args:   BatchApprovalDispatchArgs{BatchID: "secret-parent"},
	})
	require.Error(t, err)
	require.NotErrorIs(t, err, dispatchErr)
	require.Contains(t, err.Error(), batchApprovalDispatchWorkerRetryableFailure)
	require.Contains(t, err.Error(), "error_type=*errors.errorString")
	require.NotContains(t, err.Error(), sentinel)
	require.NotContains(t, err.Error(), "super-secret")
	require.NotContains(t, err.Error(), "token-secret-value")
}

var _ BatchApprovalDispatcher = (*batchApprovalDispatchTestDispatcher)(nil)
