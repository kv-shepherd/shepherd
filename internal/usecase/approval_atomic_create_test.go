package usecase

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestApprovalAtomicWriterApproveCreateAndEnqueue_CommitsDecisionVMAllocationAndJobs(t *testing.T) {
	store := newCreateApprovalAtomicStore(t)
	seedCreateApprovalAtomicRows(t, store.pool, "ticket-create-commit", "event-create-commit", "service-create-commit")

	vmID, vmName, err := approveCreateFixture(
		t.Context(),
		store.writer,
		"ticket-create-commit",
		"event-create-commit",
		"admin-commit",
		"service-create-commit",
		"requester-commit",
	)
	if err != nil {
		t.Fatalf("ApproveCreateAndEnqueue() unexpected error: %v", err)
	}
	if vmID == "" {
		t.Fatal("ApproveCreateAndEnqueue() vmID is empty")
	}
	if vmName != "team-a-orders-frontend-01" {
		t.Fatalf("ApproveCreateAndEnqueue() vmName = %q, want %q", vmName, "team-a-orders-frontend-01")
	}

	assertCreateApprovalCommitted(
		t,
		store.pool,
		"ticket-create-commit",
		"event-create-commit",
		"service-create-commit",
		"admin-commit",
		"requester-commit",
		vmID,
		vmName,
	)
}

func TestApprovalAtomicWriterApproveCreateAndEnqueue_VMInsertFailureRollsBackAllWrites(t *testing.T) {
	store := newCreateApprovalAtomicStore(t)
	seedCreateApprovalAtomicRows(t, store.pool, "ticket-create-vm-fail", "event-create-vm-fail", "service-create-vm-fail")

	if _, err := store.pool.Exec(t.Context(), `
ALTER TABLE vms
ADD CONSTRAINT reject_requester_fail_for_test
CHECK (created_by <> 'requester-fail')
`); err != nil {
		t.Fatalf("install VM failure constraint: %v", err)
	}

	vmID, vmName, err := approveCreateFixture(
		t.Context(),
		store.writer,
		"ticket-create-vm-fail",
		"event-create-vm-fail",
		"admin-vm-fail",
		"service-create-vm-fail",
		"requester-fail",
	)
	if err == nil {
		t.Fatal("ApproveCreateAndEnqueue() error = nil, want VM insert failure")
	}
	if !strings.Contains(err.Error(), "insert vm") {
		t.Fatalf("ApproveCreateAndEnqueue() error = %v, want VM insert context", err)
	}
	if vmID != "" || vmName != "" {
		t.Fatalf("ApproveCreateAndEnqueue() returned (%q, %q) on rollback, want empty values", vmID, vmName)
	}

	assertCreateApprovalRolledBack(
		t,
		store.pool,
		"ticket-create-vm-fail",
		"event-create-vm-fail",
		"service-create-vm-fail",
	)
}

func TestApprovalAtomicWriterApproveCreateAndEnqueue_SecondEnqueueFailureRollsBackAllWrites(t *testing.T) {
	store := newCreateApprovalAtomicStore(t)
	seedCreateApprovalAtomicRows(t, store.pool, "ticket-create-enqueue-fail", "event-create-enqueue-fail", "service-create-enqueue-fail")

	// vm_create is inserted first. Rejecting only vm_status_sync proves that the
	// already-inserted River job and every preceding business write roll back
	// when the second enqueue fails.
	if _, err := store.pool.Exec(t.Context(), `
ALTER TABLE river_job
ADD CONSTRAINT reject_vm_status_sync_for_test
CHECK (kind <> 'vm_status_sync')
`); err != nil {
		t.Fatalf("install River failure constraint: %v", err)
	}

	vmID, vmName, err := approveCreateFixture(
		t.Context(),
		store.writer,
		"ticket-create-enqueue-fail",
		"event-create-enqueue-fail",
		"admin-enqueue-fail",
		"service-create-enqueue-fail",
		"requester-enqueue-fail",
	)
	if err == nil {
		t.Fatal("ApproveCreateAndEnqueue() error = nil, want second enqueue failure")
	}
	if !strings.Contains(err.Error(), "enqueue vm_status_sync") {
		t.Fatalf("ApproveCreateAndEnqueue() error = %v, want vm_status_sync enqueue context", err)
	}
	if vmID != "" || vmName != "" {
		t.Fatalf("ApproveCreateAndEnqueue() returned (%q, %q) on rollback, want empty values", vmID, vmName)
	}

	assertCreateApprovalRolledBack(
		t,
		store.pool,
		"ticket-create-enqueue-fail",
		"event-create-enqueue-fail",
		"service-create-enqueue-fail",
	)
}

