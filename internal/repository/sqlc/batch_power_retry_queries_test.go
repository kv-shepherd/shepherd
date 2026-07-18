package sqlc

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestQueries_ReopenBatchPowerParentForRetry(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reopen_batch_power_parent_for_retry")
	parentID, eventID := seedBatchPowerRetryQueryFixture(
		ctx,
		t,
		pool,
		"reopen",
		"FAILED",
		"FAILED",
		"FAILED",
		[]string{"FAILED"},
	)

	updatedEventID, err := q.ReopenBatchPowerParentForRetry(ctx, parentID)
	require.NoError(t, err)
	require.Equal(t, eventID, updatedEventID)

	var parentStatus, eventStatus string
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT parent.status, event.status
		 FROM tickets AS parent
		 JOIN domain_events AS event ON event.id = parent.event_id
		 WHERE parent.id = $1`,
		parentID,
	).Scan(&parentStatus, &eventStatus))
	require.Equal(t, "EXECUTING", parentStatus)
	require.Equal(t, "PROCESSING", eventStatus)

	invalidParentID, _ := seedBatchPowerRetryQueryFixture(
		ctx,
		t,
		pool,
		"wrong-owner",
		"FAILED",
		"FAILED",
		"FAILED",
		[]string{"FAILED"},
	)
	_, err = pool.Exec(
		ctx,
		`UPDATE batch_tickets SET created_by = 'foreign-requester' WHERE id = $1`,
		invalidParentID,
	)
	require.NoError(t, err)

	_, err = q.ReopenBatchPowerParentForRetry(ctx, invalidParentID)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT parent.status, event.status
		 FROM tickets AS parent
		 JOIN domain_events AS event ON event.id = parent.event_id
		 WHERE parent.id = $1`,
		invalidParentID,
	).Scan(&parentStatus, &eventStatus))
	require.Equal(t, "FAILED", parentStatus)
	require.Equal(t, "FAILED", eventStatus)
}

func TestQueries_RefreshBatchPowerProjectionForRetry(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "refresh_batch_power_projection_for_retry")
	parentID, _ := seedBatchPowerRetryQueryFixture(
		ctx,
		t,
		pool,
		"refresh",
		"EXECUTING",
		"PROCESSING",
		"FAILED",
		[]string{"SUCCESS", "FAILED", "REJECTED", "EXECUTING"},
	)

	rows, err := q.RefreshBatchPowerProjectionForRetry(ctx, parentID)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var status string
	var childCount, successCount, failedCount, pendingCount int32
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, child_count, success_count, failed_count, pending_count
		 FROM batch_tickets WHERE id = $1`,
		parentID,
	).Scan(&status, &childCount, &successCount, &failedCount, &pendingCount))
	require.Equal(t, "IN_PROGRESS", status)
	require.EqualValues(t, 4, childCount)
	require.EqualValues(t, 1, successCount)
	require.EqualValues(t, 2, failedCount)
	require.EqualValues(t, 1, pendingCount)

	_, err = pool.Exec(ctx, `UPDATE domain_events SET created_by = 'foreign-requester' WHERE aggregate_id = $1 AND aggregate_type = 'batch'`, parentID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE batch_tickets SET status = 'FAILED', child_count = 91, success_count = 92, failed_count = 93, pending_count = 94 WHERE id = $1`, parentID)
	require.NoError(t, err)

	rows, err = q.RefreshBatchPowerProjectionForRetry(ctx, parentID)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, child_count, success_count, failed_count, pending_count
		 FROM batch_tickets WHERE id = $1`,
		parentID,
	).Scan(&status, &childCount, &successCount, &failedCount, &pendingCount))
	require.Equal(t, "FAILED", status)
	require.EqualValues(t, 91, childCount)
	require.EqualValues(t, 92, successCount)
	require.EqualValues(t, 93, failedCount)
	require.EqualValues(t, 94, pendingCount)
}

func seedBatchPowerRetryQueryFixture(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	prefix string,
	parentStatus, eventStatus, projectionStatus string,
	childStatuses []string,
) (parentID, parentEventID string) {
	t.Helper()
	parentID = "ticket-parent-power-" + prefix
	parentEventID = "event-parent-power-" + prefix
	_, err := pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES ($1, NOW(), 'BATCH_POWER_REQUESTED', 'batch', $2, '{}'::bytea, $3, 'requester-1')
`, parentEventID, parentID, eventStatus)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester, parent_ticket_id
) VALUES ($1, NOW(), NOW(), $2, 'POWER', $3, 'requester-1', NULL)
`, parentID, parentEventID, parentStatus)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by
) VALUES ($1, NOW(), NOW(), 'BATCH_POWER', $2, 0, $2, 0, $3, 'requester-1')
`, parentID, len(childStatuses), projectionStatus)
	require.NoError(t, err)

	for index, childStatus := range childStatuses {
		childID := fmt.Sprintf("ticket-child-power-%s-%d", prefix, index)
		childEventID := fmt.Sprintf("event-child-power-%s-%d", prefix, index)
		childEventStatus := map[string]string{
			"SUCCESS":   "COMPLETED",
			"FAILED":    "FAILED",
			"REJECTED":  "CANCELLED",
			"EXECUTING": "PROCESSING",
		}[childStatus]
		require.NotEmpty(t, childEventStatus, "unsupported child ticket status %q", childStatus)
		_, err = pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, '{}'::bytea, $3, 'requester-1')
`, childEventID, childID, childEventStatus)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester, parent_ticket_id
) VALUES ($1, NOW(), NOW(), $2, 'POWER', $3, 'requester-1', $4)
`, childID, childEventID, childStatus, parentID)
		require.NoError(t, err)
	}

	return parentID, parentEventID
}
