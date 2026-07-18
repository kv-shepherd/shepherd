package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type powerGuardTestStore struct {
	pool   *pgxpool.Pool
	writer *ApprovalAtomicWriter
}

type powerGuardSideEffects struct {
	events  int
	tickets int
	batches int
	jobs    int
}

func TestApprovalAtomicWriterCreatePowerApprovalRequest_PersistsEventAndTicket(t *testing.T) {
	store := newPowerGuardTestStore(t, "pga")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	input := powerGuardApprovalInput(
		"vm-approval",
		"event-approval",
		"ticket-approval",
		"VM_START_REQUESTED",
	)
	if err := store.writer.CreatePowerApprovalRequest(ctx, input); err != nil {
		t.Fatalf("CreatePowerApprovalRequest() unexpected error: %v", err)
	}

	var (
		eventType     string
		aggregateType string
		aggregateID   string
		payload       []byte
		eventStatus   string
		createdBy     string
	)
	if err := store.pool.QueryRow(ctx, `
SELECT event_type, aggregate_type, aggregate_id, payload, status, created_by
FROM domain_events
WHERE id = $1
`, input.EventID).Scan(&eventType, &aggregateType, &aggregateID, &payload, &eventStatus, &createdBy); err != nil {
		t.Fatalf("query persisted power approval event: %v", err)
	}
	if eventType != input.EventType || aggregateType != "vm" || aggregateID != input.AggregateID ||
		eventStatus != "PENDING" || createdBy != input.CreatedBy || !bytes.Equal(payload, input.Payload) {
		t.Fatalf(
			"persisted event = type:%q aggregate:%q/%q status:%q created_by:%q payload:%s, want type:%q aggregate:vm/%q status:PENDING created_by:%q payload:%s",
			eventType,
			aggregateType,
			aggregateID,
			eventStatus,
			createdBy,
			payload,
			input.EventType,
			input.AggregateID,
			input.CreatedBy,
			input.Payload,
		)
	}

	var (
		ticketEventID string
		operation     string
		ticketStatus  string
		requester     string
		reason        pgtype.Text
		parentID      pgtype.Text
	)
	if err := store.pool.QueryRow(ctx, `
SELECT event_id, operation_type, status, requester, reason, parent_ticket_id
FROM tickets
WHERE id = $1
`, input.TicketID).Scan(&ticketEventID, &operation, &ticketStatus, &requester, &reason, &parentID); err != nil {
		t.Fatalf("query persisted power approval ticket: %v", err)
	}
	if ticketEventID != input.EventID || operation != "POWER" || ticketStatus != "PENDING" ||
		requester != input.CreatedBy || !reason.Valid || reason.String != input.Reason || parentID.Valid {
		t.Fatalf(
			"persisted ticket = event:%q operation:%q status:%q requester:%q reason:%#v parent:%#v, want event:%q operation:POWER status:PENDING requester:%q reason:%q no parent",
			ticketEventID,
			operation,
			ticketStatus,
			requester,
			reason,
			parentID,
			input.EventID,
			input.CreatedBy,
			input.Reason,
		)
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, tickets: 1})
}

func TestPowerGuard_ConcurrentDirectAndApprovalForSameVMLetOnlyOneCommit(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgca")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	const vmID = "vm-concurrent-direct-approval"
	lockKey := PowerVMLockKey(vmID)
	lockConn, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire power guard lock connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1 || ':' || current_schema(), 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		t.Fatalf("hold power guard advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query power guard blocker PID: %v", err)
	}

	direct := powerGuardDirectInput("event-concurrent-direct", vmID, "VM_START_REQUESTED", "direct-actor")
	approval := powerGuardApprovalInput(
		vmID,
		"event-concurrent-approval",
		"ticket-concurrent-approval",
		"VM_STOP_REQUESTED",
	)
	type result struct {
		kind string
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var workers errgroup.Group
	workers.Go(func() error {
		<-start
		results <- result{kind: "direct", err: store.writer.CreatePowerEventAndEnqueue(ctx, direct)}
		return nil
	})
	workers.Go(func() error {
		<-start
		results <- result{kind: "approval", err: store.writer.CreatePowerApprovalRequest(ctx, approval)}
		return nil
	})
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = store.pool.QueryRow(ctx, `
WITH RECURSIVE blocked(pid) AS (
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
    AND $1 = ANY(pg_blocking_pids(activity.pid))
  UNION
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  JOIN blocked AS upstream
    ON upstream.pid = ANY(pg_blocking_pids(activity.pid))
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
)
SELECT count(*) FROM blocked
`, blockerPID).Scan(&blockedCalls)
		return blockedQueryErr != nil || blockedCalls == 2
	}, 8*time.Second, 10*time.Millisecond, "direct and approval requests did not block on the VM lock")

	_, unlockErr := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		closeCancel()
		lockHeld = false
	}
	if waitErr := workers.Wait(); waitErr != nil {
		t.Fatalf("wait for concurrent direct and approval power requests: %v", waitErr)
	}
	close(results)

	if blockedQueryErr != nil {
		t.Fatalf("query calls blocked by shared power guard: %v", blockedQueryErr)
	}
	if blockedCalls != 2 {
		t.Fatalf("calls blocked by shared power guard = %d, want 2 before release", blockedCalls)
	}
	if unlockErr != nil {
		t.Fatalf("release shared power guard: %v", unlockErr)
	}

	var (
		winner result
		loser  result
	)
	for got := range results {
		if got.err == nil {
			if winner.kind != "" {
				t.Fatalf("multiple concurrent winners: first=%+v second=%+v", winner, got)
			}
			winner = got
			continue
		}
		if active := requireActivePowerEventError(t, got.err); active.AggregateID != vmID {
			t.Fatalf("losing active conflict = %+v, want VM %q", active, vmID)
		}
		loser = got
	}
	if winner.kind == "" || loser.kind == "" || winner.kind == loser.kind {
		t.Fatalf("concurrent direct/approval results = winner:%+v loser:%+v, want one of each", winner, loser)
	}

	var (
		persistedEventID   string
		persistedEventType string
		persistedTicketID  pgtype.Text
	)
	if err := store.pool.QueryRow(ctx, `
SELECT event.id, event.event_type, ticket.id
FROM domain_events AS event
LEFT JOIN tickets AS ticket ON ticket.event_id = event.id
WHERE event.aggregate_type = 'vm' AND event.aggregate_id = $1
`, vmID).Scan(&persistedEventID, &persistedEventType, &persistedTicketID); err != nil {
		t.Fatalf("query concurrent power winner: %v", err)
	}
	active := requireActivePowerEventError(t, loser.err)
	if active.ExistingEventID != persistedEventID || active.ExistingEventType != persistedEventType ||
		active.ExistingTicketID != persistedTicketID.String || active.AggregateID != vmID {
		t.Fatalf("losing active conflict = %+v, want event %q (%s) ticket %q VM %q", active, persistedEventID, persistedEventType, persistedTicketID.String, vmID)
	}

	if winner.kind == "direct" {
		assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, jobs: 1})
		assertPowerGuardRowsAbsent(t, store, approval.EventID, approval.TicketID, "")
		assertPowerGuardJobEvent(t, store, direct.EventID)
		return
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, tickets: 1})
	assertPowerGuardRowsAbsent(t, store, direct.EventID, "", "")
}

func TestPowerGuard_DirectThenApprovalRejectsWithoutExtraSideEffects(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgda")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	direct := powerGuardDirectInput("event-direct-first", "vm-direct-first", "VM_START_REQUESTED", "direct-actor")
	if err := store.writer.CreatePowerEventAndEnqueue(ctx, direct); err != nil {
		t.Fatalf("CreatePowerEventAndEnqueue() unexpected error: %v", err)
	}

	approval := powerGuardApprovalInput(
		direct.AggregateID,
		"event-approval-second",
		"ticket-approval-second",
		"VM_STOP_REQUESTED",
	)
	err := store.writer.CreatePowerApprovalRequest(ctx, approval)
	active := requireActivePowerEventError(t, err)
	if active.ExistingEventID != direct.EventID || active.ExistingEventType != direct.EventType ||
		active.ExistingTicketID != "" || active.AggregateID != direct.AggregateID {
		t.Fatalf("active event error = %+v, want direct event %q (%s), no ticket, VM %q", active, direct.EventID, direct.EventType, direct.AggregateID)
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, jobs: 1})
	assertPowerGuardRowsAbsent(t, store, approval.EventID, approval.TicketID, "")
	assertPowerGuardJobEvent(t, store, direct.EventID)
}

func TestPowerGuard_ApprovalThenBatchRejectsWithoutBatchSideEffects(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgab")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	approval := powerGuardApprovalInput(
		"vm-approval-first",
		"event-approval-first",
		"ticket-approval-first",
		"VM_RESTART_REQUESTED",
	)
	if err := store.writer.CreatePowerApprovalRequest(ctx, approval); err != nil {
		t.Fatalf("CreatePowerApprovalRequest() unexpected error: %v", err)
	}

	batch := powerGuardBatchInput("batch-after-approval", approval.AggregateID, "VM_STOP_REQUESTED")
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, batch, nil)
	active := requireActivePowerEventError(t, err)
	if active.ExistingEventID != approval.EventID || active.ExistingEventType != approval.EventType ||
		active.ExistingTicketID != approval.TicketID || active.AggregateID != approval.AggregateID {
		t.Fatalf("active event error = %+v, want approval event %q ticket %q VM %q", active, approval.EventID, approval.TicketID, approval.AggregateID)
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, tickets: 1})
	assertPowerGuardRowsAbsent(t, store, "", batch.ParentID, batch.ParentID)
}

