package approval_test

import (
	"context"
	"testing"

	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/provider"
)

// ─── Fake provider for testing ────────────────────────────────────────────────

type fakeApprovalProvider struct {
	typeKey      string
	submitErr    error
	processErr   error
	submitCalled int
}

func (f *fakeApprovalProvider) Type() string { return f.typeKey }

func (f *fakeApprovalProvider) SubmitForApproval(_ context.Context, req *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	f.submitCalled++
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return &provider.ApprovalResponse{TicketID: req.EventID, Status: "PENDING"}, nil
}

func (f *fakeApprovalProvider) ProcessApproval(_ context.Context, _ string, _ provider.ApprovalDecision) error {
	return f.processErr
}

// Compile-time interface check.
var _ provider.ApprovalProvider = (*fakeApprovalProvider)(nil)

// ─── Tests: ApprovalProviderRouter ────────────────────────────────────────────

func TestApprovalProviderRouter_V1AlwaysBuiltin(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(builtin)

	// In V1, requesting any type (including empty) resolves to builtin.
	p := router.Resolve("")
	if p.Type() != "builtin-default" {
		t.Errorf("Resolve(\"\") = %q, want \"builtin-default\"", p.Type())
	}

	p = router.Resolve("builtin-default")
	if p.Type() != "builtin-default" {
		t.Errorf("Resolve(\"builtin-default\") = %q, want \"builtin-default\"", p.Type())
	}
}

func TestApprovalProviderRouter_UnknownTypeFallsBackToBuiltin(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(builtin)

	// Unknown provider type → controlled fallback to builtin (Stage 2.E §4).
	p := router.Resolve("external-jira")
	if p.Type() != "builtin-default" {
		t.Errorf("Resolve(\"external-jira\") = %q, want \"builtin-default\"", p.Type())
	}
}

func TestApprovalProviderRouter_RegisterExternal(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	external := &fakeApprovalProvider{typeKey: "jira"}
	router := approval.NewApprovalProviderRouter(builtin)

	if err := router.Register(external); err != nil {
		t.Fatalf("Register external: %v", err)
	}

	// Known external type resolves to the external provider.
	p := router.Resolve("jira")
	if p.Type() != "jira" {
		t.Errorf("Resolve(\"jira\") = %q, want \"jira\"", p.Type())
	}

	// Builtin still resolves normally.
	p = router.Resolve("builtin-default")
	if p.Type() != "builtin-default" {
		t.Errorf("Resolve(\"builtin-default\") = %q, want \"builtin-default\"", p.Type())
	}
}

func TestApprovalProviderRouter_DuplicateRegistrationRejected(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	external := &fakeApprovalProvider{typeKey: "jira"}
	router := approval.NewApprovalProviderRouter(builtin)

	if err := router.Register(external); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := router.Register(external); err == nil {
		t.Error("duplicate Register should have returned error, got nil")
	}
}

func TestApprovalProviderRouter_BuiltinTypeReserved(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(builtin)

	impostor := &fakeApprovalProvider{typeKey: "builtin-default"}
	if err := router.Register(impostor); err == nil {
		t.Error("registering \"builtin-default\" should have returned error, got nil")
	}
}

func TestApprovalProviderRouter_SubmitForApproval_V1(t *testing.T) {
	builtin := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(builtin)

	req := &provider.ApprovalRequest{EventID: "evt-001", Requester: "user-1", Action: "create"}
	resp, err := router.SubmitForApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}
	if resp.Status != "PENDING" {
		t.Errorf("expected status PENDING, got %q", resp.Status)
	}
	if builtin.submitCalled != 1 {
		t.Errorf("builtin.SubmitForApproval called %d times, want 1", builtin.submitCalled)
	}
}
