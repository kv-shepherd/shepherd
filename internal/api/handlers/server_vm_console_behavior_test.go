package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalpolicy"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestVMConsole_Request_TestEnvironmentIssuesDirectPreferredConsoleURL(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "APPROVED" {
		t.Fatalf("status = %q, want %q", got, "APPROVED")
	}
	if consoleType := toStringValue(resp["console_type"]); consoleType != "SERIAL" {
		t.Fatalf("console_type = %q, want %q", consoleType, "SERIAL")
	}
	if consoleURL := toStringValue(resp["console_url"]); consoleURL != "/api/v1/vms/"+vm.ID+"/serial" {
		t.Fatalf("console_url = %q, want %q", consoleURL, "/api/v1/vms/"+vm.ID+"/serial")
	}
	if vncURL := toStringValue(resp["vnc_url"]); vncURL != "" {
		t.Fatalf("vnc_url = %q, want empty for preferred serial response", vncURL)
	}
	bootstrapCookie := mustGetBootstrapCookie(t, w, "/api/v1/vms/"+vm.ID+"/serial")
	if !bootstrapCookie.HttpOnly {
		t.Fatal("expected vnc bootstrap cookie to be HttpOnly")
	}
	if toStringValue(resp["ticket_id"]) != "" {
		t.Fatalf("ticket_id = %q, want empty in test env", toStringValue(resp["ticket_id"]))
	}

	count, err := client.Ticket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 0 {
		t.Fatalf("ticket count = %d, want 0", count)
	}
}

func TestVMConsole_Request_ProductionCreatesPendingTicket(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "PENDING_APPROVAL" {
		t.Fatalf("status = %q, want %q", got, "PENDING_APPROVAL")
	}
	ticketID := toStringValue(resp["ticket_id"])
	if ticketID == "" {
		t.Fatal("ticket_id is empty")
	}
	if toStringValue(resp["vnc_url"]) != "" {
		t.Fatalf("vnc_url = %q, want empty in prod pending response", toStringValue(resp["vnc_url"]))
	}

	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	if err != nil {
		t.Fatalf("get created ticket: %v", err)
	}
	if ticket.Status != entticket.StatusPENDING {
		t.Fatalf("ticket status = %q, want %q", ticket.Status, entticket.StatusPENDING)
	}
	if ticket.Requester != "actor-1" {
		t.Fatalf("ticket requester = %q, want %q", ticket.Requester, "actor-1")
	}
	if string(ticket.OperationType) != "VNC_ACCESS" {
		t.Fatalf("ticket operation_type = %q, want %q", ticket.OperationType, "VNC_ACCESS")
	}

	event, err := client.DomainEvent.Get(t.Context(), ticket.EventID)
	if err != nil {
		t.Fatalf("get domain event: %v", err)
	}
	if event.EventType != string(domain.EventVNCAccessRequested) {
		t.Fatalf("event_type = %q, want %q", event.EventType, domain.EventVNCAccessRequested)
	}
}

func TestVMConsole_Request_ProductionExplicitNoApprovalIssuesDirectPreferredConsoleURL(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)
	mustCreateVNCApprovalPolicy(t, client, approvalpolicy.EnvironmentTypeProd, false, 1)

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "APPROVED" {
		t.Fatalf("status = %q, want %q", got, "APPROVED")
	}
	if consoleType := toStringValue(resp["console_type"]); consoleType != "SERIAL" {
		t.Fatalf("console_type = %q, want %q", consoleType, "SERIAL")
	}
	if consoleURL := toStringValue(resp["console_url"]); consoleURL != "/api/v1/vms/"+vm.ID+"/serial" {
		t.Fatalf("console_url = %q, want %q", consoleURL, "/api/v1/vms/"+vm.ID+"/serial")
	}

	count, err := client.Ticket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 0 {
		t.Fatalf("ticket count = %d, want 0", count)
	}
}

func TestVMConsole_Request_TestEnvironmentHonorsExplicitVNCPreference(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		fmt.Sprintf("/vms/%s/console/request", vm.ID),
		`{"preferred_console_type":"VNC"}`,
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if consoleType := toStringValue(resp["console_type"]); consoleType != "VNC" {
		t.Fatalf("console_type = %q, want %q", consoleType, "VNC")
	}
	if consoleURL := toStringValue(resp["console_url"]); consoleURL != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("console_url = %q, want %q", consoleURL, "/api/v1/vms/"+vm.ID+"/vnc")
	}
	_ = mustGetBootstrapCookie(t, w, "/api/v1/vms/"+vm.ID+"/vnc")
}

