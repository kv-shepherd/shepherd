package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/service"
)

func TestPowerGuardHandler_DirectStartThenApprovalStartReturnsConflictWithoutSideEffects(t *testing.T) {
	srv, client, pool := newPowerGuardHandlerTestServer(t, "hda")
	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)

	first := invokePowerGuardStart(t, srv, vmID)
	if first.Code != http.StatusAccepted {
		t.Fatalf("direct start status = %d, want %d body=%s", first.Code, http.StatusAccepted, first.Body.String())
	}
	var accepted generated.VMPowerAcceptedResponse
	if err := json.Unmarshal(first.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("decode direct start response: %v", err)
	}
	if accepted.EventId == "" || accepted.TicketId != "" || accepted.Status != generated.VMPowerAcceptedResponseStatusACCEPTED {
		t.Fatalf(
			"direct response event/ticket/status = %q/%q/%q, want non-empty/empty/%q",
			accepted.EventId,
			accepted.TicketId,
			accepted.Status,
			generated.VMPowerAcceptedResponseStatusACCEPTED,
		)
	}

	updated, err := client.NamespaceRegistry.Update().
		Where(namespaceregistry.NameEQ("team-test")).
		SetEnvironment(namespaceregistry.EnvironmentProd).
		Save(t.Context())
	if err != nil {
		t.Fatalf("switch namespace registry to prod: %v", err)
	}
	if updated != 1 {
		t.Fatalf("updated namespace registry rows = %d, want 1", updated)
	}
	vmRow, err := client.VM.Get(t.Context(), vmID)
	if err != nil {
		t.Fatalf("query VM before approval-path repeat: %v", err)
	}
	if vmRow.Status != entvm.StatusSTOPPED {
		t.Fatalf("VM status before approval-path repeat = %q, want %q", vmRow.Status, entvm.StatusSTOPPED)
	}

	second := invokePowerGuardStart(t, srv, vmID)
	conflict := requirePowerGuardHandlerError(t, second, http.StatusConflict, "POWER_OPERATION_IN_PROGRESS")
	if got := powerGuardStringParam(conflict.Params, "existing_event_id"); got != accepted.EventId {
		t.Fatalf("existing_event_id = %q, want direct event %q", got, accepted.EventId)
	}
	if got := powerGuardStringParam(conflict.Params, "vm_id"); got != vmID {
		t.Fatalf("conflict vm_id = %q, want %q", got, vmID)
	}
	if _, exists := conflict.Params["existing_ticket_id"]; exists {
		t.Fatalf("existing_ticket_id unexpectedly present for direct event: %#v", conflict.Params["existing_ticket_id"])
	}

	assertPowerGuardHandlerCounts(t, pool, powerGuardHandlerCounts{
		events:  1,
		tickets: 0,
		batches: 0,
		jobs:    1,
	})
}

