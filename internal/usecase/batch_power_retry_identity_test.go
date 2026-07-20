package usecase

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/jobs"
)

type batchPowerRetryIdentityFixture struct {
	parentID    string
	parentEvent string
	childID     string
	childEvent  string
	vmID        string
}

type batchPowerRetryIdentityState struct {
	childStatus       string
	childRejectReason string
	childAttempts     int
	childLastAttempt  time.Time
	childUpdatedAt    time.Time
	childEventStatus  string
	childEventType    string
	childAggregateID  string
	childPayload      []byte
	childRequester    string
	childEventActor   string
	parentStatus      string
	parentUpdatedAt   time.Time
	parentEventStatus string
	parentEventType   string
	parentAggregateID string
	parentEventActor  string
	projectionType    string
	projectionActor   string
	projectionStatus  string
	childCount        int
	successCount      int
	failedCount       int
	pendingCount      int
	projectionUpdated time.Time
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_IdentityMismatchRollsBackAllDurableState(t *testing.T) {
	tests := []struct {
		name                     string
		mismatchedChildPayload   bool
		mismatchedParentIdentity bool
		mutation                 string
		mutationTarget           string
		wantParentConflict       bool
		wantConflictTicket       string
	}{
		{
			name:                   "child payload VM identity",
			mismatchedChildPayload: true,
		},
		{
			name:                     "parent event aggregate identity",
			mismatchedParentIdentity: true,
			wantParentConflict:       true,
		},
		{
			name:               "parent event actor",
			mutation:           `UPDATE domain_events SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name:               "projection actor",
			mutation:           `UPDATE batch_tickets SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection type",
			mutation:           `UPDATE batch_tickets SET batch_type = 'BATCH_DELETE' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection terminal status",
			mutation:           `UPDATE batch_tickets SET status = 'COMPLETED' WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name:               "projection counters",
			mutation:           `UPDATE batch_tickets SET failed_count = 0, pending_count = 1 WHERE id = $1`,
			mutationTarget:     "parent",
			wantParentConflict: true,
		},
		{
			name: "parent payload submitter",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{submitted_by}', to_jsonb('foreign-actor'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name: "parent payload power action",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{operation}', to_jsonb('POWER_STOP'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "parent_event",
			wantParentConflict: true,
		},
		{
			name:               "selected child requester",
			mutation:           `UPDATE tickets SET requester = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "child",
			wantConflictTicket: "batch-power-retry-identity-child",
		},
		{
			name:               "selected child event actor",
			mutation:           `UPDATE domain_events SET created_by = 'foreign-actor' WHERE id = $1`,
			mutationTarget:     "child_event",
			wantConflictTicket: "batch-power-retry-identity-child",
		},
		{
			name: "selected child payload actor",
			mutation: `UPDATE domain_events
SET payload = convert_to(
  jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{actor}', to_jsonb('foreign-actor'::text))::text,
  'UTF8'
)
WHERE id = $1`,
			mutationTarget:     "child_event",
			wantConflictTicket: "batch-power-retry-identity-child",
		},
		{
			name: "unselected sibling actor",
			mutation: `WITH updated_parent_event AS (
	UPDATE domain_events
	SET payload = convert_to(
		jsonb_set(
			convert_from(payload, 'UTF8')::jsonb,
			'{items}',
			(convert_from(payload, 'UTF8')::jsonb->'items') ||
				jsonb_build_array(jsonb_build_object(
					'vm_id', 'batch-power-retry-identity-sibling',
					'operation', 'start'
				))
		)::text,
		'UTF8'
	)
	WHERE aggregate_id = $1
	RETURNING id
), updated_projection AS (
	UPDATE batch_tickets
	SET child_count = 2, success_count = 1
	WHERE id = $1
	RETURNING id
), inserted_event AS (
  INSERT INTO domain_events (
    id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
  ) VALUES (
    'batch-power-retry-identity-sibling-event', NOW(), 'VM_START_REQUESTED', 'vm',
    'batch-power-retry-identity-sibling',
    convert_to('{"vm_id":"batch-power-retry-identity-sibling","operation":"start","actor":"foreign-actor"}', 'UTF8'),
    'COMPLETED', 'foreign-actor'
  )
  RETURNING id
)
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  reason, parent_ticket_id, attempt_count
)
SELECT
  'batch-power-retry-identity-sibling', NOW(), NOW(), id, 'POWER', 'SUCCESS',
  'foreign-actor', 'foreign sibling', $1, 1
FROM inserted_event`,
			mutationTarget:     "parent",
			wantConflictTicket: "batch-power-retry-identity-sibling",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_power_retry_identity")
			fixture := seedBatchPowerRetryIdentityFixture(
				t,
				store,
				tt.mismatchedChildPayload,
				tt.mismatchedParentIdentity,
			)
			if strings.TrimSpace(tt.mutation) != "" {
				mutationID := fixture.parentID
				switch tt.mutationTarget {
				case "parent_event":
					mutationID = fixture.parentEvent
				case "child":
					mutationID = fixture.childID
				case "child_event":
					mutationID = fixture.childEvent
				}
				if _, err := store.pool.Exec(
					t.Context(),
					tt.mutation,
					mutationID,
				); err != nil {
					t.Fatalf("mutate power retry identity: %v", err)
				}
			}
			before := loadBatchPowerRetryIdentityState(t, store, fixture)
			beforeJobs := listBatchPowerRetryJobs(t, store)

			err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), BatchPowerRetryInput{
				ParentID: fixture.parentID,
				Children: []BatchPowerRetryChildInput{{
					TicketID: fixture.childID,
					EventID:  fixture.childEvent,
				}},
			})
			if tt.wantParentConflict {
				var conflict *BatchRetryParentNotEligibleError
				if !errors.As(err, &conflict) {
					t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *BatchRetryParentNotEligibleError", err)
				}
				if conflict.ParentTicketID != fixture.parentID {
					t.Fatalf("parent retry conflict = %+v, want parent %q", conflict, fixture.parentID)
				}
			} else {
				var conflict *PowerRetryNotEligibleError
				if !errors.As(err, &conflict) {
					t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *PowerRetryNotEligibleError", err)
				}
				wantTicketID := tt.wantConflictTicket
				if wantTicketID == "" {
					wantTicketID = fixture.childID
				}
				if conflict.TicketID != wantTicketID {
					t.Fatalf("child retry conflict = %+v, want ticket %q", conflict, wantTicketID)
				}
			}

			after := loadBatchPowerRetryIdentityState(t, store, fixture)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("identity-mismatch retry changed application state:\nbefore=%+v\nafter=%+v", before, after)
			}
			afterJobs := listBatchPowerRetryJobs(t, store)
			if len(afterJobs) != len(beforeJobs) || len(afterJobs) != 0 {
				t.Fatalf("identity-mismatch retry jobs = %d, want unchanged %d", len(afterJobs), len(beforeJobs))
			}
		})
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_RejectsNegativeAttemptWithoutWrites(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_power_retry_negative_attempt")
	dropTicketAttemptCountConstraintForCorruptionTest(t, store.pool)
	fixture := seedBatchPowerRetryIdentityFixture(t, store, false, false)
	if _, err := store.pool.Exec(
		t.Context(),
		`UPDATE tickets SET attempt_count = -1 WHERE id = $1`,
		fixture.childID,
	); err != nil {
		t.Fatalf("seed negative power retry attempt: %v", err)
	}
	before := loadBatchPowerRetryIdentityState(t, store, fixture)

	err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), BatchPowerRetryInput{
		ParentID: fixture.parentID,
		Children: []BatchPowerRetryChildInput{{
			TicketID: fixture.childID,
			EventID:  fixture.childEvent,
		}},
	})
	var notEligible *PowerRetryNotEligibleError
	if !errors.As(err, &notEligible) || notEligible.TicketID != fixture.childID {
		t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want negative child not eligible", err)
	}
	after := loadBatchPowerRetryIdentityState(t, store, fixture)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("negative power retry changed durable graph\nbefore: %+v\nafter:  %+v", before, after)
	}
	if queuedJobs := listBatchPowerRetryJobs(t, store); len(queuedJobs) != 0 {
		t.Fatalf("power retry jobs after negative attempt = %d, want 0", len(queuedJobs))
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_PowerSiblingGraphMismatchRollsBack(t *testing.T) {
	tests := []struct {
		name     string
		mutation string
	}{
		{
			name: "sibling action differs",
			mutation: `UPDATE domain_events
SET event_type = 'VM_STOP_REQUESTED',
    payload = convert_to(
      jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{operation}', to_jsonb('stop'::text))::text,
      'UTF8'
    )
WHERE id = 'batch-power-graph-sibling-event'`,
		},
		{
			name: "siblings target the same VM",
			mutation: `UPDATE domain_events
SET aggregate_id = 'batch-power-graph-selected',
    payload = convert_to(
      jsonb_set(convert_from(payload, 'UTF8')::jsonb, '{vm_id}', to_jsonb('batch-power-graph-selected'::text))::text,
      'UTF8'
    )
WHERE id = 'batch-power-graph-sibling-event'`,
		},
		{
			name: "unselected pending sibling",
			mutation: `WITH updated_ticket AS (
  UPDATE tickets SET status = 'PENDING'
  WHERE id = 'batch-power-graph-sibling'
  RETURNING id
), updated_event AS (
  UPDATE domain_events SET status = 'PENDING'
  WHERE id = 'batch-power-graph-sibling-event'
  RETURNING id
), updated_projection AS (
  UPDATE batch_tickets
  SET success_count = 0, failed_count = 1, pending_count = 1
  WHERE id = 'batch-power-graph-parent'
  RETURNING id
)
SELECT 1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newApprovalAtomicBehaviorStore(t, "batch_power_retry_sibling_graph")
			const parentID = "batch-power-graph-parent"
			seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
				parentID:        parentID,
				parentStatus:    "FAILED",
				parentEvent:     "FAILED",
				parentEventType: "BATCH_POWER_REQUESTED",
				operationType:   "POWER",
				batchType:       "BATCH_POWER",
				childEventType:  "VM_START_REQUESTED",
				batchStatus:     "FAILED",
				children: []batchApprovalDispatchChild{
					{
						ticketID:    "batch-power-graph-selected",
						eventID:     "batch-power-graph-selected-event",
						ticketState: "FAILED",
						eventState:  "FAILED",
					},
					{
						ticketID:     "batch-power-graph-sibling",
						eventID:      "batch-power-graph-sibling-event",
						ticketState:  "SUCCESS",
						eventState:   "COMPLETED",
						attemptCount: 1,
					},
				},
			})
			if _, err := store.pool.Exec(t.Context(), tt.mutation); err != nil {
				t.Fatalf("mutate power sibling graph: %v", err)
			}
			before := loadBatchApprovalGraphSnapshot(t, store.pool, parentID)
			err := store.writer.RetryBatchPowerAndEnqueue(t.Context(), BatchPowerRetryInput{
				ParentID: parentID,
				Children: []BatchPowerRetryChildInput{{
					TicketID: "batch-power-graph-selected",
					EventID:  "batch-power-graph-selected-event",
				}},
			})
			var conflict *PowerRetryNotEligibleError
			if !errors.As(err, &conflict) {
				t.Fatalf("RetryBatchPowerAndEnqueue() error = %v, want *PowerRetryNotEligibleError", err)
			}
			if conflict.TicketID != "batch-power-graph-sibling" {
				t.Fatalf("power sibling graph conflict = %+v, want sibling ticket", conflict)
			}
			if after := loadBatchApprovalGraphSnapshot(t, store.pool, parentID); after != before {
				t.Fatalf("power sibling graph mismatch changed durable state:\nbefore=%s\nafter=%s", before, after)
			}
			if queuedJobs := listBatchPowerRetryJobs(t, store); len(queuedJobs) != 0 {
				t.Fatalf("power sibling graph mismatch jobs = %d, want 0", len(queuedJobs))
			}
		})
	}
}