func TestPowerGuard_BatchRejectsDuplicateVMsBeforePersisting(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgbd")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	input := powerGuardBatchInput("batch-duplicate-vm", "vm-duplicate", "VM_START_REQUESTED")
	duplicate := powerGuardBatchChild("  vm-duplicate  ", "VM_RESTART_REQUESTED")
	duplicate.Reason = "duplicate normalized VM"
	input.Children = append(input.Children, duplicate)
	mustSyncPowerGuardBatchParentPayload(&input)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil)
	if err == nil {
		t.Fatal("CreateBatchPowerAndMaybeEnqueue() error = nil, want duplicate VM validation error")
	}
	lowerError := strings.ToLower(err.Error())
	if (!strings.Contains(lowerError, "duplicate") && !strings.Contains(lowerError, "repeat")) ||
		!strings.Contains(err.Error(), "vm-duplicate") {
		t.Fatalf("duplicate VM validation error = %v, want repetition and VM identity", err)
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{})
	assertPowerGuardRowsAbsent(t, store, "", input.ParentID, input.ParentID)
}

func TestPowerGuard_OnlyPendingAndProcessingEventsBlockDirectSubmission(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgsm")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	tests := []struct {
		status string
		blocks bool
	}{
		{status: "PENDING", blocks: true},
		{status: "PROCESSING", blocks: true},
		{status: "COMPLETED", blocks: false},
		{status: "FAILED", blocks: false},
		{status: "CANCELLED", blocks: false},
	}
	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			statusName := strings.ToLower(tc.status)
			vmID := "vm-state-" + statusName
			existingEventID := "event-existing-" + statusName
			newEventID := "event-new-" + statusName
			if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, $4, 'seed-actor')
`, existingEventID, vmID, []byte(`{"seed":true}`), tc.status); err != nil {
				t.Fatalf("insert %s seed event: %v", tc.status, err)
			}
			inserted, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: existingEventID}, nil)
			if err != nil {
				t.Fatalf("insert %s seed power job: %v", tc.status, err)
			}
			if inserted == nil || inserted.Job == nil || inserted.UniqueSkippedAsDuplicate {
				t.Fatalf("%s seed power job insert result = %#v, want newly inserted runnable job", tc.status, inserted)
			}

			err = store.writer.CreatePowerEventAndEnqueue(
				ctx,
				powerGuardDirectInput(newEventID, vmID, "VM_STOP_REQUESTED", "direct-state-actor"),
			)
			if tc.blocks {
				active := requireActivePowerEventError(t, err)
				if active.ExistingEventID != existingEventID || active.ExistingEventType != "VM_START_REQUESTED" ||
					active.ExistingTicketID != "" || active.AggregateID != vmID {
					t.Fatalf("active event error = %+v, want seed event %q for VM %q", active, existingEventID, vmID)
				}
				assertPowerGuardRowsAbsent(t, store, newEventID, "", "")
			} else if err != nil {
				t.Fatalf("CreatePowerEventAndEnqueue() with existing %s event unexpected error: %v", tc.status, err)
			}

			var eventCount int
			if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM domain_events
WHERE aggregate_type = 'vm' AND aggregate_id = $1
`, vmID).Scan(&eventCount); err != nil {
				t.Fatalf("count %s VM events: %v", tc.status, err)
			}
			wantEventCount := 2
			wantJobCount := 2
			if tc.blocks {
				wantEventCount = 1
				wantJobCount = 1
			}
			if eventCount != wantEventCount {
				t.Fatalf("%s VM event count = %d, want %d", tc.status, eventCount, wantEventCount)
			}
			var jobCount int
			if err := store.pool.QueryRow(ctx, `
SELECT count(*)
FROM river_job AS job
JOIN domain_events AS event ON event.id = job.args->>'event_id'
WHERE event.aggregate_type = 'vm' AND event.aggregate_id = $1
`, vmID).Scan(&jobCount); err != nil {
				t.Fatalf("count %s VM jobs: %v", tc.status, err)
			}
			if jobCount != wantJobCount {
				t.Fatalf("%s VM River job count = %d, want %d", tc.status, jobCount, wantJobCount)
			}
		})
	}
}

func TestPowerGuard_ProcessingPowerFenceBlocksWithoutRunnableRiverJob(t *testing.T) {
	tests := []struct {
		name             string
		writer           string
		eventType        string
		operation        string
		withTicket       bool
		terminalJobState string
	}{
		{name: "start direct without ticket or job", writer: "direct", eventType: "VM_START_REQUESTED", operation: "start"},
		{name: "start direct with cancelled job", writer: "direct", eventType: "VM_START_REQUESTED", operation: "start", withTicket: true, terminalJobState: "cancelled"},
		{name: "stop approval without ticket or job", writer: "approval", eventType: "VM_STOP_REQUESTED", operation: "stop"},
		{name: "stop batch with completed job", writer: "batch", eventType: "VM_STOP_REQUESTED", operation: "stop", withTicket: true, terminalJobState: "completed"},
		{name: "restart direct without ticket or job", writer: "direct", eventType: "VM_RESTART_REQUESTED", operation: "restart"},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newPowerGuardTestStore(t, fmt.Sprintf("pgrf%d", index))
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()

			const (
				vmID             = "vm-processing-power-fence"
				existingEventID  = "event-processing-power-fence"
				existingTicketID = "ticket-processing-power-fence"
			)
			if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), $2, 'vm', $3, $4, 'PROCESSING', 'seed-actor')
`, existingEventID, tc.eventType, vmID, []byte(`{"vm_id":"`+vmID+`","operation":"`+tc.operation+`"}`)); err != nil {
				t.Fatalf("insert PROCESSING power fence: %v", err)
			}
			if tc.withTicket {
				if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester, reason
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'EXECUTING', 'seed-actor', 'ambiguous restart')
`, existingTicketID, existingEventID); err != nil {
					t.Fatalf("insert EXECUTING power ticket: %v", err)
				}
			}
			if tc.terminalJobState != "" {
				inserted, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: existingEventID}, nil)
				if err != nil {
					t.Fatalf("insert power fence River job: %v", err)
				}
				if inserted == nil || inserted.Job == nil || inserted.UniqueSkippedAsDuplicate {
					t.Fatalf("power fence River insert = %#v, want new job", inserted)
				}
				if _, err := store.pool.Exec(ctx, `
UPDATE river_job
SET state = $2, finalized_at = NOW()
WHERE id = $1
`, inserted.Job.ID, tc.terminalJobState); err != nil {
					t.Fatalf("make power fence River job terminal: %v", err)
				}
			}

			var err error
			switch tc.writer {
			case "direct":
				err = store.writer.CreatePowerEventAndEnqueue(
					ctx,
					powerGuardDirectInput("event-after-processing-power", vmID, "VM_RESTART_REQUESTED", "direct-actor"),
				)
			case "approval":
				err = store.writer.CreatePowerApprovalRequest(ctx, powerGuardApprovalInput(
					vmID,
					"event-approval-after-processing-power",
					"ticket-approval-after-processing-power",
					"VM_RESTART_REQUESTED",
				))
			case "batch":
				err = store.writer.CreateBatchPowerAndMaybeEnqueue(
					ctx,
					powerGuardBatchInput("batch-after-processing-power", vmID, "VM_RESTART_REQUESTED"),
					nil,
				)
			default:
				t.Fatalf("unknown test writer %q", tc.writer)
			}

			active := requireActivePowerEventError(t, err)
			if active.ExistingEventID != existingEventID || active.ExistingEventType != tc.eventType || active.AggregateID != vmID {
				t.Fatalf("active power fence = %+v, want %s event %q for VM %q", active, tc.eventType, existingEventID, vmID)
			}
			if tc.withTicket {
				if active.ExistingTicketID != existingTicketID || active.ExistingTicketStatus != "EXECUTING" {
					t.Fatalf("active power ticket = %+v, want EXECUTING ticket %q", active, existingTicketID)
				}
			} else if active.ExistingTicketID != "" {
				t.Fatalf("active power fence unexpectedly has ticket: %+v", active)
			}

			want := powerGuardSideEffects{events: 1}
			if tc.withTicket {
				want.tickets = 1
			}
			if tc.terminalJobState != "" {
				want.jobs = 1
			}
			assertPowerGuardSideEffects(t, store, want)
			var storedStatus string
			if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, existingEventID).Scan(&storedStatus); err != nil {
				t.Fatalf("query power fence status: %v", err)
			}
			if storedStatus != "PROCESSING" {
				t.Fatalf("power fence status = %q, want PROCESSING", storedStatus)
			}
		})
	}
}