func TestApprovalAtomicWriterApproveCreateAndEnqueue_ConcurrentDuplicateCommitsOnce(t *testing.T) {
	store := newCreateApprovalAtomicStore(t)
	seedCreateApprovalAtomicRows(t, store.pool, "ticket-create-concurrent", "event-create-concurrent", "service-create-concurrent")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin concurrent approval blocker transaction: %v", err)
	}
	blockerOpen := true
	t.Cleanup(func() {
		if blockerOpen {
			_ = blocker.Rollback(context.Background())
		}
	})
	if _, lockErr := blocker.Exec(ctx, `SELECT id FROM tickets WHERE id = $1 FOR UPDATE`, "ticket-create-concurrent"); lockErr != nil {
		t.Fatalf("lock concurrent approval ticket: %v", lockErr)
	}
	var blockerPID int32
	if queryErr := blocker.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); queryErr != nil {
		t.Fatalf("query concurrent approval blocker pid: %v", queryErr)
	}

	type result struct {
		approver string
		vmID     string
		vmName   string
		err      error
	}

	start := make(chan struct{})
	results := make(chan result, 2)
	var workers errgroup.Group
	for _, approver := range []string{"admin-concurrent-a", "admin-concurrent-b"} {
		approver := approver
		workers.Go(func() error {
			<-start
			vmID, vmName, approvalErr := approveCreateFixture(
				ctx,
				store.writer,
				"ticket-create-concurrent",
				"event-create-concurrent",
				approver,
				"service-create-concurrent",
				"requester-concurrent",
			)
			results <- result{approver: approver, vmID: vmID, vmName: vmName, err: approvalErr}
			return nil
		})
	}
	close(start)

	blockedApprovals := 0
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
`, blockerPID).Scan(&blockedApprovals)
		return blockedQueryErr != nil || blockedApprovals == 2
	}, 5*time.Second, 10*time.Millisecond, "concurrent approvals did not both block on the ticket lock")
	if rollbackErr := blocker.Rollback(ctx); rollbackErr != nil {
		t.Fatalf("release concurrent approval ticket lock: %v", rollbackErr)
	}
	blockerOpen = false
	if waitErr := workers.Wait(); waitErr != nil {
		t.Fatalf("wait for concurrent approvals: %v", waitErr)
	}
	close(results)
	require.NoError(t, blockedQueryErr, "query blocked concurrent approvals")
	if blockedApprovals != 2 {
		t.Fatalf("blocked concurrent approvals = %d, want 2 before releasing ticket lock", blockedApprovals)
	}

	var successes []result
	var failures []result
	for got := range results {
		if got.err == nil {
			successes = append(successes, got)
		} else {
			failures = append(failures, got)
		}
	}
	if len(successes) != 1 || len(failures) != 1 {
		t.Fatalf("concurrent approvals produced %d successes and %d failures, want one each: successes=%+v failures=%+v", len(successes), len(failures), successes, failures)
	}
	if !strings.Contains(failures[0].err.Error(), "not pending or operation type mismatch") {
		t.Fatalf("losing approval error = %v, want pending-state conflict", failures[0].err)
	}
	if failures[0].vmID != "" || failures[0].vmName != "" {
		t.Fatalf("losing approval returned (%q, %q), want empty values", failures[0].vmID, failures[0].vmName)
	}

	winner := successes[0]
	assertCreateApprovalCommitted(
		t,
		store.pool,
		"ticket-create-concurrent",
		"event-create-concurrent",
		"service-create-concurrent",
		winner.approver,
		"requester-concurrent",
		winner.vmID,
		winner.vmName,
	)
}

type createApprovalAtomicStore struct {
	pool   *pgxpool.Pool
	writer *ApprovalAtomicWriter
}

func newCreateApprovalAtomicStore(t *testing.T) *createApprovalAtomicStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, "r")
	schemaSQL, err := os.ReadFile("../repository/sqlc/schema.sql")
	if err != nil {
		t.Fatalf("read sqlc schema: %v", err)
	}
	if _, execErr := pool.Exec(t.Context(), string(schemaSQL)); execErr != nil {
		t.Fatalf("apply sqlc schema: %v", execErr)
	}

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create River migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate River schema: %v", migrateErr)
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create River client: %v", err)
	}

	return &createApprovalAtomicStore{
		pool:   pool,
		writer: NewApprovalAtomicWriter(pool, riverClient),
	}
}

func seedCreateApprovalAtomicRows(t *testing.T, pool *pgxpool.Pool, ticketID, eventID, serviceID string) {
	t.Helper()

	if _, err := pool.Exec(t.Context(), `
