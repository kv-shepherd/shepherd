package audit

import (
	"context"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestGenerateAuditID_HasAuditPrefix(t *testing.T) {
	t.Parallel()

	id := generateAuditID()
	if id == "" {
		t.Fatal("generateAuditID() returned empty string")
	}
	if !strings.HasPrefix(id, "audit-") {
		t.Fatalf("generateAuditID() = %q, want audit- prefix", id)
	}
}

func TestLogApprovalWithDetails_UsesTicketResource(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "audit_log_approval_ticket_resource")
	auditLogger := NewLogger(client)

	err := auditLogger.LogApprovalWithDetails(
		context.Background(),
		"ticket-123",
		"approved",
		"alice",
		map[string]interface{}{"cluster_id": "cluster-a"},
	)
	if err != nil {
		t.Fatalf("LogApprovalWithDetails() unexpected error: %v", err)
	}

	row, err := client.AuditLog.Query().Only(context.Background())
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if row.Action != "approval.approved" {
		t.Fatalf("action = %q, want approval.approved", row.Action)
	}
	if row.ResourceType != "ticket" {
		t.Fatalf("resource_type = %q, want ticket", row.ResourceType)
	}
	if row.ResourceID != "ticket-123" {
		t.Fatalf("resource_id = %q, want ticket-123", row.ResourceID)
	}
	if row.Actor != "alice" {
		t.Fatalf("actor = %q, want alice", row.Actor)
	}
	if got := row.Details["decision"]; got != "approved" {
		t.Fatalf("details.decision = %v, want approved", got)
	}
	if got := row.Details["cluster_id"]; got != "cluster-a" {
		t.Fatalf("details.cluster_id = %v, want cluster-a", got)
	}
}
