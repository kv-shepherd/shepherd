package handlers

// Tests for ListApprovals covering the dynamic TicketPayload deserialization logic
// and vmTargetInfo enrichment introduced per the gap identified in untested_go_logic.md.
//
// Coverage matrix:
//   - ticketToAPI: TicketPayload is nil when payloadMap argument is nil
//   - ticketToAPI: TicketPayload is populated from deserialized payloadMap
//   - ListApprovals: CREATE operation ticket payload is populated (vm_create payload fields)
//   - ListApprovals: DELETE operation ticket payload is populated + TargetVmId/Name enriched
//   - ListApprovals: VNC_ACCESS operation ticket payload is populated
//   - ListApprovals: ticket with missing DomainEvent returns nil TicketPayload (non-fatal)
//   - ListApprovals: malformed JSON payload yields nil TicketPayload (non-fatal, logs warn)
//   - ListApprovals: pagination works correctly

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	"kv-shepherd.io/shepherd/ent/domainevent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

// ---- Pure unit tests for ticketToAPI converter -------------------------------------------

func TestTicketToAPI_NilPayload_WhenPayloadMapNil(t *testing.T) {
	t.Parallel()

	tick := &ent.Ticket{
		ID:            "ticket-1",
		EventID:       "event-1",
		OperationType: entticket.OperationTypeCREATE,
		Requester:     "user-a",
		Status:        entticket.StatusPENDING,
		CreatedAt:     time.Now(),
	}
	got := ticketToAPI(tick, nil, nil, nil, nil, approvalActorLookup{}, approvalActorLookup{})
	if got.TicketPayload != nil {
		t.Fatalf("TicketPayload = %v, want nil when payloadMap is nil", got.TicketPayload)
	}
}

func TestTicketToAPI_PopulatesTicketPayload(t *testing.T) {
	t.Parallel()

	payload := map[string]interface{}{
		"vm_id":   "vm-123",
		"vm_name": "my-vm",
		"reason":  "scale up",
	}
	tick := &ent.Ticket{
		ID:            "ticket-2",
		EventID:       "event-2",
		OperationType: entticket.OperationTypeCREATE,
		Requester:     "user-b",
		Status:        entticket.StatusPENDING,
		CreatedAt:     time.Now(),
	}
	got := ticketToAPI(tick, payload, nil, nil, nil, approvalActorLookup{}, approvalActorLookup{})
	if got.TicketPayload == nil {
		t.Fatal("TicketPayload is nil, want non-nil")
	}
	if got.TicketPayload["vm_id"] != "vm-123" {
		t.Fatalf("TicketPayload[vm_id] = %v, want %q", got.TicketPayload["vm_id"], "vm-123")
	}
	if got.TicketPayload["reason"] != "scale up" {
		t.Fatalf("TicketPayload[reason] = %v, want %q", got.TicketPayload["reason"], "scale up")
	}
}

func TestTicketToAPI_PopulatesPlacementEvaluation(t *testing.T) {
	t.Parallel()

	tick := &ent.Ticket{
		ID:            "ticket-placement",
		EventID:       "event-placement",
		OperationType: entticket.OperationTypeCREATE,
		Requester:     "user-c",
		Status:        entticket.StatusAPPROVED,
		PlacementEvaluation: map[string]interface{}{
			"selected_cluster_id":          "cluster-1",
			"selected_cluster_name":        "cluster-a",
			"selected_cluster_environment": "prod",
			"effective_storage_class":      "gold-sc",
			"eligible":                     true,
			"advisory_code":                "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
			"advisory_message":             "clone may fall back to host-assisted copy",
			"evaluated_at":                 "2026-03-08T00:00:00Z",
		},
		CreatedAt: time.Now(),
	}

	got := ticketToAPI(tick, nil, nil, nil, nil, approvalActorLookup{}, approvalActorLookup{})
	if got.PlacementEvaluation == nil {
		t.Fatal("PlacementEvaluation is nil, want non-nil")
	}
	if got.PlacementEvaluation.SelectedClusterId != "cluster-1" {
		t.Fatalf("SelectedClusterId = %q, want cluster-1", got.PlacementEvaluation.SelectedClusterId)
	}
	if !got.PlacementEvaluation.Eligible {
		t.Fatal("Eligible = false, want true")
	}
	if got.PlacementEvaluation.AdvisoryCode != "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY" {
		t.Fatalf("AdvisoryCode = %q, want PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY", got.PlacementEvaluation.AdvisoryCode)
	}
}

func TestTicketToAPI_PopulatesPlacementOverride(t *testing.T) {
	t.Parallel()

	tick := &ent.Ticket{
		ID:            "ticket-placement-override",
		EventID:       "event-placement-override",
		OperationType: entticket.OperationTypeCREATE,
		Requester:     "user-c",
		Status:        entticket.StatusAPPROVED,
		PlacementEvaluation: map[string]interface{}{
			"selected_cluster_id": "cluster-1",
			"eligible":            true,
			"override": map[string]interface{}{
				"enabled":           true,
				"cpu_request":       2.0,
				"cpu_limit":         4.0,
				"memory_request_gi": 8.0,
				"memory_limit_gi":   16.0,
				"disk_gb":           120.0,
			},
			"evaluated_at": "2026-03-08T00:00:00Z",
		},
		CreatedAt: time.Now(),
	}

	got := ticketToAPI(tick, nil, nil, nil, nil, approvalActorLookup{}, approvalActorLookup{})
	if got.PlacementEvaluation == nil {
		t.Fatal("PlacementEvaluation.Override is nil, want populated override")
	}
	if !got.PlacementEvaluation.Override.Enabled {
		t.Fatal("Override.Enabled = false, want true")
	}
	if got.PlacementEvaluation.Override.CpuRequest != 2 {
		t.Fatalf("Override.CpuRequest = %v, want 2", got.PlacementEvaluation.Override.CpuRequest)
	}
	if got.PlacementEvaluation.Override.DiskGb != 120 {
		t.Fatalf("Override.DiskGb = %v, want 120", got.PlacementEvaluation.Override.DiskGb)
	}
}