func TestPowerGuard_WaiterSeesCommittedStartAndStopProcessingFences(t *testing.T) {
	tests := []struct {
		eventType string
		operation string
	}{
		{eventType: "VM_START_REQUESTED", operation: "start"},
		{eventType: "VM_STOP_REQUESTED", operation: "stop"},
	}
	for index, tc := range tests {
		t.Run(tc.operation, func(t *testing.T) {
			store := newPowerGuardTestStore(t, fmt.Sprintf("pgpf%d", index))
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()

			vmID := "vm-processing-fence-race-" + tc.operation
			existingEventID := "event-processing-fence-race-" + tc.operation
			newEventID := "event-after-processing-fence-race-" + tc.operation
			fenceTx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
			if err != nil {
				t.Fatalf("begin PROCESSING fence transaction: %v", err)
			}
			fenceTxClosed := false
			t.Cleanup(func() {
				if !fenceTxClosed {
					_ = fenceTx.Rollback(context.Background())
				}
			})
			if err := lockPowerVM(ctx, fenceTx, vmID); err != nil {
				t.Fatalf("lock PROCESSING fence VM: %v", err)
			}
			var blockerPID int32
			if err := fenceTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
				t.Fatalf("query PROCESSING fence transaction PID: %v", err)
			}
			if _, err := fenceTx.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), $2, 'vm', $3, $4, 'PROCESSING', 'seed-actor')
`, existingEventID, tc.eventType, vmID, []byte(`{"vm_id":"`+vmID+`","operation":"`+tc.operation+`"}`)); err != nil {
				t.Fatalf("insert uncommitted PROCESSING fence: %v", err)
			}

			result := make(chan error, 1)
			var waiter errgroup.Group
			waiter.Go(func() error {
				result <- store.writer.CreatePowerEventAndEnqueue(
					ctx,
					powerGuardDirectInput(newEventID, vmID, "VM_RESTART_REQUESTED", "waiting-actor"),
				)
				return nil
			})
			waitForPowerGuardBlockedCall(t, store.pool, blockerPID)
			if err := fenceTx.Commit(ctx); err != nil {
				t.Fatalf("commit PROCESSING fence: %v", err)
			}
			fenceTxClosed = true

			select {
			case err := <-result:
				active := requireActivePowerEventError(t, err)
				if active.ExistingEventID != existingEventID || active.ExistingEventType != tc.eventType || active.AggregateID != vmID {
					t.Fatalf("waiter active fence = %+v, want %s event %q for VM %q", active, tc.eventType, existingEventID, vmID)
				}
			case <-ctx.Done():
				t.Fatalf("waiting power submission did not finish: %v", ctx.Err())
			}
			if err := waiter.Wait(); err != nil {
				t.Fatalf("wait for blocked power submission: %v", err)
			}
			assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1})
			assertPowerGuardRowsAbsent(t, store, newEventID, "", "")
		})
	}
}

func TestPowerGuard_DuplicateTicketBindingCannotHidePendingApproval(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgdt")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID            = "vm-duplicate-ticket-binding"
		eventID         = "event-duplicate-ticket-binding"
		pendingTicketID = "ticket-duplicate-ticket-pending"
		newerTicketID   = "ticket-duplicate-ticket-terminal"
		newEventID      = "event-after-duplicate-ticket-binding"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'PENDING', 'approval-actor')
`, eventID, vmID, []byte(`{"vm_id":"`+vmID+`","operation":"start"}`)); err != nil {
		t.Fatalf("insert approval-pending power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester, reason
)
VALUES
  ($1, NOW() - INTERVAL '1 minute', NOW() - INTERVAL '1 minute', $3, 'POWER', 'PENDING', 'approval-actor', 'real pending approval'),
  ($2, NOW(), NOW(), $3, 'POWER', 'SUCCESS', 'foreign-actor', 'corrupt newer duplicate')
`, pendingTicketID, newerTicketID, eventID); err != nil {
		t.Fatalf("insert duplicate power ticket bindings: %v", err)
	}

	err := store.writer.CreatePowerEventAndEnqueue(
		ctx,
		powerGuardDirectInput(newEventID, vmID, "VM_STOP_REQUESTED", "direct-actor"),
	)
	active := requireActivePowerEventError(t, err)
	if active.ExistingEventID != eventID ||
		active.ExistingTicketID != pendingTicketID ||
		active.ExistingTicketStatus != "PENDING" {
		t.Fatalf("duplicate binding active fence = %+v, want pending ticket %q", active, pendingTicketID)
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 1, tickets: 2})
	assertPowerGuardRowsAbsent(t, store, newEventID, "", "")
}

func TestPowerGuard_BatchRetryConflictPreservesTerminalStateAndDoesNotEnqueue(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrc")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID          = "vm-retry-conflict"
		parentID      = "batch-retry-conflict"
		retryEventID  = "event-retry-conflict"
		retryTicketID = "ticket-retry-conflict"
		activeEventID = "event-active-conflict"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES
  ($1, NOW(), 'VM_STOP_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor'),
  ($4, NOW(), 'VM_START_REQUESTED', 'vm', $2, $5, 'PROCESSING', 'active-actor')
`, retryEventID, vmID, mustPowerRetryEventPayload(t, vmID, "stop"), activeEventID, []byte(`{"active":true}`)); err != nil {
		t.Fatalf("insert retry and active power events: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'seed failure', $3)
`, retryTicketID, retryEventID, parentID); err != nil {
		t.Fatalf("insert terminal retry ticket: %v", err)
	}
	inserted, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: activeEventID}, nil)
	if err != nil {
		t.Fatalf("insert active power job: %v", err)
	}
	if inserted == nil || inserted.Job == nil || inserted.UniqueSkippedAsDuplicate {
		t.Fatalf("active power job insert result = %#v, want newly inserted runnable job", inserted)
	}

	err = store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{
			TicketID: retryTicketID,
			EventID:  retryEventID,
		}},
	})
	active := requireActivePowerEventError(t, err)
	if active.ExistingEventID != activeEventID || active.ExistingEventType != "VM_START_REQUESTED" ||
		active.ExistingTicketID != "" || active.AggregateID != vmID {
		t.Fatalf("retry active event error = %+v, want event %q for VM %q", active, activeEventID, vmID)
	}

	var retryEventStatus string
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, retryEventID).Scan(&retryEventStatus); err != nil {
		t.Fatalf("query retry event after conflict: %v", err)
	}
	var (
		retryTicketStatus string
		rejectReason      pgtype.Text
	)
	if err := store.pool.QueryRow(ctx, `
SELECT status, reject_reason
FROM tickets
WHERE id = $1
`, retryTicketID).Scan(&retryTicketStatus, &rejectReason); err != nil {
		t.Fatalf("query retry ticket after conflict: %v", err)
	}
	if retryEventStatus != "FAILED" || retryTicketStatus != "FAILED" ||
		!rejectReason.Valid || rejectReason.String != "seed failure" {
		t.Fatalf(
			"retry state after conflict = event:%q ticket:%q reject_reason:%#v, want FAILED/FAILED/seed failure",
			retryEventStatus,
			retryTicketStatus,
			rejectReason,
		)
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 2, tickets: 1, jobs: 1})
	assertPowerGuardJobEvent(t, store, activeEventID)
}

func TestPowerGuard_BatchRetryExhaustedPreservesTerminalStateAndDoesNotEnqueue(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgre")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-exhausted"
		parentID = "batch-retry-exhausted"
		eventID  = "event-retry-exhausted"
		ticketID = "ticket-retry-exhausted"
	)
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "stop", vmID)
	attemptedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_STOP_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "stop")); err != nil {
		t.Fatalf("seed exhausted power retry event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id, attempt_count, last_attempt_at
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'terminal failure', $3, $4, $5)
`, ticketID, eventID, parentID, domain.BatchChildMaxAttempts, attemptedAt); err != nil {
		t.Fatalf("seed exhausted power retry ticket: %v", err)
	}

	err := store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
	})
	var exhausted *BatchChildAttemptsExhaustedError
	if !errors.As(err, &exhausted) {
		t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *BatchChildAttemptsExhaustedError", err)
	}
	if exhausted.TicketID != ticketID || exhausted.AttemptCount != domain.BatchChildMaxAttempts || exhausted.MaxAttempts != domain.BatchChildMaxAttempts {
		t.Fatalf("exhausted error = %+v", exhausted)
	}

	var (
		storedTicketStatus string
		storedEventStatus  string
		storedReason       pgtype.Text
		storedAttempts     int
		storedAttemptedAt  time.Time
		jobCount           int
	)
	if err := store.pool.QueryRow(ctx, `
SELECT ticket.status, event.status, ticket.reject_reason, ticket.attempt_count,
       ticket.last_attempt_at,
       (SELECT count(*) FROM river_job WHERE kind = 'vm_power' AND args->>'event_id' = $2)
FROM tickets AS ticket
JOIN domain_events AS event ON event.id = ticket.event_id
WHERE ticket.id = $1
`, ticketID, eventID).Scan(
		&storedTicketStatus,
		&storedEventStatus,
		&storedReason,
		&storedAttempts,
		&storedAttemptedAt,
		&jobCount,
	); err != nil {
		t.Fatalf("load exhausted power retry state: %v", err)
	}
	if storedTicketStatus != "FAILED" || storedEventStatus != "FAILED" || !storedReason.Valid ||
		storedReason.String != "terminal failure" || storedAttempts != domain.BatchChildMaxAttempts ||
		!storedAttemptedAt.Equal(attemptedAt) || jobCount != 0 {
		t.Fatalf(
			"exhausted retry changed state: ticket=%s event=%s reason=%#v attempts=%d at=%s jobs=%d",
			storedTicketStatus,
			storedEventStatus,
			storedReason,
			storedAttempts,
			storedAttemptedAt,
			jobCount,
		)
	}
}

func TestPowerGuard_BatchRetryUniqueJobConflictRollsBackTerminalState(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgru")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-running-job"
		parentID = "batch-retry-running-job"
		eventID  = "event-retry-running-job"
		ticketID = "ticket-retry-running-job"
	)
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "start", vmID)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "start")); err != nil {
		t.Fatalf("insert failed power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'original worker failure', $3)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert failed power ticket: %v", err)
	}
	inserted, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: eventID}, nil)
	if err != nil {
		t.Fatalf("insert existing power job: %v", err)
	}
	if inserted == nil || inserted.Job == nil || inserted.UniqueSkippedAsDuplicate {
		t.Fatalf("existing power job insert result = %#v, want newly inserted job", inserted)
	}
	if _, updateErr := store.pool.Exec(ctx, `
UPDATE river_job
SET state = 'running', attempt = 1, attempted_at = NOW(), attempted_by = ARRAY['worker-1']
WHERE id = $1
`, inserted.Job.ID); updateErr != nil {
		t.Fatalf("mark existing power job running: %v", updateErr)
	}

	err = store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{
			TicketID: ticketID,
			EventID:  eventID,
		}},
	})
	if err == nil {
		t.Fatal("RetryBatchPowerAndEnqueue() error = nil, want running unique job conflict")
	}
	var conflict *PowerRetryJobConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *PowerRetryJobConflictError", err)
	}
	if conflict.EventID != eventID || conflict.ExistingJobID != inserted.Job.ID || conflict.ExistingJobState != "running" {
		t.Fatalf("retry job conflict = %+v, want event %q job %d state running", conflict, eventID, inserted.Job.ID)
	}

	var eventStatus string
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query retry event after unique conflict: %v", err)
	}
	var (
		ticketStatus string
		rejectReason pgtype.Text
	)
	if err := store.pool.QueryRow(ctx, `SELECT status, reject_reason FROM tickets WHERE id = $1`, ticketID).Scan(&ticketStatus, &rejectReason); err != nil {
		t.Fatalf("query retry ticket after unique conflict: %v", err)
	}
	if eventStatus != "FAILED" || ticketStatus != "FAILED" || !rejectReason.Valid || rejectReason.String != "original worker failure" {
		t.Fatalf("retry terminal state = event:%q ticket:%q reason:%#v, want FAILED/FAILED/original worker failure", eventStatus, ticketStatus, rejectReason)
	}
	var (
		jobCount int
		jobState string
	)
	if err := store.pool.QueryRow(ctx, `SELECT count(*), min(state::text) FROM river_job WHERE args->>'event_id' = $1`, eventID).Scan(&jobCount, &jobState); err != nil {
		t.Fatalf("query jobs after retry unique conflict: %v", err)
	}
	if jobCount != 1 || jobState != "running" {
		t.Fatalf("power jobs after retry unique conflict = count:%d state:%q, want 1/running", jobCount, jobState)
	}
}

