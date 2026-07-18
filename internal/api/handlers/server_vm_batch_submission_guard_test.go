package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func TestBatchSubmissionGuard_ConcurrentDifferentRequestIDsCannotBypassCooldown(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmIDs := []string{
		mustCreateBatchDeleteTargetVM(t, client),
		mustCreateBatchDeleteTargetVM(t, client),
	}
	releaseGuard, blockerPID := holdBatchSubmissionGuard(t, srv.pool)

	responses := make([]*httptest.ResponseRecorder, len(vmIDs))
	done := make([]<-chan struct{}, 0, len(vmIDs))
	for i, vmID := range vmIDs {
		body := mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchSubmitOperation("DELETE"),
			RequestId: "guard-request-" + uuid.NewString(),
			Items:     []generated.VMBatchChildItem{{VmId: vmID}},
		})
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch",
			body,
			"owner-1",
			[]string{"platform:admin"},
		)
		responses[i] = response
		done = append(done, runHandlerAsync(func() { srv.SubmitVMBatch(requestContext) }))
	}

	// The server-local gate keeps the second request out of the database pool;
	// only the current leader should wait on the cross-process advisory lock.
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	releaseGuard()
	waitForBatchHandlers(t, done)
	assertOneAcceptedOneRateLimited(t, responses)

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded parent tickets: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded child tickets: %v", err)
	}
	projectionCount, err := client.BatchTicket.Query().Where(batchticket.RequestIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded batch projections: %v", err)
	}
	if parentCount != 1 || childCount != 1 || projectionCount != 1 {
		t.Fatalf(
			"guarded side effects = parents:%d children:%d projections:%d, want 1/1/1",
			parentCount,
			childCount,
			projectionCount,
		)
	}
}

func TestBatchSubmissionGuard_ConcurrentRequestsWithoutRequestIDsCannotBypassCooldown(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmIDs := []string{
		mustCreateBatchDeleteTargetVM(t, client),
		mustCreateBatchDeleteTargetVM(t, client),
	}
	releaseGuard, blockerPID := holdBatchSubmissionGuard(t, srv.pool)

	responses := make([]*httptest.ResponseRecorder, len(vmIDs))
	done := make([]<-chan struct{}, 0, len(vmIDs))
	for i, vmID := range vmIDs {
		body := mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchSubmitOperation("DELETE"),
			Items:     []generated.VMBatchChildItem{{VmId: vmID}},
		})
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch",
			body,
			"owner-1",
			[]string{"platform:admin"},
		)
		responses[i] = response
		done = append(done, runHandlerAsync(func() { srv.SubmitVMBatch(requestContext) }))
	}

	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	releaseGuard()
	waitForBatchHandlers(t, done)
	assertOneAcceptedOneRateLimited(t, responses)

	projectionCount, err := client.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count unkeyed guarded batch projections: %v", err)
	}
	keyedProjectionCount, err := client.BatchTicket.Query().Where(batchticket.RequestIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count keyed projections after unkeyed guarded submissions: %v", err)
	}
	if projectionCount != 1 || keyedProjectionCount != 0 {
		t.Fatalf("unkeyed guarded projections = total:%d keyed:%d, want 1/0", projectionCount, keyedProjectionCount)
	}
}