func TestTicketToAPI_OperationTypePassedThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opType  entticket.OperationType
		wantAPI generated.TicketOperationType
	}{
		{"CREATE", entticket.OperationTypeCREATE, generated.TicketOperationType("CREATE")},
		{"DELETE", entticket.OperationTypeDELETE, generated.TicketOperationType("DELETE")},
		{"POWER", entticket.OperationTypePOWER, generated.TicketOperationType("POWER")},
		{"VNC_ACCESS", entticket.OperationTypeVNC_ACCESS, generated.TicketOperationType("VNC_ACCESS")},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tick := &ent.Ticket{
				ID:            "ticket-" + tc.name,
				EventID:       "event-" + tc.name,
				OperationType: tc.opType,
				Requester:     "user-a",
				Status:        entticket.StatusPENDING,
				CreatedAt:     time.Now(),
			}
			got := ticketToAPI(tick, nil, nil, nil, nil, approvalActorLookup{}, approvalActorLookup{})
			if got.OperationType != tc.wantAPI {
				t.Fatalf("OperationType = %q, want %q", got.OperationType, tc.wantAPI)
			}
		})
	}
}

func TestBuildApprovalVMTargetItemSummary_SeparatesRequestAndLatestVMStatus(t *testing.T) {
	t.Parallel()

	got := buildApprovalVMTargetItemSummary(
		map[string]interface{}{
			"vm_id":             "vm-1",
			"vm_name":           "vm-live",
			"request_vm_status": "STOPPED",
		},
		nil,
		nil,
		map[string]approvalVMContext{
			"vm-1": {
				VMID:           "vm-1",
				VMName:         "vm-live",
				LatestVMStatus: "NOT_FOUND",
				OwnerID:        "owner-1",
			},
		},
		map[string]approvalActorLookup{
			"owner-1": {
				DisplayName: "Alice Ops",
				Username:    "alice.ops",
			},
		},
	)

	if got.RequestVmStatus != "STOPPED" {
		t.Fatalf("RequestVmStatus = %q, want STOPPED", got.RequestVmStatus)
	}
	if got.LatestVmStatus != "NOT_FOUND" {
		t.Fatalf("LatestVmStatus = %q, want NOT_FOUND", got.LatestVmStatus)
	}
	if got.VmStatus != "NOT_FOUND" {
		t.Fatalf("VmStatus(alias) = %q, want NOT_FOUND", got.VmStatus)
	}
	if got.OwnerDisplayName != "Alice Ops" {
		t.Fatalf("OwnerDisplayName = %q, want Alice Ops", got.OwnerDisplayName)
	}
	if got.OwnerUsername != "alice.ops" {
		t.Fatalf("OwnerUsername = %q, want alice.ops", got.OwnerUsername)
	}
}

// ---- Integration: ListApprovals ----------------------------------------------------------

func newApprovalTestServer(t *testing.T, prefix string) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, prefix)
	return NewServer(ServerDeps{
		EntClient:     client,
		TicketService: ticketing.NewService(client, nil, nil),
	}), client
}