INSERT INTO systems (id, created_at, updated_at, name, description, created_by, tenant_id)
VALUES ('system-create', NOW(), NOW(), 'orders', '', 'seed', 'tenant-default')
`); err != nil {
		t.Fatalf("insert system: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO services (id, created_at, updated_at, name, description, next_instance_index, system_services)
VALUES ($1, NOW(), NOW(), 'frontend', '', 1, 'system-create')
`, serviceID); err != nil {
		t.Fatalf("insert service: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO domain_events (id, created_at, event_type, aggregate_type, aggregate_id, payload, status, created_by)
VALUES ($1, NOW(), 'VM_CREATE_REQUESTED', 'vm', $2, '{}'::bytea, 'PENDING', 'requester-seed')
`, eventID, ticketID); err != nil {
		t.Fatalf("insert domain event: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `
INSERT INTO tickets (id, created_at, updated_at, event_id, operation_type, status, requester, reason)
VALUES ($1, NOW(), NOW(), $2, 'CREATE', 'PENDING', 'requester-seed', 'create vm')
`, ticketID, eventID); err != nil {
		t.Fatalf("insert ticket: %v", err)
	}
}

func approveCreateFixture(
	ctx context.Context,
	writer *ApprovalAtomicWriter,
	ticketID, eventID, approver, serviceID, requesterID string,
) (vmID, vmName string, err error) {
	return writer.ApproveCreateAndEnqueue(
		ctx,
		ticketID,
		eventID,
		approver,
		"cluster-a",
		" fast-rwx ",
		serviceID,
		"team-a",
		requesterID,
		map[string]interface{}{
			"template_id": "template-1",
			"version":     3,
		},
		map[string]interface{}{
			"cpu_cores":       4,
			"memory_gi":       8,
			"dv_access_modes": []interface{}{"ReadWriteMany", "  ", "ReadWriteOnce"},
			"dv_volume_mode":  " Block ",
		},
		map[string]interface{}{
			"selected_cluster_id": "cluster-a",
			"eligible":            true,
		},
		map[string]interface{}{
			"cpu_cores": 6,
			"disk_gb":   80,
		},
	)
}

func assertCreateApprovalCommitted(
	t *testing.T,
	pool *pgxpool.Pool,
	ticketID, eventID, serviceID, approver, requesterID, vmID, vmName string,
) {
	t.Helper()

	var (
		ticketStatus     string
		gotApprover      string
		clusterID        string
		storageClass     string
		templateJSON     []byte
		instanceJSON     []byte
		placementJSON    []byte
		modifiedSpecJSON []byte
	)
	if err := pool.QueryRow(t.Context(), `
SELECT status, approver, selected_cluster_id, selected_storage_class,
       template_snapshot, instance_size_snapshot, placement_evaluation, modified_spec
FROM tickets
WHERE id = $1
`, ticketID).Scan(
		&ticketStatus,
		&gotApprover,
		&clusterID,
		&storageClass,
		&templateJSON,
		&instanceJSON,
		&placementJSON,
		&modifiedSpecJSON,
	); err != nil {
		t.Fatalf("query committed ticket: %v", err)
	}
	if ticketStatus != "APPROVED" || gotApprover != approver {
		t.Fatalf("ticket decision = (%q, %q), want (APPROVED, %q)", ticketStatus, gotApprover, approver)
	}
	if clusterID != "cluster-a" || storageClass != "fast-rwx" {
		t.Fatalf("ticket placement = (%q, %q), want (cluster-a, fast-rwx)", clusterID, storageClass)
	}
	assertCreateApprovalJSONEqual(t, []byte(`{"template_id":"template-1","version":3}`), templateJSON)
	assertCreateApprovalJSONEqual(t, []byte(`{"cpu_cores":4,"memory_gi":8,"dv_access_modes":["ReadWriteMany","  ","ReadWriteOnce"],"dv_volume_mode":" Block "}`), instanceJSON)
	assertCreateApprovalJSONEqual(t, []byte(`{"selected_cluster_id":"cluster-a","eligible":true}`), placementJSON)
	assertCreateApprovalJSONEqual(t, []byte(`{"cpu_cores":6,"disk_gb":80}`), modifiedSpecJSON)

	var eventStatus string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query committed event: %v", err)
	}
	if eventStatus != "PROCESSING" {
		t.Fatalf("event status = %q, want PROCESSING", eventStatus)
	}

	var nextInstanceIndex int
	if err := pool.QueryRow(t.Context(), `SELECT next_instance_index FROM services WHERE id = $1`, serviceID).Scan(&nextInstanceIndex); err != nil {
		t.Fatalf("query committed service allocation: %v", err)
	}
	if nextInstanceIndex != 2 {
		t.Fatalf("service next_instance_index = %d, want 2", nextInstanceIndex)
	}

	var (
		gotVMName           string
		instance            string
		namespace           string
		vmClusterID         string
		vmStatus            string
		hostname            string
		createdBy           string
		vmTicketID          string
		rootStorageClass    string
		rootAccessModesJSON []byte
		rootVolumeMode      string
		gotServiceID        string
	)
	if err := pool.QueryRow(t.Context(), `
SELECT name, instance, namespace, cluster_id, status, hostname, created_by, ticket_id,
       root_volume_storage_class, root_volume_access_modes, root_volume_volume_mode, service_vms
FROM vms
WHERE id = $1
`, vmID).Scan(
		&gotVMName,
		&instance,
		&namespace,
		&vmClusterID,
		&vmStatus,
		&hostname,
		&createdBy,
		&vmTicketID,
		&rootStorageClass,
		&rootAccessModesJSON,
		&rootVolumeMode,
		&gotServiceID,
	); err != nil {
		t.Fatalf("query committed VM: %v", err)
	}
	if gotVMName != vmName || instance != "01" || namespace != "team-a" {
		t.Fatalf("VM identity = (%q, %q, %q), want (%q, 01, team-a)", gotVMName, instance, namespace, vmName)
	}
	if vmClusterID != "cluster-a" || vmStatus != "CREATING" || hostname != vmName {
		t.Fatalf("VM placement/status = (%q, %q, %q), want (cluster-a, CREATING, %q)", vmClusterID, vmStatus, hostname, vmName)
	}
	if createdBy != requesterID || vmTicketID != ticketID || gotServiceID != serviceID {
		t.Fatalf("VM relations = created_by:%q ticket:%q service:%q, want %q/%q/%q", createdBy, vmTicketID, gotServiceID, requesterID, ticketID, serviceID)
	}
	if rootStorageClass != "fast-rwx" || rootVolumeMode != "Block" {
		t.Fatalf("VM root volume = storage:%q mode:%q, want fast-rwx/Block", rootStorageClass, rootVolumeMode)
	}
	assertCreateApprovalJSONEqual(t, []byte(`["ReadWriteMany","ReadWriteOnce"]`), rootAccessModesJSON)

	assertCreateApprovalRiverJob(t, pool, "vm_create", "vm_operations", eventID)
	assertCreateApprovalRiverJob(t, pool, jobs.VMStatusSyncJobKind, jobs.VMStatusSyncJobKind, eventID)

	var jobCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job`).Scan(&jobCount); err != nil {
		t.Fatalf("count committed River jobs: %v", err)
	}
	if jobCount != 2 {
		t.Fatalf("River job count = %d, want 2", jobCount)
	}
}

