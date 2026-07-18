package sqlc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/internal/testutil"
)

const sqlcSchemaPath = "schema.sql"

func TestQueries_AllocateServiceInstance(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "allocate_service_instance")

	systemID := "sys-allocate"
	serviceID := "svc-allocate"
	seedSystemAndService(ctx, t, pool, systemID, serviceID)

	row, err := q.AllocateServiceInstance(ctx, serviceID)
	require.NoError(t, err)
	require.Equal(t, serviceID, row.ServiceID)
	require.Equal(t, "service-"+serviceID, row.ServiceName)
	require.Equal(t, "system-"+systemID, row.SystemName)
	require.EqualValues(t, 1, row.AllocatedIndex)

	var nextIndex int32
	require.NoError(t, pool.QueryRow(ctx, `SELECT next_instance_index FROM services WHERE id=$1`, serviceID).Scan(&nextIndex))
	require.EqualValues(t, 2, nextIndex)
}

func TestQueries_AllocateServiceInstanceConcurrent(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "allocate_service_instance_concurrent")

	systemID := "sys-allocate-concurrent"
	serviceID := "svc-allocate-concurrent"
	seedSystemAndService(ctx, t, pool, systemID, serviceID)

	const allocations = 16
	results := make(chan int32, allocations)

	t.Cleanup(func() {
		close(results)

		seen := make(map[int32]struct{}, allocations)
		for idx := range results {
			seen[idx] = struct{}{}
		}
		require.Len(t, seen, allocations)
		for idx := int32(1); idx <= allocations; idx++ {
			require.Contains(t, seen, idx)
		}

		var nextIndex int32
		require.NoError(t, pool.QueryRow(ctx, `SELECT next_instance_index FROM services WHERE id=$1`, serviceID).Scan(&nextIndex))
		require.EqualValues(t, allocations+1, nextIndex)
	})

	for idx := range allocations {
		t.Run(fmt.Sprintf("allocation_%02d", idx), func(t *testing.T) {
			t.Parallel()
			row, err := q.AllocateServiceInstance(ctx, serviceID)
			require.NoError(t, err)
			results <- row.AllocatedIndex
		})
	}
}

