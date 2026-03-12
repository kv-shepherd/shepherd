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
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

// ---- Pure unit tests for ticketToAPI converter -------------------------------------------

func TestTicketToAPI_NilPayload_WhenPayloadMapNil(t *testing.T) {
	t.Parallel()

	tick := &ent.ApprovalTicket{
		ID:            "ticket-1",
		EventID:       "event-1",
		OperationType: approvalticket.OperationTypeCREATE,
		Requester:     "user-a",
		Status:        approvalticket.StatusPENDING,
		CreatedAt:     time.Now(),
	}
	got := ticketToAPI(tick, nil, nil)
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
	tick := &ent.ApprovalTicket{
		ID:            "ticket-2",
		EventID:       "event-2",
		OperationType: approvalticket.OperationTypeCREATE,
		Requester:     "user-b",
		Status:        approvalticket.StatusPENDING,
		CreatedAt:     time.Now(),
	}
	got := ticketToAPI(tick, payload, nil)
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

	tick := &ent.ApprovalTicket{
		ID:            "ticket-placement",
		EventID:       "event-placement",
		OperationType: approvalticket.OperationTypeCREATE,
		Requester:     "user-c",
		Status:        approvalticket.StatusAPPROVED,
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

	got := ticketToAPI(tick, nil, nil)
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

func TestTicketToAPI_OperationTypePassedThrough(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opType  approvalticket.OperationType
		wantAPI generated.ApprovalTicketOperationType
	}{
		{"CREATE", approvalticket.OperationTypeCREATE, generated.ApprovalTicketOperationType("CREATE")},
		{"DELETE", approvalticket.OperationTypeDELETE, generated.ApprovalTicketOperationType("DELETE")},
		{"POWER", approvalticket.OperationTypePOWER, generated.ApprovalTicketOperationType("POWER")},
		{"VNC_ACCESS", approvalticket.OperationTypeVNC_ACCESS, generated.ApprovalTicketOperationType("VNC_ACCESS")},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tick := &ent.ApprovalTicket{
				ID:            "ticket-" + tc.name,
				EventID:       "event-" + tc.name,
				OperationType: tc.opType,
				Requester:     "user-a",
				Status:        approvalticket.StatusPENDING,
				CreatedAt:     time.Now(),
			}
			got := ticketToAPI(tick, nil, nil)
			if got.OperationType != tc.wantAPI {
				t.Fatalf("OperationType = %q, want %q", got.OperationType, tc.wantAPI)
			}
		})
	}
}

// ---- Integration: ListApprovals ----------------------------------------------------------

func newApprovalTestServer(t *testing.T, prefix string) (*Server, *ent.Client) {
	t.Helper()
	_ = logger.Init("error", "json")
	client := testutil.OpenEntPostgres(t, prefix)
	return NewServer(ServerDeps{EntClient: client}), client
}

