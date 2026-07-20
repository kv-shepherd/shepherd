package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
	"kv-shepherd.io/shepherd/internal/usecase"
)

func TestBatchHandler_SubmitVMBatch_LongOpaqueRequestIDReplays(t *testing.T) {
	tests := []struct {
		name      string
		requestID string
	}{
		{name: "513 ASCII characters", requestID: strings.Repeat("x", 513)},
		{name: "long four-byte Unicode", requestID: strings.Repeat("😀", 4096)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv, client := newBatchBehaviorTestServer(t)
			vmIDs := []string{
				mustCreateBatchDeleteTargetVM(t, client),
				mustCreateBatchDeleteTargetVM(t, client),
			}

			var firstBatchID string
			for idx, vmID := range vmIDs {
				body := mustJSON(t, generated.VMBatchSubmitRequest{
					Operation: generated.VMBatchSubmitOperation("DELETE"),
					RequestId: test.requestID,
					Items:     []generated.VMBatchChildItem{{VmId: vmID}},
				})
				requestCtx, response := newAuthedGinContext(
					t,
					http.MethodPost,
					"/vms/batch",
					body,
					"owner-1",
					[]string{"platform:admin"},
				)
				srv.SubmitVMBatch(requestCtx)
				if response.Code != http.StatusAccepted {
					t.Fatalf("submit %d status = %d, want %d body=%s", idx, response.Code, http.StatusAccepted, response.Body.String())
				}
				var result generated.VMBatchSubmitResponse
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatalf("decode submit %d response: %v", idx, err)
				}
				if idx == 0 {
					firstBatchID = result.BatchId
				} else if result.BatchId != firstBatchID {
					t.Fatalf("long-key replay batch ID = %q, want %q", result.BatchId, firstBatchID)
				}
			}

			projection, err := client.BatchTicket.Query().Only(t.Context())
			if err != nil {
				t.Fatalf("query long-key projection: %v", err)
			}
			if projection.RequestID == nil {
				t.Fatalf("long-key projection %q has nil request ID", projection.ID)
			}
			if projection.ID != firstBatchID || *projection.RequestID != test.requestID {
				t.Fatalf("long-key projection = id:%q request_id length:%d, want %q/%d", projection.ID, len(*projection.RequestID), firstBatchID, len(test.requestID))
			}
		})
	}
}