func TestPowerGuard_RepeatedBatchRetryReturnsInProgressWithoutDuplicatingJob(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrr")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-repeated"
		parentID = "batch-retry-repeated"
		eventID  = "event-retry-repeated"
		ticketID = "ticket-retry-repeated"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "start")); err != nil {
		t.Fatalf("insert failed power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'original failure', $3)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert failed power ticket: %v", err)
	}
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "start", vmID)
	input := BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
	}
	if err := store.writer.RetryBatchPowerAndEnqueue(ctx, input); err != nil {
		t.Fatalf("first RetryBatchPowerAndEnqueue() error: %v", err)
	}

	err := store.writer.RetryBatchPowerAndEnqueue(ctx, input)
	var conflict *PowerRetryJobConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("repeated retry error = %v, want *PowerRetryJobConflictError", err)
	}
	if conflict.EventID != eventID || conflict.ExistingJobID <= 0 || conflict.ExistingJobState == "" {
		t.Fatalf("repeated retry conflict = %+v, want runnable job for event %q", conflict, eventID)
	}

	var (
		eventStatus  string
		ticketStatus string
		jobCount     int
	)
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query event after repeated retry: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&ticketStatus); err != nil {
		t.Fatalf("query ticket after repeated retry: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE args->>'event_id' = $1`, eventID).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs after repeated retry: %v", err)
	}
	if eventStatus != "PENDING" || ticketStatus != "EXECUTING" || jobCount != 1 {
		t.Fatalf("repeated retry state = event:%q ticket:%q jobs:%d, want PENDING/EXECUTING/1", eventStatus, ticketStatus, jobCount)
	}
	assertBatchPowerRetryParentReopened(t, store.pool, parentID, 1, 0, 0, 1)
}

func TestPowerGuard_BatchRetryIgnoresCompletedRiverJob(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrc")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-completed-job"
		parentID = "batch-retry-completed-job"
		eventID  = "event-retry-completed-job"
		ticketID = "ticket-retry-completed-job"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "start")); err != nil {
		t.Fatalf("insert failed power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'original failure', $3)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert failed power ticket: %v", err)
	}
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "start", vmID)
	existing, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: eventID}, nil)
	if err != nil {
		t.Fatalf("insert completed predecessor job: %v", err)
	}
	if existing == nil || existing.Job == nil || existing.UniqueSkippedAsDuplicate {
		t.Fatalf("predecessor insert result = %#v, want newly inserted job", existing)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE river_job
SET state = 'completed', finalized_at = NOW()
WHERE id = $1
`, existing.Job.ID); err != nil {
		t.Fatalf("mark predecessor job completed: %v", err)
	}

	if err := store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
	}); err != nil {
		t.Fatalf("RetryBatchPowerAndEnqueue() error with completed predecessor: %v", err)
	}

	var (
		eventStatus  string
		ticketStatus string
		jobCount     int
		states       []string
	)
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query retried event: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&ticketStatus); err != nil {
		t.Fatalf("query retried ticket: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT count(*), array_agg(state::text ORDER BY state::text)
FROM river_job
WHERE args->>'event_id' = $1
`, eventID).Scan(&jobCount, &states); err != nil {
		t.Fatalf("query predecessor and retry jobs: %v", err)
	}
	if eventStatus != "PENDING" || ticketStatus != "EXECUTING" || jobCount != 2 ||
		!slices.Equal(states, []string{"available", "completed"}) {
		t.Fatalf(
			"retry after completed job = event:%q ticket:%q jobs:%d states:%v, want PENDING/EXECUTING/2/[available completed]",
			eventStatus,
			ticketStatus,
			jobCount,
			states,
		)
	}
	assertBatchPowerRetryParentReopened(t, store.pool, parentID, 1, 0, 0, 1)
}

func TestPowerGuard_BatchRetryReplacesLegacyCompletedUniqueJob(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrl")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-legacy-completed-job"
		parentID = "batch-retry-legacy-completed-job"
		eventID  = "event-retry-legacy-completed-job"
		ticketID = "ticket-retry-legacy-completed-job"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "start")); err != nil {
		t.Fatalf("insert failed power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id, attempt_count
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'original failure', $3, 1)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert failed power ticket: %v", err)
	}
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "start", vmID)
	legacyInsertOpts := jobs.VMPowerArgs{}.InsertOpts()
	legacyInsertOpts.UniqueOpts.ByState = nil
	predecessor, err := store.writer.riverClient.Insert(
		ctx,
		jobs.VMPowerArgs{EventID: eventID},
		&legacyInsertOpts,
	)
	if err != nil {
		t.Fatalf("insert legacy predecessor job: %v", err)
	}
	if predecessor == nil || predecessor.Job == nil || predecessor.UniqueSkippedAsDuplicate {
		t.Fatalf("legacy predecessor insert result = %#v, want newly inserted job", predecessor)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE river_job
SET state = 'completed', finalized_at = NOW()
WHERE id = $1
`, predecessor.Job.ID); err != nil {
		t.Fatalf("mark legacy predecessor completed: %v", err)
	}

	if err := store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
	}); err != nil {
		t.Fatalf("RetryBatchPowerAndEnqueue() legacy rollout error: %v", err)
	}

	var (
		eventStatus  string
		ticketStatus string
		attemptCount int
		jobCount     int
		jobState     string
	)
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query retried event: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT status, attempt_count FROM tickets WHERE id = $1`, ticketID).Scan(&ticketStatus, &attemptCount); err != nil {
		t.Fatalf("query retried ticket: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `
SELECT count(*), min(state::text)
FROM river_job
WHERE args->>'event_id' = $1
`, eventID).Scan(&jobCount, &jobState); err != nil {
		t.Fatalf("query replacement retry job: %v", err)
	}
	if eventStatus != "PENDING" || ticketStatus != "EXECUTING" || attemptCount != 2 || jobCount != 1 || jobState != "available" {
		t.Fatalf(
			"legacy retry state = event:%q ticket:%q attempts:%d jobs:%d/%q, want PENDING/EXECUTING/2/1/available",
			eventStatus,
			ticketStatus,
			attemptCount,
			jobCount,
			jobState,
		)
	}
	assertBatchPowerRetryParentReopened(t, store.pool, parentID, 1, 0, 0, 1)
}

func TestPowerGuard_StaleCompletedRetryReturnsNotEligibleWithoutEnqueue(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrn")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const (
		vmID           = "vm-retry-not-eligible"
		parentID       = "batch-retry-not-eligible"
		eventID        = "event-retry-not-eligible"
		ticketID       = "ticket-retry-not-eligible"
		siblingVMID    = "vm-retry-not-eligible-sibling"
		siblingEventID = "event-retry-not-eligible-sibling"
		siblingTicket  = "ticket-retry-not-eligible-sibling"
	)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_STOP_REQUESTED', 'vm', $2, $3, 'COMPLETED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "stop")); err != nil {
		t.Fatalf("insert completed power event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'SUCCESS', 'retry-actor', 'completed child', $3)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert completed power ticket: %v", err)
	}
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "stop", vmID, siblingVMID)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_STOP_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, siblingEventID, siblingVMID, mustPowerRetryEventPayload(t, siblingVMID, "stop")); err != nil {
		t.Fatalf("insert failed power sibling event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor', 'sibling failure', $3)
`, siblingTicket, siblingEventID, parentID); err != nil {
		t.Fatalf("insert failed power sibling ticket: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
UPDATE batch_tickets
SET success_count = 1, failed_count = 1, status = 'PARTIAL_SUCCESS'
WHERE id = $1
`, parentID); err != nil {
		t.Fatalf("align partial-success projection: %v", err)
	}
	legacyInsertOpts := jobs.VMPowerArgs{}.InsertOpts()
	legacyInsertOpts.UniqueOpts.ByState = nil
	existing, err := store.writer.riverClient.Insert(
		ctx,
		jobs.VMPowerArgs{EventID: eventID},
		&legacyInsertOpts,
	)
	if err != nil {
		t.Fatalf("insert completed predecessor job: %v", err)
	}
	if existing == nil || existing.Job == nil || existing.UniqueSkippedAsDuplicate {
		t.Fatalf("predecessor insert result = %#v, want newly inserted job", existing)
	}
	if _, updateJobErr := store.pool.Exec(ctx, `
UPDATE river_job
SET state = 'completed', finalized_at = NOW()
WHERE id = $1
`, existing.Job.ID); updateJobErr != nil {
		t.Fatalf("mark predecessor job completed: %v", updateJobErr)
	}

	err = store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
	})
	var notEligible *PowerRetryNotEligibleError
	if !errors.As(err, &notEligible) {
		t.Fatalf("stale completed retry error = %v, want *PowerRetryNotEligibleError", err)
	}
	if notEligible.TicketID != ticketID || notEligible.EventID != eventID {
		t.Fatalf("not-eligible retry = %+v, want ticket/event %s/%s", notEligible, ticketID, eventID)
	}
	var (
		jobCount int
		jobState string
	)
	if err := store.pool.QueryRow(ctx, `
SELECT count(*), min(state::text)
FROM river_job
WHERE args->>'event_id' = $1
`, eventID).Scan(&jobCount, &jobState); err != nil {
		t.Fatalf("count jobs after stale completed retry: %v", err)
	}
	if jobCount != 1 || jobState != "completed" {
		t.Fatalf("stale completed retry jobs = %d/%q, want only the completed predecessor after probe rollback", jobCount, jobState)
	}
}

func TestPowerGuard_RetryWaiterReadsCommittedStateWithRepeatableReadDefault(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrr")
	repeatableReadStore := newRepeatableReadPowerGuardStore(t, store.pool)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	const (
		vmID     = "vm-retry-read-committed"
		parentID = "batch-retry-read-committed"
		eventID  = "event-retry-read-committed"
		ticketID = "ticket-retry-read-committed"
	)
	seedFailedBatchPowerRetryParent(t, store.pool, parentID, "start", vmID)
	if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, eventID, vmID, mustPowerRetryEventPayload(t, vmID, "start")); err != nil {
		t.Fatalf("insert retry event: %v", err)
	}
	if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', 'seed failure', $3)
`, ticketID, eventID, parentID); err != nil {
		t.Fatalf("insert retry ticket: %v", err)
	}

	mutationTx, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin terminal state mutation: %v", err)
	}
	mutationTxClosed := false
	t.Cleanup(func() {
		if !mutationTxClosed {
			_ = mutationTx.Rollback(context.Background())
		}
	})
	if _, err := mutationTx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		PowerVMLockKey(vmID),
	); err != nil {
		t.Fatalf("lock retry VM before terminal mutation: %v", err)
	}
	var blockerPID int32
	if err := mutationTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query retry mutation blocker PID: %v", err)
	}
	if _, err := mutationTx.Exec(
		ctx,
		`UPDATE domain_events SET status = 'COMPLETED' WHERE id = $1`,
		eventID,
	); err != nil {
		t.Fatalf("stage terminal event state: %v", err)
	}
	if _, err := mutationTx.Exec(
		ctx,
		`UPDATE tickets SET status = 'SUCCESS', reject_reason = NULL WHERE id = $1`,
		ticketID,
	); err != nil {
		t.Fatalf("stage terminal ticket state: %v", err)
	}
	if _, err := mutationTx.Exec(
		ctx,
		`UPDATE batch_tickets SET success_count = 1, failed_count = 0, pending_count = 0 WHERE id = $1`,
		parentID,
	); err != nil {
		t.Fatalf("stage terminal projection counters: %v", err)
	}

	retryResult := make(chan error, 1)
	var workers errgroup.Group
	workers.Go(func() error {
		retryResult <- repeatableReadStore.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
			ParentID: parentID,
			Children: []BatchPowerRetryChildInput{{TicketID: ticketID, EventID: eventID}},
		})
		return nil
	})
	waitForPowerGuardBlockedCall(t, store.pool, blockerPID)
	if err := mutationTx.Commit(ctx); err != nil {
		t.Fatalf("commit terminal state mutation: %v", err)
	}
	mutationTxClosed = true
	if err := workers.Wait(); err != nil {
		t.Fatalf("wait for retry behind terminal mutation: %v", err)
	}

	retryErr := <-retryResult
	var notEligible *PowerRetryNotEligibleError
	if !errors.As(retryErr, &notEligible) {
		t.Fatalf("retry after committed terminal mutation error = %v, want *PowerRetryNotEligibleError", retryErr)
	}
	var eventStatus, ticketStatus string
	if err := store.pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query event after terminal retry race: %v", err)
	}
	if err := store.pool.QueryRow(ctx, `SELECT status FROM tickets WHERE id = $1`, ticketID).Scan(&ticketStatus); err != nil {
		t.Fatalf("query ticket after terminal retry race: %v", err)
	}
	var jobCount int
	if err := store.pool.QueryRow(
		ctx,
		`SELECT count(*) FROM river_job WHERE args->>'event_id' = $1`,
		eventID,
	).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs after terminal retry race: %v", err)
	}
	if eventStatus != "COMPLETED" || ticketStatus != "SUCCESS" || jobCount != 0 {
		t.Fatalf(
			"terminal retry race state = event:%q ticket:%q jobs:%d, want COMPLETED/SUCCESS/0",
			eventStatus,
			ticketStatus,
			jobCount,
		)
	}
}