func TestQueries_ApproveCreateTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "approve_create_ticket")

	ticketID := "ticket-create-1"
	eventID := "event-create-1"
	seedTicket(ctx, t, pool, ticketID, eventID, "CREATE")

	templateSnapshot := []byte(`{"name":"tpl-v3"}`)
	instanceSizeSnapshot := []byte(`{"cpu_cores":4,"memory_gi":8}`)
	placementEvaluation := []byte(`{"selected_cluster_id":"cluster-a","eligible":true}`)
	modifiedSpec := []byte(`{"cpu":4}`)

	rows, err := q.ApproveCreateTicket(ctx, ApproveCreateTicketParams{
		Approver:             pgtype.Text{String: "admin-1", Valid: true},
		SelectedClusterID:    pgtype.Text{String: "cluster-a", Valid: true},
		SelectedStorageClass: "fast",
		TemplateSnapshot:     templateSnapshot,
		InstanceSizeSnapshot: instanceSizeSnapshot,
		PlacementEvaluation:  placementEvaluation,
		ModifiedSpec:         modifiedSpec,
		ID:                   ticketID,
		EventID:              eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status       string
		approver     pgtype.Text
		clusterID    pgtype.Text
		storageClass pgtype.Text
		gotTemplate  []byte
		gotSize      []byte
		gotPlacement []byte
		gotModified  []byte
	)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, approver, selected_cluster_id, selected_storage_class, template_snapshot, instance_size_snapshot, placement_evaluation, modified_spec
         FROM tickets WHERE id=$1`,
		ticketID,
	).Scan(
		&status,
		&approver,
		&clusterID,
		&storageClass,
		&gotTemplate,
		&gotSize,
		&gotPlacement,
		&gotModified,
	))

	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-1", approver.String)
	require.True(t, clusterID.Valid)
	require.Equal(t, "cluster-a", clusterID.String)
	require.True(t, storageClass.Valid)
	require.Equal(t, "fast", storageClass.String)
	assertJSONEqual(t, templateSnapshot, gotTemplate)
	assertJSONEqual(t, instanceSizeSnapshot, gotSize)
	assertJSONEqual(t, placementEvaluation, gotPlacement)
	assertJSONEqual(t, modifiedSpec, gotModified)

	rows, err = q.ApproveCreateTicket(ctx, ApproveCreateTicketParams{
		Approver:          pgtype.Text{String: "admin-2", Valid: true},
		SelectedClusterID: pgtype.Text{String: "cluster-b", Valid: true},
		ID:                ticketID,
		EventID:           eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "ticket is no longer pending and must not be re-approved")
}

func TestQueries_ApproveDeleteTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "approve_delete_ticket")

	ticketID := "ticket-delete-1"
	eventID := "event-delete-1"
	seedTicket(ctx, t, pool, ticketID, eventID, "DELETE")
	_, err := pool.Exec(ctx, `UPDATE tickets SET parent_ticket_id = 'batch-delete-1' WHERE id = $1`, ticketID)
	require.NoError(t, err)

	rows, err := q.ApproveDeleteTicket(ctx, ApproveDeleteTicketParams{
		Approver: pgtype.Text{String: "admin-delete", Valid: true},
		ID:       ticketID,
		EventID:  eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status       string
		approver     pgtype.Text
		attemptCount int32
		lastAttempt  pgtype.Timestamptz
	)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, approver, attempt_count, last_attempt_at FROM tickets WHERE id=$1`, ticketID).Scan(&status, &approver, &attemptCount, &lastAttempt))
	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-delete", approver.String)
	require.EqualValues(t, 1, attemptCount)
	require.True(t, lastAttempt.Valid)
}

func TestQueries_ApproveModifyTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "approve_modify_ticket")

	ticketID := "ticket-modify-1"
	eventID := "event-modify-1"
	seedTicket(ctx, t, pool, ticketID, eventID, "MODIFY")

	rows, err := q.ApproveModifyTicket(ctx, ApproveModifyTicketParams{
		Approver:     pgtype.Text{String: "admin-modify", Valid: true},
		ModifiedSpec: []byte(`{"cpu_request":4,"memory_request_gi":4}`),
		ID:           ticketID,
		EventID:      eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status       string
		approver     pgtype.Text
		modifiedSpec []byte
	)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, approver, modified_spec FROM tickets WHERE id=$1`, ticketID).Scan(&status, &approver, &modifiedSpec))
	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-modify", approver.String)
	require.JSONEq(t, `{"cpu_request":4,"memory_request_gi":4}`, string(modifiedSpec))
}

func TestQueries_ApprovePowerTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "approve_power_ticket")

	ticketID := "ticket-power-1"
	eventID := "event-power-1"
	seedTicket(ctx, t, pool, ticketID, eventID, "POWER")

	rows, err := q.ApprovePowerTicket(ctx, ApprovePowerTicketParams{
		Approver: pgtype.Text{String: "admin-power", Valid: true},
		ID:       ticketID,
		EventID:  eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status   string
		approver pgtype.Text
	)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, approver FROM tickets WHERE id=$1`, ticketID).Scan(&status, &approver))
	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-power", approver.String)
}