func TestListApprovals_CREATE_TicketPayloadPopulated(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_create_payload")

	sys := mustCreateSystem(t, client, "11111111-1111-1111-1111-111111111111", "shop", "owner-1")
	svc := mustCreateService(t, client, "22222222-2222-2222-2222-222222222222", "checkout", sys.ID, "frontend")
	templateID := "33333333-3333-3333-3333-" + uuid.NewString()[24:]
	instanceSizeID := "44444444-4444-4444-4444-" + uuid.NewString()[24:]
	mustCreateApprovalTemplate(t, client, templateID)
	mustCreateApprovalInstanceSize(t, client, instanceSizeID)

	// Seed DomainEvent with a CREATE-style payload (service_id, namespace, reason fields).
	eventID := "ev-" + uuid.NewString()
	createPayload := map[string]interface{}{
		"service_id":       svc.ID,
		"namespace":        "team-prod",
		"reason":           "need a VM",
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
	}
	rawPayload, _ := json.Marshal(createPayload)
	mustCreateDomainEvent(t, client, eventID, rawPayload)

	ticketID := "ticket-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.TicketPayload == nil {
		t.Fatal("TicketPayload is nil, want non-nil for CREATE ticket")
	}
	if got := found.TicketPayload["template_label"]; got != "Ubuntu 22.04" {
		t.Fatalf("template_label = %v, want %q", got, "Ubuntu 22.04")
	}
	if got := found.TicketPayload["instance_size_label"]; got != "M4 Large" {
		t.Fatalf("instance_size_label = %v, want %q", got, "M4 Large")
	}
	if got := found.TicketPayload["instance_size_disk_gb"]; got != float64(80) {
		t.Fatalf("instance_size_disk_gb = %v, want 80", got)
	}
	if got := found.TicketPayload["instance_size_dedicated_cpu"]; got != true {
		t.Fatalf("instance_size_dedicated_cpu = %v, want true", got)
	}
	if found.OperationType != generated.TicketOperationType("CREATE") {
		t.Fatalf("OperationType = %q, want CREATE", found.OperationType)
	}
	if found.RequestPrefill.SystemId.String() == "" {
		t.Fatal("RequestPrefill.SystemId is empty, want populated reusable CREATE prefill")
	}
	if found.RequestPrefill.SystemId.String() != sys.ID {
		t.Fatalf("RequestPrefill.SystemId = %q, want %q", found.RequestPrefill.SystemId.String(), sys.ID)
	}
	if found.RequestPrefill.ServiceId.String() != svc.ID {
		t.Fatalf("RequestPrefill.ServiceId = %q, want %q", found.RequestPrefill.ServiceId.String(), svc.ID)
	}
	if found.RequestPrefill.TemplateId.String() != templateID {
		t.Fatalf("RequestPrefill.TemplateId = %q, want %q", found.RequestPrefill.TemplateId.String(), templateID)
	}
	if found.RequestPrefill.InstanceSizeId.String() != instanceSizeID {
		t.Fatalf("RequestPrefill.InstanceSizeId = %q, want %q", found.RequestPrefill.InstanceSizeId.String(), instanceSizeID)
	}
	if found.RequestPrefill.Namespace != "team-prod" {
		t.Fatalf("RequestPrefill.Namespace = %q, want %q", found.RequestPrefill.Namespace, "team-prod")
	}
	if found.RequestPrefill.Reason != "need a VM" {
		t.Fatalf("RequestPrefill.Reason = %q, want %q", found.RequestPrefill.Reason, "need a VM")
	}
	if found.RequestPrefill.BatchCount != 1 {
		t.Fatalf("RequestPrefill.BatchCount = %d, want 1", found.RequestPrefill.BatchCount)
	}
	if found.Summary == nil {
		t.Fatal("Summary is nil, want non-nil for CREATE ticket")
	}
	if found.Summary.SystemId != sys.ID {
		t.Fatalf("Summary.SystemId = %q, want %q", found.Summary.SystemId, sys.ID)
	}
	if found.Summary.SystemName != sys.Name {
		t.Fatalf("Summary.SystemName = %q, want %q", found.Summary.SystemName, sys.Name)
	}
	if found.Summary.ServiceId != svc.ID {
		t.Fatalf("Summary.ServiceId = %q, want %q", found.Summary.ServiceId, svc.ID)
	}
	if found.Summary.ServiceName != svc.Name {
		t.Fatalf("Summary.ServiceName = %q, want %q", found.Summary.ServiceName, svc.Name)
	}
	if found.Summary.Namespace != "team-prod" {
		t.Fatalf("Summary.Namespace = %q, want %q", found.Summary.Namespace, "team-prod")
	}
	if found.Summary.TemplateId != templateID {
		t.Fatalf("Summary.TemplateId = %q, want %q", found.Summary.TemplateId, templateID)
	}
	if found.Summary.TemplateName != "Ubuntu 22.04" {
		t.Fatalf("Summary.TemplateName = %q, want %q", found.Summary.TemplateName, "Ubuntu 22.04")
	}
	if found.Summary.InstanceSizeId != instanceSizeID {
		t.Fatalf("Summary.InstanceSizeId = %q, want %q", found.Summary.InstanceSizeId, instanceSizeID)
	}
	if found.Summary.InstanceSizeName != "M4 Large" {
		t.Fatalf("Summary.InstanceSizeName = %q, want %q", found.Summary.InstanceSizeName, "M4 Large")
	}
	if found.Summary.TargetCpuCores != 4 {
		t.Fatalf("Summary.TargetCpuCores = %v, want 4", found.Summary.TargetCpuCores)
	}
	if found.Summary.TargetMemoryGi != 8 {
		t.Fatalf("Summary.TargetMemoryGi = %v, want 8", found.Summary.TargetMemoryGi)
	}
	if found.Summary.TargetDiskGb != 80 {
		t.Fatalf("Summary.TargetDiskGb = %d, want 80", found.Summary.TargetDiskGb)
	}
}