func TestFindBatchByRequestIDWithClient_HistoricalDuplicatesChooseOldestWithoutMutation(t *testing.T) {
	_, client := newBatchBehaviorTestServer(t)
	requestID := strings.Repeat("x", 513)
	oldestCreatedAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	rows := []struct {
		id        string
		createdAt time.Time
	}{
		{id: "batch-later", createdAt: oldestCreatedAt.Add(time.Hour)},
		{id: "batch-oldest-b", createdAt: oldestCreatedAt},
		{id: "batch-oldest-a", createdAt: oldestCreatedAt},
	}
	for _, row := range rows {
		eventID := row.id + "-event"
		payload, err := (domain.BatchVMRequestPayload{
			Operation:   "DELETE",
			RequestID:   requestID,
			SubmittedBy: "owner-1",
		}).ToJSON()
		if err != nil {
			t.Fatalf("encode historical duplicate payload %q: %v", row.id, err)
		}
		if _, err := client.DomainEvent.Create().
			SetID(eventID).
			SetCreatedAt(row.createdAt).
			SetEventType(string(domain.EventBatchDeleteRequested)).
			SetAggregateType("batch").
			SetAggregateID(row.id).
			SetPayload(payload).
			SetStatus(domainevent.StatusPENDING).
			SetCreatedBy("owner-1").
			Save(t.Context()); err != nil {
			t.Fatalf("create historical duplicate event %q: %v", row.id, err)
		}
		if _, err := client.Ticket.Create().
			SetID(row.id).
			SetCreatedAt(row.createdAt).
			SetUpdatedAt(row.createdAt).
			SetEventID(eventID).
			SetOperationType(entticket.OperationTypeDELETE).
			SetStatus(entticket.StatusPENDING).
			SetRequester("owner-1").
			Save(t.Context()); err != nil {
			t.Fatalf("create historical duplicate ticket %q: %v", row.id, err)
		}
		if _, err := client.BatchTicket.Create().
			SetID(row.id).
			SetCreatedAt(row.createdAt).
			SetUpdatedAt(row.createdAt).
			SetBatchType(batchticket.BatchTypeBATCH_DELETE).
			SetStatus(batchticket.StatusPENDING_APPROVAL).
			SetRequestID(requestID).
			SetCreatedBy("owner-1").
			Save(t.Context()); err != nil {
			t.Fatalf("create historical duplicate %q: %v", row.id, err)
		}
	}

	batchID, found, err := findBatchByRequestIDWithClient(
		t.Context(),
		client,
		"owner-1",
		"DELETE",
		requestID,
	)
	if err != nil {
		t.Fatalf("find historical duplicate request ID: %v", err)
	}
	if !found || batchID != "batch-oldest-a" {
		t.Fatalf("historical duplicate replay = %q/%v, want batch-oldest-a/true", batchID, found)
	}
	storedCount, err := client.BatchTicket.Query().
		Where(batchticket.RequestIDEQ(requestID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count preserved historical duplicates: %v", err)
	}
	if storedCount != len(rows) {
		t.Fatalf("preserved historical duplicate count = %d, want %d", storedCount, len(rows))
	}
}

func TestFindBatchByRequestIDWithClient_CorruptMatchingHistoryFailsClosed(t *testing.T) {
	const requestID = "historical-integrity-key"
	tests := []struct {
		name      string
		seed      func(t *testing.T, client *ent.Client)
		wantError string
	}{
		{
			name: "missing root ticket",
			seed: func(t *testing.T, client *ent.Client) {
				_, err := client.BatchTicket.Create().
					SetID("batch-missing-root").
					SetBatchType(batchticket.BatchTypeBATCH_DELETE).
					SetStatus(batchticket.StatusPENDING_APPROVAL).
					SetRequestID(requestID).
					SetCreatedBy("owner-1").
					Save(t.Context())
				require.NoError(t, err)
			},
			wantError: "root ticket is missing",
		},
		{
			name: "requester differs from projection owner",
			seed: func(t *testing.T, client *ent.Client) {
				seedBatchReplayIntegrityCandidate(t, client, "batch-foreign-requester", requestID, "foreign-owner", []byte(`{"operation":"DELETE","request_id":"historical-integrity-key","submitted_by":"owner-1"}`))
			},
			wantError: "root ticket identity is inconsistent",
		},
		{
			name: "malformed parent payload",
			seed: func(t *testing.T, client *ent.Client) {
				seedBatchReplayIntegrityCandidate(t, client, "batch-malformed-payload", requestID, "owner-1", []byte(`{"operation":`))
			},
			wantError: "parent event payload is malformed",
		},
		{
			name: "valid oldest candidate does not mask corrupt duplicate",
			seed: func(t *testing.T, client *ent.Client) {
				seedBatchReplayIntegrityCandidate(t, client, "batch-a-valid", requestID, "owner-1", []byte(`{"operation":"DELETE","request_id":"historical-integrity-key","submitted_by":"owner-1"}`))
				_, err := client.BatchTicket.Create().
					SetID("batch-z-missing-root").
					SetBatchType(batchticket.BatchTypeBATCH_DELETE).
					SetStatus(batchticket.StatusPENDING_APPROVAL).
					SetRequestID(requestID).
					SetCreatedBy("owner-1").
					Save(t.Context())
				require.NoError(t, err)
			},
			wantError: "root ticket is missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := newBatchBehaviorTestServer(t)
			tt.seed(t, client)

			batchID, found, err := findBatchByRequestIDWithClient(
				t.Context(),
				client,
				"owner-1",
				"DELETE",
				requestID,
			)
			require.ErrorContains(t, err, tt.wantError)
			require.Empty(t, batchID)
			require.False(t, found)
		})
	}
}

func TestFindBatchByRequestIDWithClient_BoundsMatchingHistoryBeforeGraphLoads(t *testing.T) {
	_, client := newBatchBehaviorTestServer(t)
	const requestID = "oversized-history"
	for i := 0; i <= batchreplay.CandidateLimit; i++ {
		_, err := client.BatchTicket.Create().
			SetID(fmt.Sprintf("batch-history-%03d", i)).
			SetBatchType(batchticket.BatchTypeBATCH_DELETE).
			SetStatus(batchticket.StatusPENDING_APPROVAL).
			SetRequestID(requestID).
			SetCreatedBy("owner-1").
			Save(t.Context())
		require.NoError(t, err)
	}

	batchID, found, err := findBatchByRequestIDWithClient(
		t.Context(),
		client,
		"owner-1",
		"DELETE",
		requestID,
	)
	require.ErrorContains(t, err, "more than 64 matching projections")
	require.Empty(t, batchID)
	require.False(t, found)
}

