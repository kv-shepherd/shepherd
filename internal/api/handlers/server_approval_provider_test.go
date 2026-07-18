package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/provider"
)

type captureApprovalProvider struct {
	processCalled int
	lastTicketID  string
	lastDecision  provider.ApprovalDecision
	processErr    error
}

func (p *captureApprovalProvider) Type() string { return "builtin-default" }

func (p *captureApprovalProvider) SubmitForApproval(context.Context, *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	return &provider.ApprovalResponse{TicketID: "ignored", Status: "PENDING"}, nil
}

func (p *captureApprovalProvider) ProcessApproval(_ context.Context, ticketID string, decision provider.ApprovalDecision) error {
	p.processCalled++
	p.lastTicketID = ticketID
	p.lastDecision = decision
	return p.processErr
}

func TestApproveBuiltinApprovalTask_DoesNotExposePreflightProviderCause(t *testing.T) {
	t.Parallel()

	const sentinel = "postgres://svc:super-secret@example.com Bearer token-secret-value"
	capture := &captureApprovalProvider{processErr: apperrors.Wrap(
		errors.New(sentinel),
		apperrors.CodeClusterUnhealthy,
		"approval requires live cluster preflight",
		http.StatusServiceUnavailable,
	).WithParams(map[string]interface{}{"reason": "CLUSTER_RUNTIME_UNAVAILABLE"})}
	srv := NewServer(ServerDeps{ApprovalRouter: approval.NewApprovalProviderRouter(capture)})
	body, err := json.Marshal(generated.ApprovalDecisionRequest{})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	c, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/builtin-approval/tasks/ticket-secret/approve",
		string(body),
		"admin-1",
		[]string{"builtin_approval:approve", "platform:admin"},
	)
	srv.ApproveBuiltinApprovalTask(c, "ticket-secret")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	var apiErr generated.Error
	if err := json.Unmarshal(response.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if apiErr.Code != apperrors.CodeClusterUnhealthy || apiErr.Message != "approval requires live cluster preflight" {
		t.Fatalf("public preflight error = %+v", apiErr)
	}
	if reason, _ := apiErr.Params["reason"].(string); reason != "CLUSTER_RUNTIME_UNAVAILABLE" {
		t.Fatalf("public preflight reason = %q", reason)
	}
	if strings.Contains(response.Body.String(), sentinel) || strings.Contains(response.Body.String(), "super-secret") {
		t.Fatalf("preflight HTTP response leaked provider cause: %s", response.Body.String())
	}
}

func TestApproveTicketRoutesThroughApprovalProvider(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv := NewServer(ServerDeps{
		ApprovalRouter: approval.NewApprovalProviderRouter(capture),
	})

	body, err := json.Marshal(generated.ApprovalDecisionRequest{
		SelectedClusterId:     "cluster-1",
		SelectedStorageClass:  "fast-sc",
		SelectedDvAccessModes: []string{"ReadWriteOnce"},
		SelectedDvVolumeMode:  generated.ApprovalDecisionRequestSelectedDvVolumeModeFilesystem,
		EnableOverride:        true,
		CpuRequest:            2,
		CpuLimit:              4,
		MemoryRequestGi:       8,
		MemoryLimitGi:         16,
		DiskGb:                120,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodPost, "/builtin-approval/tasks/ticket-1/approve", string(body), "admin-1", []string{"builtin_approval:approve", "platform:admin"})
	srv.ApproveBuiltinApprovalTask(c, "ticket-1")

	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}
	if capture.processCalled != 1 {
		t.Fatalf("processCalled = %d, want 1", capture.processCalled)
	}
	if capture.lastTicketID != "ticket-1" {
		t.Fatalf("lastTicketID = %q, want ticket-1", capture.lastTicketID)
	}
	if !capture.lastDecision.Approved {
		t.Fatal("lastDecision.Approved = false, want true")
	}
	if capture.lastDecision.Execution.ClusterID != "cluster-1" {
		t.Fatalf("cluster_id = %q, want cluster-1", capture.lastDecision.Execution.ClusterID)
	}
	if capture.lastDecision.Execution.StorageClass != "fast-sc" {
		t.Fatalf("storage_class = %q, want fast-sc", capture.lastDecision.Execution.StorageClass)
	}
	if capture.lastDecision.Execution.DiskGB != 120 {
		t.Fatalf("disk_gb = %d, want 120", capture.lastDecision.Execution.DiskGB)
	}
}

func TestRejectTicketRoutesThroughApprovalProvider(t *testing.T) {
	t.Parallel()

	capture := &captureApprovalProvider{}
	srv := NewServer(ServerDeps{
		ApprovalRouter: approval.NewApprovalProviderRouter(capture),
	})

	body, err := json.Marshal(generated.RejectDecisionRequest{Reason: "policy mismatch"})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodPost, "/builtin-approval/tasks/ticket-2/reject", string(body), "admin-2", []string{"builtin_approval:approve", "platform:admin"})
	srv.RejectBuiltinApprovalTask(c, "ticket-2")

	if c.Writer.Status() != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
	}
	if capture.processCalled != 1 {
		t.Fatalf("processCalled = %d, want 1", capture.processCalled)
	}
	if capture.lastTicketID != "ticket-2" {
		t.Fatalf("lastTicketID = %q, want ticket-2", capture.lastTicketID)
	}
	if capture.lastDecision.Approved {
		t.Fatal("lastDecision.Approved = true, want false")
	}
	if capture.lastDecision.RejectReason != "policy mismatch" {
		t.Fatalf("reject_reason = %q, want policy mismatch", capture.lastDecision.RejectReason)
	}
}
