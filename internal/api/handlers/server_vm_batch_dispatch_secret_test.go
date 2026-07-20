package handlers

import (
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent/batchticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestLoadBatchView_DispatchFailureExposesOnlyStablePublicReason(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "batch_dispatch_public_reason")
	const (
		parentID      = "batch-secret-parent"
		parentEventID = "batch-secret-parent-event"
		childID       = "batch-secret-child"
		childEventID  = "batch-secret-child-event"
		sentinel      = "postgres://svc:super-secret@example.com Bearer token-secret-value"
	)
	parentPayload, err := (domain.BatchVMRequestPayload{
		Operation:   "DELETE",
		SubmittedBy: "requester-secret",
		Items:       []domain.BatchVMItemPayload{{VMID: "vm-secret"}},
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal parent payload: %v", err)
	}
	childPayload, err := (domain.VMDeletePayload{
		VMID:   "vm-secret",
		VMName: "vm-secret",
		Actor:  "requester-secret",
	}).ToJSON()
	if err != nil {
		t.Fatalf("marshal child payload: %v", err)
	}
	if _, createParentEventErr := client.DomainEvent.Create().
		SetID(parentEventID).
		SetEventType(string(domain.EventBatchDeleteRequested)).
		SetAggregateType("batch").
		SetAggregateID(parentID).
		SetPayload(parentPayload).
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy("requester-secret").
		Save(t.Context()); createParentEventErr != nil {
		t.Fatalf("create parent event: %v", createParentEventErr)
	}
	if _, createChildEventErr := client.DomainEvent.Create().
		SetID(childEventID).
		SetEventType(string(domain.EventVMDeletionRequested)).
		SetAggregateType("vm").
		SetAggregateID("vm-secret").
		SetPayload(childPayload).
		SetStatus(domainevent.StatusFAILED).
		SetCreatedBy("requester-secret").
		Save(t.Context()); createChildEventErr != nil {
		t.Fatalf("create child event: %v", createChildEventErr)
	}
	if _, createParentTicketErr := client.Ticket.Create().
		SetID(parentID).
		SetEventID(parentEventID).
		SetOperationType(ticket.OperationTypeDELETE).
		SetStatus(ticket.StatusFAILED).
		SetRequester("requester-secret").
		SetApprover("approver-secret").
		Save(t.Context()); createParentTicketErr != nil {
		t.Fatalf("create parent ticket: %v", createParentTicketErr)
	}
	if _, createChildTicketErr := client.Ticket.Create().
		SetID(childID).
		SetEventID(childEventID).
		SetOperationType(ticket.OperationTypeDELETE).
		SetStatus(ticket.StatusFAILED).
		SetRequester("requester-secret").
		SetApprover("approver-secret").
		SetReason(sentinel).
		SetRejectReason(domain.BatchApprovalDispatchFailureExhausted).
		SetParentTicketID(parentID).
		SetAttemptCount(1).
		Save(t.Context()); createChildTicketErr != nil {
		t.Fatalf("create child ticket: %v", createChildTicketErr)
	}
	if _, createProjectionErr := client.BatchTicket.Create().
		SetID(parentID).
		SetBatchType(batchticket.BatchTypeBATCH_DELETE).
		SetChildCount(1).
		SetFailedCount(1).
		SetPendingCount(0).
		SetStatus(batchticket.StatusFAILED).
		SetCreatedBy("requester-secret").
		Save(t.Context()); createProjectionErr != nil {
		t.Fatalf("create batch projection: %v", createProjectionErr)
	}

	view, _, err := (&Server{client: client}).loadBatchView(t.Context(), parentID)
	if err != nil {
		t.Fatalf("loadBatchView() error = %v", err)
	}
	if len(view.Children) != 1 || view.Children[0].LastError != domain.BatchApprovalDispatchFailureExhausted {
		t.Fatalf("batch child public error = %+v, want stable dispatch reason", view.Children)
	}
	if strings.Contains(view.Children[0].LastError, sentinel) || strings.Contains(view.Children[0].LastError, "super-secret") {
		t.Fatalf("batch child public error leaked secret: %q", view.Children[0].LastError)
	}
}