func TestPowerGuard_BatchRetryLaterUniqueConflictRollsBackEarlierChildAndJob(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgrm")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	const parentID = "batch-retry-multiple"
	seedFailedBatchPowerRetryParent(
		t,
		store.pool,
		parentID,
		"start",
		"vm-retry-multiple-a",
		"vm-retry-multiple-b",
	)
	children := []struct {
		vmID         string
		eventID      string
		ticketID     string
		rejectReason string
	}{
		{
			vmID:         "vm-retry-multiple-a",
			eventID:      "event-retry-multiple-a",
			ticketID:     "ticket-retry-multiple-a",
			rejectReason: "first original failure",
		},
		{
			vmID:         "vm-retry-multiple-b",
			eventID:      "event-retry-multiple-b",
			ticketID:     "ticket-retry-multiple-b",
			rejectReason: "second original failure",
		},
	}
	for _, child := range children {
		if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, $3, 'FAILED', 'retry-actor')
`, child.eventID, child.vmID, mustPowerRetryEventPayload(t, child.vmID, "start")); err != nil {
			t.Fatalf("insert failed power event %s: %v", child.eventID, err)
		}
		if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, reject_reason, parent_ticket_id
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor',
        'retry child', $3, $4)
`, child.ticketID, child.eventID, child.rejectReason, parentID); err != nil {
			t.Fatalf("insert failed power ticket %s: %v", child.ticketID, err)
		}
	}

	type terminalSnapshot struct {
		eventStatus  string
		ticketStatus string
		rejectReason pgtype.Text
		updatedAt    time.Time
	}
	loadSnapshot := func(child struct {
		vmID         string
		eventID      string
		ticketID     string
		rejectReason string
	}) terminalSnapshot {
		t.Helper()
		var snapshot terminalSnapshot
		if err := store.pool.QueryRow(ctx, `
SELECT event.status, ticket.status, ticket.reject_reason, ticket.updated_at
FROM domain_events AS event
JOIN tickets AS ticket ON ticket.event_id = event.id
WHERE event.id = $1 AND ticket.id = $2
`, child.eventID, child.ticketID).Scan(
			&snapshot.eventStatus,
			&snapshot.ticketStatus,
			&snapshot.rejectReason,
			&snapshot.updatedAt,
		); err != nil {
			t.Fatalf("load terminal snapshot for %s: %v", child.eventID, err)
		}
		return snapshot
	}
	before := []terminalSnapshot{loadSnapshot(children[0]), loadSnapshot(children[1])}

	existing, err := store.writer.riverClient.Insert(ctx, jobs.VMPowerArgs{EventID: children[1].eventID}, nil)
	if err != nil {
		t.Fatalf("insert second child's existing power job: %v", err)
	}
	if existing == nil || existing.Job == nil || existing.UniqueSkippedAsDuplicate {
		t.Fatalf("existing second-child job result = %#v, want newly inserted job", existing)
	}
	if _, updateErr := store.pool.Exec(ctx, `
UPDATE river_job
SET state = 'running', attempt = 1, attempted_at = NOW(), attempted_by = ARRAY['worker-1']
WHERE id = $1
`, existing.Job.ID); updateErr != nil {
		t.Fatalf("mark second child's existing job running: %v", updateErr)
	}

	err = store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
		ParentID: parentID,
		Children: []BatchPowerRetryChildInput{
			{TicketID: children[0].ticketID, EventID: children[0].eventID},
			{TicketID: children[1].ticketID, EventID: children[1].eventID},
		},
	})
	var conflict *PowerRetryJobConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("multi-child retry error = %v, want *PowerRetryJobConflictError", err)
	}
	if conflict.EventID != children[1].eventID || conflict.ExistingJobID != existing.Job.ID || conflict.ExistingJobState != "running" {
		t.Fatalf("multi-child retry conflict = %+v, want second event %q job %d/running", conflict, children[1].eventID, existing.Job.ID)
	}

	for idx, child := range children {
		after := loadSnapshot(child)
		if after.eventStatus != before[idx].eventStatus ||
			after.ticketStatus != before[idx].ticketStatus ||
			after.rejectReason != before[idx].rejectReason ||
			!after.updatedAt.Equal(before[idx].updatedAt) {
			t.Fatalf("child %d terminal snapshot changed after later conflict: before=%+v after=%+v", idx, before[idx], after)
		}
	}
	var firstJobCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE args->>'event_id' = $1`, children[0].eventID).Scan(&firstJobCount); err != nil {
		t.Fatalf("count rolled-back first-child jobs: %v", err)
	}
	if firstJobCount != 0 {
		t.Fatalf("first-child River job count = %d, want 0 after transaction rollback", firstJobCount)
	}
	var (
		secondJobCount int
		secondJobState string
	)
	if err := store.pool.QueryRow(ctx, `
SELECT count(*), min(state::text)
FROM river_job
WHERE args->>'event_id' = $1
`, children[1].eventID).Scan(&secondJobCount, &secondJobState); err != nil {
		t.Fatalf("query preserved second-child job: %v", err)
	}
	if secondJobCount != 1 || secondJobState != "running" {
		t.Fatalf("second-child River jobs = count:%d state:%q, want 1/running", secondJobCount, secondJobState)
	}
}

func TestPowerGuard_BatchReplayLocksActorBeforeRequestAndSkipsValidation(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgpr")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	input := powerGuardBatchInput("batch-policy-replay-first", "vm-policy-replay", powerEventTypeStart)
	if err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil); err != nil {
		t.Fatalf("create original power batch: %v", err)
	}
	replayInput := input
	replayInput.ParentID = "batch-policy-replay-second"
	phases := make([]string, 0, 2)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, replayInput, &BatchPowerSubmissionTxPolicy{
		LockActor: func(context.Context, pgx.Tx) error {
			phases = append(phases, "lock_actor")
			return nil
		},
		Validate: func(context.Context, pgx.Tx) error {
			phases = append(phases, "validate")
			return nil
		},
	})
	var replay *BatchSubmissionReplayError
	if !errors.As(err, &replay) {
		t.Fatalf("power batch replay error = %v, want *BatchSubmissionReplayError", err)
	}
	if replay.BatchID != input.ParentID {
		t.Fatalf("power batch replay ID = %q, want %q", replay.BatchID, input.ParentID)
	}
	if !slices.Equal(phases, []string{"lock_actor"}) {
		t.Fatalf("power batch replay policy phases = %v, want actor lock only", phases)
	}
	assertPowerGuardRowsAbsent(t, store, "", replayInput.ParentID, replayInput.ParentID)
}

func TestPowerGuard_LongFourByteRequestIDPersistsAndReplaysExactly(t *testing.T) {
	store := newPowerGuardTestStore(t, "pglongid")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	requestID := strings.Repeat("😀", 4096)
	first := powerGuardBatchInput("batch-long-request-first", "vm-long-request-first", powerEventTypeStart)
	first.RequestID = requestID
	mustSyncPowerGuardBatchParentPayload(&first)
	if err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, first, nil); err != nil {
		t.Fatalf("create long-key power batch: %v", err)
	}

	replayInput := powerGuardBatchInput("batch-long-request-replay", "vm-long-request-replay", powerEventTypeStart)
	replayInput.RequestID = requestID
	mustSyncPowerGuardBatchParentPayload(&replayInput)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, replayInput, nil)
	var replay *BatchSubmissionReplayError
	if !errors.As(err, &replay) {
		t.Fatalf("long-key replay error = %v, want *BatchSubmissionReplayError", err)
	}
	if replay.BatchID != first.ParentID {
		t.Fatalf("long-key replay ID = %q, want %q", replay.BatchID, first.ParentID)
	}

	var storedRequestID string
	if err := store.pool.QueryRow(ctx, `