func TestBatchSubmissionGuard_ConcurrentPowerRequestIDsCannotBypassCooldown(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	firstVMID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	vmIDs := []string{firstVMID, mustCloneBatchPowerTargetVM(t, client, firstVMID)}
	releaseGuard, blockerPID := holdBatchSubmissionGuard(t, srv.pool)

	responses := make([]*httptest.ResponseRecorder, len(vmIDs))
	done := make([]<-chan struct{}, 0, len(vmIDs))
	for i, vmID := range vmIDs {
		body := mustJSON(t, generated.VMBatchPowerRequest{
			Operation: generated.VMBatchPowerAction("start"),
			RequestId: "power-guard-request-" + uuid.NewString(),
			Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
		})
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch/power",
			body,
			"owner-1",
			[]string{"platform:admin"},
		)
		responses[i] = response
		done = append(done, runHandlerAsync(func() { srv.SubmitVMBatchPower(requestContext) }))
	}

	// The server-local gate keeps the second request out of the database pool;
	// only the current leader should wait on the cross-process advisory lock.
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	releaseGuard()
	waitForBatchHandlers(t, done)
	assertOneAcceptedOneRateLimited(t, responses)

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded power parent tickets: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded power child tickets: %v", err)
	}
	projectionCount, err := client.BatchTicket.Query().Where(batchticket.RequestIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count guarded power batch projections: %v", err)
	}
	var jobCount int
	if err := srv.pool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM river_job WHERE kind = 'vm_power'`,
	).Scan(&jobCount); err != nil {
		t.Fatalf("count guarded power jobs: %v", err)
	}
	if parentCount != 1 || childCount != 1 || projectionCount != 1 || jobCount != 1 {
		t.Fatalf(
			"guarded power side effects = parents:%d children:%d projections:%d jobs:%d, want 1/1/1/1",
			parentCount,
			childCount,
			projectionCount,
			jobCount,
		)
	}
}

func TestBatchSubmissionGuard_PowerSubmissionCompletesWithSinglePooledConnection(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)

	limitedPool, _ := rebindBatchServerToSingleConnectionPool(t, srv)
	srv.riverClient = newBatchBehaviorTestRiverClient(t, limitedPool)

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		RequestId: "power-single-connection-" + uuid.NewString(),
		Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
	})
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	requestCtx, cancel := context.WithTimeout(requestContext.Request.Context(), 5*time.Second)
	defer cancel()
	requestContext.Request = requestContext.Request.WithContext(requestCtx)

	srv.SubmitVMBatchPower(requestContext)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusAccepted, response.Body.String())
	}
	var jobCount int
	if err := limitedPool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM river_job WHERE kind = 'vm_power'`,
	).Scan(&jobCount); err != nil {
		t.Fatalf("count power jobs after single-connection submit: %v", err)
	}
	if jobCount != 1 {
		t.Fatalf("power job count = %d, want 1", jobCount)
	}
}

func TestBatchSubmissionGuard_GenericSubmissionCompletesWithSinglePooledConnection(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	_, limitedClient := rebindBatchServerToSingleConnectionPool(t, srv)

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		RequestId: "generic-single-connection-" + uuid.NewString(),
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	requestCtx, cancel := context.WithTimeout(requestContext.Request.Context(), 5*time.Second)
	defer cancel()
	requestContext.Request = requestContext.Request.WithContext(requestCtx)

	srv.SubmitVMBatch(requestContext)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusAccepted, response.Body.String())
	}
	projectionCount, err := limitedClient.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count generic batch projections after single-connection submit: %v", err)
	}
	if projectionCount != 1 {
		t.Fatalf("generic batch projection count = %d, want 1", projectionCount)
	}
}

func TestBatchSubmissionGuard_SeparateServersSerializeGenericAndPowerSubmissions(t *testing.T) {
	firstServer, client := newBatchBehaviorTestServerWithRiver(t)
	secondServer := NewServer(ServerDeps{
		EntClient:    client,
		Pool:         firstServer.pool,
		RiverClient:  firstServer.riverClient,
		ApprovalReqs: service.NewApprovalRequirementService(client),
	})
	deleteVMID := mustCreateBatchDeleteTargetVM(t, client)
	powerVMID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)

	genericBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		RequestId: "separate-server-generic-" + uuid.NewString(),
		Items:     []generated.VMBatchChildItem{{VmId: deleteVMID}},
	})
	powerBody := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		RequestId: "separate-server-power-" + uuid.NewString(),
		Items:     []generated.VMBatchPowerItem{{VmId: powerVMID}},
	})
	genericContext, genericResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		genericBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	powerContext, powerResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		powerBody,
		"owner-1",
		[]string{"platform:admin"},
	)

	start := make(chan struct{})
	done := []<-chan struct{}{
		runHandlerAsync(func() {
			<-start
			firstServer.SubmitVMBatch(genericContext)
		}),
		runHandlerAsync(func() {
			<-start
			secondServer.SubmitVMBatchPower(powerContext)
		}),
	}
	close(start)
	waitForBatchHandlers(t, done)
	assertOneAcceptedOneRateLimited(t, []*httptest.ResponseRecorder{genericResponse, powerResponse})

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count cross-server batch parents: %v", err)
	}
	if parentCount != 1 {
		t.Fatalf("cross-server parent count = %d, want 1", parentCount)
	}
}

