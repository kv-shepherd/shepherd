package approval_test

import (
	"context"
	"errors"
	"testing"

	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/provider"
)

type fakeApprovalProvider struct {
	typeKey         string
	submitErr       error
	processErr      error
	submitCalled    int
	processCalled   int
	lastDecision    provider.ApprovalDecision
	lastSubmitEvent string
}

func (f *fakeApprovalProvider) Type() string { return f.typeKey }

func (f *fakeApprovalProvider) SubmitForApproval(_ context.Context, req *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	f.submitCalled++
	if req != nil {
		f.lastSubmitEvent = req.EventID
	}
	if f.submitErr != nil {
		return nil, f.submitErr
	}
	return &provider.ApprovalResponse{TicketID: req.EventID, Status: "PENDING"}, nil
}

func (f *fakeApprovalProvider) ProcessApproval(_ context.Context, _ string, decision provider.ApprovalDecision) error {
	f.processCalled++
	f.lastDecision = decision
	return f.processErr
}

var _ provider.ApprovalProvider = (*fakeApprovalProvider)(nil)

func TestApprovalProviderRouter_SubmitForApproval_UsesActiveProvider(t *testing.T) {
	active := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(active)

	req := &provider.ApprovalRequest{EventID: "evt-001", Requester: "user-1", Action: "create"}
	resp, err := router.SubmitForApproval(context.Background(), req)
	if err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("status = %q, want PENDING", resp.Status)
	}
	if active.submitCalled != 1 {
		t.Fatalf("submitCalled = %d, want 1", active.submitCalled)
	}
	if active.lastSubmitEvent != "evt-001" {
		t.Fatalf("lastSubmitEvent = %q, want evt-001", active.lastSubmitEvent)
	}
}

func TestApprovalProviderRouter_ProcessApproval_UsesActiveProvider(t *testing.T) {
	active := &fakeApprovalProvider{typeKey: "builtin-default"}
	router := approval.NewApprovalProviderRouter(active)

	decision := provider.ApprovalDecision{
		Approved: true,
		Approver: "admin-1",
		Execution: provider.ApprovalExecutionOptions{
			ClusterID:    "cluster-a",
			StorageClass: "fast-sc",
		},
	}
	if err := router.ProcessApproval(context.Background(), "ticket-1", decision); err != nil {
		t.Fatalf("ProcessApproval: %v", err)
	}
	if active.processCalled != 1 {
		t.Fatalf("processCalled = %d, want 1", active.processCalled)
	}
	if !active.lastDecision.Approved {
		t.Fatal("lastDecision.Approved = false, want true")
	}
	if active.lastDecision.Execution.ClusterID != "cluster-a" {
		t.Fatalf("ClusterID = %q, want cluster-a", active.lastDecision.Execution.ClusterID)
	}
}

func TestApprovalProviderRouter_ReturnsProviderErrors(t *testing.T) {
	wantErr := errors.New("submit failed")
	active := &fakeApprovalProvider{typeKey: "builtin-default", submitErr: wantErr}
	router := approval.NewApprovalProviderRouter(active)

	_, err := router.SubmitForApproval(context.Background(), &provider.ApprovalRequest{EventID: "evt-002"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("SubmitForApproval error = %v, want %v", err, wantErr)
	}
}