func TestApprovalAtomicWriterRetryBatchPowerAndEnqueue_LocksAllTicketsBeforeAnyChildEvent(t *testing.T) {
	store := newApprovalAtomicBehaviorStore(t, "batch_power_retry_lock_order")
	const (
		parentID  = "batch-power-retry-lock-parent"
		childA    = "batch-power-retry-lock-child-a"
		eventA    = "batch-power-retry-lock-event-a"
		childB    = "batch-power-retry-lock-child-b"
		eventB    = "batch-power-retry-lock-event-b"
		waitLimit = 5 * time.Second
	)
	seedBatchApprovalDispatchRows(t, store.pool, batchApprovalDispatchFixture{
		parentID:        parentID,
		parentStatus:    "FAILED",
		parentEvent:     "FAILED",
		parentEventType: "BATCH_POWER_REQUESTED",
		operationType:   "POWER",
		batchType:       "BATCH_POWER",
		childEventType:  "VM_START_REQUESTED",
		batchStatus:     "FAILED",
		children: []batchApprovalDispatchChild{
			{ticketID: childA, eventID: eventA, ticketState: "FAILED", eventState: "FAILED", attemptCount: 1},
			{ticketID: childB, eventID: eventB, ticketState: "FAILED", eventState: "FAILED", attemptCount: 1},
		},
	})

	ctx := t.Context()
	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin child event blocker: %v", err)
	}
	blockerOpen := true
	defer func() {
		if blockerOpen {
			_ = blocker.Rollback(ctx)
		}
	}()
	if err := blocker.QueryRow(ctx, `SELECT id FROM domain_events WHERE id = $1 FOR UPDATE`, eventA).Scan(new(string)); err != nil {
		t.Fatalf("lock first child event: %v", err)
	}
	var blockerPID int32
	if err := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("load event blocker pid: %v", err)
	}

	retryDone := make(chan error, 1)
	var workers errgroup.Group
	workers.Go(func() error {
		retryDone <- store.writer.RetryBatchPowerAndEnqueue(ctx, BatchPowerRetryInput{
			ParentID: parentID,
			Children: []BatchPowerRetryChildInput{{TicketID: childA, EventID: eventA}},
		})
		return nil
	})

	blocked := false
	var observeErr error
	for deadline := time.Now().Add(waitLimit); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		var count int
		observeErr = store.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_stat_activity AS activity
