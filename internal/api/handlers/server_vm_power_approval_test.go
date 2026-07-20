package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

const directPowerTestActor = "power-operator"

func TestStartVM_ProdNamespaceCreatesPendingPowerTicket(t *testing.T) {
	t.Parallel()

	client, pool := newBatchBehaviorTestStore(t, "start_vm_prod_requires_approval")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		RiverClient:  newBatchBehaviorTestRiverClient(t, pool),
		ApprovalReqs: service.NewApprovalRequirementService(client),
	})

	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentProd, entvm.StatusSTOPPED)

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/start", "", "owner-1", []string{"vm:operate", "platform:admin"})
	srv.StartVM(c, vmID)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.VMPowerAcceptedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != generated.VMPowerAcceptedResponseStatus("PENDING_APPROVAL") {
		t.Fatalf("status = %q, want %q", resp.Status, "PENDING_APPROVAL")
	}
	if resp.TicketId == "" || resp.EventId == "" {
		t.Fatalf("response ticket_id/event_id must be set, got ticket=%q event=%q", resp.TicketId, resp.EventId)
	}

	ticket, err := client.Ticket.Get(t.Context(), resp.TicketId)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.OperationType != entticket.OperationTypePOWER {
		t.Fatalf("ticket operation_type = %q, want %q", ticket.OperationType, entticket.OperationTypePOWER)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want %q", ticket.Status, entticket.StatusPENDING)
	}

	event, err := client.DomainEvent.Get(t.Context(), resp.EventId)
	if err != nil {
		t.Fatalf("query domain event: %v", err)
	}
	if event.EventType != string(domain.EventVMStartRequested) {
		t.Fatalf("event_type = %q, want %q", event.EventType, domain.EventVMStartRequested)
	}
}

func TestStopVM_StartingProdNamespaceCreatesPendingPowerTicket(t *testing.T) {
	t.Parallel()

	client, pool := newBatchBehaviorTestStore(t, "stop_vm_starting_prod_requires_approval")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		RiverClient:  newBatchBehaviorTestRiverClient(t, pool),
		ApprovalReqs: service.NewApprovalRequirementService(client),
	})

	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentProd, entvm.StatusSTARTING)

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/stop", "", "owner-1", []string{"vm:operate", "platform:admin"})
	srv.StopVM(c, vmID)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp generated.VMPowerAcceptedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != generated.VMPowerAcceptedResponseStatus("PENDING_APPROVAL") {
		t.Fatalf("status = %q, want %q", resp.Status, "PENDING_APPROVAL")
	}
	if resp.TicketId == "" || resp.EventId == "" {
		t.Fatalf("response ticket_id/event_id must be set, got ticket=%q event=%q", resp.TicketId, resp.EventId)
	}
}

func TestPowerEndpoints_RejectActiveExecutingPowerTicket(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		vmStatus  entvm.Status
		operation string
		eventType domain.EventType
		invoke    func(*Server, *gin.Context, generated.VMID)
	}{
		{
			name:      "start endpoint",
			vmStatus:  entvm.StatusSTOPPED,
			operation: "start",
			eventType: domain.EventVMStartRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.StartVM(c, vmID)
			},
		},
		{
			name:      "stop endpoint",
			vmStatus:  entvm.StatusRUNNING,
			operation: "stop",
			eventType: domain.EventVMStopRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.StopVM(c, vmID)
			},
		},
		{
			name:      "restart endpoint",
			vmStatus:  entvm.StatusRUNNING,
			operation: "restart",
			eventType: domain.EventVMRestartRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.RestartVM(c, vmID)
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client, pool := newBatchBehaviorTestStore(t, "pde_"+tc.operation)
			riverClient := newBatchBehaviorTestRiverClient(t, pool)
			_ = logger.Init("error", "json")
			srv := NewServer(ServerDeps{
				EntClient:    client,
				Pool:         pool,
				RiverClient:  riverClient,
				ApprovalReqs: service.NewApprovalRequirementService(client),
			})

			vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentProd, tc.vmStatus)
			existingTicketID := mustSeedActivePowerTicket(t, client, vmID, entticket.StatusEXECUTING, tc.eventType, tc.operation, "owner-1")
			mustInsertRunnableHandlerPowerJobForTicket(t, srv, client, existingTicketID)

			c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/"+tc.operation, "", "owner-1", []string{"vm:operate", "platform:admin"})
			tc.invoke(srv, c, vmID)

			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
			}
			assertErrorCode(t, w.Body.Bytes(), "POWER_OPERATION_IN_PROGRESS")

			resp := decodeJSONMap(t, w.Body.Bytes())
			params, ok := resp["params"].(map[string]interface{})
			if !ok {
				t.Fatalf("params type = %T, want object", resp["params"])
			}
			if got := toStringValue(params["existing_ticket_id"]); got != existingTicketID {
				t.Fatalf("existing_ticket_id = %q, want %q", got, existingTicketID)
			}
			if got := toStringValue(params["existing_ticket_status"]); got != string(entticket.StatusEXECUTING) {
				t.Fatalf("existing_ticket_status = %q, want %q", got, entticket.StatusEXECUTING)
			}
		})
	}
}