func TestVMConsole_Request_TestExplicitApprovalCreatesPendingTicket(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)
	mustCreateVNCApprovalPolicy(t, client, approvalpolicy.EnvironmentTypeTest, true, 1)

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusAccepted, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "PENDING_APPROVAL" {
		t.Fatalf("status = %q, want %q", got, "PENDING_APPROVAL")
	}
	if ticketID := toStringValue(resp["ticket_id"]); ticketID == "" {
		t.Fatal("ticket_id is empty")
	}
}

func TestVMConsole_Request_ProductionRejectsDuplicatePendingRequest(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)
	mustSeedPendingVNCRequest(t, client, vm.ID, vm.ClusterID, vm.Namespace, "actor-1")

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "DUPLICATE_PENDING_VNC_REQUEST")
}

func TestVMConsole_Request_ProductionReusesApprovedTicket(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)

	eventID := uuid.Must(uuid.NewV7()).String()
	ticketID := uuid.Must(uuid.NewV7()).String()
	payload, err := json.Marshal(vncRequestPayload{
		VMID:                 vm.ID,
		ClusterID:            vm.ClusterID,
		Namespace:            vm.Namespace,
		RequesterID:          "actor-1",
		PreferredConsoleType: "SERIAL",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, createEventErr := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(payload).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("actor-1").
		Save(t.Context())
	if createEventErr != nil {
		t.Fatalf("create approved event payload: %v", createEventErr)
	}
	_, createTicketErr := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeVNC_ACCESS).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("actor-1").
		SetApprover("admin-1").
		Save(t.Context())
	if createTicketErr != nil {
		t.Fatalf("create approved ticket: %v", createTicketErr)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		fmt.Sprintf("/vms/%s/console/request", vm.ID),
		`{"preferred_console_type":"VNC"}`,
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "APPROVED" {
		t.Fatalf("status = %q, want %q", got, "APPROVED")
	}
	if got := toStringValue(resp["ticket_id"]); got != ticketID {
		t.Fatalf("ticket_id = %q, want %q", got, ticketID)
	}
	if got := toStringValue(resp["console_type"]); got != "VNC" {
		t.Fatalf("console_type = %q, want %q", got, "VNC")
	}
	if got := toStringValue(resp["console_url"]); got != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("console_url = %q, want %q", got, "/api/v1/vms/"+vm.ID+"/vnc")
	}

	count, err := client.Ticket.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count tickets: %v", err)
	}
	if count != 1 {
		t.Fatalf("ticket count = %d, want 1", count)
	}
}