func TestQueries_InsertVM(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "insert_vm")

	systemID := "sys-insert-vm"
	serviceID := "svc-insert-vm"
	seedSystemAndService(ctx, t, pool, systemID, serviceID)

	vmID := "vm-insert-1"
	err := q.InsertVM(ctx, InsertVMParams{
		ID:         vmID,
		Name:       "vm-insert-1",
		Instance:   "system-svc-1",
		Namespace:  "dev-ns",
		ClusterID:  pgtype.Text{String: "cluster-1", Valid: true},
		Hostname:   pgtype.Text{String: "vm-insert-1.internal", Valid: true},
		CreatedBy:  "user-1",
		TicketID:   pgtype.Text{String: "ticket-create-1", Valid: true},
		ServiceVms: serviceID,
	})
	require.NoError(t, err)

	var (
		status    string
		clusterID pgtype.Text
		hostname  pgtype.Text
		createdBy string
	)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, cluster_id, hostname, created_by FROM vms WHERE id=$1`,
		vmID,
	).Scan(&status, &clusterID, &hostname, &createdBy))
	require.Equal(t, "CREATING", status)
	require.True(t, clusterID.Valid)
	require.Equal(t, "cluster-1", clusterID.String)
	require.True(t, hostname.Valid)
	require.Equal(t, "vm-insert-1.internal", hostname.String)
	require.Equal(t, "user-1", createdBy)
}

func TestQueries_SetDomainEventStatus(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "set_domain_event_status")

	eventID := "event-status-1"
	seedDomainEvent(ctx, t, pool, eventID, "PENDING")

	rows, err := q.SetDomainEventStatus(ctx, SetDomainEventStatusParams{
		ID:     eventID,
		Status: "COMPLETED",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&status))
	require.Equal(t, "COMPLETED", status)

	rows, err = q.SetDomainEventStatus(ctx, SetDomainEventStatusParams{
		ID:     eventID,
		Status: "PROCESSING",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)

	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&status))
	require.Equal(t, "COMPLETED", status)
}

func TestQueries_InsertDomainEvent(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "insert_domain_event")

	err := q.InsertDomainEvent(ctx, InsertDomainEventParams{
		ID:            "event-insert-1",
		EventType:     "VM_START_REQUESTED",
		AggregateType: "vm",
		AggregateID:   "vm-1",
		Payload:       []byte(`{"operation":"start"}`),
		Status:        "PENDING",
		CreatedBy:     "user-1",
	})
	require.NoError(t, err)

	var (
		eventType string
		status    string
		payload   []byte
	)
	require.NoError(t, pool.QueryRow(ctx, `SELECT event_type, status, payload FROM domain_events WHERE id=$1`, "event-insert-1").Scan(&eventType, &status, &payload))
	require.Equal(t, "VM_START_REQUESTED", eventType)
	require.Equal(t, "PENDING", status)
	require.JSONEq(t, `{"operation":"start"}`, string(payload))
}

func TestQueries_InsertTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "insert_ticket")

	require.NoError(t, q.InsertTicket(ctx, InsertTicketParams{
		ID:             "ticket-parent-1",
		EventID:        "event-parent-1",
		OperationType:  "POWER",
		Status:         "EXECUTING",
		Requester:      "user-1",
		Reason:         pgtype.Text{String: "batch power request", Valid: true},
		ParentTicketID: pgtype.Text{},
	}))
	require.NoError(t, q.InsertTicket(ctx, InsertTicketParams{
		ID:             "ticket-child-1",
		EventID:        "event-child-1",
		OperationType:  "POWER",
		Status:         "EXECUTING",
		Requester:      "user-1",
		Reason:         pgtype.Text{String: "child power request", Valid: true},
		ParentTicketID: pgtype.Text{String: "ticket-parent-1", Valid: true},
	}))
	var childStatus, parentID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, parent_ticket_id FROM tickets WHERE id=$1`, "ticket-child-1").Scan(&childStatus, &parentID))
	require.Equal(t, "EXECUTING", childStatus)
	require.Equal(t, "ticket-parent-1", parentID)
}