func TestListApprovals_CREATE_BatchParentRequestPrefillPopulated(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_create_batch_prefill")

	sys := mustCreateSystem(t, client, "55555555-5555-5555-5555-555555555555", "shop", "owner-1")
	svc := mustCreateService(t, client, "66666666-6666-6666-6666-666666666666", "checkout", sys.ID, "frontend")
	templateID := "77777777-7777-7777-7777-" + uuid.NewString()[24:]
	instanceSizeID := "88888888-8888-8888-8888-" + uuid.NewString()[24:]
	mustCreateApprovalTemplate(t, client, templateID)
	mustCreateApprovalInstanceSize(t, client, instanceSizeID)

	eventID := "ev-batch-" + uuid.NewString()
	createPayload := map[string]interface{}{
		"operation": "CREATE",
		"reason":    "scale service",
		"items": []map[string]interface{}{
			{
				"service_id":       svc.ID,
				"template_id":      templateID,
				"instance_size_id": instanceSizeID,
				"namespace":        "team-prod",
				"reason":           "scale service",
			},
			{
				"service_id":       svc.ID,
				"template_id":      templateID,
				"instance_size_id": instanceSizeID,
				"namespace":        "team-prod",
				"reason":           "scale service",
			},
		},
	}
	rawPayload, _ := json.Marshal(createPayload)
	mustCreateDomainEventWithAggregate(t, client, eventID, "batch", "batch-parent-1", rawPayload)

	ticketID := "ticket-batch-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.RequestPrefill.SystemId.String() == "" {
		t.Fatal("RequestPrefill.SystemId is empty, want populated reusable batch prefill")
	}
	if found.RequestPrefill.SystemId.String() != sys.ID {
		t.Fatalf("RequestPrefill.SystemId = %q, want %q", found.RequestPrefill.SystemId.String(), sys.ID)
	}
	if found.RequestPrefill.ServiceId.String() != svc.ID {
		t.Fatalf("RequestPrefill.ServiceId = %q, want %q", found.RequestPrefill.ServiceId.String(), svc.ID)
	}
	if found.RequestPrefill.BatchCount != 2 {
		t.Fatalf("RequestPrefill.BatchCount = %d, want 2", found.RequestPrefill.BatchCount)
	}
	if found.RequestPrefill.Reason != "scale service" {
		t.Fatalf("RequestPrefill.Reason = %q, want %q", found.RequestPrefill.Reason, "scale service")
	}
}

func TestListApprovals_HidesBatchChildTicketsFromMainList(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_hide_batch_children")

	parentEventID := "ev-parent-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(t, client, parentEventID, "batch", "batch-parent-1", []byte(`{"operation":"CREATE","items":[{"namespace":"prod-a"}]}`))
	parentTicketID := "ticket-parent-" + uuid.NewString()
	mustCreateTicket(t, client, parentTicketID, parentEventID, entticket.OperationTypeCREATE, "user-a")

	childEventID := "ev-child-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(t, client, childEventID, "vm", "vm-child-1", []byte(`{"namespace":"prod-a"}`))
	_, err := client.Ticket.Create().
		SetID("ticket-child-" + uuid.NewString()).
		SetEventID(childEventID).
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("user-a").
		SetParentTicketID(parentTicketID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create child ticket: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != parentTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, parentTicketID)
	}
}

func TestListApprovals_DELETE_TicketPayloadAndVMTargetEnriched(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_delete_payload")

	sys := mustCreateSystem(t, client, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "payments", "owner-1")
	svc := mustCreateService(t, client, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "billing", sys.ID, "worker")
	templateID := "cccccccc-cccc-cccc-cccc-" + uuid.NewString()[24:]
	instanceSizeID := "dddddddd-dddd-dddd-dddd-" + uuid.NewString()[24:]
	clusterID := "eeeeeeee-eeee-eeee-eeee-" + uuid.NewString()[24:]
	createEventID := "ev-create-" + uuid.NewString()
	createTicketID := "ticket-create-" + uuid.NewString()
	mustCreateApprovalTemplate(t, client, templateID)
	mustCreateApprovalInstanceSize(t, client, instanceSizeID)
	mustCreateDomainEvent(t, client, createEventID, mustApprovalJSON(t, map[string]interface{}{
		"service_id":       svc.ID,
		"namespace":        "team-prod",
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
		"reason":           "seed vm",
	}))
	mustCreateTicket(t, client, createTicketID, createEventID, entticket.OperationTypeCREATE, "user-a")
	mustCreateClusterWithEnv(t, client, clusterID, entcluster.EnvironmentProd)
	if err := client.Cluster.UpdateOneID(clusterID).
		SetDisplayName("Prod Cluster A").
		Exec(t.Context()); err != nil {
		t.Fatalf("set cluster display name: %v", err)
	}
	if _, err := client.VM.Create().
		SetID("vm-del-123").
		SetName("my-delete-target").
		SetInstance("01").
		SetNamespace("team-prod").
		SetStatus(entvm.StatusRUNNING).
		SetCreatedBy("user-a").
		SetServiceID(svc.ID).
		SetClusterID(clusterID).
		SetTicketID(createTicketID).
		Save(t.Context()); err != nil {
		t.Fatalf("create delete target VM: %v", err)
	}

	eventID := "ev-" + uuid.NewString()
	// DELETE events carry vm_id + vm_name in their payload (domain.VMDeletePayload shape).
	deletePayload := map[string]interface{}{
		"vm_id":   "vm-del-123",
		"vm_name": "my-delete-target",
		"actor":   "user-a",
	}
	rawPayload, _ := json.Marshal(deletePayload)
	mustCreateDomainEvent(t, client, eventID, rawPayload)

	ticketID := "ticket-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeDELETE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)

	// TicketPayload must be populated.
	if found.TicketPayload == nil {
		t.Fatal("TicketPayload is nil, want non-nil for DELETE ticket")
	}
	// DELETE-specific enrichment: TargetVmId and TargetVmName must be set.
	if found.TargetVmId != "vm-del-123" {
		t.Fatalf("TargetVmId = %q, want %q", found.TargetVmId, "vm-del-123")
	}
	if found.TargetVmName != "my-delete-target" {
		t.Fatalf("TargetVmName = %q, want %q", found.TargetVmName, "my-delete-target")
	}
	if found.Summary == nil {
		t.Fatal("Summary is nil, want non-nil for DELETE ticket")
	}
	if !found.Summary.Irreversible {
		t.Fatal("Summary.Irreversible = false, want true for DELETE ticket")
	}
	if found.Summary.VmId != "vm-del-123" {
		t.Fatalf("Summary.VmId = %q, want %q", found.Summary.VmId, "vm-del-123")
	}
	if found.Summary.VmName != "my-delete-target" {
		t.Fatalf("Summary.VmName = %q, want %q", found.Summary.VmName, "my-delete-target")
	}
	if found.Summary.VmStatus != string(entvm.StatusRUNNING) {
		t.Fatalf("Summary.VmStatus = %q, want %q", found.Summary.VmStatus, entvm.StatusRUNNING)
	}
	if found.Summary.SystemId != sys.ID {
		t.Fatalf("Summary.SystemId = %q, want %q", found.Summary.SystemId, sys.ID)
	}
	if found.Summary.SystemName != sys.Name {
		t.Fatalf("Summary.SystemName = %q, want %q", found.Summary.SystemName, sys.Name)
	}
	if found.Summary.ServiceId != svc.ID {
		t.Fatalf("Summary.ServiceId = %q, want %q", found.Summary.ServiceId, svc.ID)
	}
	if found.Summary.ServiceName != svc.Name {
		t.Fatalf("Summary.ServiceName = %q, want %q", found.Summary.ServiceName, svc.Name)
	}
	if found.Summary.Namespace != "team-prod" {
		t.Fatalf("Summary.Namespace = %q, want %q", found.Summary.Namespace, "team-prod")
	}
	if found.Summary.ClusterId != clusterID {
		t.Fatalf("Summary.ClusterId = %q, want %q", found.Summary.ClusterId, clusterID)
	}
	if found.Summary.ClusterName != "Prod Cluster A" {
		t.Fatalf("Summary.ClusterName = %q, want %q", found.Summary.ClusterName, "Prod Cluster A")
	}
	if found.Summary.ClusterEnvironment != string(entcluster.EnvironmentProd) {
		t.Fatalf("Summary.ClusterEnvironment = %q, want %q", found.Summary.ClusterEnvironment, entcluster.EnvironmentProd)
	}
	if found.Summary.TemplateId != templateID {
		t.Fatalf("Summary.TemplateId = %q, want %q", found.Summary.TemplateId, templateID)
	}
	if found.Summary.TemplateName != "Ubuntu 22.04" {
		t.Fatalf("Summary.TemplateName = %q, want %q", found.Summary.TemplateName, "Ubuntu 22.04")
	}
	if found.Summary.InstanceSizeId != instanceSizeID {
		t.Fatalf("Summary.InstanceSizeId = %q, want %q", found.Summary.InstanceSizeId, instanceSizeID)
	}
	if found.Summary.InstanceSizeName != "M4 Large" {
		t.Fatalf("Summary.InstanceSizeName = %q, want %q", found.Summary.InstanceSizeName, "M4 Large")
	}
}