func TestVMConsole_Request_ProductionReusesLatestApprovedTicketEvenIfNewerRequestRejected(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)

	approvedEventID := uuid.Must(uuid.NewV7()).String()
	approvedTicketID := uuid.Must(uuid.NewV7()).String()
	approvedPayload, err := json.Marshal(vncRequestPayload{
		VMID:                 vm.ID,
		ClusterID:            vm.ClusterID,
		Namespace:            vm.Namespace,
		RequesterID:          "actor-1",
		PreferredConsoleType: "SERIAL",
	})
	if err != nil {
		t.Fatalf("marshal approved payload: %v", err)
	}
	_, createApprovedEventErr := client.DomainEvent.Create().
		SetID(approvedEventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(approvedPayload).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("actor-1").
		Save(t.Context())
	if createApprovedEventErr != nil {
		t.Fatalf("create approved event payload: %v", createApprovedEventErr)
	}
	_, createApprovedTicketErr := client.Ticket.Create().
		SetID(approvedTicketID).
		SetEventID(approvedEventID).
		SetOperationType(entticket.OperationTypeVNC_ACCESS).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("actor-1").
		SetApprover("admin-1").
		Save(t.Context())
	if createApprovedTicketErr != nil {
		t.Fatalf("create approved ticket: %v", createApprovedTicketErr)
	}

	rejectedEventID := uuid.Must(uuid.NewV7()).String()
	rejectedTicketID := uuid.Must(uuid.NewV7()).String()
	rejectedPayload, err := json.Marshal(vncRequestPayload{
		VMID:                 vm.ID,
		ClusterID:            vm.ClusterID,
		Namespace:            vm.Namespace,
		RequesterID:          "actor-1",
		PreferredConsoleType: "VNC",
	})
	if err != nil {
		t.Fatalf("marshal rejected payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID(rejectedEventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(rejectedPayload).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("actor-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create rejected event payload: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(rejectedTicketID).
		SetEventID(rejectedEventID).
		SetOperationType(entticket.OperationTypeVNC_ACCESS).
		SetStatus(entticket.StatusREJECTED).
		SetRequester("actor-1").
		SetApprover("admin-2").
		SetRejectReason("policy denies temporary vnc").
		Save(t.Context()); err != nil {
		t.Fatalf("create rejected ticket: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		fmt.Sprintf("/vms/%s/console/request", vm.ID),
		`{"preferred_console_type":"VNC"}`,
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	resp := decodeJSONMap(t, w.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "APPROVED" {
		t.Fatalf("status = %q, want %q", got, "APPROVED")
	}
	if got := toStringValue(resp["ticket_id"]); got != approvedTicketID {
		t.Fatalf("ticket_id = %q, want %q", got, approvedTicketID)
	}
}

func TestVMConsole_Request_RejectsNonRunningVM(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusSTOPPED)
	seedVMConsoleTargetInMock(mock, vm)

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_RUNNING")
}

func TestVMConsole_Request_RejectsWhenLiveStatusIsNotRunning(t *testing.T) {
	t.Parallel()

	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "vm_console_live_status")
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:            vm.Name,
		Namespace:       vm.Namespace,
		Cluster:         vm.ClusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-console-live-1",
	}})

	srv := NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: service.NewApprovalRequirementService(client),
		VMService:    service.NewVMService(mock),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-vnc-signing-key-123456789012345678901234"),
			Issuer:     "shepherd-test",
			ExpiresIn:  2 * time.Hour,
		},
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})

	c, w := newAuthedGinContext(t, http.MethodPost, fmt.Sprintf("/vms/%s/console/request", vm.ID), "", "actor-1", []string{"vnc:access"})
	srv.RequestVMConsoleAccess(c, vm.ID)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "VM_NOT_RUNNING")
}

func TestVMConsole_Status_ProductionPendingAndApproved(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)
	ticketID := mustSeedPendingVNCRequest(t, client, vm.ID, vm.ClusterID, vm.Namespace, "actor-1")

	pendingCtx, pendingW := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/console/status", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.GetVMConsoleStatus(pendingCtx, vm.ID)

	if pendingW.Code != http.StatusOK {
		t.Fatalf("pending status = %d, want %d body=%s", pendingW.Code, http.StatusOK, pendingW.Body.String())
	}
	pendingResp := decodeJSONMap(t, pendingW.Body.Bytes())
	if got := toStringValue(pendingResp["status"]); got != "PENDING_APPROVAL" {
		t.Fatalf("pending response status = %q, want %q", got, "PENDING_APPROVAL")
	}

	if _, err := client.Ticket.UpdateOneID(ticketID).
		SetStatus(entticket.StatusAPPROVED).
		SetApprover("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("approve seeded ticket: %v", err)
	}
	if _, err := client.DomainEvent.UpdateOneID(mustTicketEventID(t, client, ticketID)).
		SetStatus(domainevent.StatusCOMPLETED).
		Save(t.Context()); err != nil {
		t.Fatalf("set seeded event completed: %v", err)
	}

	approvedCtx, approvedW := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/console/status", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.GetVMConsoleStatus(approvedCtx, vm.ID)

	if approvedW.Code != http.StatusOK {
		t.Fatalf("approved status = %d, want %d body=%s", approvedW.Code, http.StatusOK, approvedW.Body.String())
	}
	approvedResp := decodeJSONMap(t, approvedW.Body.Bytes())
	if got := toStringValue(approvedResp["status"]); got != "APPROVED" {
		t.Fatalf("approved response status = %q, want %q", got, "APPROVED")
	}
	if consoleType := toStringValue(approvedResp["console_type"]); consoleType != "SERIAL" {
		t.Fatalf("approved response console_type = %q, want %q", consoleType, "SERIAL")
	}
	if consoleURL := toStringValue(approvedResp["console_url"]); consoleURL != "/api/v1/vms/"+vm.ID+"/serial" {
		t.Fatalf("approved response console_url = %q, want %q", consoleURL, "/api/v1/vms/"+vm.ID+"/serial")
	}
	_ = mustGetBootstrapCookie(t, approvedW, "/api/v1/vms/"+vm.ID+"/serial")
}