func TestFindBatchByRequestIDWithClient_UsesDigestLookupIndex(t *testing.T) {
	srv, _ := newBatchBehaviorTestServer(t)
	conn, err := srv.pool.Acquire(t.Context())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(t.Context(), `SET enable_seqscan = off`)
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(context.Background(), `RESET enable_seqscan`)
	}()

	const requestID = "indexed-replay-key"
	rows, err := conn.Query(t.Context(), `
EXPLAIN (COSTS OFF)
SELECT id
FROM batch_tickets
WHERE created_by = $1
  AND batch_type = $2
  AND request_id IS NOT NULL
  AND "shepherd_batch_replay_sha256"(BTRIM(request_id, `+batchreplay.PostgreSQLTrimCutsetLiteral+`)) = $3
  AND BTRIM(request_id, `+batchreplay.PostgreSQLTrimCutsetLiteral+`) = $4
`, "owner-1", string(batchticket.BatchTypeBATCH_DELETE), batchreplay.Digest(requestID), requestID)
	require.NoError(t, err)
	defer rows.Close()
	planLines := make([]string, 0, 4)
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		planLines = append(planLines, line)
	}
	require.NoError(t, rows.Err())
	require.Contains(t, strings.Join(planLines, "\n"), batchreplay.LookupIndexName)
}

func TestFindBatchByRequestIDWithClient_ReplaysTrimmedHistoricalProjectionWithoutMutation(t *testing.T) {
	_, client := newBatchBehaviorTestServer(t)
	const (
		batchID       = "batch-historical-whitespace"
		storedRequest = "\u3000\t historical-key \u0085\n"
		normalizedKey = "historical-key"
	)
	seedBatchReplayIntegrityCandidate(
		t,
		client,
		batchID,
		storedRequest,
		"owner-1",
		[]byte(`{"operation":"DELETE","request_id":"\u3000\t historical-key \u0085\n","submitted_by":"owner-1"}`),
	)

	replayedID, found, err := findBatchByRequestIDWithClient(
		t.Context(),
		client,
		"owner-1",
		"DELETE",
		normalizedKey,
	)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, batchID, replayedID)
	projection, err := client.BatchTicket.Get(t.Context(), batchID)
	require.NoError(t, err)
	require.NotNil(t, projection.RequestID)
	require.Equal(t, storedRequest, *projection.RequestID)
}

func seedBatchReplayIntegrityCandidate(
	t *testing.T,
	client *ent.Client,
	batchID, requestID, requester string,
	payload []byte,
) {
	t.Helper()
	eventID := batchID + "-event"
	_, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventBatchDeleteRequested)).
		SetAggregateType("batch").
		SetAggregateID(batchID).
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("owner-1").
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.Ticket.Create().
		SetID(batchID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeDELETE).
		SetStatus(entticket.StatusPENDING).
		SetRequester(requester).
		Save(t.Context())
	require.NoError(t, err)
	_, err = client.BatchTicket.Create().
		SetID(batchID).
		SetBatchType(batchticket.BatchTypeBATCH_DELETE).
		SetStatus(batchticket.StatusPENDING_APPROVAL).
		SetRequestID(requestID).
		SetCreatedBy("owner-1").
		Save(t.Context())
	require.NoError(t, err)
}