func TestQueries_StartInitialBatchChildAttempt(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "start_initial_batch_child_attempt")

	const parentID = "ticket-parent-attempt"
	seedTicket(ctx, t, pool, parentID, "event-parent-attempt", "POWER")
	seedTicket(ctx, t, pool, "ticket-child-attempt", "event-child-attempt", "POWER")
	_, err := pool.Exec(
		ctx,
		`UPDATE tickets SET parent_ticket_id = $1 WHERE id = $2`,
		parentID,
		"ticket-child-attempt",
	)
	require.NoError(t, err)

	params := StartInitialBatchChildAttemptParams{
		ID:             "ticket-child-attempt",
		EventID:        "event-child-attempt",
		ParentTicketID: pgtype.Text{String: parentID, Valid: true},
	}
	rows, err := q.StartInitialBatchChildAttempt(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var attemptCount int32
	var lastAttempt pgtype.Timestamptz
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT attempt_count, last_attempt_at FROM tickets WHERE id = $1`,
		params.ID,
	).Scan(&attemptCount, &lastAttempt))
	require.EqualValues(t, 1, attemptCount)
	require.True(t, lastAttempt.Valid)
	require.False(t, lastAttempt.Time.IsZero())

	rows, err = q.StartInitialBatchChildAttempt(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows, "an initial logical attempt must only be recorded once")

	seedTicket(ctx, t, pool, "ticket-child-mismatch", "event-child-mismatch", "POWER")
	_, err = pool.Exec(
		ctx,
		`UPDATE tickets SET parent_ticket_id = $1 WHERE id = $2`,
		parentID,
		"ticket-child-mismatch",
	)
	require.NoError(t, err)
	rows, err = q.StartInitialBatchChildAttempt(ctx, StartInitialBatchChildAttemptParams{
		ID:             "ticket-child-mismatch",
		EventID:        "wrong-event",
		ParentTicketID: pgtype.Text{String: parentID, Valid: true},
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT attempt_count, last_attempt_at FROM tickets WHERE id = $1`,
		"ticket-child-mismatch",
	).Scan(&attemptCount, &lastAttempt))
	require.Zero(t, attemptCount)
	require.False(t, lastAttempt.Valid)
}

