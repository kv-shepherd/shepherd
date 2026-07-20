package sqlc

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestQueries_ReopenBatchApprovalDispatch(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reopen_batch_approval_dispatch")

	t.Run("eligible failed dispatch is reopened", func(t *testing.T) {
		parentID, eventID := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "reopen-eligible",
			operation:         "CREATE",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "PARTIAL_SUCCESS",
			requester:         "requester-eligible",
			eventCreatedBy:    "requester-eligible",
			batchCreatedBy:    "requester-eligible",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, parentID)

		execution := []byte(`{"dispatch_id":"dispatch-retry-1","dry_run":false}`)
		rows, err := q.ReopenBatchApprovalDispatch(ctx, ReopenBatchApprovalDispatchParams{
			Approver:             pgtype.Text{String: "approver-retry", Valid: true},
			SelectedClusterID:    pgtype.Text{String: "cluster-retry", Valid: true},
			SelectedStorageClass: pgtype.Text{String: "storage-retry", Valid: true},
			ExecutionOptions:     execution,
			ID:                   parentID,
			EventID:              eventID,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)

		after := readBatchApprovalRetryTicket(ctx, t, pool, parentID)
		require.Equal(t, "EXECUTING", after.status)
		require.Equal(t, pgtype.Text{String: "approver-retry", Valid: true}, after.approver)
		require.Equal(t, pgtype.Text{String: "cluster-retry", Valid: true}, after.selectedClusterID)
		require.Equal(t, pgtype.Text{String: "storage-retry", Valid: true}, after.selectedStorageClass)
		require.JSONEq(t, `{"preserved":"parent","batch_approval_execution":{"dispatch_id":"dispatch-retry-1","dry_run":false}}`, string(after.modifiedSpec))
		require.False(t, after.rejectReason.Valid)
		require.Equal(t, before.attemptCount, after.attemptCount, "reopening the parent must not consume a child attempt")
		require.Equal(t, before.lastAttemptAt, after.lastAttemptAt)
		require.True(t, after.updatedAt.After(before.updatedAt))
	})

	t.Run("stale parent event is not reopened", func(t *testing.T) {
		parentID, eventID := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "reopen-stale",
			operation:         "CREATE",
			parentStatus:      "FAILED",
			parentEventStatus: "COMPLETED",
			batchStatus:       "FAILED",
			requester:         "requester-stale",
			eventCreatedBy:    "requester-stale",
			batchCreatedBy:    "requester-stale",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, parentID)

		rows, err := q.ReopenBatchApprovalDispatch(ctx, ReopenBatchApprovalDispatchParams{
			Approver:             pgtype.Text{String: "unexpected-approver", Valid: true},
			SelectedClusterID:    pgtype.Text{String: "unexpected-cluster", Valid: true},
			SelectedStorageClass: pgtype.Text{String: "unexpected-storage", Valid: true},
			ExecutionOptions:     []byte(`{"unexpected":true}`),
			ID:                   parentID,
			EventID:              eventID,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, before, readBatchApprovalRetryTicket(ctx, t, pool, parentID))
	})

	t.Run("foreign event identity is not reopened", func(t *testing.T) {
		parentID, eventID := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "reopen-foreign",
			operation:         "CREATE",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "FAILED",
			requester:         "requester-owner",
			eventCreatedBy:    "requester-foreign",
			batchCreatedBy:    "requester-owner",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, parentID)

		rows, err := q.ReopenBatchApprovalDispatch(ctx, ReopenBatchApprovalDispatchParams{
			Approver:             pgtype.Text{String: "unexpected-approver", Valid: true},
			SelectedClusterID:    pgtype.Text{String: "unexpected-cluster", Valid: true},
			SelectedStorageClass: pgtype.Text{String: "unexpected-storage", Valid: true},
			ExecutionOptions:     []byte(`{"unexpected":true}`),
			ID:                   parentID,
			EventID:              eventID,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, before, readBatchApprovalRetryTicket(ctx, t, pool, parentID))
	})
}