func TestVMConsole_Status_ApprovedHonorsPreferredVNCFromTicketPayload(t *testing.T) {
	t.Parallel()

	srv, client, mock := newVMConsoleBehaviorTestServer(t)
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentProd, entvm.StatusRUNNING)
	seedVMConsoleTargetInMock(mock, vm)
	eventID := uuid.Must(uuid.NewV7()).String()
	ticketID := uuid.Must(uuid.NewV7()).String()
	payload, err := json.Marshal(vncRequestPayload{
		VMID:                 vm.ID,
		ClusterID:            vm.ClusterID,
		Namespace:            vm.Namespace,
		RequesterID:          "actor-1",
		PreferredConsoleType: "VNC",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vm.ID).
		SetPayload(payload).
		SetStatus(domainevent.StatusCOMPLETED).
		SetCreatedBy("actor-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create event payload: %v", err)
	}
	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(entticket.OperationTypeVNC_ACCESS).
		SetStatus(entticket.StatusAPPROVED).
		SetRequester("actor-1").
		SetApprover("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create approved ticket: %v", err)
	}

	approvedCtx, approvedW := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/console/status", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.GetVMConsoleStatus(approvedCtx, vm.ID)

	if approvedW.Code != http.StatusOK {
		t.Fatalf("approved status = %d, want %d body=%s", approvedW.Code, http.StatusOK, approvedW.Body.String())
	}
	approvedResp := decodeJSONMap(t, approvedW.Body.Bytes())
	if consoleType := toStringValue(approvedResp["console_type"]); consoleType != "VNC" {
		t.Fatalf("approved response console_type = %q, want %q", consoleType, "VNC")
	}
	if consoleURL := toStringValue(approvedResp["console_url"]); consoleURL != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("approved response console_url = %q, want %q", consoleURL, "/api/v1/vms/"+vm.ID+"/vnc")
	}
	_ = mustGetBootstrapCookie(t, approvedW, "/api/v1/vms/"+vm.ID+"/vnc")
}

func TestVMConsole_OpenVNC_PreviewDoesNotConsumeBootstrapToken(t *testing.T) {
	t.Parallel()

	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "vm_console_preview_reuse_token")
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Cluster:   vm.ClusterID,
		Status:    domain.VMStatusRunning,
	}})
	mock.SetSerialOpenError(fmt.Errorf("serial unavailable"))

	srv := NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: service.NewApprovalRequirementService(client),
		VMService:    service.NewVMService(mock),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-vnc-signing-key-123456789012345678901234"),
			Issuer:     "shepherd-test",
			ExpiresIn:  2 * time.Hour,
		},
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodPost,
		fmt.Sprintf("/vms/%s/console/request", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	srv.RequestVMConsoleAccess(reqCtx, vm.ID)
	if reqW.Code != http.StatusOK {
		t.Fatalf("request status = %d, want %d body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}
	bootstrapCookie := mustGetBootstrapCookie(t, reqW, "/api/v1/vms/"+vm.ID+"/vnc")

	openCtx1, openW1 := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/vnc", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	openCtx1.Request.AddCookie(&http.Cookie{Name: vncBootstrapCookieName, Value: bootstrapCookie.Value})
	srv.OpenVMVNC(openCtx1, vm.ID)
	if openW1.Code != http.StatusOK {
		t.Fatalf("first open status = %d, want %d body=%s", openW1.Code, http.StatusOK, openW1.Body.String())
	}
	firstResp := decodeJSONMap(t, openW1.Body.Bytes())
	if got := toStringValue(firstResp["websocket_path"]); got != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("first websocket_path = %q, want %q", got, "/api/v1/vms/"+vm.ID+"/vnc")
	}

	openCtx2, openW2 := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/vnc", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	openCtx2.Request.AddCookie(&http.Cookie{Name: vncBootstrapCookieName, Value: bootstrapCookie.Value})
	srv.OpenVMVNC(openCtx2, vm.ID)

	if openW2.Code != http.StatusOK {
		t.Fatalf("second open status = %d, want %d body=%s", openW2.Code, http.StatusOK, openW2.Body.String())
	}
	secondResp := decodeJSONMap(t, openW2.Body.Bytes())
	if got := toStringValue(secondResp["websocket_path"]); got != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("second websocket_path = %q, want %q", got, "/api/v1/vms/"+vm.ID+"/vnc")
	}
}