func TestListApprovals_CREATE_TicketPayloadPopulated(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_create_payload")

	templateID := "tpl-" + uuid.NewString()
	instanceSizeID := "size-" + uuid.NewString()
	mustCreateApprovalTemplate(t, client, templateID, "ubuntu-golden", "Ubuntu 22.04")
	mustCreateApprovalInstanceSize(t, client, instanceSizeID, "m4.large", "M4 Large", 80, true)

	// Seed DomainEvent with a CREATE-style payload (service_id, namespace, reason fields).
	eventID := "ev-" + uuid.NewString()
	createPayload := map[string]interface{}{
		"service_id":       "svc-abc",
		"namespace":        "team-prod",
		"reason":           "need a VM",
		"template_id":      templateID,
		"instance_size_id": instanceSizeID,
	}
	rawPayload, _ := json.Marshal(createPayload)
	mustCreateDomainEvent(t, client, eventID, rawPayload)

	ticketID := "ticket-" + uuid.NewString()
	mustCreateApprovalTicket(t, client, ticketID, eventID, approvalticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

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
	if found.OperationType != generated.ApprovalTicketOperationType("CREATE") {
		t.Fatalf("OperationType = %q, want CREATE", found.OperationType)
	}
}

func TestListApprovals_HidesBatchChildTicketsFromMainList(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_hide_batch_children")

	parentEventID := "ev-parent-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(t, client, parentEventID, "batch", "batch-parent-1", []byte(`{"operation":"CREATE","items":[{"namespace":"prod-a"}]}`))
	parentTicketID := "ticket-parent-" + uuid.NewString()
	mustCreateApprovalTicket(t, client, parentTicketID, parentEventID, approvalticket.OperationTypeCREATE, "user-a")

	childEventID := "ev-child-" + uuid.NewString()
	mustCreateDomainEventWithAggregate(t, client, childEventID, "vm", "vm-child-1", []byte(`{"namespace":"prod-a"}`))
	_, err := client.ApprovalTicket.Create().
		SetID("ticket-child-" + uuid.NewString()).
		SetEventID(childEventID).
		SetOperationType(approvalticket.OperationTypeCREATE).
		SetStatus(approvalticket.StatusPENDING).
		SetRequester("user-a").
		SetParentTicketID(parentTicketID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create child approval ticket: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ApprovalTicketList
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
	mustCreateApprovalTicket(t, client, ticketID, eventID, approvalticket.OperationTypeDELETE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

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
	mustCreateApprovalTicket(t, client, ticketID, eventID, approvalticket.OperationTypeVNC_ACCESS, "user-vnc")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	found := findTicketInList(t, w.Body.Bytes(), ticketID)
	if found.TicketPayload == nil {
		t.Fatal("TicketPayload is nil, want non-nil for VNC_ACCESS ticket")
	}
	if found.OperationType != generated.ApprovalTicketOperationType("VNC_ACCESS") {
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
	mustCreateApprovalTicket(t, client, ticketID, "ev-does-not-exist", approvalticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

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
	mustCreateApprovalTicket(t, client, ticketID, eventID, approvalticket.OperationTypeCREATE, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{})

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
	mustCreateApprovalTicket(t, client, createTicketID, createEventID, approvalticket.OperationTypeCREATE, "user-a")

	powerEventID := "ev-power-" + uuid.NewString()
	mustCreateDomainEvent(t, client, powerEventID, []byte(`{"seed":"power"}`))
	powerTicketID := "ticket-power-" + uuid.NewString()
	mustCreateApprovalTicket(t, client, powerTicketID, powerEventID, approvalticket.OperationTypePOWER, "user-a")

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals?operation_type=POWER", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{
		OperationType: generated.ListApprovalsParamsOperationType("POWER"),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ApprovalTicketList
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

func TestListApprovals_FiltersBySelectedClusterAndPlacementSnapshot(t *testing.T) {
	t.Parallel()

	srv, client := newApprovalTestServer(t, "approval_filter_placement")

	withPlacementEventID := "ev-placement-" + uuid.NewString()
	mustCreateDomainEvent(t, client, withPlacementEventID, []byte(`{"seed":"placement"}`))
	withPlacementTicketID := "ticket-placement-" + uuid.NewString()
	mustCreateApprovalTicket(t, client, withPlacementTicketID, withPlacementEventID, approvalticket.OperationTypeCREATE, "user-a")
	if err := client.ApprovalTicket.UpdateOneID(withPlacementTicketID).
		SetStatus(approvalticket.StatusAPPROVED).
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
	mustCreateApprovalTicket(t, client, withoutPlacementTicketID, withoutPlacementEventID, approvalticket.OperationTypeCREATE, "user-a")
	if err := client.ApprovalTicket.UpdateOneID(withoutPlacementTicketID).
		SetStatus(approvalticket.StatusAPPROVED).
		SetSelectedClusterID("cluster-b").
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket without placement: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals?selected_cluster_id=cluster-a&placement_snapshot=present", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{
		SelectedClusterId: "cluster-a",
		PlacementSnapshot: generated.ListApprovalsParamsPlacementSnapshot("present"),
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ApprovalTicketList
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
	mustCreateApprovalTicket(t, client, withAdvisoryTicketID, withAdvisoryEventID, approvalticket.OperationTypeCREATE, "user-a")
	if err := client.ApprovalTicket.UpdateOneID(withAdvisoryTicketID).
		SetStatus(approvalticket.StatusAPPROVED).
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
	mustCreateApprovalTicket(t, client, withoutAdvisoryTicketID, withoutAdvisoryEventID, approvalticket.OperationTypeCREATE, "user-a")
	if err := client.ApprovalTicket.UpdateOneID(withoutAdvisoryTicketID).
		SetStatus(approvalticket.StatusAPPROVED).
		SetPlacementEvaluation(map[string]interface{}{
			"selected_cluster_id": "cluster-b",
			"eligible":            true,
			"evaluated_at":        "2026-03-08T00:00:00Z",
		}).
		Exec(t.Context()); err != nil {
		t.Fatalf("update ticket without placement advisory: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals?placement_advisory_code=PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{
		PlacementAdvisoryCode: "PVC_CLONE_HOST_ASSISTED_FALLBACK_LIKELY",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ApprovalTicketList
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
		mustCreateApprovalTicket(t, client, ticketID, eventID, approvalticket.OperationTypeCREATE, "user-pag")
	}

	// Request page 1 with perPage=2 → total must be ≥ 3, total_pages ≥ 2.
	c, w := newAuthedGinContext(t, http.MethodGet, "/approvals?page=1&per_page=2", "", "admin-1", []string{"approval:view", "platform:admin"})
	srv.ListApprovals(c, generated.ListApprovalsParams{
		Page:    1,
		PerPage: 2,
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ApprovalTicketList
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

func mustCreateApprovalTicket(
	t *testing.T,
	client *ent.Client,
	ticketID, eventID string,
	opType approvalticket.OperationType,
	requester string,
) {
	t.Helper()
	_, err := client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetOperationType(opType).
		SetStatus(approvalticket.StatusPENDING).
		SetRequester(requester).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create approval ticket %s: %v", ticketID, err)
	}
}

func mustCreateApprovalTemplate(
	t *testing.T,
	client *ent.Client,
	id, name, displayName string,
) {
	t.Helper()
	_, err := client.Template.Create().
		SetID(id).
		SetName(name).
		SetDisplayName(displayName).
		SetCreatedBy("test-seed").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create template %s: %v", id, err)
	}
}

func mustCreateApprovalInstanceSize(
	t *testing.T,
	client *ent.Client,
	id, name, displayName string,
	diskGB int,
	dedicatedCPU bool,
) {
	t.Helper()
	create := client.InstanceSize.Create().
		SetID(id).
		SetName(name).
		SetDisplayName(displayName).
		SetCPUCores(4).
		SetMemoryGi(8).
		SetCreatedBy("test-seed")
	if diskGB > 0 {
		create = create.SetDiskGB(diskGB)
	}
	if dedicatedCPU {
		create = create.SetDedicatedCPU(true)
	}
	if _, err := create.Save(t.Context()); err != nil {
		t.Fatalf("create instance size %s: %v", id, err)
	}
}

// findTicketInList decodes the response body as ApprovalTicketList and returns the
// ticket matching the given id. Calls t.Fatal if the ticket is not found.
func findTicketInList(t *testing.T, body []byte, ticketID string) generated.ApprovalTicket {
	t.Helper()
	var resp generated.ApprovalTicketList
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode ApprovalTicketList: %v", err)
	}
	for i := range resp.Items {
		item := &resp.Items[i]
		if item.Id == ticketID {
			return *item
		}
	}
	t.Fatalf("ticket %q not found in list response", ticketID)
	return generated.ApprovalTicket{}
}
