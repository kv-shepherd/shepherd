package registry

import (
	"context"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type fallbackProvider struct {
	submitCalled int
}

func (f *fallbackProvider) Type() string { return "builtin-default" }

func (f *fallbackProvider) SubmitForApproval(context.Context, *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	f.submitCalled++
	return &provider.ApprovalResponse{TicketID: "fallback", Status: "PENDING"}, nil
}

func (f *fallbackProvider) ProcessApproval(context.Context, string, provider.ApprovalDecision) error {
	return nil
}

func TestServiceCreateListUpdateDeleteProtectsSigningKey(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_approval_registry_crud")
	service := NewService(client, []byte("0123456789abcdef0123456789abcdef"))
	enabled := false
	timeoutSeconds := 45

	created, err := service.Create(t.Context(), CreateInput{
		Name:           "enterprise-approval",
		ProviderType:   ProviderTypeWebhook,
		Enabled:        &enabled,
		WebhookURL:     " https://approval.example.com/shepherd ",
		WebhookHeaders: map[string]string{" X-Shepherd-Source ": " shepherd "},
		TimeoutSeconds: &timeoutSeconds,
		SigningKey:     "webhook-secret",
		CreatedBy:      "admin-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Enabled {
		t.Fatal("created.Enabled = true, want false")
	}
	if got := created.WebhookHeaders["X-Shepherd-Source"]; got != "shepherd" {
		t.Fatalf("created header = %q, want shepherd", got)
	}

	stored, err := client.ExternalApprovalSystem.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("get stored system: %v", err)
	}
	if stored.SigningKeyCiphertext == "" || stored.SigningKeyCiphertext == "webhook-secret" {
		t.Fatalf("stored signing key = %q, want protected ciphertext", stored.SigningKeyCiphertext)
	}
	if !strings.HasPrefix(stored.SigningKeyCiphertext, "approval-signing-key:v1:") {
		t.Fatalf("stored signing key prefix = %q", stored.SigningKeyCiphertext)
	}

	listed, err := service.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || !listed[0].SigningKeySet {
		t.Fatalf("listed systems = %+v, want one system with signing key set", listed)
	}

	enabled = true
	updatedName := "enterprise-approval-updated"
	updated, err := service.Update(t.Context(), created.ID, UpdateInput{
		Name:       &updatedName,
		Enabled:    &enabled,
		SigningKey: ptr(ProtectedSigningKeyMask),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != updatedName || !updated.Enabled {
		t.Fatalf("updated system = %+v", updated)
	}

	updatedStored, err := client.ExternalApprovalSystem.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("get updated system: %v", err)
	}
	if updatedStored.SigningKeyCiphertext != stored.SigningKeyCiphertext {
		t.Fatal("protected signing key mask should preserve existing ciphertext")
	}

	deleteErr := service.Delete(t.Context(), created.ID)
	if deleteErr != nil {
		t.Fatalf("Delete: %v", deleteErr)
	}
	listed, err = service.List(t.Context())
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed systems after delete = %d, want 0", len(listed))
	}
}

func TestServiceActiveProviderUsesFirstEnabledWebhook(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_approval_registry_active")
	service := NewService(client, []byte("0123456789abcdef0123456789abcdef"))

	disabled := false
	if _, err := service.Create(t.Context(), CreateInput{
		Name:       "disabled-webhook",
		Enabled:    &disabled,
		WebhookURL: "https://approval-disabled.example.com/shepherd",
		SigningKey: "disabled-secret",
		CreatedBy:  "admin-1",
	}); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}
	sortOrder := -10
	if _, err := service.Create(t.Context(), CreateInput{
		Name:       "active-webhook",
		WebhookURL: "https://approval.example.com/shepherd",
		SigningKey: "active-secret",
		SortOrder:  &sortOrder,
		CreatedBy:  "admin-1",
	}); err != nil {
		t.Fatalf("Create active: %v", err)
	}

	fallback := &fallbackProvider{}
	active, err := service.ActiveProvider(t.Context(), fallback)
	if err != nil {
		t.Fatalf("ActiveProvider: %v", err)
	}
	if got := active.Type(); got != ProviderTypeWebhook {
		t.Fatalf("active provider type = %q, want webhook", got)
	}
}

func TestServiceValidationRejectsInvalidWebhookURL(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_approval_registry_validation")
	service := NewService(client, []byte("0123456789abcdef0123456789abcdef"))

	_, err := service.Create(t.Context(), CreateInput{
		Name:       "insecure-webhook",
		WebhookURL: "http://approval.example.com/shepherd",
		SigningKey: "webhook-secret",
		CreatedBy:  "admin-1",
	})
	if !IsValidationError(err) {
		t.Fatalf("Create error = %v, want validation error", err)
	}
}

func ptr[T any](value T) *T {
	return &value
}