func TestQueries_ResetBatchApprovalRetryChild(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reset_batch_approval_retry_child")

	t.Run("eligible failed child starts the next logical attempt", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "child-eligible",
			operation:         "MODIFY",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "PARTIAL_SUCCESS",
			requester:         "requester-eligible",
			eventCreatedBy:    "requester-eligible",
			batchCreatedBy:    "requester-eligible",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "child-eligible",
			operation:      "MODIFY",
			status:         "FAILED",
			eventStatus:    "CANCELLED",
			parentID:       parentID,
			requester:      "requester-eligible",
			eventCreatedBy: "requester-eligible",
			attemptCount:   1,
			rejectReason:   "retryable provider failure",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryChild(ctx, ResetBatchApprovalRetryChildParams{
			ID:             childID,
			EventID:        eventID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
			MaxAttempts:    3,
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)

		after := readBatchApprovalRetryTicket(ctx, t, pool, childID)
		require.Equal(t, "PENDING", after.status)
		require.False(t, after.rejectReason.Valid)
		require.EqualValues(t, 2, after.attemptCount)
		require.True(t, after.lastAttemptAt.Valid)
		require.True(t, after.lastAttemptAt.Time.After(before.lastAttemptAt.Time))
		require.True(t, after.updatedAt.After(before.updatedAt))
		require.Equal(t, before.approver, after.approver)
		require.Equal(t, before.selectedClusterID, after.selectedClusterID)
		require.Equal(t, before.selectedStorageClass, after.selectedStorageClass)
		require.JSONEq(t, string(before.modifiedSpec), string(after.modifiedSpec), "retry reset must preserve execution metadata")
	})

	t.Run("exhausted child is not reset", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "child-stale",
			operation:         "MODIFY",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "FAILED",
			requester:         "requester-stale",
			eventCreatedBy:    "requester-stale",
			batchCreatedBy:    "requester-stale",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "child-stale",
			operation:      "MODIFY",
			status:         "FAILED",
			eventStatus:    "FAILED",
			parentID:       parentID,
			requester:      "requester-stale",
			eventCreatedBy: "requester-stale",
			attemptCount:   3,
			rejectReason:   "terminal provider failure",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryChild(ctx, ResetBatchApprovalRetryChildParams{
			ID:             childID,
			EventID:        eventID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
			MaxAttempts:    3,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, before, readBatchApprovalRetryTicket(ctx, t, pool, childID))
	})

	t.Run("child owned by another requester is not reset", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "child-foreign",
			operation:         "MODIFY",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "FAILED",
			requester:         "requester-owner",
			eventCreatedBy:    "requester-owner",
			batchCreatedBy:    "requester-owner",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "child-foreign",
			operation:      "MODIFY",
			status:         "FAILED",
			eventStatus:    "FAILED",
			parentID:       parentID,
			requester:      "requester-foreign",
			eventCreatedBy: "requester-foreign",
			attemptCount:   1,
			rejectReason:   "must remain",
		})
		before := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryChild(ctx, ResetBatchApprovalRetryChildParams{
			ID:             childID,
			EventID:        eventID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
			MaxAttempts:    3,
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, before, readBatchApprovalRetryTicket(ctx, t, pool, childID))
	})
}