func TestListApprovals_VNC_ACCESS_TicketPayloadPopulated(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_vnc_payload")

	eventID := "ev-" + uuid.NewString()
	vncPayload := map[string]interface{}{
		"vm_id":     "vm-vnc-001",
		"requester": "user-vnc",
		"duration":  "1h",
	}
	rawPayload, _ := json.Marshal(vncPayload)
	mustCreateDomainEvent(t, client, eventID, rawPayload)

	ticketID := "ticket-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeVNC_ACCESS, "user-vnc")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.TicketPayload == nil {
		t.Fatal("TicketPayload is nil, want non-nil for VNC_ACCESS ticket")
	}
	if found.OperationType != generated.TicketOperationType("VNC_ACCESS") {
		t.Fatalf("OperationType = %q, want VNC_ACCESS", found.OperationType)
	}
	// Non-DELETE tickets should not expose DELETE-only target fields.
	if found.TargetVmId != "" {
		t.Fatalf("TargetVmId = %q, want empty for non-DELETE ticket", found.TargetVmId)
	}
	if found.TargetVmName != "" {
		t.Fatalf("TargetVmName = %q, want empty for non-DELETE ticket", found.TargetVmName)
	}
}

func TestListApprovals_MissingDomainEvent_NilPayloadNonFatal(t *testing.T) {
	t.Parallel()

	// Ticket references an event_id that does NOT exist in domain_events.
	// The handler must still return 200 with nil TicketPayload (non-fatal path).
	srv, client := newApprovalTestServer(t, "approval_missing_event")

	ticketID := "ticket-" + uuid.NewString()
	// Use a non-existent event ID to simulate missing DomainEvent.
	mustCreateTicket(t, client, ticketID, "ev-does-not-exist", entticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	// Must still return 200 (non-fatal: log warn and continue).
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	// No domain event → TicketPayload should be nil.
	if found.TicketPayload != nil {
		t.Fatalf("TicketPayload = %v, want nil when event is missing", found.TicketPayload)
	}
}

func TestListApprovals_MalformedEventPayload_NilPayloadNonFatal(t *testing.T) {
	t.Parallel()

	// DomainEvent exists but has malformed JSON payload.
	// Handler must log warn and still return the ticket with nil TicketPayload.
	srv, client := newApprovalTestServer(t, "approval_malformed_payload")

	eventID := "ev-" + uuid.NewString()
	// store invalid JSON bytes directly.
	mustCreateDomainEvent(t, client, eventID, []byte("not-valid-json{{"))

	ticketID := "ticket-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	// Malformed payload → deserialization fails → TicketPayload must be nil.
	if found.TicketPayload != nil {
		t.Fatalf("TicketPayload = %v, want nil when event payload is malformed", found.TicketPayload)
	}
}

func TestListApprovals_FiltersByOperationType(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_filter_op_type")

	createEventID := "ev-create-" + uuid.NewString()
	mustCreateDomainEvent(t, client, createEventID, []byte(`{"seed":"create"}`))
	createTicketID := "ticket-create-" + uuid.NewString()
	mustCreateTicket(t, client, createTicketID, createEventID, entticket.OperationTypeCREATE, "user-a")

	powerEventID := "ev-power-" + uuid.NewString()
	mustCreateDomainEvent(t, client, powerEventID, []byte(`{"seed":"power"}`))
	powerTicketID := "ticket-power-" + uuid.NewString()
	mustCreateTicket(t, client, powerTicketID, powerEventID, entticket.OperationTypePOWER, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets?operation_type=POWER", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{
		OperationType: generated.ListTicketsParamsOperationType("POWER"),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != powerTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, powerTicketID)
	}
}

