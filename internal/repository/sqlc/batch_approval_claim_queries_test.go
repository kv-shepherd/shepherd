package sqlc

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestQueries_ClaimBatchApprovalDispatch(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "claim_batch_approval_dispatch")
	fixture := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"dispatch-valid",
		"CREATE",
		"PENDING",
		"PENDING",
		"PENDING_APPROVAL",
	)
	params := ClaimBatchApprovalDispatchParams{
		Approver:             pgtype.Text{String: "approver-1", Valid: true},
		SelectedClusterID:    pgtype.Text{String: "cluster-1", Valid: true},
		SelectedStorageClass: pgtype.Text{String: "fast", Valid: true},
		ExecutionOptions:     []byte(`{"parallelism":3,"continue_on_error":true}`),
		ID:                   fixture.parentID,
		EventID:              fixture.eventID,
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	txQueries := q.WithTx(tx)
	rows, err := txQueries.ClaimBatchApprovalDispatch(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalTicketState(ctx, t, tx, fixture.parentID, "EXECUTING", "approver-1")
	rows, err = txQueries.ClaimBatchApprovalDispatch(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "the PENDING claim must be single-use within one transaction")
	require.NoError(t, tx.Rollback(ctx))
	requireBatchApprovalTicketState(ctx, t, pool, fixture.parentID, "PENDING", "")

	rows, err = q.ClaimBatchApprovalDispatch(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status          string
		approver        pgtype.Text
		clusterID       pgtype.Text
		storageClass    pgtype.Text
		modifiedSpec    []byte
		rejectionReason pgtype.Text
	)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, approver, selected_cluster_id, selected_storage_class,
		        modified_spec, reject_reason
		 FROM tickets WHERE id = $1`,
		fixture.parentID,
	).Scan(&status, &approver, &clusterID, &storageClass, &modifiedSpec, &rejectionReason))
	require.Equal(t, "EXECUTING", status)
	require.Equal(t, "approver-1", approver.String)
	require.True(t, approver.Valid)
	require.Equal(t, "cluster-1", clusterID.String)
	require.True(t, clusterID.Valid)
	require.Equal(t, "fast", storageClass.String)
	require.True(t, storageClass.Valid)
	require.JSONEq(t, `{
		"seed":"preserved",
		"batch_approval_execution":{"parallelism":3,"continue_on_error":true}
	}`, string(modifiedSpec))
	require.False(t, rejectionReason.Valid)

	changedParams := params
	changedParams.Approver = pgtype.Text{String: "approver-2", Valid: true}
	changedParams.ExecutionOptions = []byte(`{"parallelism":99}`)
	rows, err = q.ClaimBatchApprovalDispatch(ctx, changedParams)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "an already claimed parent must not be overwritten")
	requireBatchApprovalTicketState(ctx, t, pool, fixture.parentID, "EXECUTING", "approver-1")

	identityMismatch := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"dispatch-identity-mismatch",
		"CREATE",
		"PENDING",
		"PENDING",
		"PENDING_APPROVAL",
	)
	_, err = pool.Exec(ctx, `UPDATE domain_events SET created_by = 'other-requester' WHERE id = $1`, identityMismatch.eventID)
	require.NoError(t, err)
	badParams := params
	badParams.ID = identityMismatch.parentID
	badParams.EventID = identityMismatch.eventID
	rows, err = q.ClaimBatchApprovalDispatch(ctx, badParams)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalTicketState(ctx, t, pool, identityMismatch.parentID, "PENDING", "")
}

func TestQueries_ClaimBatchApprovalEventProcessing(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "claim_batch_approval_event_processing")
	fixture := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"claim-event-valid",
		"POWER",
		"PENDING",
		"PENDING",
		"PENDING_APPROVAL",
	)
	params := ClaimBatchApprovalEventProcessingParams{
		EventID:  fixture.eventID,
		ParentID: fixture.parentID,
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	txQueries := q.WithTx(tx)
	rows, err := txQueries.ClaimBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalEventStatus(ctx, t, tx, fixture.eventID, "PROCESSING")
	rows, err = txQueries.ClaimBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "the PENDING event claim must be single-use within one transaction")
	require.NoError(t, tx.Rollback(ctx))
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "PENDING")

	rows, err = q.ClaimBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "PROCESSING")
	rows, err = q.ClaimBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "PROCESSING")

	identityMismatch := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"claim-event-identity-mismatch",
		"POWER",
		"PENDING",
		"PENDING",
		"PENDING_APPROVAL",
	)
	_, err = pool.Exec(ctx, `UPDATE batch_tickets SET created_by = 'other-requester' WHERE id = $1`, identityMismatch.parentID)
	require.NoError(t, err)
	rows, err = q.ClaimBatchApprovalEventProcessing(ctx, ClaimBatchApprovalEventProcessingParams{
		EventID:  identityMismatch.eventID,
		ParentID: identityMismatch.parentID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, identityMismatch.eventID, "PENDING")

	wrongState := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"claim-event-wrong-state",
		"POWER",
		"PENDING",
		"FAILED",
		"PENDING_APPROVAL",
	)
	rows, err = q.ClaimBatchApprovalEventProcessing(ctx, ClaimBatchApprovalEventProcessingParams{
		EventID:  wrongState.eventID,
		ParentID: wrongState.parentID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, wrongState.eventID, "FAILED")
}

func TestQueries_SetBatchApprovalEventProcessing(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "set_batch_approval_event_processing")
	fixture := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"set-event-valid",
		"CREATE",
		"EXECUTING",
		"FAILED",
		"FAILED",
	)
	params := SetBatchApprovalEventProcessingParams{
		EventID:  fixture.eventID,
		ParentID: fixture.parentID,
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	rows, err := q.WithTx(tx).SetBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalEventStatus(ctx, t, tx, fixture.eventID, "PROCESSING")
	require.NoError(t, tx.Rollback(ctx))
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "FAILED")

	rows, err = q.SetBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "PROCESSING")
	rows, err = q.SetBatchApprovalEventProcessing(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows, "PostgreSQL counts the permitted PROCESSING-to-PROCESSING update")
	requireBatchApprovalEventStatus(ctx, t, pool, fixture.eventID, "PROCESSING")

	identityMismatch := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"set-event-identity-mismatch",
		"CREATE",
		"EXECUTING",
		"FAILED",
		"FAILED",
	)
	_, err = pool.Exec(ctx, `UPDATE domain_events SET aggregate_id = 'other-parent' WHERE id = $1`, identityMismatch.eventID)
	require.NoError(t, err)
	rows, err = q.SetBatchApprovalEventProcessing(ctx, SetBatchApprovalEventProcessingParams{
		EventID:  identityMismatch.eventID,
		ParentID: identityMismatch.parentID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, identityMismatch.eventID, "FAILED")

	wrongState := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"set-event-wrong-state",
		"CREATE",
		"EXECUTING",
		"COMPLETED",
		"FAILED",
	)
	rows, err = q.SetBatchApprovalEventProcessing(ctx, SetBatchApprovalEventProcessingParams{
		EventID:  wrongState.eventID,
		ParentID: wrongState.parentID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalEventStatus(ctx, t, pool, wrongState.eventID, "COMPLETED")
}

func TestQueries_RefreshBatchApprovalProjectionForDispatch(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "refresh_batch_approval_projection_for_dispatch")
	fixture := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"refresh-valid",
		"CREATE",
		"EXECUTING",
		"PROCESSING",
		"PENDING_APPROVAL",
	)
	for index, status := range []string{"SUCCESS", "FAILED", "REJECTED", "CANCELLED", "PENDING", "EXECUTING"} {
		seedBatchApprovalChildTicket(ctx, t, pool, fixture, index, status)
	}

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	rows, err := q.WithTx(tx).RefreshBatchApprovalProjectionForDispatch(ctx, fixture.parentID)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalProjection(ctx, t, tx, fixture.parentID, 6, 1, 2, 2, "IN_PROGRESS")
	require.NoError(t, tx.Rollback(ctx))
	requireBatchApprovalProjection(ctx, t, pool, fixture.parentID, 91, 17, 19, 23, "PENDING_APPROVAL")

	rows, err = q.RefreshBatchApprovalProjectionForDispatch(ctx, fixture.parentID)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	requireBatchApprovalProjection(ctx, t, pool, fixture.parentID, 6, 1, 2, 2, "IN_PROGRESS")

	identityMismatch := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"refresh-identity-mismatch",
		"CREATE",
		"EXECUTING",
		"PROCESSING",
		"PENDING_APPROVAL",
	)
	seedBatchApprovalChildTicket(ctx, t, pool, identityMismatch, 0, "SUCCESS")
	_, err = pool.Exec(ctx, `UPDATE domain_events SET created_by = 'other-requester' WHERE id = $1`, identityMismatch.eventID)
	require.NoError(t, err)
	rows, err = q.RefreshBatchApprovalProjectionForDispatch(ctx, identityMismatch.parentID)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalProjection(ctx, t, pool, identityMismatch.parentID, 91, 17, 19, 23, "PENDING_APPROVAL")

	wrongState := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"refresh-wrong-state",
		"CREATE",
		"PENDING",
		"PROCESSING",
		"PENDING_APPROVAL",
	)
	seedBatchApprovalChildTicket(ctx, t, pool, wrongState, 0, "SUCCESS")
	rows, err = q.RefreshBatchApprovalProjectionForDispatch(ctx, wrongState.parentID)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	requireBatchApprovalProjection(ctx, t, pool, wrongState.parentID, 91, 17, 19, 23, "PENDING_APPROVAL")

	noChildren := seedBatchApprovalQueryFixture(
		ctx,
		t,
		pool,
		"refresh-no-children",
		"CREATE",
		"EXECUTING",
		"PROCESSING",
		"PENDING_APPROVAL",
	)
	rows, err = q.RefreshBatchApprovalProjectionForDispatch(ctx, noChildren.parentID)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "an empty child set must not fabricate a zero-count projection")
	requireBatchApprovalProjection(ctx, t, pool, noChildren.parentID, 91, 17, 19, 23, "PENDING_APPROVAL")
}

type batchApprovalQueryFixture struct {
	parentID  string
	eventID   string
	requester string
	operation string
	batchType string
	eventType string
}

func seedBatchApprovalQueryFixture(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	suffix string,
	operation string,
	ticketStatus string,
	eventStatus string,
	batchStatus string,
) batchApprovalQueryFixture {
	t.Helper()

	fixture := batchApprovalQueryFixture{
		parentID:  "parent-" + suffix,
		eventID:   "event-" + suffix,
		requester: "requester-" + suffix,
		operation: operation,
	}
	switch operation {
	case "CREATE":
		fixture.batchType = "BATCH_CREATE"
		fixture.eventType = "BATCH_CREATE_REQUESTED"
	case "MODIFY":
		fixture.batchType = "BATCH_MODIFY"
		fixture.eventType = "BATCH_MODIFY_REQUESTED"
	case "DELETE":
		fixture.batchType = "BATCH_DELETE"
		fixture.eventType = "BATCH_DELETE_REQUESTED"
	case "POWER":
		fixture.batchType = "BATCH_POWER"
		fixture.eventType = "BATCH_POWER_REQUESTED"
	default:
		t.Fatalf("unsupported approval operation %q", operation)
	}

	_, err := pool.Exec(ctx, `
		INSERT INTO domain_events (
			id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
		) VALUES ($1, NOW(), $2, 'batch', $3, '{}'::bytea, $4, $5)`,
		fixture.eventID,
		fixture.eventType,
		fixture.parentID,
		eventStatus,
		fixture.requester,
	)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO tickets (
			id, created_at, updated_at, event_id, operation_type, status, requester,
			reject_reason, modified_spec
		) VALUES ($1, NOW(), NOW(), $2, $3, $4, $5, 'stale rejection', '{"seed":"preserved"}'::jsonb)`,
		fixture.parentID,
		fixture.eventID,
		fixture.operation,
		ticketStatus,
		fixture.requester,
	)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO batch_tickets (
			id, created_at, updated_at, batch_type, child_count, success_count,
			failed_count, pending_count, status, created_by
		) VALUES ($1, NOW(), NOW(), $2, 91, 17, 19, 23, $3, $4)`,
		fixture.parentID,
		fixture.batchType,
		batchStatus,
		fixture.requester,
	)
	require.NoError(t, err)

	return fixture
}

func seedBatchApprovalChildTicket(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	fixture batchApprovalQueryFixture,
	index int,
	status string,
) {
	t.Helper()
	_, err := pool.Exec(ctx, `
		INSERT INTO tickets (
			id, created_at, updated_at, event_id, operation_type, status, requester, parent_ticket_id
		) VALUES ($1, NOW(), NOW(), $2, $3, $4, $5, $6)`,
		fixture.parentID+"-child-"+strconv.Itoa(index),
		fixture.eventID+"-child-"+strconv.Itoa(index),
		fixture.operation,
		status,
		fixture.requester,
		fixture.parentID,
	)
	require.NoError(t, err)
}

type batchApprovalQueryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func requireBatchApprovalTicketState(
	ctx context.Context,
	t *testing.T,
	db batchApprovalQueryRower,
	parentID string,
	wantStatus string,
	wantApprover string,
) {
	t.Helper()
	var status string
	var approver pgtype.Text
	require.NoError(t, db.QueryRow(ctx, `SELECT status, approver FROM tickets WHERE id = $1`, parentID).Scan(&status, &approver))
	require.Equal(t, wantStatus, status)
	if wantApprover == "" {
		require.False(t, approver.Valid)
		return
	}
	require.True(t, approver.Valid)
	require.Equal(t, wantApprover, approver.String)
}

func requireBatchApprovalEventStatus(
	ctx context.Context,
	t *testing.T,
	db batchApprovalQueryRower,
	eventID string,
	wantStatus string,
) {
	t.Helper()
	var status string
	require.NoError(t, db.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&status))
	require.Equal(t, wantStatus, status)
}

func requireBatchApprovalProjection(
	ctx context.Context,
	t *testing.T,
	db batchApprovalQueryRower,
	parentID string,
	wantChildCount int32,
	wantSuccessCount int32,
	wantFailedCount int32,
	wantPendingCount int32,
	wantStatus string,
) {
	t.Helper()
	var childCount, successCount, failedCount, pendingCount int32
	var status string
	require.NoError(t, db.QueryRow(ctx, `
		SELECT child_count, success_count, failed_count, pending_count, status
		FROM batch_tickets WHERE id = $1`, parentID).Scan(
		&childCount,
		&successCount,
		&failedCount,
		&pendingCount,
		&status,
	))
	require.Equal(t, wantChildCount, childCount)
	require.Equal(t, wantSuccessCount, successCount)
	require.Equal(t, wantFailedCount, failedCount)
	require.Equal(t, wantPendingCount, pendingCount)
	require.Equal(t, wantStatus, status)
}