SELECT request_id FROM batch_tickets WHERE id = $1
`, first.ParentID).Scan(&storedRequestID); err != nil {
		t.Fatalf("query persisted long request ID: %v", err)
	}
	if storedRequestID != requestID {
		t.Fatalf("persisted long request ID byte length = %d, want %d", len(storedRequestID), len(requestID))
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 2, tickets: 2, batches: 1, jobs: 1})
	assertPowerGuardRowsAbsent(t, store, "", replayInput.ParentID, replayInput.ParentID)
}

func TestPowerGuard_HistoricalDuplicateRequestIDChoosesOldestWithoutMutation(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgduplicateid")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	requestID := strings.Repeat("😀", 513)
	oldestCreatedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows := []struct {
		id        string
		createdAt time.Time
	}{
		{id: "batch-historical-later", createdAt: oldestCreatedAt.Add(time.Hour)},
		{id: "batch-historical-oldest-b", createdAt: oldestCreatedAt},
		{id: "batch-historical-oldest-a", createdAt: oldestCreatedAt},
	}
	for _, row := range rows {
		eventID := row.id + "-event"
		payload := []byte(fmt.Sprintf(
			`{"operation":"POWER_START","request_id":%q,"submitted_by":"power-guard-actor","items":[]}`,
			requestID,
		))
		if _, err := store.pool.Exec(ctx, `
INSERT INTO domain_events (
    id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, $2, 'BATCH_POWER_REQUESTED', 'batch', $3, $4, 'PENDING', 'power-guard-actor')
`, eventID, row.createdAt, row.id, payload); err != nil {
			t.Fatalf("insert historical duplicate event %q: %v", eventID, err)
		}
		if _, err := store.pool.Exec(ctx, `
INSERT INTO tickets (
    id, created_at, updated_at, event_id, operation_type, status, requester
)
VALUES ($1, $2, $2, $3, 'POWER', 'PENDING', 'power-guard-actor')
`, row.id, row.createdAt, eventID); err != nil {
			t.Fatalf("insert historical duplicate ticket %q: %v", row.id, err)
		}
		if _, err := store.pool.Exec(ctx, `
INSERT INTO batch_tickets (
    id, created_at, updated_at, batch_type, child_count, pending_count,
    status, request_id, created_by
)
VALUES ($1, $2, $2, 'BATCH_POWER', 0, 0, 'PENDING_APPROVAL', $3, 'power-guard-actor')
`, row.id, row.createdAt, requestID); err != nil {
			t.Fatalf("insert historical duplicate %q: %v", row.id, err)
		}
	}

	input := powerGuardBatchInput("batch-historical-replay", "vm-historical-replay", powerEventTypeStart)
	input.RequestID = requestID
	mustSyncPowerGuardBatchParentPayload(&input)
	err := store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil)
	var replay *BatchSubmissionReplayError
	if !errors.As(err, &replay) {
		t.Fatalf("historical duplicate replay error = %v, want *BatchSubmissionReplayError", err)
	}
	if replay.BatchID != "batch-historical-oldest-a" {
		t.Fatalf("historical duplicate replay ID = %q, want batch-historical-oldest-a", replay.BatchID)
	}

	var preservedCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(*) FROM batch_tickets WHERE request_id = $1
`, requestID).Scan(&preservedCount); err != nil {
		t.Fatalf("count preserved historical duplicate keys: %v", err)
	}
	if preservedCount != len(rows) {
		t.Fatalf("preserved historical duplicate count = %d, want %d", preservedCount, len(rows))
	}
	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{
		events:  len(rows),
		tickets: len(rows),
		batches: len(rows),
	})
	assertPowerGuardRowsAbsent(t, store, "", input.ParentID, input.ParentID)
}

func TestPowerGuard_ConcurrentSameBatchRequestIDAcrossPowerOperationsUsesSeparateScopes(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgid")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	const requestID = "request-concurrent-idempotency"
	lockKey := BatchSubmissionAdvisoryLockKey
	lockConn, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire batch idempotency lock connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1 || ':' || current_schema(), 0))`, lockKey); err != nil {
		t.Fatalf("hold global batch submission advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query batch idempotency blocker PID: %v", err)
	}

	inputs := []BatchPowerSubmissionInput{
		powerGuardBatchInput("batch-idempotency-a", "vm-idempotency-a", "VM_START_REQUESTED"),
		powerGuardBatchInput("batch-idempotency-b", "vm-idempotency-b", "VM_STOP_REQUESTED"),
	}
	for idx := range inputs {
		inputs[idx].RequestID = requestID
		if idx == 0 {
			inputs[idx].Operation = "POWER_START"
		} else {
			inputs[idx].Operation = "POWER_STOP"
		}
		mustSyncPowerGuardBatchParentPayload(&inputs[idx])
	}
	type result struct {
		parentID string
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, len(inputs))
	var workers errgroup.Group
	for _, input := range inputs {
		input := input
		workers.Go(func() error {
			<-start
			results <- result{
				parentID: input.ParentID,
				err:      store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil),
			}
			return nil
		})
	}
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = store.pool.QueryRow(ctx, `
WITH RECURSIVE blocked(pid) AS (
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
    AND $1 = ANY(pg_blocking_pids(activity.pid))
  UNION
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  JOIN blocked AS upstream
    ON upstream.pid = ANY(pg_blocking_pids(activity.pid))
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
)
SELECT count(*) FROM blocked
`, blockerPID).Scan(&blockedCalls)
		return blockedQueryErr != nil || blockedCalls == len(inputs)
	}, 8*time.Second, 10*time.Millisecond, "operation-scoped batch submissions did not block on the global lock")

	_, unlockErr := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1 || ':' || current_schema(), 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		closeCancel()
		lockHeld = false
	}
	if waitErr := workers.Wait(); waitErr != nil {
		t.Fatalf("wait for concurrent idempotent batch submissions: %v", waitErr)
	}
	close(results)

	if blockedQueryErr != nil {
		t.Fatalf("query calls blocked by batch idempotency lock: %v", blockedQueryErr)
	}
	if blockedCalls != len(inputs) {
		t.Fatalf("calls blocked by batch idempotency lock = %d, want %d before releasing lock", blockedCalls, len(inputs))
	}
	if unlockErr != nil {
		t.Fatalf("release batch idempotency advisory lock: %v", unlockErr)
	}

	successes := 0
	for got := range results {
		if got.err != nil {
			t.Fatalf("concurrent operation-scoped batch %q: %v", got.parentID, got.err)
		}
		successes++
	}
	if successes != len(inputs) {
		t.Fatalf("successful operation-scoped batches = %d, want %d", successes, len(inputs))
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 4, tickets: 4, batches: 2, jobs: 2})
	var persistedCount int
	if err := store.pool.QueryRow(ctx, `SELECT count(*) FROM batch_tickets WHERE request_id = $1`, requestID).Scan(&persistedCount); err != nil {
		t.Fatalf("count committed operation-scoped idempotency keys: %v", err)
	}
	if persistedCount != len(inputs) {
		t.Fatalf("committed operation-scoped idempotency keys = %d, want %d", persistedCount, len(inputs))
	}
}

func TestPowerGuard_ConcurrentBatchesForSameVMLetOnlyOneCommit(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgbc")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	const vmID = "vm-concurrent-batches"
	lockKey := "power:vm:" + vmID
	lockConn, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire power guard lock connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		t.Fatalf("hold power guard advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query power guard blocker PID: %v", err)
	}

	inputs := []BatchPowerSubmissionInput{
		powerGuardBatchInput("batch-concurrent-a", vmID, "VM_START_REQUESTED"),
		powerGuardBatchInput("batch-concurrent-b", vmID, "VM_STOP_REQUESTED"),
	}
	type result struct {
		parentID string
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, len(inputs))
	var workers errgroup.Group
	for _, input := range inputs {
		input := input
		workers.Go(func() error {
			<-start
			results <- result{
				parentID: input.ParentID,
				err:      store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil),
			}
			return nil
		})
	}
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = store.pool.QueryRow(ctx, `
WITH RECURSIVE blocked(pid) AS (
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
    AND $1 = ANY(pg_blocking_pids(activity.pid))
  UNION
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  JOIN blocked AS upstream
    ON upstream.pid = ANY(pg_blocking_pids(activity.pid))
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
)
SELECT count(*) FROM blocked
`, blockerPID).Scan(&blockedCalls)
		return blockedQueryErr != nil || blockedCalls == len(inputs)
	}, 8*time.Second, 10*time.Millisecond, "overlapping power batches did not block on the VM lock")

	_, unlockErr := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		closeCancel()
		lockHeld = false
	}
	if waitErr := workers.Wait(); waitErr != nil {
		t.Fatalf("wait for concurrent power batches: %v", waitErr)
	}
	close(results)

	if blockedQueryErr != nil {
		t.Fatalf("query calls blocked by power guard lock: %v", blockedQueryErr)
	}
	if blockedCalls != len(inputs) {
		t.Fatalf("calls blocked by power guard lock = %d, want %d before releasing lock", blockedCalls, len(inputs))
	}
	if unlockErr != nil {
		t.Fatalf("release power guard advisory lock: %v", unlockErr)
	}

	var (
		winner      result
		loser       result
		successes   int
		activeError *ActivePowerEventError
	)
	for got := range results {
		if got.err == nil {
			winner = got
			successes++
			continue
		}
		var active *ActivePowerEventError
		if !errors.As(got.err, &active) {
			t.Fatalf("concurrent batch %q error = %v, want *ActivePowerEventError", got.parentID, got.err)
		}
		if activeError != nil {
			t.Fatalf("multiple concurrent batch conflicts: first=%+v second=%+v", activeError, active)
		}
		loser = got
		activeError = active
	}
	if successes != 1 || activeError == nil {
		t.Fatalf("concurrent batches produced %d successes and active error %#v, want one of each", successes, activeError)
	}
	if winner.parentID == loser.parentID || loser.parentID == "" {
		t.Fatalf("concurrent batch results = winner:%+v loser:%+v, want distinct requests", winner, loser)
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 2, tickets: 2, batches: 1, jobs: 1})
	var persistedParentID string
	if err := store.pool.QueryRow(ctx, `SELECT id FROM batch_tickets`).Scan(&persistedParentID); err != nil {
		t.Fatalf("query committed batch projection: %v", err)
	}
	if persistedParentID != winner.parentID {
		t.Fatalf("committed batch ID = %q, want successful batch %q", persistedParentID, winner.parentID)
	}
	assertPowerGuardRowsAbsent(t, store, "", loser.parentID, loser.parentID)

	var (
		childEventID   string
		childEventType string
		childTicketID  string
		childParentID  pgtype.Text
	)
	if err := store.pool.QueryRow(ctx, `