func TestListApprovals_SupportsQuickSearchAcrossTicketIDRequesterAndSelectedCluster(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_search")

	firstEventID := "ev-search-first-" + uuid.NewString()
	mustCreateDomainEvent(t, client, firstEventID, []byte(`{"seed":"search-first"}`))
	firstTicketID := "ticket-search-first-" + uuid.NewString()
	mustCreateTicket(t, client, firstTicketID, firstEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(firstTicketID).
		SetSelectedClusterID("cluster-finance").
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_id":   "cluster-finance",
			"selected_cluster_name": "Finance Production",
		}).
		Exec(t.Context()); err != nil {
		t.Fatalf("update first ticket cluster: %v", err)
	}

	secondEventID := "ev-search-second-" + uuid.NewString()
	mustCreateDomainEvent(t, client, secondEventID, []byte(`{"seed":"search-second"}`))
	secondTicketID := "ticket-search-second-" + uuid.NewString()
	mustCreateTicket(t, client, secondTicketID, secondEventID, entticket.OperationTypeCREATE, "approver-b")

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/tickets?search=Finance%20Production",
		"",
		"admin-1",
		[]string{"ticket:view", "platform:admin"},
	)
	srv.ListTickets(c, generated.ListTicketsParams{
		Search: "Finance Production",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != firstTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, firstTicketID)
	}
}

func TestListApprovals_SupportsQuickSearchAcrossReason(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_search_reason")

	firstEventID := "ev-search-reason-first-" + uuid.NewString()
	mustCreateDomainEvent(t, client, firstEventID, []byte(`{"seed":"search-reason-first"}`))
	firstTicketID := "ticket-search-reason-first-" + uuid.NewString()
	mustCreateTicket(t, client, firstTicketID, firstEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(firstTicketID).
		SetReason("Restore finance database backups").
		Exec(t.Context()); err != nil {
		t.Fatalf("update first ticket reason: %v", err)
	}

	secondEventID := "ev-search-reason-second-" + uuid.NewString()
	mustCreateDomainEvent(t, client, secondEventID, []byte(`{"seed":"search-reason-second"}`))
	secondTicketID := "ticket-search-reason-second-" + uuid.NewString()
	mustCreateTicket(t, client, secondTicketID, secondEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(secondTicketID).
		SetReason("Rotate production certificates").
		Exec(t.Context()); err != nil {
		t.Fatalf("update second ticket reason: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/tickets?search=finance%20database",
		"",
		"admin-1",
		[]string{"ticket:view", "platform:admin"},
	)
	srv.ListTickets(c, generated.ListTicketsParams{
		Search: "finance database",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != firstTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, firstTicketID)
	}
}

func TestListApprovals_IncludesReadableRequesterAndApproverFields(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_actor_labels")

	requesterID := "user-requester-" + uuid.NewString()
	approverID := "user-approver-" + uuid.NewString()
	mustCreateApprovalUser(t, client, requesterID, "alice.ops", "Alice Ops")
	mustCreateApprovalUser(t, client, approverID, "bob.ops", "Bob Ops")

	eventID := "ev-actor-labels-" + uuid.NewString()
	mustCreateDomainEvent(t, client, eventID, []byte(`{"seed":"actor-labels"}`))
	ticketID := "ticket-actor-labels-" + uuid.NewString()
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, requesterID)
	if err := client.Ticket.UpdateOneID(ticketID).
		SetApprover(approverID).
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket approver: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.RequesterDisplayName != "Alice Ops" {
		t.Fatalf("requester_display_name = %q, want %q", found.RequesterDisplayName, "Alice Ops")
	}
	if found.RequesterUsername != "alice.ops" {
		t.Fatalf("requester_username = %q, want %q", found.RequesterUsername, "alice.ops")
	}
	if found.ApproverDisplayName != "Bob Ops" {
		t.Fatalf("approver_display_name = %q, want %q", found.ApproverDisplayName, "Bob Ops")
	}
	if found.ApproverUsername != "bob.ops" {
		t.Fatalf("approver_username = %q, want %q", found.ApproverUsername, "bob.ops")
	}
}

func TestListApprovals_FiltersBySelectedClusterAndPlacementSnapshot(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_filter_placement")

	withPlacementEventID := "ev-placement-" + uuid.NewString()
	mustCreateDomainEvent(t, client, withPlacementEventID, []byte(`{"seed":"placement"}`))
	withPlacementTicketID := "ticket-placement-" + uuid.NewString()
	mustCreateTicket(t, client, withPlacementTicketID, withPlacementEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(withPlacementTicketID).
		SetStatus(entticket.StatusAPPROVED).
		SetSelectedClusterID("cluster-a").
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_id":          "cluster-a",
			"selected_cluster_name":        "cluster-a-name",
			"selected_cluster_environment": "prod",
			"eligible":                     true,
			"evaluated_at":                 "2026-03-08T00:00:00Z",
		}).
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket with placement: %v", err)
	}

	withoutPlacementEventID := "ev-no-placement-" + uuid.NewString()
	mustCreateDomainEvent(t, client, withoutPlacementEventID, []byte(`{"seed":"no-placement"}`))
	withoutPlacementTicketID := "ticket-no-placement-" + uuid.NewString()
	mustCreateTicket(t, client, withoutPlacementTicketID, withoutPlacementEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(withoutPlacementTicketID).
		SetStatus(entticket.StatusAPPROVED).
		SetSelectedClusterID("cluster-b").
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket without placement: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets?selected_cluster_id=cluster-a&placement_snapshot=present", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{
		SelectedClusterId: "cluster-a",
		PlacementSnapshot: generated.ListTicketsParamsPlacementSnapshot("present"),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != withPlacementTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, withPlacementTicketID)
	}
}

