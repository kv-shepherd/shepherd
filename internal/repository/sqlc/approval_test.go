package sqlc

import (
	"context"
	"encoding/json"
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
	seedSystemAndService(ctx, t, pool, systemID, serviceID, 1)

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

	rows, err := q.ApproveDeleteTicket(ctx, ApproveDeleteTicketParams{
		Approver: pgtype.Text{String: "admin-delete", Valid: true},
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
	require.Equal(t, "admin-delete", approver.String)
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
	seedSystemAndService(ctx, t, pool, systemID, serviceID, 1)

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
}

func TestQueries_SetVMStatus(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "set_vm_status")

	systemID := "sys-set-vm-status"
	serviceID := "svc-set-vm-status"
	seedSystemAndService(ctx, t, pool, systemID, serviceID, 1)
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

func seedSystemAndService(ctx context.Context, t *testing.T, pool *pgxpool.Pool, systemID, serviceID string, nextIndex int32) {
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
		nextIndex,
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