func TestQueries_ResetBatchApprovalRetryEvent(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reset_batch_approval_retry_event")

	t.Run("eligible cancelled event returns to pending", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "event-eligible",
			operation:         "DELETE",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "PARTIAL_SUCCESS",
			requester:         "requester-eligible",
			eventCreatedBy:    "requester-eligible",
			batchCreatedBy:    "requester-eligible",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "event-eligible",
			operation:      "DELETE",
			status:         "PENDING",
			eventStatus:    "CANCELLED",
			parentID:       parentID,
			requester:      "requester-eligible",
			eventCreatedBy: "requester-eligible",
			attemptCount:   2,
			rejectReason:   "ticket metadata must be preserved",
		})
		childBefore := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryEvent(ctx, ResetBatchApprovalRetryEventParams{
			EventID:        eventID,
			TicketID:       childID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, rows)
		require.Equal(t, "PENDING", readBatchApprovalRetryEventStatus(ctx, t, pool, eventID))
		require.Equal(t, childBefore, readBatchApprovalRetryTicket(ctx, t, pool, childID), "event reset must not change attempt, rejection, or execution fields")
	})

	t.Run("event for stale failed child is not reset", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "event-stale",
			operation:         "DELETE",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "FAILED",
			requester:         "requester-stale",
			eventCreatedBy:    "requester-stale",
			batchCreatedBy:    "requester-stale",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "event-stale",
			operation:      "DELETE",
			status:         "FAILED",
			eventStatus:    "CANCELLED",
			parentID:       parentID,
			requester:      "requester-stale",
			eventCreatedBy: "requester-stale",
			attemptCount:   1,
			rejectReason:   "must remain failed",
		})
		childBefore := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryEvent(ctx, ResetBatchApprovalRetryEventParams{
			EventID:        eventID,
			TicketID:       childID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, "CANCELLED", readBatchApprovalRetryEventStatus(ctx, t, pool, eventID))
		require.Equal(t, childBefore, readBatchApprovalRetryTicket(ctx, t, pool, childID))
	})

	t.Run("event for child owned by another requester is not reset", func(t *testing.T) {
		parentID, _ := seedBatchApprovalRetryParent(ctx, t, pool, batchApprovalRetryParentSeed{
			prefix:            "event-foreign",
			operation:         "DELETE",
			parentStatus:      "FAILED",
			parentEventStatus: "FAILED",
			batchStatus:       "FAILED",
			requester:         "requester-owner",
			eventCreatedBy:    "requester-owner",
			batchCreatedBy:    "requester-owner",
		})
		childID, eventID := seedBatchApprovalRetryChild(ctx, t, pool, batchApprovalRetryChildSeed{
			prefix:         "event-foreign",
			operation:      "DELETE",
			status:         "PENDING",
			eventStatus:    "CANCELLED",
			parentID:       parentID,
			requester:      "requester-foreign",
			eventCreatedBy: "requester-foreign",
			attemptCount:   2,
			rejectReason:   "must remain foreign",
		})
		childBefore := readBatchApprovalRetryTicket(ctx, t, pool, childID)

		rows, err := q.ResetBatchApprovalRetryEvent(ctx, ResetBatchApprovalRetryEventParams{
			EventID:        eventID,
			TicketID:       childID,
			ParentTicketID: pgtype.Text{String: parentID, Valid: true},
		})
		require.NoError(t, err)
		require.EqualValues(t, 0, rows)
		require.Equal(t, "CANCELLED", readBatchApprovalRetryEventStatus(ctx, t, pool, eventID))
		require.Equal(t, childBefore, readBatchApprovalRetryTicket(ctx, t, pool, childID))
	})
}

type batchApprovalRetryParentSeed struct {
	prefix            string
	operation         string
	parentStatus      string
	parentEventStatus string
	batchStatus       string
	requester         string
	eventCreatedBy    string
	batchCreatedBy    string
}

func seedBatchApprovalRetryParent(ctx context.Context, t *testing.T, pool *pgxpool.Pool, seed batchApprovalRetryParentSeed) (parentID, eventID string) {
	t.Helper()
	parentID = "parent-" + seed.prefix
	eventID = "parent-event-" + seed.prefix
	eventType, batchType := batchApprovalRetryTypes(t, seed.operation)

	_, err := pool.Exec(ctx, `
INSERT INTO domain_events (
    id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES ($1, NOW(), $2, 'batch', $3, $4, $5, $6)
`, eventID, eventType, parentID, []byte(`{"seed":"parent"}`), seed.parentEventStatus, seed.eventCreatedBy)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO tickets (
    id, created_at, updated_at, event_id, operation_type, status, requester,
    approver, reject_reason, selected_cluster_id, selected_storage_class,
    modified_spec, attempt_count, last_attempt_at
) VALUES (
    $1, TIMESTAMPTZ '2000-01-01 00:00:00+00', TIMESTAMPTZ '2000-01-01 00:00:00+00',
    $2, $3, $4, $5, 'approver-original', 'original rejection', 'cluster-original',
    'storage-original', '{"preserved":"parent","batch_approval_execution":{"seed":true}}'::jsonb,
    2, TIMESTAMPTZ '2000-01-01 00:00:00+00'
)
`, parentID, eventID, seed.operation, seed.parentStatus, seed.requester)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO batch_tickets (
    id, created_at, updated_at, batch_type, child_count, success_count,
    failed_count, pending_count, status, created_by
) VALUES (
    $1, NOW(), NOW(), $2, 1, 0, 1, 0, $3, $4
)
`, parentID, batchType, seed.batchStatus, seed.batchCreatedBy)
	require.NoError(t, err)

	return parentID, eventID
}