func TestBatchHandler_SubmitVMBatchPower_SameRequestIDUsesSeparateActionScopes(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	firstVMID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	secondVMID := mustCloneBatchPowerTargetVM(t, client, firstVMID)
	requestID := "opaque-power-key-" + uuid.NewString()
	if _, err := client.RateLimitUserOverride.Create().
		SetID("owner-1").
		SetCooldownSeconds(0).
		SetUpdatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed zero-cooldown override: %v", err)
	}

	submit := func(operation generated.VMBatchPowerAction, vmID string) generated.VMBatchSubmitResponse {
		t.Helper()
		body := mustJSON(t, generated.VMBatchPowerRequest{
			Operation: operation,
			RequestId: requestID,
			Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
		})
		ctx, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch/power",
			body,
			"owner-1",
			[]string{"platform:admin"},
		)
		srv.SubmitVMBatchPower(ctx)
		if response.Code != http.StatusAccepted {
			t.Fatalf("power %s submit status = %d, want %d body=%s", operation, response.Code, http.StatusAccepted, response.Body.String())
		}
		var result generated.VMBatchSubmitResponse
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("decode power %s response: %v", operation, err)
		}
		return result
	}

	first := submit(generated.VMBatchPowerAction("START"), firstVMID)
	second := submit(generated.VMBatchPowerAction("STOP"), secondVMID)
	if second.BatchId == first.BatchId {
		t.Fatalf("cross-action batch ID = %q, want distinct START and STOP batches", second.BatchId)
	}

	wants := []struct {
		batchID   string
		eventType domain.EventType
		vmID      string
	}{
		{batchID: first.BatchId, eventType: domain.EventVMStartRequested, vmID: firstVMID},
		{batchID: second.BatchId, eventType: domain.EventVMStopRequested, vmID: secondVMID},
	}
	for _, want := range wants {
		child, err := client.Ticket.Query().Where(entticket.ParentTicketIDEQ(want.batchID)).Only(t.Context())
		if err != nil {
			t.Fatalf("query power child for batch %s: %v", want.batchID, err)
		}
		event, err := client.DomainEvent.Get(t.Context(), child.EventID)
		if err != nil {
			t.Fatalf("query power event for batch %s: %v", want.batchID, err)
		}
		if event.EventType != string(want.eventType) || event.AggregateID != want.vmID {
			t.Fatalf(
				"persisted action-scoped payload = type:%q vm:%q, want %q/%q",
				event.EventType,
				event.AggregateID,
				want.eventType,
				want.vmID,
			)
		}
	}
	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count power replay parents: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count power replay children: %v", err)
	}
	if parentCount != 2 || childCount != 2 {
		t.Fatalf("action-scoped power rows = parents:%d children:%d, want 2/2", parentCount, childCount)
	}
}

