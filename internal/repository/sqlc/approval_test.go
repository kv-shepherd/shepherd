package sqlc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

const sqlcSchemaPath = "schema.sql"

var nonIdentChars = regexp.MustCompile(`[^a-z0-9_]+`)

func TestQueries_AllocateServiceInstance(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "allocate_service_instance")

	systemID := "sys-allocate"
	serviceID := "svc-allocate"
	seedSystemAndService(t, ctx, pool, systemID, serviceID, 1)

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
	seedApprovalTicket(t, ctx, pool, ticketID, eventID, "CREATE", "PENDING")

	templateSnapshot := []byte(`{"name":"tpl-v3"}`)
	instanceSizeSnapshot := []byte(`{"cpu_cores":4,"memory_mb":8192}`)
	modifiedSpec := []byte(`{"cpu":4}`)

	rows, err := q.ApproveCreateTicket(ctx, ApproveCreateTicketParams{
		Approver:                pgtype.Text{String: "admin-1", Valid: true},
		SelectedClusterID:       pgtype.Text{String: "cluster-a", Valid: true},
		SelectedTemplateVersion: pgtype.Int4{Int32: 3, Valid: true},
		SelectedStorageClass:    "fast",
		TemplateSnapshot:        templateSnapshot,
		InstanceSizeSnapshot:    instanceSizeSnapshot,
		ModifiedSpec:            modifiedSpec,
		ID:                      ticketID,
		EventID:                 eventID,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var (
		status          string
		approver        pgtype.Text
		clusterID       pgtype.Text
		templateVersion pgtype.Int4
		storageClass    pgtype.Text
		gotTemplate     []byte
		gotSize         []byte
		gotModified     []byte
	)
	require.NoError(t, pool.QueryRow(
		ctx,
		`SELECT status, approver, selected_cluster_id, selected_template_version, selected_storage_class, template_snapshot, instance_size_snapshot, modified_spec
         FROM approval_tickets WHERE id=$1`,
		ticketID,
	).Scan(
		&status,
		&approver,
		&clusterID,
		&templateVersion,
		&storageClass,
		&gotTemplate,
		&gotSize,
		&gotModified,
	))

	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-1", approver.String)
	require.True(t, clusterID.Valid)
	require.Equal(t, "cluster-a", clusterID.String)
	require.True(t, templateVersion.Valid)
	require.EqualValues(t, 3, templateVersion.Int32)
	require.True(t, storageClass.Valid)
	require.Equal(t, "fast", storageClass.String)
	assertJSONEqual(t, templateSnapshot, gotTemplate)
	assertJSONEqual(t, instanceSizeSnapshot, gotSize)
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
	seedApprovalTicket(t, ctx, pool, ticketID, eventID, "DELETE", "PENDING")

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
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, approver FROM approval_tickets WHERE id=$1`, ticketID).Scan(&status, &approver))
	require.Equal(t, "APPROVED", status)
	require.True(t, approver.Valid)
	require.Equal(t, "admin-delete", approver.String)
}

func TestQueries_InsertVM(t *testing.T) {
	ctx := context.Background()
	q, pool := newSQLCTestQueries(t, "insert_vm")

	systemID := "sys-insert-vm"
	serviceID := "svc-insert-vm"
	seedSystemAndService(t, ctx, pool, systemID, serviceID, 1)

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
	seedDomainEvent(t, ctx, pool, eventID, "PENDING")

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
	seedSystemAndService(t, ctx, pool, systemID, serviceID, 1)
	seedVM(t, ctx, pool, "vm-status-1", serviceID, "CREATING")

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
	seedDomainEvent(t, ctx, pool, eventID, "PENDING")

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
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatalf("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	adminPool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, adminPool.Ping(ctx))

	schema := newSchemaName(prefix)
	_, err = adminPool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	require.NoError(t, err)

	schemaDSN, err := dsnWithSearchPath(dsn, schema)
	require.NoError(t, err)

	testPool, err := pgxpool.New(ctx, schemaDSN)
	require.NoError(t, err)
	require.NoError(t, testPool.Ping(ctx))

	schemaSQL, err := os.ReadFile(sqlcSchemaPath)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(schemaSQL))
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
		adminPool.Close()
	})
	t.Cleanup(testPool.Close)

	return New(testPool), testPool
}

func seedSystemAndService(t *testing.T, ctx context.Context, pool *pgxpool.Pool, systemID, serviceID string, nextIndex int32) {
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

func seedApprovalTicket(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ticketID, eventID, opType, status string) {
	t.Helper()
	_, err := pool.Exec(
		ctx,
		`INSERT INTO approval_tickets (
             id, created_at, updated_at, event_id, operation_type, status, requester,
             approver, reason, reject_reason, selected_cluster_id, selected_template_version,
             selected_storage_class, template_snapshot, instance_size_snapshot, modified_spec, parent_ticket_id
         ) VALUES (
             $1, NOW(), NOW(), $2, $3, $4, 'requester-1',
             NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL
         )`,
		ticketID,
		eventID,
		opType,
		status,
	)
	require.NoError(t, err)
}

func seedDomainEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID, status string) {
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

func seedVM(t *testing.T, ctx context.Context, pool *pgxpool.Pool, vmID, serviceID, status string) {
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

func dsnWithSearchPath(dsn, schema string) (string, error) {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	if strings.Contains(dsn, "search_path=") {
		re := regexp.MustCompile(`search_path=\S+`)
		return re.ReplaceAllString(dsn, "search_path="+schema), nil
	}
	return dsn + " search_path=" + schema, nil
}

func newSchemaName(prefix string) string {
	base := strings.ToLower(prefix)
	base = strings.ReplaceAll(base, "-", "_")
	base = nonIdentChars.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "sqlc"
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	const maxPostgresIdentLen = 63
	maxBaseLen := maxPostgresIdentLen - len("t__") - len(suffix)
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	return fmt.Sprintf("t_%s_%s", base, suffix)
}

func assertJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()
	var wantObj interface{}
	var gotObj interface{}
	require.NoError(t, json.Unmarshal(want, &wantObj))
	require.NoError(t, json.Unmarshal(got, &gotObj))
	require.Equal(t, wantObj, gotObj)
}