func TestVMConsole_OpenVNC_PreviewDoesNotEagerlyDialNativeStream(t *testing.T) {
	t.Parallel()

	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "vm_console_preview_native_error")
	vm := mustCreateVMConsoleTarget(t, client, namespaceregistry.EnvironmentTest, entvm.StatusRUNNING)

	mock := provider.NewMockProvider()
	mock.Seed([]*domain.VM{{
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Cluster:   vm.ClusterID,
		Status:    domain.VMStatusRunning,
	}})
	mock.SetVNCOpenError(fmt.Errorf(`open vnc stream %s/%s: no graphics devices are present`, vm.Namespace, vm.Name))
	mock.SetSerialOpenError(fmt.Errorf("serial unavailable"))

	srv := NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: service.NewApprovalRequirementService(client),
		VMService:    service.NewVMService(mock),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-vnc-signing-key-123456789012345678901234"),
			Issuer:     "shepherd-test",
			ExpiresIn:  2 * time.Hour,
		},
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})

	token, _, err := srv.vncTokens.Issue("actor-1", vm.ID, vm.ClusterID, vm.Namespace)
	if err != nil {
		t.Fatalf("issue bootstrap token: %v", err)
	}

	openCtx, openW := newAuthedGinContext(
		t,
		http.MethodGet,
		fmt.Sprintf("/vms/%s/vnc", vm.ID),
		"",
		"actor-1",
		[]string{"vnc:access"},
	)
	openCtx.Request.AddCookie(&http.Cookie{Name: vncBootstrapCookieName, Value: token})
	srv.OpenVMVNC(openCtx, vm.ID)

	if openW.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d body=%s", openW.Code, http.StatusOK, openW.Body.String())
	}
	resp := decodeJSONMap(t, openW.Body.Bytes())
	if got := toStringValue(resp["status"]); got != "SESSION_READY" {
		t.Fatalf("status = %q, want %q", got, "SESSION_READY")
	}
	if got := toStringValue(resp["websocket_path"]); got != "/api/v1/vms/"+vm.ID+"/vnc" {
		t.Fatalf("websocket_path = %q, want %q", got, "/api/v1/vms/"+vm.ID+"/vnc")
	}
}

