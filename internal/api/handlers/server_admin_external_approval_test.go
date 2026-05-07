package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestExternalApprovalSystemsCRUDProtectsSigningKey(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_approval_handler_crud")
	srv := NewServer(ServerDeps{
		EntClient:     client,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/external-approval-systems",
		`{
			"name":"Enterprise Approval",
			"type":"webhook",
			"enabled":true,
			"webhook_url":"https://approval.example.com/shepherd",
			"webhook_headers":{"X-Shepherd-Source":"shepherd"},
			"timeout_seconds":30,
			"retry_count":3,
			"retry_backoff_seconds":2,
			"signing_key":"webhook-secret",
			"sort_order":5
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateExternalApprovalSystem(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}
	var created generated.ExternalApprovalSystem
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if created.Id == "" || !created.SigningKeySet {
		t.Fatalf("created system = %+v, want id and signing key set", created)
	}
	if got := created.WebhookHeaders["X-Shepherd-Source"]; got != "shepherd" {
		t.Fatalf("response header = %q, want shepherd", got)
	}

	stored, err := client.ExternalApprovalSystem.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("get stored system: %v", err)
	}
	if stored.SigningKeyCiphertext == "" || stored.SigningKeyCiphertext == "webhook-secret" {
		t.Fatalf("stored signing key = %q, want encrypted value", stored.SigningKeyCiphertext)
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/external-approval-systems",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListExternalApprovalSystems(listCtx)
	if listW.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}
	var listed generated.ExternalApprovalSystemList
	mustDecodeJSON(t, listW.Body.Bytes(), &listed)
	if len(listed.Items) != 1 || listed.Items[0].Id != created.Id {
		t.Fatalf("listed systems = %+v", listed.Items)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/external-approval-systems/"+created.Id,
		`{
			"name":"Enterprise Approval Updated",
			"signing_key":"`+approvalregistry.ProtectedSigningKeyMask+`"
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateExternalApprovalSystem(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}
	updatedStored, err := client.ExternalApprovalSystem.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("get updated system: %v", err)
	}
	if updatedStored.SigningKeyCiphertext != stored.SigningKeyCiphertext {
		t.Fatal("protected signing key mask should preserve stored ciphertext")
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/external-approval-systems/"+created.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteExternalApprovalSystem(deleteCtx, created.Id)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}
}

func TestCreateExternalApprovalSystemRejectsInsecureWebhook(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_approval_handler_validation")
	srv := NewServer(ServerDeps{
		EntClient:     client,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})
	ctx, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/external-approval-systems",
		`{
			"name":"Enterprise Approval",
			"type":"webhook",
			"webhook_url":"http://approval.example.com/shepherd",
			"signing_key":"webhook-secret"
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateExternalApprovalSystem(ctx)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}