func TestQueries_InsertBatchTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "insert_batch_ticket")

	require.NoError(t, q.InsertBatchTicket(ctx, InsertBatchTicketParams{
		ID:           "ticket-parent-1",
		BatchType:    "BATCH_POWER",
		ChildCount:   1,
		PendingCount: 1,
		Status:       "IN_PROGRESS",
		CreatedBy:    "user-1",
		RequestID:    pgtype.Text{String: "request-1", Valid: true},
		Reason:       pgtype.Text{String: "batch power request", Valid: true},
	}))

	var batchType, createdBy string
	var pendingCount int32
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT batch_type, pending_count, created_by FROM batch_tickets WHERE id=$1`,
		"ticket-parent-1",
	).Scan(&batchType, &pendingCount, &createdBy))
	require.Equal(t, "BATCH_POWER", batchType)
	require.EqualValues(t, 1, pendingCount)
	require.Equal(t, "user-1", createdBy)
}

func TestQueries_ResetPowerRetryTicket(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reset_power_retry_ticket")
	_, err := pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES (
  'event-parent-retry', NOW(), 'BATCH_POWER_REQUESTED', 'batch',
  'ticket-parent-retry', '{}'::bytea, 'FAILED', 'requester-1'
);
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester
) VALUES (
  'ticket-parent-retry', NOW(), NOW(), 'event-parent-retry', 'POWER', 'FAILED', 'requester-1'
);
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by
) VALUES (
  'ticket-parent-retry', NOW(), NOW(), 'BATCH_POWER', 5, 0, 5, 0, 'FAILED', 'requester-1'
)
`)
	require.NoError(t, err)
	seedPowerRetryEvent := func(eventID, vmID, status string) {
		t.Helper()
		_, seedErr := pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES ($1, NOW(), 'VM_START_REQUESTED', 'vm', $2, '{}'::bytea, $3, 'requester-1')
`, eventID, vmID, status)
		require.NoError(t, seedErr)
	}

	seedPowerRetryEvent("event-child-retry", "vm-child-retry", "FAILED")
	seedTicket(ctx, t, pool, "ticket-child-retry", "event-child-retry", "POWER")
	_, err = pool.Exec(ctx, `UPDATE tickets SET status='FAILED', reject_reason='seed failure', parent_ticket_id='ticket-parent-retry', attempt_count=1, last_attempt_at=NOW() - interval '1 minute' WHERE id='ticket-child-retry'`)
	require.NoError(t, err)

	rows, err := q.ResetPowerRetryTicket(ctx, ResetPowerRetryTicketParams{
		ID:             "ticket-child-retry",
		EventID:        "event-child-retry",
		ParentTicketID: pgtype.Text{String: "ticket-parent-retry", Valid: true},
		MaxAttempts:    3,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var ticketStatus string
	var rejectReason pgtype.Text
	var attemptCount int32
	var lastAttempt pgtype.Timestamptz
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, reject_reason, attempt_count, last_attempt_at FROM tickets WHERE id=$1`, "ticket-child-retry").Scan(&ticketStatus, &rejectReason, &attemptCount, &lastAttempt))
	require.Equal(t, "EXECUTING", ticketStatus)
	require.False(t, rejectReason.Valid)
	require.EqualValues(t, 2, attemptCount)
	require.True(t, lastAttempt.Valid)

	seedPowerRetryEvent("event-child-rejected", "vm-child-rejected", "CANCELLED")
	seedTicket(ctx, t, pool, "ticket-child-rejected", "event-child-rejected", "POWER")
	_, err = pool.Exec(ctx, `UPDATE tickets SET status='REJECTED', reject_reason='seed rejection', parent_ticket_id='ticket-parent-retry' WHERE id='ticket-child-rejected'`)
	require.NoError(t, err)
	rows, err = q.ResetPowerRetryTicket(ctx, ResetPowerRetryTicketParams{
		ID:             "ticket-child-rejected",
		EventID:        "event-child-rejected",
		ParentTicketID: pgtype.Text{String: "ticket-parent-retry", Valid: true},
		MaxAttempts:    3,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, reject_reason FROM tickets WHERE id=$1`, "ticket-child-rejected").Scan(&ticketStatus, &rejectReason))
	require.Equal(t, "REJECTED", ticketStatus)
	require.Equal(t, "seed rejection", rejectReason.String)

	seedPowerRetryEvent("event-child-mismatch", "vm-child-mismatch", "FAILED")
	seedTicket(ctx, t, pool, "ticket-child-mismatch", "event-child-mismatch", "POWER")
	_, err = pool.Exec(ctx, `UPDATE tickets SET status='FAILED', reject_reason='seed failure', parent_ticket_id='ticket-parent-retry' WHERE id='ticket-child-mismatch'`)
	require.NoError(t, err)
	rows, err = q.ResetPowerRetryTicket(ctx, ResetPowerRetryTicketParams{
		ID:             "ticket-child-mismatch",
		EventID:        "event-child-other",
		ParentTicketID: pgtype.Text{String: "ticket-parent-retry", Valid: true},
		MaxAttempts:    3,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)

	seedPowerRetryEvent("event-child-pending", "vm-child-pending", "FAILED")
	seedTicket(ctx, t, pool, "ticket-child-pending", "event-child-pending", "POWER")
	_, err = pool.Exec(ctx, `UPDATE tickets SET status='PENDING', parent_ticket_id='ticket-parent-retry' WHERE id='ticket-child-pending'`)
	require.NoError(t, err)
	rows, err = q.ResetPowerRetryTicket(ctx, ResetPowerRetryTicketParams{
		ID:             "ticket-child-pending",
		EventID:        "event-child-pending",
		ParentTicketID: pgtype.Text{String: "ticket-parent-retry", Valid: true},
		MaxAttempts:    3,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)

	seedPowerRetryEvent("event-child-exhausted", "vm-child-exhausted", "FAILED")
	seedTicket(ctx, t, pool, "ticket-child-exhausted", "event-child-exhausted", "POWER")
	_, err = pool.Exec(ctx, `UPDATE tickets SET status='FAILED', reject_reason='terminal failure', parent_ticket_id='ticket-parent-retry', attempt_count=3, last_attempt_at=NOW() - interval '1 minute' WHERE id='ticket-child-exhausted'`)
	require.NoError(t, err)
	rows, err = q.ResetPowerRetryTicket(ctx, ResetPowerRetryTicketParams{
		ID:             "ticket-child-exhausted",
		EventID:        "event-child-exhausted",
		ParentTicketID: pgtype.Text{String: "ticket-parent-retry", Valid: true},
		MaxAttempts:    3,
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, reject_reason, attempt_count FROM tickets WHERE id=$1`, "ticket-child-exhausted").Scan(&ticketStatus, &rejectReason, &attemptCount))
	require.Equal(t, "FAILED", ticketStatus)
	require.Equal(t, "terminal failure", rejectReason.String)
	require.EqualValues(t, 3, attemptCount)
}