type batchApprovalRetryChildSeed struct {
	prefix         string
	operation      string
	status         string
	eventStatus    string
	parentID       string
	requester      string
	eventCreatedBy string
	attemptCount   int32
	rejectReason   string
}

func seedBatchApprovalRetryChild(ctx context.Context, t *testing.T, pool *pgxpool.Pool, seed batchApprovalRetryChildSeed) (childID, eventID string) {
	t.Helper()
	childID = "child-" + seed.prefix
	eventID = "child-event-" + seed.prefix
	eventType, _ := batchApprovalRetryTypes(t, seed.operation)
	vmEventType := map[string]string{
		"CREATE": "VM_CREATION_REQUESTED",
		"MODIFY": "VM_MODIFY_REQUESTED",
		"DELETE": "VM_DELETION_REQUESTED",
	}[seed.operation]
	require.NotEmpty(t, vmEventType, "unsupported operation %q (batch event type %q)", seed.operation, eventType)

	_, err := pool.Exec(ctx, `
INSERT INTO domain_events (
    id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES ($1, NOW(), $2, 'vm', $3, $4, $5, $6)
`, eventID, vmEventType, "vm-"+seed.prefix, []byte(`{"seed":"child"}`), seed.eventStatus, seed.eventCreatedBy)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
INSERT INTO tickets (
    id, created_at, updated_at, event_id, operation_type, status, requester,
    approver, reject_reason, selected_cluster_id, selected_storage_class,
    modified_spec, parent_ticket_id, attempt_count, last_attempt_at
) VALUES (
    $1, TIMESTAMPTZ '2000-01-01 00:00:00+00', TIMESTAMPTZ '2000-01-01 00:00:00+00',
    $2, $3, $4, $5, 'child-approver', $6, 'child-cluster', 'child-storage',
    '{"preserved":"child","batch_approval_execution":{"dispatch_id":"original"}}'::jsonb,
    $7, $8, TIMESTAMPTZ '2000-01-01 00:00:00+00'
)
`, childID, eventID, seed.operation, seed.status, seed.requester, seed.rejectReason, seed.parentID, seed.attemptCount)
	require.NoError(t, err)

	return childID, eventID
}

func batchApprovalRetryTypes(t *testing.T, operation string) (eventType, batchType string) {
	t.Helper()
	types := map[string][2]string{
		"CREATE": {"BATCH_CREATE_REQUESTED", "BATCH_CREATE"},
		"MODIFY": {"BATCH_MODIFY_REQUESTED", "BATCH_MODIFY"},
		"DELETE": {"BATCH_DELETE_REQUESTED", "BATCH_DELETE"},
	}
	values, ok := types[operation]
	require.True(t, ok, "unsupported batch approval operation %q", operation)
	return values[0], values[1]
}

type batchApprovalRetryTicketState struct {
	status               string
	approver             pgtype.Text
	rejectReason         pgtype.Text
	selectedClusterID    pgtype.Text
	selectedStorageClass pgtype.Text
	modifiedSpec         []byte
	attemptCount         int32
	lastAttemptAt        pgtype.Timestamptz
	updatedAt            time.Time
}

func readBatchApprovalRetryTicket(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ticketID string) batchApprovalRetryTicketState {
	t.Helper()
	var state batchApprovalRetryTicketState
	require.NoError(t, pool.QueryRow(ctx, `
SELECT status, approver, reject_reason, selected_cluster_id, selected_storage_class,
       modified_spec, attempt_count, last_attempt_at, updated_at
FROM tickets
WHERE id = $1
`, ticketID).Scan(
		&state.status,
		&state.approver,
		&state.rejectReason,
		&state.selectedClusterID,
		&state.selectedStorageClass,
		&state.modifiedSpec,
		&state.attemptCount,
		&state.lastAttemptAt,
		&state.updatedAt,
	))
	return state
}

func readBatchApprovalRetryEventStatus(ctx context.Context, t *testing.T, pool *pgxpool.Pool, eventID string) string {
	t.Helper()
	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&status))
	return status
}
