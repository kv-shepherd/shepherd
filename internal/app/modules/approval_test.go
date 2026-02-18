package modules

import (
	"os"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
)

func TestNewApprovalModule_RequiresInfraDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		infra *Infrastructure
	}{
		{name: "nil infra", infra: nil},
		{name: "missing all core deps", infra: &Infrastructure{}},
		{name: "missing pool and river", infra: &Infrastructure{EntClient: &ent.Client{}}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewApprovalModule(tc.infra); err == nil {
				t.Fatalf("NewApprovalModule(%s) expected error, got nil", tc.name)
			}
		})
	}
}

func TestApprovalModule_WiringContract(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("approval.go")
	if err != nil {
		t.Fatalf("read approval.go: %v", err)
	}
	text := string(src)

	required := []string{
		"approval.NewGateway(",
		"notification.NewTriggers(",
		"gateway.SetNotifier(",
		"usecase.NewApprovalAtomicWriter(",
	}
	for _, fragment := range required {
		if !strings.Contains(text, fragment) {
			t.Fatalf("approval module missing required wiring fragment %q", fragment)
		}
	}
}