func TestListApprovals_FiltersByPlacementAdvisoryCode(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_filter_advisory")

	withAdvisoryEventID := "ev-advisory-" + uuid.NewString()
	mustCreateDomainEvent(t, client, withAdvisoryEventID, []byte(`{"seed":"advisory"}`))
	withAdvisoryTicketID := "ticket-advisory-" + uuid.NewString()
	mustCreateTicket(t, client, withAdvisoryTicketID, withAdvisoryEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(withAdvisoryTicketID).
		SetStatus(entticket.StatusAPPROVED).
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_id": "cluster-a",
			"eligible":            true,
			"advisory_code":       "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
			"advisory_message":    "clone may fall back to host-assisted copy",
			"evaluated_at":        "2026-03-08T00:00:00Z",
		}).
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket with placement advisory: %v", err)
	}

	withoutAdvisoryEventID := "ev-no-advisory-" + uuid.NewString()
	mustCreateDomainEvent(t, client, withoutAdvisoryEventID, []byte(`{"seed":"no-advisory"}`))
	withoutAdvisoryTicketID := "ticket-no-advisory-" + uuid.NewString()
	mustCreateTicket(t, client, withoutAdvisoryTicketID, withoutAdvisoryEventID, entticket.OperationTypeCREATE, "user-a")
	if err := client.Ticket.UpdateOneID(withoutAdvisoryTicketID).
		SetStatus(entticket.StatusAPPROVED).
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_id": "cluster-b",
			"eligible":            true,
			"evaluated_at":        "2026-03-08T00:00:00Z",
		}).
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket without placement advisory: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets?placement_advisory_code=PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{
		PlacementAdvisoryCode: "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != withAdvisoryTicketID {
		t.Fatalf("ticket id = %q, want %q", resp.Items[0].Id, withAdvisoryTicketID)
	}
}

func TestListApprovals_PaginationReturnsTotalAndPages(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_pagination")

	// Create 3 tickets.
	for i := range 3 {
		eventID := "ev-pag-" + uuid.NewString()
		mustCreateDomainEvent(t, client, eventID, []byte(`{"seed":true}`))
		ticketID := "ticket-pag-" + uuid.NewString()
		_ = i
		mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-pag")
	}

	// Request page 1 with perPage=2 → total must be ≥ 3, total_pages ≥ 2.
	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets?page=1&per_page=2", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{
		Page:    1,
		PerPage: 2,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Pagination.Total < 3 {
		t.Fatalf("pagination.total = %d, want >= 3", resp.Pagination.Total)
	}
	if resp.Pagination.TotalPages < 2 {
		t.Fatalf("pagination.total_pages = %d, want >= 2", resp.Pagination.TotalPages)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("items length = %d, want 2 (per_page=2)", len(resp.Items))
	}
}