func TestStartVM_UnregisteredNamespaceReturnsBadRequest(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "start_vm_missing_namespace_registry")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: service.NewApprovalRequirementService(client),
	})

	svcID := mustCreateServiceForVM(t, client, "owner-1")
	vmID := "vm-" + uuid.NewString()
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace("missing-ns").
		SetStatus(entvm.StatusSTOPPED).
		SetCreatedBy("owner-1").
		SetClusterID("cluster-a").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/start", "", "owner-1", []string{"vm:operate", "platform:admin"})
	srv.StartVM(c, vmID)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "NAMESPACE_NOT_REGISTERED")
}

func TestPowerVM_DirectPathPersistsEventAndRiverJobForValidStateMatrix(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdm")

	testCases := []struct {
		name      string
		action    generated.VMPowerRequestAction
		status    entvm.Status
		eventType domain.EventType
	}{
		{name: "start stopped", action: generated.Start, status: entvm.StatusSTOPPED, eventType: domain.EventVMStartRequested},
		{name: "start paused", action: generated.Start, status: entvm.StatusPAUSED, eventType: domain.EventVMStartRequested},
		{name: "stop running", action: generated.Stop, status: entvm.StatusRUNNING, eventType: domain.EventVMStopRequested},
		{name: "stop starting", action: generated.Stop, status: entvm.StatusSTARTING, eventType: domain.EventVMStopRequested},
		{name: "restart running", action: generated.Restart, status: entvm.StatusRUNNING, eventType: domain.EventVMRestartRequested},
	}
	if len(testCases) != 5 {
		t.Fatalf("direct power state matrix has %d cases, want 5", len(testCases))
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vmID := env.seedVM(t, tc.status)
			w := invokePowerVM(t, env.server, vmID, tc.action)
			assertDirectPowerAcceptedAndPersisted(t, env, w, vmID, string(tc.action), tc.status, tc.eventType, directPowerTestActor)
		})
	}
}

func TestDedicatedPowerEndpoints_EnterDirectPath(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pde")

	testCases := []struct {
		name      string
		operation string
		status    entvm.Status
		eventType domain.EventType
		invoke    func(*Server, *gin.Context, generated.VMID)
	}{
		{
			name:      "start accepts paused",
			operation: "start",
			status:    entvm.StatusPAUSED,
			eventType: domain.EventVMStartRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.StartVM(c, vmID)
			},
		},
		{
			name:      "stop accepts starting",
			operation: "stop",
			status:    entvm.StatusSTARTING,
			eventType: domain.EventVMStopRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.StopVM(c, vmID)
			},
		},
		{
			name:      "restart accepts running",
			operation: "restart",
			status:    entvm.StatusRUNNING,
			eventType: domain.EventVMRestartRequested,
			invoke: func(srv *Server, c *gin.Context, vmID generated.VMID) {
				srv.RestartVM(c, vmID)
			},
		},
	}
	if len(testCases) != 3 {
		t.Fatalf("dedicated power endpoint matrix has %d cases, want 3", len(testCases))
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vmID := env.seedVM(t, tc.status)
			c, w := newAuthedGinContext(
				t,
				http.MethodPost,
				"/vms/"+vmID+"/"+tc.operation,
				"",
				"endpoint-operator",
				[]string{"vm:operate", "platform:admin"},
			)
			tc.invoke(env.server, c, vmID)
			assertDirectPowerAcceptedAndPersisted(t, env, w, vmID, tc.operation, tc.status, tc.eventType, "endpoint-operator")
		})
	}
}