func TestBatchSubmissionGuard_GenericWaitsForPolicyMutationAndReadsCommittedOverride(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	if _, err := client.DomainEvent.Create().
		SetID("policy-race-seed-" + uuid.NewString()).
		SetEventType(string(domain.EventBatchDeleteRequested)).
		SetAggregateType("batch").
		SetAggregateID("policy-race-batch-" + uuid.NewString()).
		SetPayload([]byte(`{"seed":true}`)).
		SetStatus("COMPLETED").
		SetCreatedBy("owner-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed cooldown event: %v", err)
	}
	repeatableReadPool, _ := rebindBatchServerToRepeatableReadPool(t, srv)

	policyTx, blockerPID := beginUserMutationBlocker(t, repeatableReadPool, "owner-1")
	policyTxClosed := false
	t.Cleanup(func() {
		if !policyTxClosed {
			_ = policyTx.Rollback(context.Background())
		}
	})
	if _, err := policyTx.Exec(t.Context(), `
INSERT INTO rate_limit_user_overrides (
  id, created_at, updated_at, cooldown_seconds, updated_by
) VALUES ($1, NOW(), NOW(), 0, $2)
ON CONFLICT (id) DO UPDATE
SET cooldown_seconds = EXCLUDED.cooldown_seconds,
    updated_at = NOW(),
    updated_by = EXCLUDED.updated_by
`, "owner-1", "admin-1"); err != nil {
		t.Fatalf("stage cooldown override: %v", err)
	}

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		RequestId: "policy-race-" + uuid.NewString(),
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	done := runHandlerAsync(func() { srv.SubmitVMBatch(requestContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	if err := policyTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit cooldown override: %v", err)
	}
	policyTxClosed = true
	waitForHandlerCompletion(t, done, "batch waiting for policy mutation")

	if response.Code != http.StatusAccepted {
		t.Fatalf(
			"status after committed cooldown override = %d, want %d body=%s",
			response.Code,
			http.StatusAccepted,
			response.Body.String(),
		)
	}
}

func TestBatchSubmissionGuard_PowerRejectsActorDeletedBeforeMutationLock(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	repeatableReadPool, _ := rebindBatchServerToRepeatableReadPool(t, srv)
	srv.riverClient = newBatchBehaviorTestRiverClient(t, repeatableReadPool)

	deleteTx, blockerPID := beginUserMutationBlocker(t, repeatableReadPool, "owner-1")
	deleteTxClosed := false
	t.Cleanup(func() {
		if !deleteTxClosed {
			_ = deleteTx.Rollback(context.Background())
		}
	})
	if _, err := deleteTx.Exec(t.Context(), `DELETE FROM users WHERE id = $1`, "owner-1"); err != nil {
		t.Fatalf("stage batch actor delete: %v", err)
	}

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		RequestId: "deleted-actor-power-" + uuid.NewString(),
		Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
	})
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	done := runHandlerAsync(func() { srv.SubmitVMBatchPower(requestContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	if err := deleteTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit batch actor delete: %v", err)
	}
	deleteTxClosed = true
	waitForHandlerCompletion(t, done, "power batch waiting for actor delete")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf(
			"status after actor delete = %d, want %d body=%s",
			response.Code,
			http.StatusUnauthorized,
			response.Body.String(),
		)
	}
	projectionCount, err := client.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count projections after actor delete: %v", err)
	}
	if projectionCount != 0 {
		t.Fatalf("projection count after actor delete = %d, want 0", projectionCount)
	}
	var jobCount int
	if err := srv.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'vm_power'`).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs after actor delete: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("power job count after actor delete = %d, want 0", jobCount)
	}
}

func TestBatchSubmissionGuard_PowerWaitsForPolicyDeletionAndReadsCommittedDefault(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	if _, err := client.DomainEvent.Create().
		SetID("policy-delete-seed-" + uuid.NewString()).
		SetEventType(string(domain.EventBatchPowerRequested)).
		SetAggregateType("batch").
		SetAggregateID("policy-delete-batch-" + uuid.NewString()).
		SetPayload([]byte(`{"seed":true}`)).
		SetStatus("COMPLETED").
		SetCreatedBy("owner-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed power cooldown event: %v", err)
	}
	if _, err := client.RateLimitUserOverride.Create().
		SetID("owner-1").
		SetCooldownSeconds(0).
		SetUpdatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed zero-cooldown override: %v", err)
	}

	repeatableReadPool, repeatableReadClient := rebindBatchServerToRepeatableReadPool(t, srv)
	srv.riverClient = newBatchBehaviorTestRiverClient(t, repeatableReadPool)
	deleteTx, blockerPID := beginUserMutationBlocker(t, repeatableReadPool, "owner-1")
	deleteTxClosed := false
	t.Cleanup(func() {
		if !deleteTxClosed {
			_ = deleteTx.Rollback(context.Background())
		}
	})
	if _, err := deleteTx.Exec(
		t.Context(),
		`DELETE FROM rate_limit_user_overrides WHERE id = $1`,
		"owner-1",
	); err != nil {
		t.Fatalf("stage cooldown override deletion: %v", err)
	}

	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("start"),
		RequestId: "deleted-policy-power-" + uuid.NewString(),
		Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
	})
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	done := runHandlerAsync(func() { srv.SubmitVMBatchPower(requestContext) })
	waitForBlockedAdvisoryCalls(t, repeatableReadPool, blockerPID, 1)
	if err := deleteTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit cooldown override deletion: %v", err)
	}
	deleteTxClosed = true
	waitForHandlerCompletion(t, done, "power batch waiting for policy deletion")

	if response.Code != http.StatusTooManyRequests {
		t.Fatalf(
			"status after committed cooldown override deletion = %d, want %d body=%s",
			response.Code,
			http.StatusTooManyRequests,
			response.Body.String(),
		)
	}
	projectionCount, err := repeatableReadClient.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count projections after policy deletion: %v", err)
	}
	if projectionCount != 0 {
		t.Fatalf("projection count after policy deletion = %d, want 0", projectionCount)
	}
	var jobCount int
	if err := repeatableReadPool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM river_job WHERE kind = 'vm_power'`,
	).Scan(&jobCount); err != nil {
		t.Fatalf("count jobs after policy deletion: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("power job count after policy deletion = %d, want 0", jobCount)
	}
}

