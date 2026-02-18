package approval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type fakeAtomicWriter struct {
	called bool

	ticketID    string
	eventID     string
	approver    string
	clusterID   string
	storage     string
	serviceID   string
	namespace   string
	requesterID string
}

func init() {
	_ = logger.Init("error", "json")
}

func (f *fakeAtomicWriter) ApproveCreateAndEnqueue(
	_ context.Context,
	ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
	_ int,
	_ map[string]interface{},
	_ map[string]interface{},
	_ map[string]interface{},
) (string, string, error) {
	f.called = true
	f.ticketID = ticketID
	f.eventID = eventID
	f.approver = approver
	f.clusterID = clusterID
	f.storage = storageClass
	f.serviceID = serviceID
	f.namespace = namespace
	f.requesterID = requesterID
	return "vm-1", "vm-name", nil
}

func (f *fakeAtomicWriter) ApproveDeleteAndEnqueue(_ context.Context, _, _, _, _ string) error {
	return nil
}

func TestGatewayApproveCreate_CallsAtomicWriterWithResolvedIDs(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_approve")

	eventID := "event-1"
	ticketID := "ticket-1"
	requester := "user-1"
	payload := domain.VMCreationPayload{
		RequesterID:    requester,
		ServiceID:      "svc-1",
		TemplateID:     "tpl-base",
		InstanceSizeID: "size-base",
		Namespace:      "team-a",
	}
	payloadRaw, err := payload.ToJSON()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	_, err = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy(requester).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create domain event: %v", err)
	}

	_, err = client.Template.Create().
		SetID("tpl-override").
		SetName("tpl").
		SetVersion(3).
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	_, err = client.InstanceSize.Create().
		SetID("size-override").
		SetName("size").
		SetCPUCores(2).
		SetMemoryMB(2048).
		SetCreatedBy("seed").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create instance size: %v", err)
	}

	_, err = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester(requester).
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		SetModifiedSpec(map[string]interface{}{
			"template_id":      "tpl-override",
			"instance_size_id": "size-override",
		}).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	// Isolate this test to gateway orchestration; validator behavior is covered separately.
	gw.validator = nil

	if err := gw.Approve(context.Background(), ticketID, "admin-1", "cluster-1", "sc-fast"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !writer.called {
		t.Fatal("atomic writer not called")
	}
	if writer.ticketID != ticketID || writer.eventID != eventID {
		t.Fatalf("writer ids mismatch: ticket=%s event=%s", writer.ticketID, writer.eventID)
	}
	if writer.approver != "admin-1" || writer.clusterID != "cluster-1" {
		t.Fatalf("writer approver/cluster mismatch: approver=%s cluster=%s", writer.approver, writer.clusterID)
	}
	if writer.storage != "sc-fast" {
		t.Fatalf("writer storage class = %s, want sc-fast", writer.storage)
	}
	if writer.serviceID != "svc-1" || writer.namespace != "team-a" || writer.requesterID != requester {
		t.Fatalf("writer payload mismatch: %+v", writer)
	}
}

func TestGatewayApproveCreate_RequiresClusterSelection(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cluster_required")

	eventID := "event-1"
	ticketID := "ticket-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":      "user-1",
		"service_id":        "svc-1",
		"template_id":       "tpl-1",
		"instance_size_id":  "size-1",
		"namespace":         "team-a",
		"selected_cluster":  "",
		"selected_storage":  "",
		"selected_template": "",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Approve(context.Background(), ticketID, "admin-1", "", ""); err == nil {
		t.Fatal("Approve() expected error when cluster id is empty, got nil")
	}
}

func TestGatewayApproveVNC_TransitionsTicketAndEventWithoutAtomicWriter(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_vnc_approve")

	eventID := "event-vnc-approve-1"
	ticketID := "ticket-vnc-approve-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"vm_id":        "vm-1",
		"cluster_id":   "cluster-a",
		"namespace":    "team-a",
		"requester_id": "user-1",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVNCAccessRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeVNC_ACCESS).
		SetReason("vnc access request").
		Save(context.Background())

	writer := &fakeAtomicWriter{}
	gw := NewGateway(client, nil, writer)
	if err := gw.Approve(context.Background(), ticketID, "admin-1", "", ""); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if writer.called {
		t.Fatal("atomic writer called for VNC ticket, want no call")
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusAPPROVED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusAPPROVED)
	}
	if ticket.Approver != "admin-1" {
		t.Fatalf("ticket approver = %s, want admin-1", ticket.Approver)
	}

	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCOMPLETED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCOMPLETED)
	}
}

func TestGatewayReject_TransitionsTicketAndEvent(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_reject")

	eventID := "event-reject-1"
	ticketID := "ticket-reject-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "user-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("user-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("user-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Reject(context.Background(), ticketID, "admin-1", "policy mismatch"); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusREJECTED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusREJECTED)
	}
	if ticket.Approver != "admin-1" {
		t.Fatalf("ticket approver = %s, want admin-1", ticket.Approver)
	}
	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCANCELLED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCANCELLED)
	}
}

func TestGatewayCancel_OnlyRequesterCanCancel(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cancel")

	eventID := "event-cancel-1"
	ticketID := "ticket-cancel-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "requester-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("requester-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("requester-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	err := gw.Cancel(context.Background(), ticketID, "other-user")
	if err == nil {
		t.Fatal("Cancel() expected forbidden error for non-requester, got nil")
	}
	if !strings.Contains(err.Error(), "only requester can cancel") {
		t.Fatalf("unexpected cancel error: %v", err)
	}
}

func TestGatewayCancel_RequesterTransitionsTicketAndEvent(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "gateway_behavior_cancel_ok")

	eventID := "event-cancel-ok-1"
	ticketID := "ticket-cancel-ok-1"
	payloadRaw, _ := json.Marshal(map[string]interface{}{
		"requester_id":     "requester-1",
		"service_id":       "svc-1",
		"template_id":      "tpl-1",
		"instance_size_id": "size-1",
		"namespace":        "team-a",
	})
	_, _ = client.DomainEvent.Create().
		SetID(eventID).
		SetEventType(string(domain.EventVMCreationRequested)).
		SetAggregateType("vm").
		SetAggregateID("svc-1").
		SetPayload(payloadRaw).
		SetCreatedBy("requester-1").
		Save(context.Background())
	_, _ = client.ApprovalTicket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("requester-1").
		SetStatus(approvalticket.StatusPENDING).
		SetOperationType(approvalticket.OperationTypeCREATE).
		Save(context.Background())

	gw := NewGateway(client, nil, &fakeAtomicWriter{})
	if err := gw.Cancel(context.Background(), ticketID, "requester-1"); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	ticket, err := client.ApprovalTicket.Get(context.Background(), ticketID)
	if err != nil {
		t.Fatalf("query ticket: %v", err)
	}
	if ticket.Status != approvalticket.StatusCANCELLED {
		t.Fatalf("ticket status = %s, want %s", ticket.Status, approvalticket.StatusCANCELLED)
	}
	event, err := client.DomainEvent.Get(context.Background(), eventID)
	if err != nil {
		t.Fatalf("query event: %v", err)
	}
	if event.Status != domainevent.StatusCANCELLED {
		t.Fatalf("event status = %s, want %s", event.Status, domainevent.StatusCANCELLED)
	}
}
