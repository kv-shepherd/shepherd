package modules

import (
	"context"
	"os"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

type approvalModuleFallbackProvider struct{}

func (approvalModuleFallbackProvider) Type() string { return "fallback" }

func (approvalModuleFallbackProvider) SubmitForApproval(
	context.Context,
	*approvalcontract.ApprovalRequest,
) (*approvalcontract.ApprovalResponse, error) {
	return &approvalcontract.ApprovalResponse{TicketID: "fallback", Status: "PENDING"}, nil
}

func (approvalModuleFallbackProvider) ProcessApproval(
	context.Context,
	string,
	approvalcontract.ApprovalDecision,
) error {
	return nil
}

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
			// vmSvc=nil is valid (DryRun gate is optional; nil means disabled).
			if _, err := NewApprovalModule(tc.infra, nil); err == nil {
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
		"ticketing.NewService(",
		"service.NewApprovalRequirementService(",
		"notification.NewTriggers(",
		"ticketService.SetNotifier(",
		"usecase.NewApprovalAtomicWriter(",
		"ticketService.SetVMService(", // P1-A: DryRun Pre-flight Gate wiring
		"approvalregistry.NewService(",
		"approvalProviderOrFallback(",
		"deps.ExternalApprovalRegistry = m.registry",
	}
	for _, fragment := range required {
		if !strings.Contains(text, fragment) {
			t.Fatalf("approval module missing required wiring fragment %q", fragment)
		}
	}
}

func TestApprovalModule_TicketService(t *testing.T) {
	t.Parallel()

	var nilModule *ApprovalModule
	if got := nilModule.TicketService(); got != nil {
		t.Fatalf("nil module TicketService() = %p, want nil", got)
	}

	want := &ticketing.Service{}
	module := &ApprovalModule{ticketService: want}
	if got := module.TicketService(); got != want {
		t.Fatalf("TicketService() = %p, want %p", got, want)
	}
}

func TestApprovalProviderOrFallback_ReturnsFallbackWhenRegistryUnavailable(t *testing.T) {
	t.Parallel()

	fallback := approvalModuleFallbackProvider{}
	got := approvalProviderOrFallback(nil, fallback)
	if got == nil {
		t.Fatal("approvalProviderOrFallback returned nil, want fallback provider")
	}
	if got.Type() != fallback.Type() {
		t.Fatalf("approvalProviderOrFallback Type() = %q, want %q", got.Type(), fallback.Type())
	}
}
