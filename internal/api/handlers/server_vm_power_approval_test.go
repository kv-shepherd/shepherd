package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

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

func TestStartVM_ProdNamespaceCreatesPendingPowerTicket(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "start_vm_prod_requires_approval")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:    client,
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

	client := testutil.OpenEntPostgres(t, "stop_vm_starting_prod_requires_approval")
	_ = logger.Init("error", "json")
	srv := NewServer(ServerDeps{
		EntClient:    client,
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

func TestPowerEndpoints_RejectDuplicateActivePowerTicket(t *testing.T) {
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

			client := testutil.OpenEntPostgres(t, "power_duplicate_"+tc.operation)
			_ = logger.Init("error", "json")
			srv := NewServer(ServerDeps{
				EntClient:    client,
				ApprovalReqs: service.NewApprovalRequirementService(client),
			})

			vmID := seedPowerTestVM(t, client, namespaceregistry.EnvironmentProd, tc.vmStatus)
			existingTicketID := mustSeedActivePowerTicket(t, client, vmID, entticket.StatusEXECUTING, tc.eventType, tc.operation, "owner-1")

			c, w := newAuthedGinContext(t, http.MethodPost, "/vms/"+vmID+"/"+tc.operation, "", "owner-1", []string{"vm:operate", "platform:admin"})
			tc.invoke(srv, c, vmID)

			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
			}
			assertErrorCode(t, w.Body.Bytes(), "DUPLICATE_PENDING_REQUEST")

			resp := decodeJSONMap(t, w.Body.Bytes())
			params, ok := resp["params"].(map[string]interface{})
			if !ok {
				t.Fatalf("params type = %T, want object", resp["params"])
			}
			if got := toStringValue(params["existing_ticket_id"]); got != existingTicketID {
				t.Fatalf("existing_ticket_id = %q, want %q", got, existingTicketID)
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
		VMID:      vmID,
		Operation: operation,
		Actor:     requester,
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
