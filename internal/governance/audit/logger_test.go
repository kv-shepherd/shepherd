package audit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
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

func TestLogActionWithClient_AppendsThroughCallerTransaction(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "audit_log_caller_transaction")
	tx, txErr := client.Tx(t.Context())
	if txErr != nil {
		t.Fatalf("begin audit transaction: %v", txErr)
	}
	if err := LogActionWithClient(
		t.Context(),
		tx.Client(),
		"vm.metadata.update",
		"vm",
		"vm-transactional",
		"operator-1",
		map[string]interface{}{"field": "description"},
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("LogActionWithClient() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit audit transaction: %v", err)
	}

	row, err := client.AuditLog.Query().Only(t.Context())
	if err != nil {
		t.Fatalf("load caller-transaction audit: %v", err)
	}
	if row.Action != "vm.metadata.update" || row.ResourceType != "vm" ||
		row.ResourceID != "vm-transactional" || row.Actor != "operator-1" ||
		row.Details["field"] != "description" {
		t.Fatalf("caller-transaction audit = %+v, want complete mutation evidence", row)
	}
}

func TestLogActionWithClient_RollsBackWithCallerTransaction(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "audit_log_caller_rollback")
	tx, txErr := client.Tx(t.Context())
	if txErr != nil {
		t.Fatalf("begin audit rollback transaction: %v", txErr)
	}
	if err := LogActionWithClient(
		t.Context(),
		tx.Client(),
		"vm.metadata.update",
		"vm",
		"vm-rolled-back",
		"operator-1",
		map[string]interface{}{"field": "description"},
	); err != nil {
		_ = tx.Rollback()
		t.Fatalf("LogActionWithClient() error = %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback audit transaction: %v", err)
	}

	count, err := client.AuditLog.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count rolled-back audits: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back audit count = %d, want 0", count)
	}
}

func TestLogActionWithClient_ReportsPersistenceFailure(t *testing.T) {
	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("initialize audit test logger: %v", err)
	}
	client := testutil.OpenEntPostgres(t, "audit_log_persistence_failure")
	persistErr := errors.New("audit storage unavailable")
	client.AuditLog.Use(hook.On(hook.FixedError(persistErr), ent.OpCreate))

	err := LogActionWithClient(
		t.Context(),
		client,
		"vm.metadata.update",
		"vm",
		"vm-not-recorded",
		"operator-1",
		map[string]interface{}{"field": "description"},
	)
	if !errors.Is(err, persistErr) || !strings.Contains(err.Error(), "write audit log") {
		t.Fatalf("LogActionWithClient() error = %v, want wrapped persistence failure", err)
	}
	count, countErr := client.AuditLog.Query().Count(t.Context())
	if countErr != nil {
		t.Fatalf("count audits after persistence failure: %v", countErr)
	}
	if count != 0 {
		t.Fatalf("audit count after persistence failure = %d, want 0", count)
	}
}
