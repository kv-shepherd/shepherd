package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
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

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "ticket-1", body, time.Now()))

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
	timestamp := time.Now()
	c.Request.Header.Set(externalApprovalTimestampHeader, timestamp.UTC().Format(time.RFC3339))

	srv.ReceiveExternalApprovalDecision(c, generated.ReceiveExternalApprovalDecisionParams{
		XExternalApprovalSystemID: system.ID,
		XShepherdTimestamp:        timestamp.UTC(),
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

func TestReceiveExternalApprovalDecisionRequiresTimestamp(t *testing.T) {
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
		XSignature256:             approvalwebhook.SignPayload([]byte(body), []byte("callback-secret")),
		XTicketID:                 "ticket-1",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestReceiveExternalApprovalDecisionRejectsStaleTimestamp(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-1",
		Approved: true,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/webhooks/approval-callback", body)

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "ticket-1", body, time.Now().Add(-10*time.Minute)))

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

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "ticket-2", body, time.Now()))

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

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "ticket-2", body, time.Now()))

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

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "ticket-3", body, time.Now()))

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

	srv.ReceiveExternalApprovalDecision(c, signedCallbackParams(t, c, system.ID, "different-ticket", body, time.Now()))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "TICKET_ID_MISMATCH")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func TestListExternalApprovalPendingTicketsReturnsSignedPendingTickets(t *testing.T) {
	t.Parallel()

	srv, client, system, _ := newExternalApprovalCallbackTestServerWithClient(t, true)
	mustCreateDomainEvent(t, client, "event-pending", []byte(`{"vm_name":"vm-a"}`))
	mustCreateTicket(t, client, "ticket-pending", "event-pending", entticket.OperationTypeCREATE, "user-a")
	mustCreateDomainEvent(t, client, "event-approved", []byte(`{"vm_name":"vm-b"}`))
	mustCreateTicket(t, client, "ticket-approved", "event-approved", entticket.OperationTypeCREATE, "user-b")
	if _, err := client.Ticket.UpdateOneID("ticket-approved").
		SetStatus(entticket.StatusAPPROVED).
		Save(t.Context()); err != nil {
		t.Fatalf("mark approved ticket: %v", err)
	}

	c, w := newPublicGinContext(t, http.MethodGet, "/api/v1/external-approval/pending?page=1&per_page=20", "")

	srv.ListExternalApprovalPendingTickets(c, signedPollingParams(t, c, system.ID, time.Now()))

	if w.Code != http.StatusOK {
		t.Fatalf("polling status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.TicketList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if got := len(resp.Items); got != 1 {
		t.Fatalf("items len = %d, want 1: %+v", got, resp.Items)
	}
	if resp.Items[0].Id != "ticket-pending" || resp.Items[0].Status != generated.TicketStatus(entticket.StatusPENDING) {
		t.Fatalf("item = %+v, want pending ticket-pending", resp.Items[0])
	}
	if resp.Pagination.Total != 1 || resp.Pagination.Page != 1 || resp.Pagination.PerPage != 20 {
		t.Fatalf("pagination = %+v, want total=1 page=1 per_page=20", resp.Pagination)
	}
}

func TestListExternalApprovalPendingTicketsRejectsStaleSignature(t *testing.T) {
	t.Parallel()

	srv, _, system, _ := newExternalApprovalCallbackTestServerWithClient(t, true)
	c, w := newPublicGinContext(t, http.MethodGet, "/api/v1/external-approval/pending?page=1&per_page=20", "")

	srv.ListExternalApprovalPendingTickets(c, signedPollingParams(t, c, system.ID, time.Now().Add(-10*time.Minute)))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("polling status = %d, want %d body=%s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "UNAUTHORIZED")
}

func TestReceiveExternalApprovalTicketDecisionRoutesSignedApproval(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId:           "ticket-5",
		Approved:           true,
		Approver:           "external.approver@example.com",
		ProviderDecisionId: "chg-10005",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/external-approval/tickets/ticket-5/decision", body)

	srv.ReceiveExternalApprovalTicketDecision(c, "ticket-5", signedTicketDecisionParams(t, c, system.ID, body, time.Now()))

	if w.Code != http.StatusOK {
		t.Fatalf("decision status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp generated.ExternalApprovalDecisionResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Status != generated.Accepted || resp.TicketId != "ticket-5" || !resp.Approved {
		t.Fatalf("response = %+v, want accepted ticket-5 approval", resp)
	}
	if capture.processCalled != 1 || capture.lastTicketID != "ticket-5" {
		t.Fatalf("processCalled=%d ticket=%q, want 1 ticket-5", capture.processCalled, capture.lastTicketID)
	}
}

func TestReceiveExternalApprovalTicketDecisionRejectsPathMismatch(t *testing.T) {
	t.Parallel()

	srv, system, capture := newExternalApprovalCallbackTestServer(t, true)
	body := mustJSON(t, generated.ExternalApprovalDecisionRequest{
		TicketId: "ticket-6",
		Approved: true,
		Approver: "external.approver@example.com",
	})
	c, w := newPublicGinContext(t, http.MethodPost, "/external-approval/tickets/different-ticket/decision", body)

	srv.ReceiveExternalApprovalTicketDecision(c, "different-ticket", signedTicketDecisionParams(t, c, system.ID, body, time.Now()))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("decision status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "TICKET_ID_MISMATCH")
	if capture.processCalled != 0 {
		t.Fatalf("processCalled = %d, want 0", capture.processCalled)
	}
}

func newExternalApprovalCallbackTestServer(t *testing.T, enabled bool) (*Server, *approvalregistry.System, *captureApprovalProvider) {
	t.Helper()

	srv, _, system, capture := newExternalApprovalCallbackTestServerWithClient(t, enabled)
	return srv, system, capture
}

func newExternalApprovalCallbackTestServerWithClient(t *testing.T, enabled bool) (*Server, *ent.Client, *approvalregistry.System, *captureApprovalProvider) {
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
		EntClient:                client,
		ExternalApprovalRegistry: registry,
		ApprovalRouter:           approval.NewApprovalProviderRouter(capture),
	})
	return srv, client, system, capture
}

func signedCallbackParams(
	t *testing.T,
	c *gin.Context,
	systemID string,
	ticketID string,
	body string,
	timestamp time.Time,
) generated.ReceiveExternalApprovalDecisionParams {
	t.Helper()

	timestampHeader := timestamp.UTC().Format(time.RFC3339)
	c.Request.Header.Set(externalApprovalTimestampHeader, timestampHeader)
	return generated.ReceiveExternalApprovalDecisionParams{
		XExternalApprovalSystemID: systemID,
		XShepherdTimestamp:        timestamp.UTC(),
		XSignature256: approvalwebhook.SignPayload(
			externalApprovalDecisionSignaturePayload(c, timestampHeader, []byte(body)),
			[]byte("callback-secret"),
		),
		XTicketID: ticketID,
	}
}

func signedTicketDecisionParams(
	t *testing.T,
	c *gin.Context,
	systemID string,
	body string,
	timestamp time.Time,
) generated.ReceiveExternalApprovalTicketDecisionParams {
	t.Helper()

	timestampHeader := timestamp.UTC().Format(time.RFC3339)
	c.Request.Header.Set(externalApprovalTimestampHeader, timestampHeader)
	return generated.ReceiveExternalApprovalTicketDecisionParams{
		XExternalApprovalSystemID: systemID,
		XShepherdTimestamp:        timestamp.UTC(),
		XSignature256: approvalwebhook.SignPayload(
			externalApprovalDecisionSignaturePayload(c, timestampHeader, []byte(body)),
			[]byte("callback-secret"),
		),
	}
}

func signedPollingParams(
	t *testing.T,
	c *gin.Context,
	systemID string,
	timestamp time.Time,
) generated.ListExternalApprovalPendingTicketsParams {
	t.Helper()

	timestampHeader := timestamp.UTC().Format(time.RFC3339)
	c.Request.Header.Set(externalApprovalTimestampHeader, timestampHeader)
	return generated.ListExternalApprovalPendingTicketsParams{
		Page:                      1,
		PerPage:                   20,
		XExternalApprovalSystemID: systemID,
		XShepherdTimestamp:        timestamp.UTC(),
		XSignature256: approvalwebhook.SignPayload(
			externalApprovalPollingSignaturePayload(c, timestampHeader),
			[]byte("callback-secret"),
		),
	}
}