func TestBatchHandler_SubmitVMBatch_ConcurrentRequestIDCommitsOneBatch(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmIDs := []string{
		mustCreateBatchDeleteTargetVM(t, client),
		mustCreateBatchDeleteTargetVM(t, client),
	}
	const (
		actor     = "owner-1"
		operation = "DELETE"
	)
	requestID := "request-concurrent-" + uuid.NewString()
	lockKey := usecase.BatchIdempotencyLockKey(actor, operation, requestID)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	lockConn, err := srv.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire batch idempotency lock connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, lockErr := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); lockErr != nil {
		t.Fatalf("hold batch idempotency advisory lock: %v", lockErr)
	}
	lockHeld = true
	var blockerPID int32
	if pidErr := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); pidErr != nil {
		t.Fatalf("query batch idempotency blocker PID: %v", pidErr)
	}

	type request struct {
		ctx      *gin.Context
		response *httptest.ResponseRecorder
	}
	requests := make([]request, 0, len(vmIDs))
	for _, vmID := range vmIDs {
		body := mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchSubmitOperation(operation),
			RequestId: requestID,
			Items: []generated.VMBatchChildItem{
				{VmId: vmID},
			},
		})
		requestCtx, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch",
			body,
			actor,
			[]string{"platform:admin"},
		)
		requests = append(requests, request{ctx: requestCtx, response: response})
	}

	start := make(chan struct{})
	done := make([]<-chan struct{}, 0, len(requests))
	for _, req := range requests {
		req := req
		done = append(done, runHandlerAsync(func() {
			<-start
			srv.SubmitVMBatch(req.ctx)
		}))
	}
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = srv.pool.QueryRow(ctx, `
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
		return blockedQueryErr != nil || blockedCalls == 1
	}, 8*time.Second, 10*time.Millisecond, "handler did not block on the batch idempotency lock")

	_, unlockErr := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		closeCancel()
		lockHeld = false
	}
	for idx, requestDone := range done {
		waitForHandlerCompletion(t, requestDone, "concurrent batch request "+strconv.Itoa(idx))
	}

	if blockedQueryErr != nil {
		t.Fatalf("query handlers blocked by batch idempotency lock: %v", blockedQueryErr)
	}
	if blockedCalls != 1 {
		t.Fatalf("handlers blocked by batch idempotency lock = %d, want 1 database leader before release", blockedCalls)
	}
	if unlockErr != nil {
		t.Fatalf("release batch idempotency advisory lock: %v", unlockErr)
	}

	batchIDs := make([]string, 0, len(requests))
	for idx, req := range requests {
		if req.response.Code != http.StatusAccepted {
			t.Fatalf("response %d status = %d, want %d body=%s", idx, req.response.Code, http.StatusAccepted, req.response.Body.String())
		}
		var response generated.VMBatchSubmitResponse
		if decodeErr := json.Unmarshal(req.response.Body.Bytes(), &response); decodeErr != nil {
			t.Fatalf("decode response %d: %v", idx, decodeErr)
		}
		batchIDs = append(batchIDs, response.BatchId)
	}
	if batchIDs[0] == "" || batchIDs[1] != batchIDs[0] {
		t.Fatalf("concurrent batch IDs = %q, want the same non-empty ID", batchIDs)
	}

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(ctx)
	if err != nil {
		t.Fatalf("count parent tickets: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(ctx)
	if err != nil {
		t.Fatalf("count child tickets: %v", err)
	}
	batchCount, err := client.BatchTicket.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count batch projections: %v", err)
	}
	eventCount, err := client.DomainEvent.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count batch events: %v", err)
	}
	if parentCount != 1 || childCount != 1 || batchCount != 1 || eventCount != 2 {
		t.Fatalf(
			"concurrent side effects = parents:%d children:%d batches:%d events:%d, want 1/1/1/2",
			parentCount,
			childCount,
			batchCount,
			eventCount,
		)
	}
	projection, err := client.BatchTicket.Query().Only(ctx)
	if err != nil {
		t.Fatalf("query committed batch projection: %v", err)
	}
	if projection.ID != batchIDs[0] || projection.RequestID == nil || *projection.RequestID != requestID {
		t.Fatalf("committed projection = id:%q request_id:%v, want %q/%q", projection.ID, projection.RequestID, batchIDs[0], requestID)
	}
	childEvent, err := client.DomainEvent.Query().
		Where(domainevent.AggregateTypeEQ("vm")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query committed child event: %v", err)
	}
	if childEvent.AggregateID != vmIDs[0] && childEvent.AggregateID != vmIDs[1] {
		t.Fatalf("committed child VM = %q, want one of %q", childEvent.AggregateID, vmIDs)
	}
}

func TestBatchHandler_SubmitVMBatch_TrimsLongRequestIDBeforePersistenceAndReplay(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmIDs := []string{
		mustCreateBatchDeleteTargetVM(t, client),
		mustCreateBatchDeleteTargetVM(t, client),
	}
	requestID := strings.Repeat("opaque-key-", 64)
	requestIDs := []string{" \t" + requestID + "\n ", requestID}

	var firstBatchID string
	for idx, requestIDVariant := range requestIDs {
		body := mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchSubmitOperation("DELETE"),
			RequestId: requestIDVariant,
			Items: []generated.VMBatchChildItem{
				{VmId: vmIDs[idx]},
			},
		})
		requestCtx, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/batch",
			body,
			"owner-1",
			[]string{"platform:admin"},
		)
		srv.SubmitVMBatch(requestCtx)
		if response.Code != http.StatusAccepted {
			t.Fatalf("submit %d status = %d, want %d body=%s", idx, response.Code, http.StatusAccepted, response.Body.String())
		}
		var submitResponse generated.VMBatchSubmitResponse
		if err := json.Unmarshal(response.Body.Bytes(), &submitResponse); err != nil {
			t.Fatalf("decode submit %d response: %v", idx, err)
		}
		if idx == 0 {
			firstBatchID = submitResponse.BatchId
		} else if submitResponse.BatchId != firstBatchID {
			t.Fatalf("trimmed replay batch_id = %q, want %q", submitResponse.BatchId, firstBatchID)
		}
	}

	projectionCount, err := client.BatchTicket.Query().
		Where(batchticket.RequestIDEQ(requestID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count trimmed request ID projections: %v", err)
	}
	if projectionCount != 1 {
		t.Fatalf("trimmed request ID projection count = %d, want 1", projectionCount)
	}
}

func TestBatchHandler_SubmitVMBatch_ReplayCommittedDuringRateLimitWindowReturnsExisting(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmIDs := []string{
		mustCreateBatchDeleteTargetVM(t, client),
		mustCreateBatchDeleteTargetVM(t, client),
	}
	requestID := "request-rate-window-" + uuid.NewString()

	bodyForVM := func(vmID string) string {
		return mustJSON(t, generated.VMBatchSubmitRequest{
			Operation: generated.VMBatchSubmitOperation("DELETE"),
			RequestId: requestID,
			Items: []generated.VMBatchChildItem{
				{VmId: vmID},
			},
		})
	}
	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		bodyForVM(vmIDs[0]),
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(firstCtx)
	if firstResponse.Code != http.StatusAccepted {
		t.Fatalf("first submit status = %d, want %d body=%s", firstResponse.Code, http.StatusAccepted, firstResponse.Body.String())
	}
	var firstSubmit generated.VMBatchSubmitResponse
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &firstSubmit); err != nil {
		t.Fatalf("decode first submit response: %v", err)
	}

	replayCtx, replayResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		bodyForVM(vmIDs[1]),
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(replayCtx)
	if replayResponse.Code != http.StatusAccepted {
		t.Fatalf("replay status = %d, want %d body=%s", replayResponse.Code, http.StatusAccepted, replayResponse.Body.String())
	}
	if retryAfter := replayResponse.Header().Get("Retry-After"); retryAfter != "" {
		t.Fatalf("replay Retry-After = %q, want no rate-limit header", retryAfter)
	}
	var replaySubmit generated.VMBatchSubmitResponse
	if err := json.Unmarshal(replayResponse.Body.Bytes(), &replaySubmit); err != nil {
		t.Fatalf("decode replay submit response: %v", err)
	}
	if replaySubmit.BatchId != firstSubmit.BatchId {
		t.Fatalf("rate-window replay batch_id = %q, want %q", replaySubmit.BatchId, firstSubmit.BatchId)
	}

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count rate-window parent tickets: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count rate-window child tickets: %v", err)
	}
	batchCount, err := client.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count rate-window batch projections: %v", err)
	}
	if parentCount != 1 || childCount != 1 || batchCount != 1 {
		t.Fatalf("rate-window side effects = parents:%d children:%d batches:%d, want 1/1/1", parentCount, childCount, batchCount)
	}
}

func TestBatchHandler_SubmitVMBatch_ReplayPrecedesMutableTargetPreparation(t *testing.T) {
	srv, client := newBatchBehaviorTestServer(t)
	vmID := mustCreateBatchDeleteTargetVM(t, client)
	requestID := "delete-replay-after-target-removal-" + uuid.NewString()
	body := mustJSON(t, generated.VMBatchSubmitRequest{
		Operation: generated.VMBatchSubmitOperation("DELETE"),
		RequestId: requestID,
		Items:     []generated.VMBatchChildItem{{VmId: vmID}},
	})

	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(firstCtx)
	require.Equal(t, http.StatusAccepted, firstResponse.Code, firstResponse.Body.String())
	var first generated.VMBatchSubmitResponse
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))

	require.NoError(t, client.VM.DeleteOneID(vmID).Exec(t.Context()))
	replayCtx, replayResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatch(replayCtx)
	require.Equal(t, http.StatusAccepted, replayResponse.Code, replayResponse.Body.String())
	var replay generated.VMBatchSubmitResponse
	require.NoError(t, json.Unmarshal(replayResponse.Body.Bytes(), &replay))
	require.Equal(t, first.BatchId, replay.BatchId)
	require.Equal(t, 1, mustCountBatchProjections(t, client))
}

func TestBatchHandler_SubmitVMBatchPower_ReplayCommittedDuringRateLimitWindowReturnsExisting(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	firstVMID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	vmIDs := []string{firstVMID, mustCloneBatchPowerTargetVM(t, client, firstVMID)}
	requestID := "power-request-rate-window-" + uuid.NewString()

	bodyForVM := func(vmID string) string {
		return mustJSON(t, generated.VMBatchPowerRequest{
			Operation: generated.VMBatchPowerAction("start"),
			RequestId: requestID,
			Items: []generated.VMBatchPowerItem{
				{VmId: vmID},
			},
		})
	}
	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		bodyForVM(vmIDs[0]),
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatchPower(firstCtx)
	if firstResponse.Code != http.StatusAccepted {
		t.Fatalf("first power submit status = %d, want %d body=%s", firstResponse.Code, http.StatusAccepted, firstResponse.Body.String())
	}
	var firstSubmit generated.VMBatchSubmitResponse
	if err := json.Unmarshal(firstResponse.Body.Bytes(), &firstSubmit); err != nil {
		t.Fatalf("decode first power submit response: %v", err)
	}

	replayCtx, replayResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		bodyForVM(vmIDs[1]),
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatchPower(replayCtx)
	if replayResponse.Code != http.StatusAccepted {
		t.Fatalf("power replay status = %d, want %d body=%s", replayResponse.Code, http.StatusAccepted, replayResponse.Body.String())
	}
	if retryAfter := replayResponse.Header().Get("Retry-After"); retryAfter != "" {
		t.Fatalf("power replay Retry-After = %q, want no rate-limit header", retryAfter)
	}
	var replaySubmit generated.VMBatchSubmitResponse
	if err := json.Unmarshal(replayResponse.Body.Bytes(), &replaySubmit); err != nil {
		t.Fatalf("decode power replay response: %v", err)
	}
	if replaySubmit.BatchId != firstSubmit.BatchId {
		t.Fatalf("power rate-window replay batch_id = %q, want %q", replaySubmit.BatchId, firstSubmit.BatchId)
	}

	parentCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDIsNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count power rate-window parent tickets: %v", err)
	}
	childCount, err := client.Ticket.Query().Where(entticket.ParentTicketIDNotNil()).Count(t.Context())
	if err != nil {
		t.Fatalf("count power rate-window child tickets: %v", err)
	}
	batchCount, err := client.BatchTicket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count power rate-window batch projections: %v", err)
	}
	var jobCount int
	if err := srv.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'vm_power'`).Scan(&jobCount); err != nil {
		t.Fatalf("count power rate-window River jobs: %v", err)
	}
	if parentCount != 1 || childCount != 1 || batchCount != 1 || jobCount != 1 {
		t.Fatalf(
			"power rate-window side effects = parents:%d children:%d batches:%d jobs:%d, want 1/1/1/1",
			parentCount,
			childCount,
			batchCount,
			jobCount,
		)
	}
}