func TestAcquireBatchSubmissionGuard_CanceledLocalWaiterReturnsBeforeLeaderReleases(t *testing.T) {
	srv, _ := newBatchBehaviorTestServer(t)
	releaseLeader, err := srv.acquireBatchSubmissionGuard(t.Context())
	if err != nil {
		t.Fatalf("acquire leader batch submission guard: %v", err)
	}
	defer releaseLeader()

	waiterCtx, cancelWaiter := context.WithCancel(t.Context())
	waiterResult := make(chan error, 1)
	runHandlerAsync(func() {
		releaseWaiter, acquireErr := srv.acquireBatchSubmissionGuard(waiterCtx)
		if releaseWaiter != nil {
			releaseWaiter()
		}
		waiterResult <- acquireErr
	})
	cancelWaiter()

	select {
	case acquireErr := <-waiterResult:
		if !errors.Is(acquireErr, context.Canceled) {
			t.Fatalf("canceled waiter error = %v, want context canceled", acquireErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled batch submission waiter did not return while leader still held the gate")
	}
}

func TestBatchSubmissionGuard_CanceledDatabaseWaiterReleasesTransactionAndLocalGate(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	releaseBlocker, blockerPID := holdBatchSubmissionGuard(t, srv.pool)

	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		RequestId: "canceled-database-waiter-" + uuid.NewString(),
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})
	requestContext, _ := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	waiterCtx, cancelWaiter := context.WithCancel(requestContext.Request.Context())
	requestContext.Request = requestContext.Request.WithContext(waiterCtx)
	done := runHandlerAsync(func() { srv.SubmitVMBatch(requestContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	cancelWaiter()
	waitForHandlerCompletion(t, done, "canceled database-lock waiter")

	if got := len(srv.batchSubmissionGate); got != 0 {
		t.Fatalf("local batch submission gate length = %d after cancellation, want 0", got)
	}
	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count parents after canceled database-lock waiter: %v", err)
	}
	if parentCount != 0 {
		t.Fatalf("parent count after canceled database-lock waiter = %d, want 0", parentCount)
	}

	releaseBlocker()
	freshContext, freshResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(freshContext)
	if freshResponse.Code != http.StatusAccepted {
		t.Fatalf(
			"fresh submit after canceled waiter status = %d, want %d body=%s",
			freshResponse.Code,
			http.StatusAccepted,
			freshResponse.Body.String(),
		)
	}
}

func TestLockBatchSubmissionTransaction_ReleasesWithBusinessTransaction(t *testing.T) {
	srv, _ := newBatchBehaviorTestServer(t)
	businessTx, err := srv.client.Tx(t.Context())
	if err != nil {
		t.Fatalf("begin batch business transaction: %v", err)
	}
	rolledBack := false
	t.Cleanup(func() {
		if !rolledBack {
			_ = businessTx.Rollback()
		}
	})
	if err := lockBatchSubmissionTransaction(t.Context(), businessTx); err != nil {
		t.Fatalf("lock batch business transaction: %v", err)
	}

	tryLock := func() bool {
		t.Helper()
		tx, err := srv.pool.Begin(t.Context())
		if err != nil {
			t.Fatalf("begin advisory lock probe: %v", err)
		}
		defer func() { _ = tx.Rollback(context.Background()) }()
		var acquired bool
		if err := tx.QueryRow(
			t.Context(),
			`SELECT pg_try_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
			usecase.BatchSubmissionAdvisoryLockKey,
		).Scan(&acquired); err != nil {
			t.Fatalf("probe transaction advisory lock: %v", err)
		}
		return acquired
	}

	if tryLock() {
		t.Fatal("second transaction acquired batch guard while business transaction held it")
	}
	if err := businessTx.Rollback(); err != nil {
		t.Fatalf("roll back batch business transaction: %v", err)
	}
	rolledBack = true
	if !tryLock() {
		t.Fatal("batch guard remained held after business transaction rollback")
	}
}

func TestLockBatchSubmissionTransaction_BackendTerminationRollsBackBusinessWrite(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	businessTx, beginErr := client.BeginTx(t.Context(), &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if beginErr != nil {
		t.Fatalf("begin batch business transaction: %v", beginErr)
	}
	businessTxClosed := false
	t.Cleanup(func() {
		if !businessTxClosed {
			_ = businessTx.Rollback()
		}
	})
	if err := lockBatchSubmissionTransaction(t.Context(), businessTx); err != nil {
		t.Fatalf("lock batch business transaction: %v", err)
	}
	eventID := "terminated-business-write-" + uuid.NewString()
	if _, err := businessTx.Client().DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventBatchDeleteRequested)).
		SetAggregateType("batch").
		SetAggregateID("terminated-batch-" + uuid.NewString()).
		SetPayload([]byte(`{"test":true}`)).
		SetStatus("PENDING").
		SetCreatedBy("owner-1").
		Save(t.Context()); err != nil {
		t.Fatalf("stage business write: %v", err)
	}

	probeTx, probeBeginErr := srv.pool.Begin(t.Context())
	if probeBeginErr != nil {
		t.Fatalf("begin batch lock probe: %v", probeBeginErr)
	}
	probeClosed := false
	t.Cleanup(func() {
		if !probeClosed {
			_ = probeTx.Rollback(context.Background())
		}
	})
	var probePID int32
	if err := probeTx.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&probePID); err != nil {
		t.Fatalf("query batch lock probe PID: %v", err)
	}
	probeResult := make(chan error, 1)
	probeDone := runHandlerAsync(func() {
		_, probeErr := probeTx.Exec(
			context.Background(),
			`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
			usecase.BatchSubmissionAdvisoryLockKey,
		)
		probeResult <- probeErr
	})
	holderPID := waitForBlockingBackend(t, srv.pool, probePID)
	var terminated bool
	if err := srv.pool.QueryRow(t.Context(), `SELECT pg_terminate_backend($1)`, holderPID).Scan(&terminated); err != nil {
		t.Fatalf("terminate batch business backend: %v", err)
	}
	if !terminated {
		t.Fatalf("pg_terminate_backend(%d) returned false", holderPID)
	}

	select {
	case probeErr := <-probeResult:
		if probeErr != nil {
			t.Fatalf("probe failed after terminating lock holder: %v", probeErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not acquire global lock after terminating business backend")
	}
	waitForHandlerCompletion(t, probeDone, "batch lock probe")
	if err := businessTx.Commit(); err == nil {
		t.Fatal("commit after terminating business backend succeeded")
	}
	businessTxClosed = true
	if err := probeTx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back batch lock probe: %v", err)
	}
	probeClosed = true

	exists, queryErr := client.DomainEvent.Query().Where(domainevent.IDEQ(eventID)).Exist(t.Context())
	if queryErr != nil {
		t.Fatalf("query terminated business write: %v", queryErr)
	}
	if exists {
		t.Fatal("business write survived termination of the transaction holding the global lock")
	}
}

func TestResolveBatchUserLimitPolicy_TracksEachDefaultSourceIndependently(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	ctx := t.Context()

	if _, err := client.RateLimitUserOverride.Create().
		SetID("all-defaults").
		SetReason("administrative note only").
		SetUpdatedBy("admin-1").
		Save(ctx); err != nil {
		t.Fatalf("create reason-only override: %v", err)
	}
	parents := 7
	if _, err := client.RateLimitUserOverride.Create().
		SetID("parents-only").
		SetMaxPendingParents(parents).
		SetUpdatedBy("admin-1").
		Save(ctx); err != nil {
		t.Fatalf("create parents-only override: %v", err)
	}

	reasonOnly, err := srv.resolveBatchUserLimitPolicy(ctx, "all-defaults")
	if err != nil {
		t.Fatalf("resolve reason-only policy: %v", err)
	}
	if reasonOnly.MaxPendingParents != maxPendingBatchParentsUser ||
		reasonOnly.MaxPendingChildren != maxPendingBatchChildrenUser ||
		reasonOnly.Cooldown != batchSubmitCooldown ||
		!reasonOnly.UsesDefaultParents ||
		!reasonOnly.UsesDefaultChildren ||
		!reasonOnly.UsesDefaultCooldown {
		t.Fatalf("reason-only policy did not retain every default: %+v", reasonOnly)
	}

	parentsOnly, err := srv.resolveBatchUserLimitPolicy(ctx, "parents-only")
	if err != nil {
		t.Fatalf("resolve parents-only policy: %v", err)
	}
	if parentsOnly.MaxPendingParents != parents ||
		parentsOnly.UsesDefaultParents ||
		!parentsOnly.UsesDefaultChildren ||
		!parentsOnly.UsesDefaultCooldown {
		t.Fatalf("parents-only policy source flags = %+v", parentsOnly)
	}
}

func TestResolveBatchUserLimitPolicy_IgnoresExpiredExemptionWithoutMutatingIt(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	const actor = "expired-exemption-user"
	if _, err := client.RateLimitExemption.Create().
		SetID(actor).
		SetExemptedBy("admin-1").
		SetExpiresAt(time.Now().UTC().Add(-time.Minute)).
		Save(t.Context()); err != nil {
		t.Fatalf("seed expired exemption: %v", err)
	}

	policy, err := srv.resolveBatchUserLimitPolicy(t.Context(), actor)
	if err != nil {
		t.Fatalf("resolve policy with expired exemption: %v", err)
	}
	if policy.Exempt {
		t.Fatal("expired exemption resolved as active")
	}
	if _, err := client.RateLimitExemption.Get(t.Context(), actor); err != nil {
		t.Fatalf("policy resolution mutated expired exemption: %v", err)
	}
}

func TestEvaluateAdditionalBatchSubmissionLimits_ContactAdminTracksViolatedLimitSource(t *testing.T) {
	_, client := newBatchBehaviorTestServer(t)
	ctx := t.Context()
	policy := defaultBatchUserLimitPolicy()
	policy.MaxPendingChildren = 0

	violation, err := evaluateAdditionalBatchSubmissionLimitsWithReader(
		ctx,
		entBatchSubmissionLimitReader{client: client},
		"owner-1",
		1,
		policy,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("evaluate default child limit: %v", err)
	}
	if violation == nil || violation.Reason != "user_pending_child_limit" || !violation.ContactAdmin {
		t.Fatalf("default child-limit violation = %+v, want user_pending_child_limit with contact_admin", violation)
	}

	policy.UsesDefaultChildren = false
	violation, err = evaluateAdditionalBatchSubmissionLimitsWithReader(
		ctx,
		entBatchSubmissionLimitReader{client: client},
		"owner-1",
		1,
		policy,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("evaluate custom child limit: %v", err)
	}
	if violation == nil || violation.Reason != "user_pending_child_limit" || violation.ContactAdmin {
		t.Fatalf("custom child-limit violation = %+v, want user_pending_child_limit without contact_admin", violation)
	}
}

func rebindBatchServerToSingleConnectionPool(
	t *testing.T,
	srv *Server,
) (*pgxpool.Pool, *ent.Client) {
	t.Helper()
	return rebindBatchServerToConfiguredPool(t, srv, 1, "")
}

func rebindBatchServerToRepeatableReadPool(
	t *testing.T,
	srv *Server,
) (*pgxpool.Pool, *ent.Client) {
	t.Helper()
	return rebindBatchServerToConfiguredPool(t, srv, 4, "repeatable read")
}

func rebindBatchServerToConfiguredPool(
	t *testing.T,
	srv *Server,
	maxConnections int32,
	defaultIsolation string,
) (pool *pgxpool.Pool, client *ent.Client) {
	t.Helper()
	limitedConfig := srv.pool.Config().Copy()
	limitedConfig.MinConns = 0
	limitedConfig.MaxConns = maxConnections
	if defaultIsolation != "" {
		if limitedConfig.ConnConfig.RuntimeParams == nil {
			limitedConfig.ConnConfig.RuntimeParams = make(map[string]string)
		}
		limitedConfig.ConnConfig.RuntimeParams["default_transaction_isolation"] = defaultIsolation
	}
	limitedPool, err := pgxpool.NewWithConfig(t.Context(), limitedConfig)
	if err != nil {
		t.Fatalf("create single-connection pool: %v", err)
	}
	t.Cleanup(limitedPool.Close)
	if err := limitedPool.Ping(t.Context()); err != nil {
		t.Fatalf("ping single-connection pool: %v", err)
	}

	db := stdlib.OpenDBFromPool(limitedPool)
	t.Cleanup(func() { _ = db.Close() })
	limitedClient := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	srv.pool = limitedPool
	srv.client = limitedClient
	srv.approvalReqs = service.NewApprovalRequirementService(limitedClient)
	return limitedPool, limitedClient
}

func beginUserMutationBlocker(
	t *testing.T,
	pool *pgxpool.Pool,
	userID string,
) (tx pgx.Tx, blockerPID int32) {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin user mutation blocker: %v", err)
	}
	if _, err := tx.Exec(
		t.Context(),
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(userID),
	); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("lock user mutation blocker: %v", err)
	}
	if err := tx.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		_ = tx.Rollback(context.Background())
		t.Fatalf("query user mutation blocker PID: %v", err)
	}
	return tx, blockerPID
}