func TestConsoleOriginAllowedRequiresOriginHeader(t *testing.T) {
	t.Parallel()

	srv := &Server{allowedOrigins: []string{"https://console.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "https://console.example.com/api/v1/vms/vm-1/vnc", http.NoBody)

	if srv.consoleOriginAllowed(req) {
		t.Fatal("consoleOriginAllowed() = true, want false for missing Origin")
	}
}

func TestConsoleOriginAllowedAcceptsSameOrigin(t *testing.T) {
	t.Parallel()

	srv := &Server{allowedOrigins: []string{"https://other.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "https://console.example.com/api/v1/vms/vm-1/vnc", http.NoBody)
	req.Header.Set("Origin", "https://console.example.com")

	if !srv.consoleOriginAllowed(req) {
		t.Fatal("consoleOriginAllowed() = false, want true for same Origin")
	}
}

func TestConsoleOriginAllowedAcceptsConfiguredOrigin(t *testing.T) {
	t.Parallel()

	srv := &Server{allowedOrigins: []string{"https://console.example.com"}}
	req := httptest.NewRequest(http.MethodGet, "https://internal.example.com/api/v1/vms/vm-1/vnc", http.NoBody)
	req.Header.Set("Origin", "https://console.example.com")

	if !srv.consoleOriginAllowed(req) {
		t.Fatal("consoleOriginAllowed() = false, want true for configured Origin")
	}
}

func newVMConsoleBehaviorTestServer(t *testing.T) (*Server, *ent.Client, *provider.MockProvider) {
	t.Helper()
	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, "vm_console_behavior")
	mock := provider.NewMockProvider()
	return NewServer(ServerDeps{
		EntClient:    client,
		ApprovalReqs: service.NewApprovalRequirementService(client),
		VMService:    service.NewVMService(mock),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-vnc-signing-key-123456789012345678901234"),
			Issuer:     "shepherd-test",
			ExpiresIn:  2 * time.Hour,
		},
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}), client, mock
}

func seedVMConsoleTargetInMock(mock *provider.MockProvider, vm *ent.VM) {
	if mock == nil || vm == nil {
		return
	}
	var status domain.VMStatus
	switch vm.Status {
	case entvm.StatusRUNNING:
		status = domain.VMStatusRunning
	case entvm.StatusSTOPPED:
		status = domain.VMStatusStopped
	case entvm.StatusFAILED:
		status = domain.VMStatusFailed
	default:
		status = domain.VMStatusUnknown
	}
	mock.Seed([]*domain.VM{{
		Name:      vm.Name,
		Namespace: vm.Namespace,
		Cluster:   vm.ClusterID,
		Status:    status,
	}})
}

func mustCreateVMConsoleTarget(
	t *testing.T,
	client *ent.Client,
	environment namespaceregistry.Environment,
	status entvm.Status,
) *ent.VM {
	t.Helper()
	actor := "actor-1"

	systemID := "sys-" + uuid.NewString()
	serviceID := "svc-" + uuid.NewString()
	vmID := "vm-" + uuid.NewString()
	namespace := fmt.Sprintf("%s-ns-%s", environment, uuid.NewString()[:8])

	sys := mustCreateSystem(t, client, systemID, "sys-"+systemID[len(systemID)-4:], actor)
	svc := mustCreateService(t, client, serviceID, "svc-"+serviceID[len(serviceID)-4:], sys.ID, "svc")
	mustCreateSystemBinding(t, client, actor, sys.ID, "owner")
	mustCreateGlobalEnvRoleBinding(t, client, actor, []string{"vnc:access"}, []string{string(environment)})

	_, err := client.NamespaceRegistry.Create().
		SetID("ns-" + uuid.NewString()).
		SetName(namespace).
		SetEnvironment(environment).
		SetCreatedBy(actor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create namespace registry: %v", err)
	}

	vm, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-" + vmID[len(vmID)-6:]).
		SetInstance("01").
		SetNamespace(namespace).
		SetClusterID("cluster-a").
		SetStatus(status).
		SetCreatedBy(actor).
		SetServiceID(svc.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create vm: %v", err)
	}
	return vm
}

func mustSeedPendingVNCRequest(t *testing.T, client *ent.Client, vmID, clusterID, namespace, actor string) string {
	t.Helper()

	eventID := "ev-vnc-" + uuid.NewString()
	ticketID := "ticket-vnc-" + uuid.NewString()
	payload := mustJSON(t, map[string]string{
		"vm_id":        vmID,
		"cluster_id":   clusterID,
		"namespace":    namespace,
		"requester_id": actor,
	})

	if _, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload([]byte(payload)).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy(actor).
		Save(t.Context()); err != nil {
		t.Fatalf("create vnc domain event: %v", err)
	}

	if _, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester(actor).
		SetStatus(entticket.StatusPENDING).
		SetOperationType(entticket.OperationType("VNC_ACCESS")).
		SetReason("vnc access request").
		Save(t.Context()); err != nil {
		t.Fatalf("create vnc ticket: %v", err)
	}

	return ticketID
}

func mustCreateVNCApprovalPolicy(
	t *testing.T,
	client *ent.Client,
	environment approvalpolicy.EnvironmentType,
	requiresApproval bool,
	priority int,
) {
	t.Helper()

	if _, err := client.ApprovalPolicy.Create().
		SetID("policy-vnc-" + uuid.NewString()).
		SetName(fmt.Sprintf("vnc-%s-%t", environment, requiresApproval)).
		SetEnvironmentType(environment).
		SetOperation(approvalpolicy.OperationVNC_ACCESS).
		SetRequiresApproval(requiresApproval).
		SetPriority(priority).
		SetEnabled(true).
		SetCreatedBy("tester").
		Save(t.Context()); err != nil {
		t.Fatalf("create vnc approval policy: %v", err)
	}
}

func mustTicketEventID(t *testing.T, client *ent.Client, ticketID string) string {
	t.Helper()
	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	if err != nil {
		t.Fatalf("get ticket %s: %v", ticketID, err)
	}
	return ticket.EventID
}

func decodeJSONMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode response json: %v", err)
	}
	return out
}

func toStringValue(v interface{}) string {
	s, _ := v.(string)
	return s
}

func mustGetBootstrapCookie(t *testing.T, recorder *httptest.ResponseRecorder, expectedPath string) *http.Cookie {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name != vncBootstrapCookieName {
			continue
		}
		if cookie.Path != expectedPath {
			t.Fatalf("cookie path = %q, want %q", cookie.Path, expectedPath)
		}
		if cookie.MaxAge <= 0 {
			t.Fatalf("cookie max-age = %d, want > 0", cookie.MaxAge)
		}
		return cookie
	}
	t.Fatalf("missing %q cookie", vncBootstrapCookieName)
	return nil
}