func TestBatchHandler_SubmitVMBatchPower_ReplayPrecedesMutableTargetPreparation(t *testing.T) {
	srv, client := newBatchBehaviorTestServerWithRiver(t)
	vmID := mustCreateBatchPowerTargetVM(t, client, namespaceregistry.EnvironmentTest)
	requestID := "power-replay-after-target-removal-" + uuid.NewString()
	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("START"),
		RequestId: requestID,
		Items:     []generated.VMBatchPowerItem{{VmId: vmID}},
	})

	firstCtx, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatchPower(firstCtx)
	require.Equal(t, http.StatusAccepted, firstResponse.Code, firstResponse.Body.String())
	var first generated.VMBatchSubmitResponse
	require.NoError(t, json.Unmarshal(firstResponse.Body.Bytes(), &first))

	require.NoError(t, client.VM.DeleteOneID(vmID).Exec(t.Context()))
	replayCtx, replayResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"platform:admin"},
	)
	srv.SubmitVMBatchPower(replayCtx)
	require.Equal(t, http.StatusAccepted, replayResponse.Code, replayResponse.Body.String())
	var replay generated.VMBatchSubmitResponse
	require.NoError(t, json.Unmarshal(replayResponse.Body.Bytes(), &replay))
	require.Equal(t, first.BatchId, replay.BatchId)
	require.Equal(t, 1, mustCountBatchProjections(t, client))
}

func mustCountBatchProjections(t *testing.T, client *ent.Client) int {
	t.Helper()
	count, err := client.BatchTicket.Query().Count(t.Context())
	require.NoError(t, err)
	return count
}

func mustCloneBatchPowerTargetVM(t *testing.T, client *ent.Client, sourceVMID string) string {
	t.Helper()
	source, err := client.VM.Query().
		Where(entvm.IDEQ(sourceVMID)).
		WithService().
		Only(t.Context())
	if err != nil {
		t.Fatalf("load source power VM: %v", err)
	}
	if source.Edges.Service == nil {
		t.Fatal("source power VM service edge is nil")
	}
	vmID := "vm-" + uuid.NewString()
	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vmname" + vmID[len(vmID)-4:]).
		SetInstance("02").
		SetNamespace(source.Namespace).
		SetClusterID(source.ClusterID).
		SetStatus(source.Status).
		SetCreatedBy(source.CreatedBy).
		SetServiceID(source.Edges.Service.ID).
		Save(t.Context()); err != nil {
		t.Fatalf("clone power target VM: %v", err)
	}
	return vmID
}