SELECT event.id, event.event_type, ticket.id, ticket.parent_ticket_id
FROM domain_events AS event
JOIN tickets AS ticket ON ticket.event_id = event.id
WHERE event.aggregate_type = 'vm'
  AND event.aggregate_id = $1
`, vmID).Scan(&childEventID, &childEventType, &childTicketID, &childParentID); err != nil {
		t.Fatalf("query committed batch child: %v", err)
	}
	if !childParentID.Valid || childParentID.String != winner.parentID {
		t.Fatalf("committed child parent = %#v, want %q", childParentID, winner.parentID)
	}
	if activeError.ExistingEventID != childEventID || activeError.ExistingEventType != childEventType ||
		activeError.ExistingTicketID != childTicketID || activeError.AggregateID != vmID {
		t.Fatalf("losing batch active error = %+v, want committed child event %q ticket %q VM %q", activeError, childEventID, childTicketID, vmID)
	}
	assertPowerGuardJobEvent(t, store, childEventID)
}

func TestPowerGuard_ConcurrentOverlappingBatchesUseSortedVMLockOrder(t *testing.T) {
	store := newPowerGuardTestStore(t, "pgso")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	const (
		firstVM  = "vm-lock-order-a"
		secondVM = "vm-lock-order-z"
	)
	lockKey := PowerVMLockKey(firstVM)
	lockConn, err := store.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire sorted-lock blocker connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		t.Fatalf("hold first sorted VM advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query sorted-lock blocker PID: %v", err)
	}

	firstInput := powerGuardBatchInput("batch-lock-order-first", firstVM, "VM_START_REQUESTED")
	firstExtra := powerGuardBatchChild(secondVM, "VM_START_REQUESTED")
	firstExtra.Reason = "first batch second VM"
	firstInput.Children = append(firstInput.Children, firstExtra)
	mustSyncPowerGuardBatchParentPayload(&firstInput)
	secondInput := powerGuardBatchInput("batch-lock-order-second", secondVM, "VM_STOP_REQUESTED")
	secondExtra := powerGuardBatchChild(firstVM, "VM_STOP_REQUESTED")
	secondExtra.Reason = "second batch first VM"
	secondInput.Children = append(secondInput.Children, secondExtra)
	mustSyncPowerGuardBatchParentPayload(&secondInput)

	type result struct {
		parentID string
		err      error
	}
	results := make(chan result, 2)
	var workers errgroup.Group
	startWriter := func(input BatchPowerSubmissionInput) <-chan struct{} {
		started := make(chan struct{})
		workers.Go(func() error {
			close(started)
			results <- result{
				parentID: input.ParentID,
				err:      store.writer.CreateBatchPowerAndMaybeEnqueue(ctx, input, nil),
			}
			return nil
		})
		return started
	}
	waitForBlocked := func(want int) (int, error) {
		blockedCalls := 0
		var queryErr error
		require.Eventually(t, func() bool {
			queryErr = store.pool.QueryRow(ctx, `
WITH RECURSIVE blocked(pid) AS (
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
    AND $1 = ANY(pg_blocking_pids(activity.pid))
  UNION
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  JOIN blocked AS upstream
    ON upstream.pid = ANY(pg_blocking_pids(activity.pid))
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
)
SELECT count(*) FROM blocked
`, blockerPID).Scan(&blockedCalls)
			return queryErr != nil || blockedCalls >= want
		}, 8*time.Second, 10*time.Millisecond, "power batch did not reach %d blocked database calls", want)
		return blockedCalls, queryErr
	}

	<-startWriter(firstInput)
	blockedCalls, blockedQueryErr := waitForBlocked(1)
	if blockedQueryErr != nil {
		t.Fatalf("query first batch blocked on sorted VM lock: %v", blockedQueryErr)
	}
	if blockedCalls != 1 {
		t.Fatalf("first batch blocked calls = %d, want 1 before starting reverse-order batch", blockedCalls)
	}
	<-startWriter(secondInput)
	blockedCalls, blockedQueryErr = waitForBlocked(2)
	if blockedQueryErr != nil {
		t.Fatalf("query both batches blocked on sorted VM lock: %v", blockedQueryErr)
	}
	if blockedCalls != 2 {
		t.Fatalf("batches blocked on shared first VM lock = %d, want 2 before release", blockedCalls)
	}

	_, unlockErr := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		closeCancel()
		lockHeld = false
	}
	if unlockErr != nil {
		t.Fatalf("release first sorted VM advisory lock: %v", unlockErr)
	}

	got := []result{<-results, <-results}
	if waitErr := workers.Wait(); waitErr != nil {
		t.Fatalf("wait for overlapping batch writers: %v", waitErr)
	}
	var (
		winner result
		loser  result
		active *ActivePowerEventError
	)
	for _, item := range got {
		if item.err == nil {
			if winner.parentID != "" {
				t.Fatalf("overlapping batches both committed: first=%+v second=%+v", winner, item)
			}
			winner = item
			continue
		}
		var activeErr *ActivePowerEventError
		if !errors.As(item.err, &activeErr) {
			t.Fatalf("overlapping batch %q error = %v, want active conflict rather than deadlock/lock-order error", item.parentID, item.err)
		}
		loser = item
		active = activeErr
	}
	if winner.parentID == "" || loser.parentID == "" || active == nil {
		t.Fatalf("overlapping batch results = %+v, want one commit and one active conflict", got)
	}
	if active.AggregateID != firstVM {
		t.Fatalf("sorted conflict VM = %q, want lexicographically first VM %q", active.AggregateID, firstVM)
	}

	assertPowerGuardSideEffects(t, store, powerGuardSideEffects{events: 3, tickets: 3, batches: 1, jobs: 2})
	assertPowerGuardRowsAbsent(t, store, "", loser.parentID, loser.parentID)
	var committedVMCount int
	if err := store.pool.QueryRow(ctx, `
SELECT count(DISTINCT aggregate_id)
FROM domain_events
WHERE aggregate_type = 'vm'
  AND aggregate_id IN ($1, $2)
`, firstVM, secondVM).Scan(&committedVMCount); err != nil {
		t.Fatalf("count committed overlapping batch VMs: %v", err)
	}
	if committedVMCount != 2 {
		t.Fatalf("committed overlapping batch VM count = %d, want 2", committedVMCount)
	}
}

func newPowerGuardTestStore(t *testing.T, prefix string) *powerGuardTestStore {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	pool := testutil.OpenPGXPool(t, prefix)
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(ctx, string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create River migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate River schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create River client: %v", err)
	}
	return &powerGuardTestStore{
		pool:   pool,
		writer: NewApprovalAtomicWriter(pool, riverClient),
	}
}

func newRepeatableReadPowerGuardStore(t *testing.T, sourcePool *pgxpool.Pool) *powerGuardTestStore {
	t.Helper()
	config := sourcePool.Config().Copy()
	config.MinConns = 0
	config.MaxConns = 2
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = make(map[string]string)
	}
	config.ConnConfig.RuntimeParams["default_transaction_isolation"] = "repeatable read"
	pool, createErr := pgxpool.NewWithConfig(t.Context(), config)
	if createErr != nil {
		t.Fatalf("create repeatable-read power guard pool: %v", createErr)
	}
	t.Cleanup(pool.Close)
	if pingErr := pool.Ping(t.Context()); pingErr != nil {
		t.Fatalf("ping repeatable-read power guard pool: %v", pingErr)
	}
	riverClient, riverErr := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if riverErr != nil {
		t.Fatalf("create repeatable-read River client: %v", riverErr)
	}
	return &powerGuardTestStore{
		pool:   pool,
		writer: NewApprovalAtomicWriter(pool, riverClient),
	}
}

func waitForPowerGuardBlockedCall(t *testing.T, pool *pgxpool.Pool, blockerPID int32) {
	t.Helper()
	var (
		blockedCalls int
		queryErr     error
	)
	require.Eventually(t, func() bool {
		queryErr = pool.QueryRow(t.Context(), `
SELECT count(*)
FROM pg_stat_activity AS activity
WHERE activity.datname = current_database()
  AND activity.state = 'active'
  AND $1 = ANY(pg_blocking_pids(activity.pid))