func TestPowerGuardHandler_ApprovalStartThenBatchStartReturnsTicketConflictWithoutBatchSideEffects(t *testing.T) {
	srv, client, pool := newPowerGuardHandlerTestServer(t, "hab")
	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentProd, entvm.StatusSTOPPED)

	approvalRecorder := invokePowerGuardStart(t, srv, vmID)
	if approvalRecorder.Code != http.StatusAccepted {
		t.Fatalf("approval start status = %d, want %d body=%s", approvalRecorder.Code, http.StatusAccepted, approvalRecorder.Body.String())
	}
	var approval generated.VMPowerAcceptedResponse
	if err := json.Unmarshal(approvalRecorder.Body.Bytes(), &approval); err != nil {
		t.Fatalf("decode approval start response: %v", err)
	}
	if approval.EventId == "" || approval.TicketId == "" || approval.Status != generated.VMPowerAcceptedResponseStatus("PENDING_APPROVAL") {
		t.Fatalf(
			"approval response event/ticket/status = %q/%q/%q, want non-empty/non-empty/PENDING_APPROVAL",
			approval.EventId,
			approval.TicketId,
			approval.Status,
		)
	}

	batchRecorder := invokePowerGuardBatchStart(t, srv, vmID)
	conflict := requirePowerGuardHandlerError(t, batchRecorder, http.StatusConflict, "DUPLICATE_PENDING_REQUEST")
	if got := powerGuardStringParam(conflict.Params, "existing_event_id"); got != approval.EventId {
		t.Fatalf("batch conflict existing_event_id = %q, want %q", got, approval.EventId)
	}
	if got := powerGuardStringParam(conflict.Params, "existing_ticket_id"); got != approval.TicketId {
		t.Fatalf("batch conflict existing_ticket_id = %q, want %q", got, approval.TicketId)
	}
	if got := powerGuardStringParam(conflict.Params, "existing_ticket_status"); got != string(entticket.StatusPENDING) {
		t.Fatalf("batch conflict existing_ticket_status = %q, want %q", got, entticket.StatusPENDING)
	}
	if got := powerGuardStringParam(conflict.Params, "vm_id"); got != vmID {
		t.Fatalf("batch conflict vm_id = %q, want %q", got, vmID)
	}

	assertPowerGuardHandlerCounts(t, pool, powerGuardHandlerCounts{
		events:  1,
		tickets: 1,
		batches: 0,
		jobs:    0,
	})
}

func TestPowerGuardHandler_BatchRejectsDuplicateVMBeforeAnyPersistence(t *testing.T) {
	srv, client, pool := newPowerGuardHandlerTestServer(t, "hdb")
	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)

	response := invokePowerGuardBatchStart(t, srv, vmID, "  "+vmID+"  ")
	validation := requirePowerGuardHandlerError(t, response, http.StatusBadRequest, "INVALID_BATCH_ITEM")
	if got := powerGuardStringParam(validation.Params, "vm_id"); got != vmID {
		t.Fatalf("duplicate batch vm_id = %q, want normalized %q", got, vmID)
	}
	if got, ok := validation.Params["first_index"].(float64); !ok || got != 1 {
		t.Fatalf("duplicate batch first_index = %#v, want 1", validation.Params["first_index"])
	}
	if got, ok := validation.Params["duplicate_index"].(float64); !ok || got != 2 {
		t.Fatalf("duplicate batch duplicate_index = %#v, want 2", validation.Params["duplicate_index"])
	}

	assertPowerGuardHandlerCounts(t, pool, powerGuardHandlerCounts{})
}

func TestPowerGuardHandler_BatchStartThenDirectStartReturnsExecutionConflictWithoutSideEffects(t *testing.T) {
	srv, client, pool := newPowerGuardHandlerTestServer(t, "hbd")
	vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)

	batchRecorder := invokePowerGuardBatchStart(t, srv, vmID)
	if batchRecorder.Code != http.StatusAccepted {
		t.Fatalf("batch start status = %d, want %d body=%s", batchRecorder.Code, http.StatusAccepted, batchRecorder.Body.String())
	}
	var batch generated.VMBatchSubmitResponse
	if err := json.Unmarshal(batchRecorder.Body.Bytes(), &batch); err != nil {
		t.Fatalf("decode batch start response: %v", err)
	}
	if batch.BatchId == "" {
		t.Fatal("batch start response batch_id is empty")
	}
	child, err := client.Ticket.Query().
		Where(entticket.ParentTicketIDEQ(batch.BatchId)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query direct batch child ticket: %v", err)
	}

	directRecorder := invokePowerGuardStart(t, srv, vmID)
	conflict := requirePowerGuardHandlerError(t, directRecorder, http.StatusConflict, "POWER_OPERATION_IN_PROGRESS")
	if got := powerGuardStringParam(conflict.Params, "existing_event_id"); got != child.EventID {
		t.Fatalf("direct conflict existing_event_id = %q, want batch child event %q", got, child.EventID)
	}
	if got := powerGuardStringParam(conflict.Params, "existing_ticket_id"); got != child.ID {
		t.Fatalf("direct conflict existing_ticket_id = %q, want batch child ticket %q", got, child.ID)
	}
	if got := powerGuardStringParam(conflict.Params, "existing_ticket_status"); got != string(entticket.StatusEXECUTING) {
		t.Fatalf("direct conflict existing_ticket_status = %q, want %q", got, entticket.StatusEXECUTING)
	}
	if got := powerGuardStringParam(conflict.Params, "existing_parent_ticket_id"); got != batch.BatchId {
		t.Fatalf("direct conflict existing_parent_ticket_id = %q, want %q", got, batch.BatchId)
	}

	assertPowerGuardHandlerCounts(t, pool, powerGuardHandlerCounts{
		events:  2,
		tickets: 2,
		batches: 1,
		jobs:    1,
	})
}