WHERE activity.datname = current_database()
  AND activity.state = 'active'
  AND POSITION('batch_approval_lock_child_events' IN activity.query) > 0
  AND $1 = ANY(pg_blocking_pids(activity.pid))
`, blockerPID).Scan(&count)
		if observeErr != nil || count == 1 {
			blocked = count == 1
			break
		}
	}

	var probeErr error
	if observeErr == nil && blocked {
		probe, beginErr := store.pool.Begin(ctx)
		if beginErr != nil {
			probeErr = beginErr
		} else {
			if _, setErr := probe.Exec(ctx, `SET LOCAL lock_timeout = '200ms'`); setErr != nil {
				probeErr = setErr
			} else {
				probeErr = probe.QueryRow(ctx, `SELECT id FROM tickets WHERE id = $1 FOR UPDATE`, childB).Scan(new(string))
			}
			_ = probe.Rollback(ctx)
		}
	}
	if err := blocker.Rollback(ctx); err != nil {
		t.Fatalf("release child event blocker: %v", err)
	}
	blockerOpen = false

	select {
	case err := <-retryDone:
		if err != nil {
			t.Fatalf("RetryBatchPowerAndEnqueue() after lock release error = %v", err)
		}
	case <-time.After(waitLimit):
		t.Fatal("RetryBatchPowerAndEnqueue() did not finish after lock release")
	}
	if err := workers.Wait(); err != nil {
		t.Fatalf("wait for power retry worker: %v", err)
	}
	if observeErr != nil {
		t.Fatalf("observe blocked child-event query: %v", observeErr)
	}
	if !blocked {
		t.Fatal("power retry did not block while acquiring the ordered child-event set")
	}
	var pgErr *pgconn.PgError
	if !errors.As(probeErr, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("probe second child ticket lock error = %v, want SQLSTATE 55P03", probeErr)
	}

	assertBatchApprovalChildState(t, store.pool, childA, "EXECUTING", "PENDING", 2)
	assertBatchApprovalChildState(t, store.pool, childB, "FAILED", "FAILED", 1)
	if queuedJobs := listBatchPowerRetryJobs(t, store); len(queuedJobs) != 1 {
		t.Fatalf("power retry jobs = %d, want 1", len(queuedJobs))
	}
}

func seedBatchPowerRetryIdentityFixture(
	t *testing.T,
	store approvalAtomicBehaviorStore,
	mismatchedChildPayload bool,
	mismatchedParentIdentity bool,
) batchPowerRetryIdentityFixture {
	t.Helper()
	fixture := batchPowerRetryIdentityFixture{
		parentID:    "batch-power-retry-identity-parent",
		parentEvent: "batch-power-retry-identity-parent-event",
		childID:     "batch-power-retry-identity-child",
		childEvent:  "batch-power-retry-identity-child-event",
		vmID:        "batch-power-retry-identity-vm",
	}
	payloadVMID := fixture.vmID
	if mismatchedChildPayload {
		payloadVMID = fixture.vmID + "-different"
	}
	payload, err := (domain.VMPowerPayload{
		VMID:         payloadVMID,
		VMName:       fixture.vmID,
		ClusterID:    "cluster-retry-identity",
		Namespace:    "team-retry-identity",
		Operation:    "start",
		Actor:        "retry-actor",
		DispatchMode: domain.VMPowerDispatchTicket,
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal power retry identity payload: %v", err)
	}
	parentAggregateID := fixture.parentID
	if mismatchedParentIdentity {
		parentAggregateID = fixture.parentID + "-different"
	}
	parentPayload, err := json.Marshal(domain.BatchVMRequestPayload{
		Operation:   "POWER_START",
		SubmittedBy: "retry-actor",
		Items: []domain.BatchVMItemPayload{{
			VMID:      fixture.vmID,
			VMName:    fixture.vmID,
			ClusterID: "cluster-retry-identity",
			Namespace: "team-retry-identity",
			Operation: "start",
		}},
	})
	if err != nil {
		t.Fatalf("marshal power retry identity parent payload: %v", err)
	}

	if _, err := store.pool.Exec(t.Context(), `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
)
VALUES
  ($1, NOW(), 'BATCH_POWER_REQUESTED', 'batch', $2, $3, 'FAILED', 'retry-actor'),
  ($4, NOW(), 'VM_START_REQUESTED', 'vm', $5, $6, 'FAILED', 'retry-actor')