func TestListApprovals_ReturnsNewestTicketsFirst(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_newest_first")

	olderEventID := "ev-oldest-" + uuid.NewString()
	mustCreateDomainEvent(t, client, olderEventID, []byte(`{"seed":"older"}`))
	olderCreatedAt := time.Now().Add(-2 * time.Hour).UTC()
	olderTicketID := "ticket-oldest-" + uuid.NewString()
	if _, err := client.Ticket.Create().
		SetID(olderTicketID).
		SetEventID(olderEventID).
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("user-oldest").
		SetCreatedAt(olderCreatedAt).
		Save(t.Context()); err != nil {
		t.Fatalf("create older ticket: %v", err)
	}

	newerEventID := "ev-newest-" + uuid.NewString()
	mustCreateDomainEvent(t, client, newerEventID, []byte(`{"seed":"newer"}`))
	newerCreatedAt := olderCreatedAt.Add(90 * time.Minute)
	newerTicketID := "ticket-newest-" + uuid.NewString()
	if _, err := client.Ticket.Create().
		SetID(newerTicketID).
		SetEventID(newerEventID).
		SetOperationType(entticket.OperationTypeCREATE).
		SetStatus(entticket.StatusPENDING).
		SetRequester("user-newest").
		SetCreatedAt(newerCreatedAt).
		Save(t.Context()); err != nil {
		t.Fatalf("create newer ticket: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets", "", "admin-1", []string{"ticket:view", "platform:admin"})
	srv.ListTickets(c, generated.ListTicketsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) < 2 {
		t.Fatalf("items length = %d, want at least 2", len(resp.Items))
	}
	if resp.Items[0].Id != newerTicketID {
		t.Fatalf("first ticket id = %q, want newest %q", resp.Items[0].Id, newerTicketID)
	}
	if resp.Items[1].Id != olderTicketID {
		t.Fatalf("second ticket id = %q, want older %q", resp.Items[1].Id, olderTicketID)
	}
}

func TestListApprovals_MineFiltersByRequesterWithoutApprovalViewPermission(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_list_mine_filter")

	mustCreateDomainEvent(t, client, "ev-mine-a", []byte(`{"reason":"mine"}`))
	mustCreateTicket(t, client, "ticket-mine-a", "ev-mine-a", entticket.OperationTypeCREATE, "user-a")
	mustCreateDomainEvent(t, client, "ev-mine-b", []byte(`{"reason":"other"}`))
	mustCreateTicket(t, client, "ticket-mine-b", "ev-mine-b", entticket.OperationTypeCREATE, "user-b")

	c, w := newAuthedGinContext(t, http.MethodGet, "/tickets?mine=true", "", "user-a", nil)
	srv.ListTickets(c, generated.ListTicketsParams{Mine: true})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.TicketList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode TicketList: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != "ticket-mine-a" {
		t.Fatalf("ticket id = %q, want ticket-mine-a", resp.Items[0].Id)
	}
}

func TestCancelTicket_RequesterDoesNotRequireGlobalPermission(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_cancel_requester")

	eventID := "ev-cancel-own"
	ticketID := "ticket-cancel-own"
	mustCreateDomainEvent(t, client, eventID, []byte(`{"reason":"cancel me"}`))
	mustCreateTicket(t, client, ticketID, eventID, entticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodPost, "/tickets/"+ticketID+"/cancel", "", "user-a", nil)
	srv.CancelTicket(c, ticketID)

	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}

	ticket, err := client.Ticket.Get(t.Context(), ticketID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Status != entticket.StatusCANCELLED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, entticket.StatusCANCELLED)
	}
}

// ---- local seed helpers ------------------------------------------------------------------

func mustCreateDomainEvent(t *testing.T, client *ent.Client, eventID string, payload []byte) {
	t.Helper()
	mustCreateDomainEventWithAggregate(t, client, eventID, "vm", "ag-"+uuid.NewString(), payload)
}

func mustCreateDomainEventWithAggregate(
	t *testing.T,
	client *ent.Client,
	eventID, aggregateType, aggregateID string,
	payload []byte,
) {
	t.Helper()
	_, err := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("vm.create_requested").
		SetAggregateType(aggregateType).
		SetAggregateID(aggregateID).
		SetPayload(payload).
		SetStatus(domainevent.StatusPENDING).
		SetCreatedBy("test-seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create domain event %s: %v", eventID, err)
	}
}

func mustCreateTicket(
	t *testing.T,
	client *ent.Client,
	ticketID, eventID string,
	opType entticket.OperationType,
	requester string,
) {
	t.Helper()
	_, err := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(opType).
		SetStatus(entticket.StatusPENDING).
		SetRequester(requester).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create ticket %s: %v", ticketID, err)
	}
}

func mustCreateApprovalTemplate(
	t *testing.T,
	client *ent.Client,
	id string,
) {
	t.Helper()
	_, err := client.Template.Create().
		SetID(id).
		SetName("ubuntu-golden").
		SetDisplayName("Ubuntu 22.04").
		SetCreatedBy("test-seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template %s: %v", id, err)
	}
}

func mustCreateApprovalInstanceSize(
	t *testing.T,
	client *ent.Client,
	id string,
) {
	t.Helper()
	create := client.InstanceSize.Create().
		SetID(id).
		SetName("m4.large").
		SetDisplayName("M4 Large").
		SetCPUCores(4).
		SetMemoryGi(8).
		SetDiskGB(80).
		SetDedicatedCPU(true).
		SetCreatedBy("test-seed")
	if _, err := create.Save(t.Context()); err != nil {
		t.Fatalf("create instance size %s: %v", id, err)
	}
}

func mustApprovalJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}

func mustCreateApprovalUser(
	t *testing.T,
	client *ent.Client,
	id, username, displayName string,
) {
	t.Helper()
	create := client.User.Create().
		SetID(id).
		SetUsername(username).
		SetEnabled(true)
	if strings.TrimSpace(displayName) != "" {
		create = create.SetDisplayName(displayName)
	}
	if _, err := create.Save(t.Context()); err != nil {
		t.Fatalf("create user %s: %v", id, err)
	}
}

// findTicketInList decodes the response body as TicketList and returns the
// ticket matching the given id. Calls t.Fatal if the ticket is not found.
func findTicketInList(t *testing.T, body []byte, ticketID string) generated.Ticket {
	t.Helper()
	var resp generated.TicketList
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode TicketList: %v", err)
	}
	for i := range resp.Items {
		item := &resp.Items[i]
		if item.Id == ticketID {
			return *item
		}
	}
	t.Fatalf("ticket %q not found in list response", ticketID)
	return generated.Ticket{}
}