type powerGuardHandlerCounts struct {
	events  int
	tickets int
	batches int
	jobs    int
}

func newPowerGuardHandlerTestServer(t *testing.T, prefix string) (*Server, *ent.Client, *pgxpool.Pool) {
	t.Helper()

	client, pool := newBatchBehaviorTestStore(t, prefix)
	return NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		RiverClient:  newBatchBehaviorTestRiverClient(t, pool),
		ApprovalReqs: service.NewApprovalRequirementService(client),
	}), client, pool
}

func invokePowerGuardStart(t *testing.T, srv *Server, vmID string) *httptest.ResponseRecorder {
	t.Helper()

	c, recorder := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/"+vmID+"/start",
		"",
		"owner-1",
		[]string{"vm:operate", "platform:admin"},
	)
	srv.StartVM(c, vmID)
	return recorder
}

func invokePowerGuardBatchStart(t *testing.T, srv *Server, vmIDs ...string) *httptest.ResponseRecorder {
	t.Helper()

	items := make([]generated.VMBatchPowerItem, 0, len(vmIDs))
	for _, vmID := range vmIDs {
		items = append(items, generated.VMBatchPowerItem{VmId: vmID})
	}
	body := mustJSON(t, generated.VMBatchPowerRequest{
		Operation: generated.VMBatchPowerAction("START"),
		Items:     items,
	})
	c, recorder := newAuthedGinContext(
		t,
		http.MethodPost,
		"/vms/batch/power",
		body,
		"owner-1",
		[]string{"vm:operate", "platform:admin"},
	)
	srv.SubmitVMBatchPower(c)
	return recorder
}

func requirePowerGuardHandlerError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	wantStatus int,
	wantCode string,
) generated.Error {
	t.Helper()

	if recorder.Code != wantStatus {
		t.Fatalf("response status = %d, want %d body=%s", recorder.Code, wantStatus, recorder.Body.String())
	}
	var response generated.Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.Code != wantCode {
		t.Fatalf("error code = %q, want %q body=%s", response.Code, wantCode, recorder.Body.String())
	}
	return response
}

func powerGuardStringParam(params map[string]interface{}, key string) string {
	value, _ := params[key].(string)
	return value
}

func assertPowerGuardHandlerCounts(t *testing.T, pool *pgxpool.Pool, want powerGuardHandlerCounts) {
	t.Helper()

	checks := []struct {
		label string
		query string
		want  int
	}{
		{label: "domain events", query: `SELECT count(*) FROM domain_events`, want: want.events},
		{label: "tickets", query: `SELECT count(*) FROM tickets`, want: want.tickets},
		{label: "batch projections", query: `SELECT count(*) FROM batch_tickets`, want: want.batches},
		{label: "River jobs", query: `SELECT count(*) FROM river_job`, want: want.jobs},
	}
	for _, check := range checks {
		var got int
		if err := pool.QueryRow(t.Context(), check.query).Scan(&got); err != nil {
			t.Fatalf("count %s: %v", check.label, err)
		}
		if got != check.want {
			t.Fatalf("%s count = %d, want %d", check.label, got, check.want)
		}
	}
}