`, fixture.parentEvent, parentAggregateID, parentPayload, fixture.childEvent, fixture.vmID, payload); err != nil {
		t.Fatalf("seed power retry identity events: %v", err)
	}
	if _, err := store.pool.Exec(t.Context(), `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  approver, reason, reject_reason, parent_ticket_id, attempt_count, last_attempt_at
)
VALUES
  ($1, NOW(), NOW(), $2, 'POWER', 'FAILED', 'retry-actor', 'approver-1', 'batch retry', 'parent failure', NULL, 0, NULL),
  ($3, NOW(), NOW(), $4, 'POWER', 'FAILED', 'retry-actor', NULL, 'child retry', 'original child failure', $1, 1, NOW() - INTERVAL '5 minutes')
`, fixture.parentID, fixture.parentEvent, fixture.childID, fixture.childEvent); err != nil {
		t.Fatalf("seed power retry identity tickets: %v", err)
	}
	if _, err := store.pool.Exec(t.Context(), `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by, reason
)
VALUES ($1, NOW(), NOW(), 'BATCH_POWER', 1, 0, 1, 0, 'FAILED', 'retry-actor', 'batch retry')
`, fixture.parentID); err != nil {
		t.Fatalf("seed power retry identity projection: %v", err)
	}
	return fixture
}

func loadBatchPowerRetryIdentityState(
	t *testing.T,
	store approvalAtomicBehaviorStore,
	fixture batchPowerRetryIdentityFixture,
) batchPowerRetryIdentityState {
	t.Helper()
	var state batchPowerRetryIdentityState
	if err := store.pool.QueryRow(t.Context(), `