func TestPowerVM_DirectPathRejectsInvalidOperationAndStatesWithoutSideEffects(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdr")

	t.Run("invalid operation", func(t *testing.T) {
		vmID := env.seedVM(t, entvm.StatusSTOPPED)
		c, w := newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/"+vmID+"/power",
			`{"action":"hibernate"}`,
			"power-operator",
			[]string{"vm:operate", "platform:admin"},
		)
		env.server.PowerVM(c, vmID)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "INVALID_POWER_ACTION")
	})

	allStatuses := []entvm.Status{
		entvm.StatusCREATING,
		entvm.StatusSTARTING,
		entvm.StatusRUNNING,
		entvm.StatusSTOPPING,
		entvm.StatusSTOPPED,
		entvm.StatusDELETING,
		entvm.StatusFAILED,
		entvm.StatusPENDING,
		entvm.StatusMIGRATING,
		entvm.StatusPAUSED,
		entvm.StatusUNKNOWN,
		entvm.StatusNOT_FOUND,
	}
	operations := []struct {
		action  generated.VMPowerRequestAction
		allowed map[entvm.Status]bool
	}{
		{action: generated.Start, allowed: map[entvm.Status]bool{entvm.StatusSTOPPED: true, entvm.StatusPAUSED: true}},
		{action: generated.Stop, allowed: map[entvm.Status]bool{entvm.StatusRUNNING: true, entvm.StatusSTARTING: true}},
		{action: generated.Restart, allowed: map[entvm.Status]bool{entvm.StatusRUNNING: true}},
	}

	for _, operation := range operations {
		for _, status := range allStatuses {
			if operation.allowed[status] {
				continue
			}
			operation := operation
			status := status
			t.Run(string(operation.action)+" "+string(status), func(t *testing.T) {
				vmID := env.seedVM(t, status)
				w := invokePowerVM(t, env.server, vmID, operation.action)
				if w.Code != http.StatusConflict {
					t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
				}
				assertErrorCode(t, w.Body.Bytes(), "INVALID_STATE_TRANSITION")
			})
		}
	}

	assertDirectPowerTableCounts(t, env, 0, 0)
}

func TestPowerVM_DirectPathRiverInsertFailureRollsBackEvent(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdi")
	vmID := env.seedVM(t, entvm.StatusSTOPPED)
	installVMPowerRiverInsertFailure(t, env.pool)

	w := invokePowerVM(t, env.server, vmID, generated.Start)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	assertVMPowerRiverInsertFailureTriggered(t, env.pool)
	assertErrorCode(t, w.Body.Bytes(), "INTERNAL_ERROR")
	assertDirectPowerTableCounts(t, env, 0, 0)
}