func TestQueries_ResetBatchPowerRetryEvent(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "reset_batch_power_retry_event")
	_, err := pool.Exec(ctx, `
INSERT INTO domain_events (
  id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by
) VALUES
  ('event-parent-power-retry', NOW(), 'BATCH_POWER_REQUESTED', 'batch',
   'ticket-parent-power-retry', '{}'::bytea, 'FAILED', 'requester-1'),
  ('event-child-power-retry', NOW(), 'VM_START_REQUESTED', 'vm',
   'vm-child-power-retry', '{}'::bytea, 'FAILED', 'requester-1');
INSERT INTO tickets (
  id, created_at, updated_at, event_id, operation_type, status, requester,
  parent_ticket_id
) VALUES
  ('ticket-parent-power-retry', NOW(), NOW(), 'event-parent-power-retry',
   'POWER', 'FAILED', 'requester-1', NULL),
  ('ticket-child-power-retry', NOW(), NOW(), 'event-child-power-retry',
   'POWER', 'EXECUTING', 'requester-1', 'ticket-parent-power-retry');
INSERT INTO batch_tickets (
  id, created_at, updated_at, batch_type, child_count, success_count,
  failed_count, pending_count, status, created_by
) VALUES (
  'ticket-parent-power-retry', NOW(), NOW(), 'BATCH_POWER', 1, 0, 1, 0,
  'FAILED', 'requester-1'
)
`)
	require.NoError(t, err)

	params := ResetBatchPowerRetryEventParams{
		EventID:        "event-child-power-retry",
		TicketID:       "ticket-child-power-retry",
		ParentTicketID: pgtype.Text{String: "ticket-parent-power-retry", Valid: true},
	}
	rows, err := q.ResetBatchPowerRetryEvent(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	var eventStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, params.EventID).Scan(&eventStatus))
	require.Equal(t, "PENDING", eventStatus)

	_, err = pool.Exec(ctx, `UPDATE domain_events SET status = 'FAILED', created_by = 'foreign-actor' WHERE id = $1`, params.EventID)
	require.NoError(t, err)
	rows, err = q.ResetBatchPowerRetryEvent(ctx, params)
	require.NoError(t, err)
	require.EqualValues(t, 0, rows)
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id = $1`, params.EventID).Scan(&eventStatus))
	require.Equal(t, "FAILED", eventStatus)
}

func TestQueries_SetVMStatus(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "set_vm_status")

	systemID := "sys-set-vm-status"
	serviceID := "svc-set-vm-status"
	seedSystemAndService(ctx, t, pool, systemID, serviceID)
	seedVM(ctx, t, pool, "vm-status-1", serviceID, "CREATING")

	rows, err := q.SetVMStatus(ctx, SetVMStatusParams{
		ID:     "vm-status-1",
		Status: "RUNNING",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var status string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM vms WHERE id=$1`, "vm-status-1").Scan(&status))
	require.Equal(t, "RUNNING", status)
}