`, blockerPID).Scan(&blockedCalls)
		return queryErr != nil || blockedCalls > 0
	}, 8*time.Second, 10*time.Millisecond, "power guard waiter did not block before timeout")
	require.NoError(t, queryErr, "query calls blocked by power guard")
}

func seedFailedBatchPowerRetryParent(
	t *testing.T,
	pool *pgxpool.Pool,
	parentID string,
	operation string,
	vmIDs ...string,
) {
	t.Helper()
	operation = strings.ToLower(strings.TrimSpace(operation))
	items := make([]domain.BatchVMItemPayload, 0, len(vmIDs))
	for _, vmID := range vmIDs {
		items = append(items, domain.BatchVMItemPayload{
			VMID:      strings.TrimSpace(vmID),
			VMName:    strings.TrimSpace(vmID),
			ClusterID: "cluster-retry-test",
			Namespace: "namespace-retry-test",
			Operation: operation,
		})
	}
	parentPayload, err := json.Marshal(domain.BatchVMRequestPayload{
		Operation:   "POWER_" + strings.ToUpper(operation),
		SubmittedBy: "retry-actor",
		Items:       items,
	})
	if err != nil {
		t.Fatalf("marshal failed batch power parent payload: %v", err)
	}
	parentEventID := parentID + "-event"
	if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES ($1, NOW(), 'BATCH_POWER_REQUESTED', 'batch', $2, $3, 'FAILED', 'retry-actor')
`, parentEventID, parentID, parentPayload); err != nil {
		t.Fatalf("seed failed batch power parent event: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester, reason
)
VALUES ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor', 'failed power batch')
`, parentID, parentEventID); err != nil {
		t.Fatalf("seed failed batch power parent ticket: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by
)
VALUES ($1, NOW(), NOW(), 'BATCH_POWER', $2, 0, $2, 0, 'FAILED', 'retry-actor')
`, parentID, len(vmIDs)); err != nil {
		t.Fatalf("seed failed batch power projection: %v", err)
	}
}

func assertBatchPowerRetryParentReopened(
	t *testing.T,
	pool *pgxpool.Pool,
	parentID string,
	wantChildren, wantSuccess, wantFailed, wantPending int,
) {
	t.Helper()
	var (
		parentStatus      string
		parentEventStatus string
		projectionStatus  string
		childCount        int
		successCount      int
		failedCount       int
		pendingCount      int
	)
	if err := pool.QueryRow(t.Context(), `
SELECT parent.status, event.status, batch.status, batch.child_count,
       batch.success_count, batch.failed_count, batch.pending_count
FROM tickets AS parent
JOIN domain_events AS event ON event.id = parent.event_id
JOIN batch_tickets AS batch ON batch.id = parent.id
WHERE parent.id = $1
`, parentID).Scan(
		&parentStatus,
		&parentEventStatus,
		&projectionStatus,
		&childCount,
		&successCount,
		&failedCount,
		&pendingCount,
	); err != nil {
		t.Fatalf("query reopened batch power parent: %v", err)
	}
	if parentStatus != "EXECUTING" || parentEventStatus != "PROCESSING" || projectionStatus != "IN_PROGRESS" ||
		childCount != wantChildren || successCount != wantSuccess || failedCount != wantFailed || pendingCount != wantPending {
		t.Fatalf(
			"reopened parent = ticket:%q event:%q projection:%q counts:%d/%d/%d/%d, want EXECUTING/PROCESSING/IN_PROGRESS %d/%d/%d/%d",
			parentStatus,
			parentEventStatus,
			projectionStatus,
			childCount,
			successCount,
			failedCount,
			pendingCount,
			wantChildren,
			wantSuccess,
			wantFailed,
			wantPending,
		)
	}
}

func powerGuardApprovalInput(vmID, eventID, ticketID, eventType string) PowerApprovalRequestInput {
	_, action, _, ok := batchPowerOperationIdentity(eventType)
	if !ok {
		action = "start"
	}
	payload, err := json.Marshal(domain.VMPowerPayload{
		VMID:         vmID,
		VMName:       vmID,
		ClusterID:    "cluster-power-guard",
		Namespace:    "namespace-power-guard",
		Operation:    action,
		Actor:        "power-guard-actor",
		DispatchMode: domain.VMPowerDispatchTicket,
	})
	if err != nil {
		panic(err)
	}
	return PowerApprovalRequestInput{
		EventID:     eventID,
		TicketID:    ticketID,
		EventType:   eventType,
		AggregateID: vmID,
		Payload:     payload,
		CreatedBy:   "power-guard-actor",
		Reason:      "power guard integration test",
	}
}

func powerGuardDirectInput(eventID, vmID, eventType, actor string) PowerEventInput {
	_, action, _, ok := batchPowerOperationIdentity(eventType)
	if !ok {
		panic("invalid direct power guard event type: " + eventType)
	}
	payload, err := json.Marshal(domain.VMPowerPayload{
		VMID:         strings.TrimSpace(vmID),
		VMName:       strings.TrimSpace(vmID),
		ClusterID:    "cluster-power-guard",
		Namespace:    "namespace-power-guard",
		Operation:    action,
		Actor:        strings.TrimSpace(actor),
		DispatchMode: domain.VMPowerDispatchDirect,
	})
	if err != nil {
		panic(err)
	}
	return PowerEventInput{
		EventID:       strings.TrimSpace(eventID),
		EventType:     strings.TrimSpace(eventType),
		AggregateType: "vm",
		AggregateID:   strings.TrimSpace(vmID),
		Payload:       payload,
		CreatedBy:     strings.TrimSpace(actor),
	}
}

func powerGuardBatchInput(parentID, vmID, eventType string) BatchPowerSubmissionInput {
	requestID := "request-" + parentID
	input := BatchPowerSubmissionInput{
		ParentID:  parentID,
		Actor:     "power-guard-actor",
		Operation: eventType,
		RequestID: requestID,
		Reason:    "power guard batch integration test",
		Children: []BatchPowerChildInput{
			powerGuardBatchChild(vmID, eventType),
		},
	}
	mustSyncPowerGuardBatchParentPayload(&input)
	return input
}

func powerGuardBatchChild(vmID, eventType string) BatchPowerChildInput {
	_, action, _, ok := batchPowerOperationIdentity(eventType)
	if !ok {
		panic("invalid power guard event type: " + eventType)
	}
	payload, err := json.Marshal(domain.VMPowerPayload{
		VMID:         strings.TrimSpace(vmID),
		VMName:       strings.TrimSpace(vmID),
		ClusterID:    "cluster-power-guard",
		Namespace:    "namespace-power-guard",
		Operation:    action,
		Actor:        "power-guard-actor",
		DispatchMode: domain.VMPowerDispatchTicket,
	})
	if err != nil {
		panic(err)
	}
	return BatchPowerChildInput{
		EventType:   eventType,
		AggregateID: vmID,
		Payload:     payload,
		Reason:      "power guard child integration test",
	}
}

func mustSyncPowerGuardBatchParentPayload(input *BatchPowerSubmissionInput) {
	items := make([]domain.BatchVMItemPayload, 0, len(input.Children))
	for _, child := range input.Children {
		var payload domain.VMPowerPayload
		if err := json.Unmarshal(child.Payload, &payload); err != nil {
			panic(err)
		}
		items = append(items, batchPowerPayloadItem(payload))
	}
	payload, err := json.Marshal(domain.BatchVMRequestPayload{
		Operation:   input.Operation,
		RequestID:   input.RequestID,
		Reason:      input.Reason,
		SubmittedBy: input.Actor,
		Items:       items,
	})
	if err != nil {
		panic(err)
	}
	input.ParentPayload = payload
}

func requireActivePowerEventError(t *testing.T, err error) *ActivePowerEventError {
	t.Helper()
	if err == nil {
		t.Fatal("power writer error = nil, want *ActivePowerEventError")
	}
	var active *ActivePowerEventError
	if !errors.As(err, &active) {
		t.Fatalf("power writer error = %v, want *ActivePowerEventError", err)
	}
	return active
}

func assertPowerGuardSideEffects(t *testing.T, store *powerGuardTestStore, want powerGuardSideEffects) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	queries := []struct {
		label string
		query string
		want  int
	}{
		{label: "domain events", query: `SELECT count(*) FROM domain_events`, want: want.events},
		{label: "tickets", query: `SELECT count(*) FROM tickets`, want: want.tickets},
		{label: "batch projections", query: `SELECT count(*) FROM batch_tickets`, want: want.batches},
		{label: "River jobs", query: `SELECT count(*) FROM river_job`, want: want.jobs},
	}
	for _, check := range queries {
		var got int
		if err := store.pool.QueryRow(ctx, check.query).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", check.label, err)
		}
		if got != check.want {
			t.Fatalf("%s count = %d, want %d", check.label, got, check.want)
		}
	}
}

func assertPowerGuardRowsAbsent(t *testing.T, store *powerGuardTestStore, eventID, ticketID, batchID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	checks := []struct {
		label string
		table string
		id    string
	}{
		{label: "event", table: "domain_events", id: eventID},
		{label: "ticket", table: "tickets", id: ticketID},
		{label: "batch projection", table: "batch_tickets", id: batchID},
	}
	for _, check := range checks {
		if check.id == "" {
			continue
		}
		var exists bool
		query := `SELECT EXISTS (SELECT 1 FROM ` + check.table + ` WHERE id = $1)`
		if err := store.pool.QueryRow(ctx, query, check.id).Scan(&exists); err != nil {
			t.Fatalf("check absent %s %q: %v", check.label, check.id, err)
		}
		if exists {
			t.Fatalf("%s %q exists, want transaction without that side effect", check.label, check.id)
		}
	}
}

func assertPowerGuardJobEvent(t *testing.T, store *powerGuardTestStore, wantEventID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var (
		kind    string
		eventID string
	)
	if err := store.pool.QueryRow(ctx, `SELECT kind, args->>'event_id' FROM river_job`).Scan(&kind, &eventID); err != nil {
		t.Fatalf("query power River job: %v", err)
	}
	if kind != "vm_power" || eventID != wantEventID {
		t.Fatalf("River job = kind:%q event:%q, want kind:vm_power event:%q", kind, eventID, wantEventID)
	}
}