func assertCreateApprovalRiverJob(t *testing.T, pool *pgxpool.Pool, kind, queue, eventID string) {
	t.Helper()

	var (
		gotQueue    string
		maxAttempts int
		gotEventID  string
	)
	if err := pool.QueryRow(t.Context(), `
SELECT queue, max_attempts, args->>'event_id'
FROM river_job
WHERE kind = $1
`, kind).Scan(&gotQueue, &maxAttempts, &gotEventID); err != nil {
		t.Fatalf("query %s River job: %v", kind, err)
	}
	if gotQueue != queue || maxAttempts != 3 || gotEventID != eventID {
		t.Fatalf("%s River job = queue:%q attempts:%d event:%q, want %q/3/%q", kind, gotQueue, maxAttempts, gotEventID, queue, eventID)
	}
}

func assertCreateApprovalRolledBack(t *testing.T, pool *pgxpool.Pool, ticketID, eventID, serviceID string) {
	t.Helper()

	var (
		ticketStatus string
		approver     *string
		clusterID    *string
		storageClass *string
		templateJSON []byte
	)
	if err := pool.QueryRow(t.Context(), `
SELECT status, approver, selected_cluster_id, selected_storage_class, template_snapshot
FROM tickets
WHERE id = $1
`, ticketID).Scan(&ticketStatus, &approver, &clusterID, &storageClass, &templateJSON); err != nil {
		t.Fatalf("query rolled-back ticket: %v", err)
	}
	if ticketStatus != "PENDING" || approver != nil || clusterID != nil || storageClass != nil || templateJSON != nil {
		t.Fatalf("ticket was partially committed: status=%q approver=%v cluster=%v storage=%v template=%s", ticketStatus, approver, clusterID, storageClass, templateJSON)
	}

	var eventStatus string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM domain_events WHERE id = $1`, eventID).Scan(&eventStatus); err != nil {
		t.Fatalf("query rolled-back event: %v", err)
	}
	if eventStatus != "PENDING" {
		t.Fatalf("event status = %q after rollback, want PENDING", eventStatus)
	}

	var nextInstanceIndex int
	if err := pool.QueryRow(t.Context(), `SELECT next_instance_index FROM services WHERE id = $1`, serviceID).Scan(&nextInstanceIndex); err != nil {
		t.Fatalf("query rolled-back service allocation: %v", err)
	}
	if nextInstanceIndex != 1 {
		t.Fatalf("service next_instance_index = %d after rollback, want 1", nextInstanceIndex)
	}

	var vmCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM vms`).Scan(&vmCount); err != nil {
		t.Fatalf("count VMs after rollback: %v", err)
	}
	if vmCount != 0 {
		t.Fatalf("VM count after rollback = %d, want 0", vmCount)
	}

	var jobCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job`).Scan(&jobCount); err != nil {
		t.Fatalf("count River jobs after rollback: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("River job count after rollback = %d, want 0", jobCount)
	}
}

func assertCreateApprovalJSONEqual(t *testing.T, want, got []byte) {
	t.Helper()

	var wantValue interface{}
	if err := json.Unmarshal(want, &wantValue); err != nil {
		t.Fatalf("unmarshal expected JSON %q: %v", want, err)
	}
	var gotValue interface{}
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal actual JSON %q: %v", got, err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON = %s, want %s", got, want)
	}
}