func TestQueries_WithTx(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "with_tx")

	eventID := "event-tx-1"
	seedDomainEvent(ctx, t, pool, eventID, "PENDING")

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)

	qtx := q.WithTx(tx)
	require.NotNil(t, qtx)
	require.NotSame(t, q, qtx)

	rows, err := qtx.SetDomainEventStatus(ctx, SetDomainEventStatusParams{
		ID:     eventID,
		Status: "COMPLETED",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var inTxStatus string
	require.NoError(t, tx.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&inTxStatus))
	require.Equal(t, "COMPLETED", inTxStatus)

	require.NoError(t, tx.Rollback(ctx))

	var persisted string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status FROM domain_events WHERE id=$1`, eventID).Scan(&persisted))
	require.Equal(t, "PENDING", persisted, "rollback must discard updates executed through WithTx")
}

func newSQLCTestQueries(t *testing.T, prefix string) (*Queries, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	testPool := testutil.OpenPGXPool(t, prefix)

	schemaSQL, err := os.ReadFile(sqlcSchemaPath)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(schemaSQL))
	require.NoError(t, err)

	return New(testPool), testPool
}

func seedSystemAndService(ctx context.Context, t *testing.T, pool *pgxpool.Pool, systemID, serviceID string) {
	t.Helper()
	_, err := pool.Exec(
		ctx,
		`INSERT INTO systems (id, created_at, updated_at, name, description, created_by, tenant_id)
         VALUES ($1, NOW(), NOW(), $2, '', 'seed', 'tenant-default')`,
		systemID,
		"system-"+systemID,
	)
	require.NoError(t, err)
	_, err = pool.Exec(
		ctx,
		`INSERT INTO services (id, created_at, updated_at, name, description, next_instance_index, system_services)
         VALUES ($1, NOW(), NOW(), $2, '', $3, $4)`,
		serviceID,
		"service-"+serviceID,
		1,
		systemID,
	)
	require.NoError(t, err)
}

func seedTicket(ctx context.Context, t *testing.T, pool *pgxpool.Pool, ticketID, eventID, opType string) {
	t.Helper()
	_, err := pool.Exec(
		ctx,
		`INSERT INTO tickets (
             id, created_at, updated_at, event_id, operation_type, status, requester,
             approver, reason, reject_reason, selected_cluster_id,
             selected_storage_class, template_snapshot, instance_size_snapshot, modified_spec, parent_ticket_id
         ) VALUES (
             $1, NOW(), NOW(), $2, $3, $4, 'requester-1',
             NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL
         )`,
		ticketID,
		eventID,
		opType,
		"PENDING",
	)
	require.NoError(t, err)
}

func seedDomainEvent(ctx context.Context, t *testing.T, pool *pgxpool.Pool, eventID, status string) {
	t.Helper()
	_, err := pool.Exec(
		ctx,
		`INSERT INTO domain_events (
             id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by, archived_at
         ) VALUES (
             $1, NOW(), 'VM_CREATE_REQUESTED', 'vm', 'agg-1', '{}'::bytea, $2, 'seed', NULL
         )`,
		eventID,
		status,
	)
	require.NoError(t, err)
}

func seedVM(ctx context.Context, t *testing.T, pool *pgxpool.Pool, vmID, serviceID, status string) {
	t.Helper()
	_, err := pool.Exec(
		ctx,
		`INSERT INTO vms (
             id, created_at, updated_at, name, instance, namespace, cluster_id, status, hostname, created_by, ticket_id, service_vms
         ) VALUES (
             $1, NOW(), NOW(), 'vm-name', 'system-service-1', 'dev', NULL, $2, NULL, 'seed', NULL, $3
         )`,
		vmID,
		status,
		serviceID,
	)
	require.NoError(t, err)
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantObj interface{}
	var gotObj interface{}
	require.NoError(t, json.Unmarshal(want, &wantObj))
	require.NoError(t, json.Unmarshal(got, &gotObj))
	require.Equal(t, wantObj, gotObj)
}
