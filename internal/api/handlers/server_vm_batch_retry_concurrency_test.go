package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestBatchHandler_RetryVMBatch_ConcurrentNonPowerRetriesReturnInProgressConflict(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)

	submitBody := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})
	submitCtx, submitResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		submitBody,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(submitCtx)
	if submitResponse.Code != http.StatusAccepted {
		t.Fatalf(
			"submit status = %d, want %d body=%s",
			submitResponse.Code,
			http.StatusAccepted,
			submitResponse.Body.String(),
		)
	}

	var submitted generated.VMBatchSubmitResponse
	if err := json.Unmarshal(submitResponse.Body.Bytes(), &submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	child, queryChildErr := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(submitted.BatchId)).
		Only(t.Context())
	if queryChildErr != nil {
		t.Fatalf("query batch child ticket: %v", queryChildErr)
	}
	if _, seedTicketErr := client.Ticket.UpdateOneID(child.ID).
		SetStatus(entticket.StatusFAILED).
		SetRejectReason("seed retryable failure").
		Save(t.Context()); seedTicketErr != nil {
		t.Fatalf("seed failed child ticket: %v", seedTicketErr)
	}
	if _, seedEventErr := client.DomainEvent.UpdateOneID(child.EventID).
		SetStatus(domainevent.StatusFAILED).
		Save(t.Context()); seedEventErr != nil {
		t.Fatalf("seed failed child event: %v", seedEventErr)
	}
	mustSetBatchParentFailedForMutationSafety(t, client, submitted.BatchId, "original-approver")

	testCtx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	blockerTx, beginBlockerErr := srv.pool.Begin(testCtx)
	if beginBlockerErr != nil {
		t.Fatalf("begin child retry blocker transaction: %v", beginBlockerErr)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = blockerTx.Rollback(cleanupCtx)
	})

	var blockerPID int32
	if queryPIDErr := blockerTx.QueryRow(testCtx, `SELECT pg_backend_pid()`).Scan(&blockerPID); queryPIDErr != nil {
		t.Fatalf("query child retry blocker PID: %v", queryPIDErr)
	}
	var lockedTicketID string
	if lockTicketErr := blockerTx.QueryRow(
		testCtx,
		`SELECT id FROM tickets WHERE id = $1 FOR UPDATE`,
		child.ID,
	).Scan(&lockedTicketID); lockTicketErr != nil {
		t.Fatalf("lock retryable child ticket: %v", lockTicketErr)
	}
	if lockedTicketID != child.ID {
		t.Fatalf("locked ticket ID = %q, want %q", lockedTicketID, child.ID)
	}

	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+submitted.BatchId+"/retry",
		"",
		"owner-1",
		[]string{"vm:delete"},
	)
	secondCtx, secondResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+submitted.BatchId+"/retry",
		"",
		"owner-1",
		[]string{"vm:delete"},
	)
	start := make(chan struct{})
	firstDone := runHandlerAsync(func() {
		<-start
		srv.RetryVMBatch(firstCtx, submitted.BatchId)
	})
	secondDone := runHandlerAsync(func() {
		<-start
		srv.RetryVMBatch(secondCtx, submitted.BatchId)
	})
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = srv.pool.QueryRow(testCtx, `
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
	}, 8*time.Second, 10*time.Millisecond, "concurrent retry handlers did not both block on the child ticket")

	releaseErr := blockerTx.Commit(testCtx)
	if releaseErr != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = blockerTx.Rollback(cleanupCtx)
		cleanupCancel()
	}
	waitForHandlerCompletion(t, firstDone, "first concurrent non-power batch retry")
	waitForHandlerCompletion(t, secondDone, "second concurrent non-power batch retry")

	require.NoError(t, blockedQueryErr, "query handlers blocked on retryable child ticket")
	if blockedCalls != 2 {
		t.Fatalf("handlers blocked on retryable child ticket = %d, want 2 before release", blockedCalls)
	}
	if releaseErr != nil {
		t.Fatalf("release retryable child ticket lock: %v", releaseErr)
	}

	var successResponse, conflictResponse *httptest.ResponseRecorder
	for idx, response := range []*httptest.ResponseRecorder{firstResponse, secondResponse} {
		switch response.Code {
		case http.StatusOK:
			if successResponse != nil {
				t.Fatalf("response %d is a second success: body=%s", idx, response.Body.String())
			}
			successResponse = response
		case http.StatusConflict:
			if conflictResponse != nil {
				t.Fatalf("response %d is a second conflict: body=%s", idx, response.Body.String())
			}
			conflictResponse = response
		default:
			t.Fatalf(
				"response %d status = %d, want one %d and one %d body=%s",
				idx,
				response.Code,
				http.StatusOK,
				http.StatusConflict,
				response.Body.String(),
			)
		}
	}
	if successResponse == nil || conflictResponse == nil {
		t.Fatalf(
			"concurrent retry statuses = %d/%d, want one %d and one %d",
			firstResponse.Code,
			secondResponse.Code,
			http.StatusOK,
			http.StatusConflict,
		)
	}

	var succeeded generated.VMBatchActionResponse
	if decodeSuccessErr := json.Unmarshal(successResponse.Body.Bytes(), &succeeded); decodeSuccessErr != nil {
		t.Fatalf("decode successful retry response: %v", decodeSuccessErr)
	}
	if succeeded.AffectedCount != 1 {
		t.Fatalf("successful retry affected_count = %d, want 1", succeeded.AffectedCount)
	}
	if len(succeeded.AffectedTicketIds) != 1 || succeeded.AffectedTicketIds[0] != child.ID {
		t.Fatalf("successful retry affected_ticket_ids = %v, want [%s]", succeeded.AffectedTicketIds, child.ID)
	}

	var conflict generated.Error
	if decodeConflictErr := json.Unmarshal(conflictResponse.Body.Bytes(), &conflict); decodeConflictErr != nil {
		t.Fatalf("decode stale retry conflict response: %v", decodeConflictErr)
	}
	if conflict.Code != "BATCH_RETRY_IN_PROGRESS" {
		t.Fatalf("concurrent retry conflict code = %q, want %q", conflict.Code, "BATCH_RETRY_IN_PROGRESS")
	}
	if got, _ := conflict.Params["batch_id"].(string); got != submitted.BatchId {
		t.Fatalf("stale retry conflict batch_id = %q, want %q", got, submitted.BatchId)
	}
	if got, _ := conflict.Params["existing_job_id"].(float64); got <= 0 {
		t.Fatalf("concurrent retry existing_job_id = %v, want positive", conflict.Params["existing_job_id"])
	}
	if got, _ := conflict.Params["existing_job_state"].(string); got == "" {
		t.Fatalf("concurrent retry existing_job_state = %v, want non-empty", conflict.Params["existing_job_state"])
	}

	storedTicket, loadTicketErr := client.Ticket.Get(t.Context(), child.ID)
	if loadTicketErr != nil {
		t.Fatalf("reload retried child ticket: %v", loadTicketErr)
	}
	if storedTicket.Status != entticket.StatusPENDING {
		t.Fatalf("retried child ticket status = %q, want %q", storedTicket.Status, entticket.StatusPENDING)
	}
	if storedTicket.RejectReason != "" {
		t.Fatalf("retried child reject reason = %q, want empty", storedTicket.RejectReason)
	}
	if storedTicket.AttemptCount != 1 || storedTicket.LastAttemptAt == nil {
		t.Fatalf(
			"retried child attempt = %d at %v, want exactly 1 with timestamp",
			storedTicket.AttemptCount,
			storedTicket.LastAttemptAt,
		)
	}
	storedEvent, loadEventErr := client.DomainEvent.Get(t.Context(), child.EventID)
	if loadEventErr != nil {
		t.Fatalf("reload retried child event: %v", loadEventErr)
	}
	if storedEvent.Status != domainevent.StatusPENDING {
		t.Fatalf("retried child event status = %q, want %q", storedEvent.Status, domainevent.StatusPENDING)
	}
}

func TestBatchHandler_RetryVMBatch_ConcurrentPowerRetriesReturnInProgressConflict(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	batchID, childID := mustSeedPowerBatchForRetry(t, client, "start")
	child, err := client.Ticket.Get(t.Context(), childID)
	if err != nil {
		t.Fatalf("query power retry child: %v", err)
	}

	testCtx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	blockerTx, err := srv.pool.Begin(testCtx)
	if err != nil {
		t.Fatalf("begin power retry blocker transaction: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = blockerTx.Rollback(cleanupCtx)
	})
	var blockerPID int32
	if queryPIDErr := blockerTx.QueryRow(testCtx, `SELECT pg_backend_pid()`).Scan(&blockerPID); queryPIDErr != nil {
		t.Fatalf("query power retry blocker PID: %v", queryPIDErr)
	}
	var lockedTicketID string
	if lockTicketErr := blockerTx.QueryRow(
		testCtx,
		`SELECT id FROM tickets WHERE id = $1 FOR UPDATE`,
		child.ID,
	).Scan(&lockedTicketID); lockTicketErr != nil {
		t.Fatalf("lock power retry child: %v", lockTicketErr)
	}

	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"owner-1",
		[]string{"vm:operate"},
	)
	secondCtx, secondResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/"+batchID+"/retry",
		"",
		"owner-1",
		[]string{"vm:operate"},
	)
	start := make(chan struct{})
	firstDone := runHandlerAsync(func() {
		<-start
		srv.RetryVMBatch(firstCtx, batchID)
	})
	secondDone := runHandlerAsync(func() {
		<-start
		srv.RetryVMBatch(secondCtx, batchID)
	})
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = srv.pool.QueryRow(testCtx, `
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
	}, 8*time.Second, 10*time.Millisecond, "concurrent power retries did not both reach the serialized writer")

	releaseErr := blockerTx.Commit(testCtx)
	waitForHandlerCompletion(t, firstDone, "first concurrent power batch retry")
	waitForHandlerCompletion(t, secondDone, "second concurrent power batch retry")
	require.NoError(t, blockedQueryErr, "query blocked power retry handlers")
	if blockedCalls != 2 {
		t.Fatalf("blocked power retry handlers = %d, want 2", blockedCalls)
	}
	if releaseErr != nil {
		t.Fatalf("release power retry child lock: %v", releaseErr)
	}

	var successResponse, conflictResponse *httptest.ResponseRecorder
	for idx, response := range []*httptest.ResponseRecorder{firstResponse, secondResponse} {
		switch response.Code {
		case http.StatusOK:
			if successResponse != nil {
				t.Fatalf("power response %d is a second success: %s", idx, response.Body.String())
			}
			successResponse = response
		case http.StatusConflict:
			if conflictResponse != nil {
				t.Fatalf("power response %d is a second conflict: %s", idx, response.Body.String())
			}
			conflictResponse = response
		default:
			t.Fatalf("power response %d status = %d, want 200/409 body=%s", idx, response.Code, response.Body.String())
		}
	}
	if successResponse == nil || conflictResponse == nil {
		t.Fatalf("concurrent power retry statuses = %d/%d, want one 200 and one 409", firstResponse.Code, secondResponse.Code)
	}

	var succeeded generated.VMBatchActionResponse
	if decodeSuccessErr := json.Unmarshal(successResponse.Body.Bytes(), &succeeded); decodeSuccessErr != nil {
		t.Fatalf("decode successful power retry response: %v", decodeSuccessErr)
	}
	if succeeded.AffectedCount != 1 || len(succeeded.AffectedTicketIds) != 1 || succeeded.AffectedTicketIds[0] != child.ID {
		t.Fatalf("successful power retry response = %+v, want child %q", succeeded, child.ID)
	}
	var conflict generated.Error
	if decodeConflictErr := json.Unmarshal(conflictResponse.Body.Bytes(), &conflict); decodeConflictErr != nil {
		t.Fatalf("decode concurrent power retry conflict: %v", decodeConflictErr)
	}
	if conflict.Code != "BATCH_RETRY_IN_PROGRESS" {
		t.Fatalf("concurrent power retry code = %q, want BATCH_RETRY_IN_PROGRESS", conflict.Code)
	}
	if got, _ := conflict.Params["batch_id"].(string); got != batchID {
		t.Fatalf("concurrent power retry batch_id = %q, want %q", got, batchID)
	}
	if got, _ := conflict.Params["event_id"].(string); got != child.EventID {
		t.Fatalf("concurrent power retry event_id = %q, want %q", got, child.EventID)
	}
	if got, _ := conflict.Params["existing_job_id"].(float64); got <= 0 {
		t.Fatalf("concurrent power retry existing_job_id = %#v, want positive", conflict.Params["existing_job_id"])
	}
	if got, _ := conflict.Params["existing_job_state"].(string); got == "" {
		t.Fatalf("concurrent power retry existing_job_state = %#v, want non-empty", conflict.Params["existing_job_state"])
	}

	storedTicket, err := client.Ticket.Get(t.Context(), child.ID)
	if err != nil {
		t.Fatalf("reload power retry ticket: %v", err)
	}
	if storedTicket.Status != entticket.StatusEXECUTING || storedTicket.AttemptCount != 2 || storedTicket.LastAttemptAt == nil {
		t.Fatalf("power retry ticket = status %s attempt %d at %v, want EXECUTING/2/timestamp", storedTicket.Status, storedTicket.AttemptCount, storedTicket.LastAttemptAt)
	}
	storedEvent, err := client.DomainEvent.Get(t.Context(), child.EventID)
	if err != nil {
		t.Fatalf("reload power retry event: %v", err)
	}
	if storedEvent.Status != domainevent.StatusPENDING {
		t.Fatalf("power retry event status = %s, want PENDING", storedEvent.Status)
	}
	if jobs := mustCountPowerJobsForEvent(t, srv.pool, child.EventID); jobs != 1 {
		t.Fatalf("power retry jobs = %d, want 1", jobs)
	}
}
