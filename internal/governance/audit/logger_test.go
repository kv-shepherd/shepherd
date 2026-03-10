package audit

import (
	"strings"
	"testing"
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