SELECT
  child.status,
  COALESCE(child.reject_reason, ''),
  child.attempt_count,
  child.last_attempt_at,
  child.updated_at,
  child_event.status,
  child_event.event_type,
  child_event.aggregate_id,
  child_event.payload,
  child.requester,
  child_event.created_by,
  parent.status,
  parent.updated_at,
  parent_event.status,
  parent_event.event_type,
  parent_event.aggregate_id,
  parent_event.created_by,
  projection.batch_type,
  projection.created_by,
  projection.status,
  projection.child_count,
  projection.success_count,
  projection.failed_count,
  projection.pending_count,
  projection.updated_at
FROM tickets AS child
JOIN domain_events AS child_event ON child_event.id = child.event_id
JOIN tickets AS parent ON parent.id = child.parent_ticket_id
JOIN domain_events AS parent_event ON parent_event.id = parent.event_id
JOIN batch_tickets AS projection ON projection.id = parent.id
WHERE child.id = $1
`, fixture.childID).Scan(
		&state.childStatus,
		&state.childRejectReason,
		&state.childAttempts,
		&state.childLastAttempt,
		&state.childUpdatedAt,
		&state.childEventStatus,
		&state.childEventType,
		&state.childAggregateID,
		&state.childPayload,
		&state.childRequester,
		&state.childEventActor,
		&state.parentStatus,
		&state.parentUpdatedAt,
		&state.parentEventStatus,
		&state.parentEventType,
		&state.parentAggregateID,
		&state.parentEventActor,
		&state.projectionType,
		&state.projectionActor,
		&state.projectionStatus,
		&state.childCount,
		&state.successCount,
		&state.failedCount,
		&state.pendingCount,
		&state.projectionUpdated,
	); err != nil {
		t.Fatalf("load power retry identity state: %v", err)
	}
	return state
}

func listBatchPowerRetryJobs(t *testing.T, store approvalAtomicBehaviorStore) []*rivertype.JobRow {
	t.Helper()
	result, err := store.riverClient.JobList(t.Context(), river.NewJobListParams().Kinds(jobs.VMPowerArgs{}.Kind()))
	if err != nil {
		t.Fatalf("list power retry jobs: %v", err)
	}
	return result.Jobs
}