func holdBatchSubmissionGuard(t *testing.T, pool *pgxpool.Pool) (release func(), blockerPID int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire batch submission guard connection: %v", err)
	}
	lockTx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		t.Fatalf("begin batch submission guard transaction: %v", err)
	}
	if _, err := lockTx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		usecase.BatchSubmissionAdvisoryLockKey,
	); err != nil {
		_ = lockTx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("hold batch submission guard: %v", err)
	}
	if err := lockTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		_ = lockTx.Rollback(context.Background())
		conn.Release()
		t.Fatalf("query batch submission blocker PID: %v", err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			if err := lockTx.Rollback(cleanupCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				_ = conn.Conn().Close(cleanupCtx)
				conn.Release()
				t.Fatalf("release batch submission guard transaction: %v", err)
			}
			conn.Release()
		})
	}
	t.Cleanup(release)
	return release, blockerPID
}

type blockedCallObserver interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func waitForBlockedAdvisoryCalls(t *testing.T, observer blockedCallObserver, blockerPID int32, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var blocked int
	for {
		// PostgreSQL may cache cumulative-statistics snapshots for the life of
		// a transaction. Refresh it when a blocker transaction is the observer.
		if tx, ok := observer.(pgx.Tx); ok {
			if _, err := tx.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
				t.Fatalf("refresh blocked-call statistics: %v", err)
			}
		}
		err := observer.QueryRow(ctx, `
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
`, blockerPID).Scan(&blocked)
		if err != nil {
			t.Fatalf("query blocked database calls: %v", err)
		}
		if blocked == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("blocked database calls = %d, want %d: %v", blocked, want, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForBlockingBackend(t *testing.T, pool *pgxpool.Pool, blockedPID int32) int32 {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		var blockerPIDs []int32
		if err := pool.QueryRow(ctx, `SELECT pg_blocking_pids($1)`, blockedPID).Scan(&blockerPIDs); err != nil {
			t.Fatalf("query blocking backend for PID %d: %v", blockedPID, err)
		}
		if len(blockerPIDs) > 0 {
			return blockerPIDs[0]
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PID %d was not blocked before timeout: %v", blockedPID, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func waitForBatchHandlers(t *testing.T, handlers []<-chan struct{}) {
	t.Helper()
	for idx, done := range handlers {
		waitForHandlerCompletion(t, done, "batch submission guard handler "+strconv.Itoa(idx))
	}
}

func assertOneAcceptedOneRateLimited(t *testing.T, responses []*httptest.ResponseRecorder) {
	t.Helper()
	statuses := make([]int, 0, len(responses))
	for _, response := range responses {
		statuses = append(statuses, response.Code)
		if response.Code == http.StatusTooManyRequests {
			var body generated.Error
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode guarded rate-limit response: %v", err)
			}
			if body.Code != "BATCH_RATE_LIMITED" {
				t.Fatalf("guarded rate-limit code = %q, want BATCH_RATE_LIMITED", body.Code)
			}
			if reason, _ := body.Params["reason"].(string); reason != "user_submit_cooldown" {
				t.Fatalf("guarded rate-limit reason = %q, want user_submit_cooldown", reason)
			}
		}
	}
	sort.Ints(statuses)
	want := []int{http.StatusAccepted, http.StatusTooManyRequests}
	sort.Ints(want)
	if statuses[0] != want[0] || statuses[1] != want[1] {
		t.Fatalf("guarded response statuses = %v, want %v", statuses, want)
	}
}
