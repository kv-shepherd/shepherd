package builtin

import (
	"context"
	"testing"

	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestNewProvider_PanicsOnNilTicketService(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("NewProvider() should panic when ticket service is nil")
		}
	}()

	_ = NewProvider(nil)
}

func TestProvider_SubmitForApproval_ReturnsPendingTicket(t *testing.T) {
	t.Parallel()

	_ = logger.Init("error", "json")
	p := NewProvider(&ticketing.Service{})

	resp, err := p.SubmitForApproval(context.Background(), &provider.ApprovalRequest{
		EventID:   "evt-123",
		Requester: "alice",
		Action:    "create",
	})
	if err != nil {
		t.Fatalf("SubmitForApproval() unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("SubmitForApproval() returned nil response")
		return
	}
	if resp.TicketID != "evt-123" {
		t.Fatalf("SubmitForApproval() ticket_id = %q, want evt-123", resp.TicketID)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("SubmitForApproval() status = %q, want PENDING", resp.Status)
	}
}

func TestProvider_ProcessApproval_RequiresTicketID(t *testing.T) {
	t.Parallel()

	_ = logger.Init("error", "json")
	p := NewProvider(&ticketing.Service{})

	err := p.ProcessApproval(context.Background(), " ", provider.ApprovalDecision{Approved: true})
	if err == nil {
		t.Fatal("ProcessApproval() expected error for empty ticket id")
	}
}
