package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	approvalwebhook "kv-shepherd.io/shepherd/internal/governance/approval/webhook"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestReceiveExternalApprovalDecisionRoutesSignedApproval(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId:           "ticket-1",
		Approved:           true,
		Approver:           "external.approver@example.com",
		ProviderDecisionId: "chg-10001",
		Execution: generated.ApprovalDecisionRequest{
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
		},
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(system.ID, "ticket-1", body))

	if w.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.ExternalApprovalDecisionResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Status != generated.Accepted || resp.TicketId != "ticket-1" || !resp.Approved {
		t.Fatalf("response = %+v, want accepted ticket-1 approval", resp)
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
	if capture.lastDecision.Approver != "external.approver@example.com" {
		t.Fatalf("approver = %q", capture.lastDecision.Approver)
	}
	if capture.lastDecision.Execution.ClusterID != "cluster-1" {
		t.Fatalf("cluster_id = %q, want cluster-1", capture.lastDecision.Execution.ClusterID)
	}
	if capture.lastDecision.Execution.DiskGB != 120 {
		t.Fatalf("disk_gb = %d, want 120", capture.lastDecision.Execution.DiskGB)
	}
}

func TestReceiveExternalApprovalDecisionRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-1",
		Approved: true,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, generated.ReceiveExternalApprovalDecisionParams{
		XExternalApprovalSystemID: system.ID,
		XSignature256:             "sha256=deadbeef",
		XTicketID:                 "ticket-1",
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestReceiveExternalApprovalDecisionRequiresRejectReason(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-2",
		Approved: false,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(system.ID, "ticket-2", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "REJECT_REASON_REQUIRED")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestReceiveExternalApprovalDecisionRequiresApprovedField(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := `{"ticket_id":"ticket-2","approver":"external.approver@example.com","reject_reason":"policy mismatch"}`
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(system.ID, "ticket-2", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestReceiveExternalApprovalDecisionRejectsDisabledSystem(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, false)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-3",
		Approved: true,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(system.ID, "ticket-3", body))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestReceiveExternalApprovalDecisionRejectsTicketHeaderMismatch(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-4",
		Approved: true,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(system.ID, "different-ticket", body))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "TICKET_ID_MISMATCH")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func newExternalApprovalCallbackTestServer(t *testing.T, enabled bool) (*Server, *approvalregistry.System, *captureApprovalProvider) {
	t.Helper()

	client := testutil.OpenEntPostgres(t, "external_approval_callback")
	registry := approvalregistry.NewService(client, []byte("0123456789abcdef0123456789abcdef"))
	system, err := registry.Create(t.Context(), approvalregistry.CreateInput{
		Name:       "callback-webhook",
		Enabled:    &enabled,
		WebhookURL: "https://approval.example.com/shepherd",
		SigningKey: "callback-secret",
		CreatedBy:  "admin-1",
	})
	if err != nil {
		t.Fatalf("Create external approval system: %v", err)
	}
	capture := &captureApprovalProvider{}
	srv := NewServer(ServerDeps{
		ExternalApprovalRegistry: registry,
		ApprovalRouter:           approval.NewApprovalProviderRouter(capture),
	})
	return srv, system, capture
}

func signedCallbackParams(systemID, ticketID, body string) generated.ReceiveExternalApprovalDecisionParams {
	return generated.ReceiveExternalApprovalDecisionParams{
		XExternalApprovalSystemID: systemID,
		XSignature256:             approvalwebhook.SignPayload([]byte(body), []byte("callback-secret")),
		XTicketID:                 ticketID,
	}
}
