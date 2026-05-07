package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

type fallbackProvider struct {
	submitCalled  int
	processCalled int
	lastDecision  approvalcontract.ApprovalDecision
}

func (f *fallbackProvider) Type() string { return "builtin-default" }

func (f *fallbackProvider) SubmitForApproval(_ context.Context, req *approvalcontract.ApprovalRequest) (*approvalcontract.ApprovalResponse, error) {
	f.submitCalled++
	return &approvalcontract.ApprovalResponse{TicketID: req.EventID, Status: "PENDING"}, nil
}

func (f *fallbackProvider) ProcessApproval(_ context.Context, _ string, decision approvalcontract.ApprovalDecision) error {
	f.processCalled++
	f.lastDecision = decision
	return nil
}

func TestProviderSubmitForApprovalSendsSignedWebhook(t *testing.T) {
	secret := "webhook-secret"
	var gotPayload submitPayload
	var gotTicketID string
	var gotSource string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !VerifySignature(body, []byte(secret), r.Header.Get(signatureHeader)) {
			t.Fatal("webhook signature did not verify")
		}
		gotTicketID = r.Header.Get(ticketIDHeader)
		gotSource = r.Header.Get("X-Shepherd-Source")
		if err := json.Unmarshal(body, &gotPayload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	fallback := &fallbackProvider{}
	provider, err := NewProvider(Config{
		WebhookURL:        server.URL,
		SigningKey:        secret,
		Headers:           map[string]string{"X-Shepherd-Source": "shepherd"},
		RetryCount:        1,
		AllowInsecureHTTP: true,
	}, fallback)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	provider.now = func() time.Time {
		return time.Date(2026, 3, 17, 0, 0, 0, 0, time.UTC)
	}

	resp, err := provider.SubmitForApproval(context.Background(), &approvalcontract.ApprovalRequest{
		EventID:   "ticket-1",
		Requester: "alice",
		Action:    "vm.create",
		Reason:    "capacity",
		Metadata:  map[string]interface{}{"vm_name": "vm-a"},
	})
	if err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}
	if resp.Status != "PENDING_EXTERNAL" {
		t.Fatalf("status = %q, want PENDING_EXTERNAL", resp.Status)
	}
	if fallback.submitCalled != 0 {
		t.Fatalf("fallback submitCalled = %d, want 0", fallback.submitCalled)
	}
	if gotTicketID != "ticket-1" || gotPayload.EventID != "ticket-1" || gotPayload.Requester != "alice" {
		t.Fatalf("unexpected webhook payload: ticket header=%q payload=%+v", gotTicketID, gotPayload)
	}
	if gotSource != "shepherd" {
		t.Fatalf("custom source header = %q, want shepherd", gotSource)
	}
	if gotPayload.SubmittedAt != "2026-03-17T00:00:00Z" {
		t.Fatalf("submitted_at = %q", gotPayload.SubmittedAt)
	}
}

func TestProviderSubmitForApprovalFallsBackAfterWebhookFailures(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	fallback := &fallbackProvider{}
	provider, err := NewProvider(Config{
		WebhookURL:        server.URL,
		SigningKey:        "webhook-secret",
		RetryCount:        1,
		RetryBackoff:      time.Millisecond,
		AllowInsecureHTTP: true,
	}, fallback)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	provider.sleep = func(context.Context, time.Duration) error { return nil }

	resp, err := provider.SubmitForApproval(context.Background(), &approvalcontract.ApprovalRequest{EventID: "ticket-2"})
	if err != nil {
		t.Fatalf("SubmitForApproval: %v", err)
	}
	if resp.Status != "PENDING" {
		t.Fatalf("status = %q, want PENDING fallback", resp.Status)
	}
	if callCount != 2 {
		t.Fatalf("webhook callCount = %d, want 2", callCount)
	}
	if fallback.submitCalled != 1 {
		t.Fatalf("fallback submitCalled = %d, want 1", fallback.submitCalled)
	}
}

func TestProviderSubmitForApprovalDoesNotFallbackOnCallerCancellation(t *testing.T) {
	fallback := &fallbackProvider{}
	provider, err := NewProvider(Config{
		WebhookURL:        "https://approval.example.test/hook",
		SigningKey:        "webhook-secret",
		RetryCount:        1,
		RetryBackoff:      time.Millisecond,
		AllowInsecureHTTP: true,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, context.Canceled
			}),
		},
	}, fallback)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = provider.SubmitForApproval(ctx, &approvalcontract.ApprovalRequest{EventID: "ticket-cancelled"})
	if err == nil {
		t.Fatal("SubmitForApproval succeeded after caller cancellation")
	}
	if fallback.submitCalled != 0 {
		t.Fatalf("fallback submitCalled = %d, want 0", fallback.submitCalled)
	}
}

func TestProviderRequiresHTTPSByDefault(t *testing.T) {
	_, err := NewProvider(Config{
		WebhookURL: "http://approval.example.test/hook",
		SigningKey: "webhook-secret",
	}, &fallbackProvider{})
	if err == nil {
		t.Fatal("NewProvider succeeded with insecure HTTP URL")
	}
}

func TestProviderRejectsInvalidCustomHeader(t *testing.T) {
	_, err := NewProvider(Config{
		WebhookURL:        "http://approval.example.test/hook",
		SigningKey:        "webhook-secret",
		Headers:           map[string]string{"bad header": "value"},
		AllowInsecureHTTP: true,
	}, &fallbackProvider{})
	if err == nil {
		t.Fatal("NewProvider succeeded with an invalid custom header")
	}
}

func TestProviderProcessApprovalDelegatesToFallback(t *testing.T) {
	fallback := &fallbackProvider{}
	provider, err := NewProvider(Config{
		WebhookURL: "https://approval.example.test/hook",
		SigningKey: "webhook-secret",
	}, fallback)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}

	err = provider.ProcessApproval(context.Background(), "ticket-1", approvalcontract.ApprovalDecision{
		Approved: true,
		Approver: "admin",
	})
	if err != nil {
		t.Fatalf("ProcessApproval: %v", err)
	}
	if fallback.processCalled != 1 || !fallback.lastDecision.Approved {
		t.Fatalf("fallback process state = called %d decision %+v", fallback.processCalled, fallback.lastDecision)
	}
}

func TestSignPayloadAndVerifySignature(t *testing.T) {
	body := []byte(`{"ticket_id":"ticket-1"}`)
	secret := []byte("webhook-secret")
	signature := SignPayload(body, secret)

	if !VerifySignature(body, secret, signature) {
		t.Fatal("signature did not verify")
	}
	if VerifySignature([]byte(`{"ticket_id":"ticket-2"}`), secret, signature) {
		t.Fatal("signature verified for a modified body")
	}
	if VerifySignature(body, secret, "sha256=not-hex") {
		t.Fatal("malformed signature verified")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