func TestPowerVM_DirectPathConcurrentDuplicateDoesNotPersistDuplicateSideEffects(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdd")
	vmID := env.seedVM(t, entvm.StatusSTOPPED)
	body := mustJSON(t, generated.VMPowerRequest{Action: generated.Start})
	lockKey := "power:vm:" + vmID

	lockConn, err := env.pool.Acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire direct power lock connection: %v", err)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(t.Context(), `SELECT pg_advisory_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		t.Fatalf("hold direct power advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query direct power lock backend pid: %v", err)
	}

	contexts := make([]*gin.Context, 2)
	recorders := make([]*httptest.ResponseRecorder, 2)
	for i := range contexts {
		contexts[i], recorders[i] = newAuthedGinContext(
			t,
			http.MethodPost,
			"/vms/"+vmID+"/power",
			body,
			"power-operator",
			[]string{"vm:operate", "platform:admin"},
		)
	}

	start := make(chan struct{})
	var group errgroup.Group
	for i := range contexts {
		requestIndex := i
		group.Go(func() error {
			<-start
			env.server.PowerVM(contexts[requestIndex], vmID)
			return nil
		})
	}
	close(start)

	blockedRequests := 0
	var activityErr error
	require.Eventually(t, func() bool {
		activityErr = env.pool.QueryRow(t.Context(), `
SELECT count(*)
FROM pg_stat_activity AS activity
WHERE activity.datname = current_database()
  AND activity.state = 'active'
  AND $1 = ANY(pg_blocking_pids(activity.pid))
  AND activity.query LIKE '%pg_advisory_xact_lock%'
`, blockerPID).Scan(&blockedRequests)
		return activityErr != nil || blockedRequests == len(contexts)
	}, 10*time.Second, 10*time.Millisecond, "direct power requests did not all block on the advisory lock")

	_, unlockErr := lockConn.Exec(t.Context(), `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, lockKey)
	if unlockErr == nil {
		lockHeld = false
	} else {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = lockConn.Conn().Close(closeCtx)
		cancel()
		lockHeld = false
	}
	if waitErr := group.Wait(); waitErr != nil {
		t.Fatalf("wait for concurrent direct power requests: %v", waitErr)
	}
	require.NoError(t, activityErr, "query blocked direct power requests")
	if blockedRequests != len(contexts) {
		t.Fatalf("blocked direct power requests = %d, want %d before releasing advisory lock", blockedRequests, len(contexts))
	}
	if unlockErr != nil {
		t.Fatalf("release direct power advisory lock: %v", unlockErr)
	}

	eventIDs := make([]string, len(recorders))
	for i, recorder := range recorders {
		if recorder.Code != http.StatusAccepted {
			t.Fatalf("request %d status = %d, want %d body=%s", i, recorder.Code, http.StatusAccepted, recorder.Body.String())
		}
		var resp generated.VMPowerAcceptedResponse
		if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode request %d response: %v", i, err)
		}
		if resp.EventId == "" || resp.Status != generated.VMPowerAcceptedResponseStatusACCEPTED {
			t.Fatalf("request %d response event/status = %q/%q, want non-empty/ACCEPTED", i, resp.EventId, resp.Status)
		}
		eventIDs[i] = resp.EventId
	}
	if eventIDs[0] != eventIDs[1] {
		t.Fatalf("concurrent duplicate event_ids = %q/%q, want the same idempotent event", eventIDs[0], eventIDs[1])
	}

	eventCount, jobCount := directPowerEffectCounts(t, env, vmID, domain.EventVMStartRequested)
	if eventCount != 1 || jobCount != 1 {
		t.Fatalf(
			"concurrent duplicate side effects = events:%d jobs:%d, want 1/1",
			eventCount,
			jobCount,
		)
	}
}

func TestPowerVM_DirectPathRejectsDifferentActiveOperation(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdc")
	vmID := env.seedVM(t, entvm.StatusRUNNING)

	stopResponse := invokePowerVM(t, env.server, vmID, generated.Stop)
	if stopResponse.Code != http.StatusAccepted {
		t.Fatalf("stop status = %d, want %d body=%s", stopResponse.Code, http.StatusAccepted, stopResponse.Body.String())
	}

	restartResponse := invokePowerVM(t, env.server, vmID, generated.Restart)
	if restartResponse.Code != http.StatusConflict {
		t.Fatalf("restart status = %d, want %d body=%s", restartResponse.Code, http.StatusConflict, restartResponse.Body.String())
	}
	assertErrorCode(t, restartResponse.Body.Bytes(), "POWER_OPERATION_IN_PROGRESS")
	response := decodeJSONMap(t, restartResponse.Body.Bytes())
	params, ok := response["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("conflict params type = %T, want object", response["params"])
	}
	if got := toStringValue(params["existing_event_type"]); got != string(domain.EventVMStopRequested) {
		t.Fatalf("existing_event_type = %q, want %q", got, domain.EventVMStopRequested)
	}
	if got := toStringValue(params["existing_event_id"]); got == "" {
		t.Fatal("existing_event_id is empty")
	}

	stopEvents, stopJobs := directPowerEffectCounts(t, env, vmID, domain.EventVMStopRequested)
	restartEvents, restartJobs := directPowerEffectCounts(t, env, vmID, domain.EventVMRestartRequested)
	if stopEvents != 1 || stopJobs != 1 || restartEvents != 0 || restartJobs != 0 {
		t.Fatalf(
			"power effects after conflict = stop %d/%d restart %d/%d, want 1/1 and 0/0",
			stopEvents,
			stopJobs,
			restartEvents,
			restartJobs,
		)
	}
}

func TestPowerVM_DirectPathDoesNotMasqueradeTicketBackedEventAsAccepted(t *testing.T) {
	env := newDirectPowerTestEnvironment(t, "pdt")
	vmID := env.seedVM(t, entvm.StatusSTOPPED)
	ticketID := mustSeedActivePowerTicket(
		t,
		env.client,
		vmID,
		entticket.StatusPENDING,
		domain.EventVMStartRequested,
		"start",
		"approval-requester",
	)

	responseRecorder := invokePowerVM(t, env.server, vmID, generated.Start)
	if responseRecorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", responseRecorder.Code, http.StatusConflict, responseRecorder.Body.String())
	}
	assertErrorCode(t, responseRecorder.Body.Bytes(), "DUPLICATE_PENDING_REQUEST")
	response := decodeJSONMap(t, responseRecorder.Body.Bytes())
	params, ok := response["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("conflict params type = %T, want object", response["params"])
	}
	if got := toStringValue(params["existing_ticket_id"]); got != ticketID {
		t.Fatalf("existing_ticket_id = %q, want %q", got, ticketID)
	}

	eventCount, jobCount := directPowerEffectCounts(t, env, vmID, domain.EventVMStartRequested)
	if eventCount != 1 || jobCount != 0 {
		t.Fatalf("ticket-backed direct conflict effects = events:%d jobs:%d, want 1/0", eventCount, jobCount)
	}
}

type directPowerTestEnvironment struct {
	server    *Server
	client    *ent.Client
	pool      *pgxpool.Pool
	namespace string
	serviceID string
}

func newDirectPowerTestEnvironment(t *testing.T, prefix string) *directPowerTestEnvironment {
	t.Helper()

	client, pool := newBatchBehaviorTestStore(t, prefix)
	riverClient := newBatchBehaviorTestRiverClient(t, pool)
	deps := ServerDeps{
		EntClient:    client,
		Pool:         pool,
		RiverClient:  riverClient,
		ApprovalReqs: service.NewApprovalRequirementService(client),
	}

	namespace := "power-" + uuid.NewString()
	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(namespaceregistry.EnvironmentTest).
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create direct power namespace registry: %v", err)
	}

	return &directPowerTestEnvironment{
		server:    NewServer(deps),
		client:    client,
		pool:      pool,
		namespace: namespace,
		serviceID: mustCreateServiceForVM(t, client, "owner-1"),
	}
}

func (env *directPowerTestEnvironment) seedVM(t *testing.T, status entvm.Status) string {
	t.Helper()

	vmID := "vm-" + uuid.NewString()
	_, err := env.client.VM.Create().
		SetID(vmID).
		SetName(vmID).
		SetInstance("01").
		SetNamespace(env.namespace).
		SetStatus(status).
		SetCreatedBy("owner-1").
		SetClusterID("cluster-a").
		SetServiceID(env.serviceID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create direct power VM: %v", err)
	}
	return vmID
}

func invokePowerVM(
	t *testing.T,
	srv *Server,
	vmID string,
	action generated.VMPowerRequestAction,
) *httptest.ResponseRecorder {
	t.Helper()

	body := mustJSON(t, generated.VMPowerRequest{Action: action})
	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/"+vmID+"/power",
		body,
		directPowerTestActor,
		[]string{"vm:operate", "platform:admin"},
	)
	srv.PowerVM(c, vmID)
	return w
}

func assertDirectPowerAcceptedAndPersisted(
	t *testing.T,
	env *directPowerTestEnvironment,
	w *httptest.ResponseRecorder,
	vmID string,
	operation string,
	requestStatus entvm.Status,
	eventType domain.EventType,
	actor string,
) {
	t.Helper()

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}
	var resp generated.VMPowerAcceptedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.EventId == "" {
		t.Fatal("response event_id is empty")
	}
	if resp.TicketId != "" {
		t.Fatalf("response ticket_id = %q, want empty for direct path", resp.TicketId)
	}
	if resp.Status != generated.VMPowerAcceptedResponseStatusACCEPTED {
		t.Fatalf("response status = %q, want %q", resp.Status, generated.VMPowerAcceptedResponseStatusACCEPTED)
	}

	vmRow, err := env.client.VM.Query().
		Where(entvm.IDEQ(vmID)).
		WithService(func(query *ent.ServiceQuery) {
			query.WithSystem()
		}).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query power VM: %v", err)
	}
	event, err := env.client.DomainEvent.Get(t.Context(), resp.EventId)
	if err != nil {
		t.Fatalf("query direct power event: %v", err)
	}
	if event.EventType != string(eventType) || event.AggregateType != "vm" || event.AggregateID != vmID {
		t.Fatalf(
			"event identity = type:%q aggregate:%q/%q, want %q vm/%q",
			event.EventType,
			event.AggregateType,
			event.AggregateID,
			eventType,
			vmID,
		)
	}
	if event.Status != domainevent.StatusPENDING || event.CreatedBy != actor {
		t.Fatalf("event state/actor = %q/%q, want PENDING/%q", event.Status, event.CreatedBy, actor)
	}

	var payload domain.VMPowerPayload
	if decodeErr := json.Unmarshal(event.Payload, &payload); decodeErr != nil {
		t.Fatalf("decode direct power payload: %v", decodeErr)
	}
	if payload.VMID != vmRow.ID || payload.VMName != vmRow.Name || payload.Namespace != vmRow.Namespace || payload.ClusterID != vmRow.ClusterID {
		t.Fatalf(
			"payload VM identity = %q/%q/%q/%q, want %q/%q/%q/%q",
			payload.VMID,
			payload.VMName,
			payload.Namespace,
			payload.ClusterID,
			vmRow.ID,
			vmRow.Name,
			vmRow.Namespace,
			vmRow.ClusterID,
		)
	}
	if payload.Operation != operation || payload.Actor != actor || payload.RequestVMStatus != string(requestStatus) {
		t.Fatalf(
			"payload operation/actor/status = %q/%q/%q, want %q/%q/%q",
			payload.Operation,
			payload.Actor,
			payload.RequestVMStatus,
			operation,
			actor,
			requestStatus,
		)
	}
	if payload.OwnerID != vmRow.CreatedBy || vmRow.Edges.Service == nil || payload.ServiceID != vmRow.Edges.Service.ID {
		t.Fatalf("payload owner/service = %q/%q, want %q/%q", payload.OwnerID, payload.ServiceID, vmRow.CreatedBy, env.serviceID)
	}
	if vmRow.Edges.Service.Edges.System == nil || payload.SystemID != vmRow.Edges.Service.Edges.System.ID {
		t.Fatalf("payload system_id = %q, want loaded VM service system", payload.SystemID)
	}

	var (
		queue       string
		maxAttempts int
		jobEventID  string
	)
	if queryErr := env.pool.QueryRow(t.Context(), `
SELECT queue, max_attempts, args->>'event_id'
FROM river_job
WHERE kind = 'vm_power' AND args->>'event_id' = $1
`, resp.EventId).Scan(&queue, &maxAttempts, &jobEventID); queryErr != nil {
		t.Fatalf("query direct power River job: %v", queryErr)
	}
	if queue != "vm_operations" || maxAttempts != 3 || jobEventID != resp.EventId {
		t.Fatalf(
			"River job = queue:%q attempts:%d event:%q, want vm_operations/3/%q",
			queue,
			maxAttempts,
			jobEventID,
			resp.EventId,
		)
	}

	eventCount, jobCount := directPowerEffectCounts(t, env, vmID, eventType)
	if eventCount != 1 || jobCount != 1 {
		t.Fatalf("direct power effects = events:%d jobs:%d, want 1/1", eventCount, jobCount)
	}
	ticketCount, err := env.client.Ticket.Query().
		Where(entticket.EventIDEQ(resp.EventId)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count direct power tickets: %v", err)
	}
	if ticketCount != 0 {
		t.Fatalf("direct power ticket count = %d, want 0", ticketCount)
	}
}

func directPowerEffectCounts(
	t *testing.T,
	env *directPowerTestEnvironment,
	vmID string,
	eventType domain.EventType,
) (eventCount, jobCount int) {
	t.Helper()

	if err := env.pool.QueryRow(t.Context(), `
SELECT count(*)
FROM domain_events
WHERE aggregate_type = 'vm' AND aggregate_id = $1 AND event_type = $2
`, vmID, string(eventType)).Scan(&eventCount); err != nil {
		t.Fatalf("count direct power events: %v", err)
	}

	if err := env.pool.QueryRow(t.Context(), `
SELECT count(*)
FROM river_job AS job
JOIN domain_events AS event ON event.id = job.args->>'event_id'
WHERE job.kind = 'vm_power'
  AND event.aggregate_type = 'vm'
  AND event.aggregate_id = $1
  AND event.event_type = $2
`, vmID, string(eventType)).Scan(&jobCount); err != nil {
		t.Fatalf("count direct power River jobs: %v", err)
	}
	return eventCount, jobCount
}

func assertDirectPowerTableCounts(t *testing.T, env *directPowerTestEnvironment, wantEvents, wantJobs int) {
	t.Helper()

	var eventCount int
	if err := env.pool.QueryRow(t.Context(), `SELECT count(*) FROM domain_events`).Scan(&eventCount); err != nil {
		t.Fatalf("count domain events: %v", err)
	}
	var jobCount int
	if err := env.pool.QueryRow(t.Context(), `SELECT count(*) FROM river_job WHERE kind = 'vm_power'`).Scan(&jobCount); err != nil {
		t.Fatalf("count vm_power River jobs: %v", err)
	}
	if eventCount != wantEvents || jobCount != wantJobs {
		t.Fatalf("power table counts = events:%d jobs:%d, want %d/%d", eventCount, jobCount, wantEvents, wantJobs)
	}
}

func seedPowerTestVM(t *testing.T, client *ent.Client, env namespaceregistry.Environment, status entvm.Status) string {
	t.Helper()

	namespace := "team-test"
	if env == namespaceregistry.EnvironmentProd {
		namespace = "team-prod"
	}
	if _, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(env).
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	svcID := mustCreateServiceForVM(t, client, "owner-1")
	vmID := "vm-" + uuid.NewString()
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm" + vmID[len(vmID)-4:]).
		SetInstance("01").
		SetNamespace(namespace).
		SetStatus(status).
		SetCreatedBy("owner-1").
		SetClusterID("cluster-a").
		SetServiceID(svcID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return vmID
}

func mustSeedActivePowerTicket(
	t *testing.T,
	client *ent.Client,
	vmID string,
	status entticket.Status,
	eventType domain.EventType,
	operation string,
	requester string,
) string {
	t.Helper()

	eventID := uuid.Must(uuid.NewV7()).String()
	ticketID := uuid.Must(uuid.NewV7()).String()
	payloadBytes, err := domain.VMPowerPayload{
		VMID:         vmID,
		Operation:    operation,
		Actor:        requester,
		DispatchMode: domain.VMPowerDispatchTicket,
	}.ToJSON()
	if err != nil {
		t.Fatalf("marshal power payload: %v", err)
	}

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(eventType)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(requester).
		Save(t.Context()); err != nil {
		t.Fatalf("create power event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypePOWER).
		SetStatus(status).
		SetRequester(requester).
		SetReason("duplicate guard seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create power ticket: %v", err)
	}

	return ticketID
}
