package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestGetExternalAuthPlatformSettings_UsesServerConfigFallback(t *testing.T) {
	t.Parallel()

	srv, _ := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://fallback.example.com")
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/platform-settings/external-auth", "", "admin-1", []string{"auth_provider:read"})

	srv.GetExternalAuthPlatformSettings(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ExternalAuthPlatformSettings
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Source != generated.ServerConfig {
		t.Fatalf("source = %q, want %q", resp.Source, generated.ServerConfig)
	}
	if resp.EffectivePublicBaseUrl != "https://fallback.example.com" {
		t.Fatalf("effective_public_base_url = %q", resp.EffectivePublicBaseUrl)
	}
}

func TestUpdateExternalAuthPlatformSettings_PersistsOverride(t *testing.T) {
	t.Parallel()

	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://fallback.example.com")
	c, w := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/platform-settings/external-auth",
		`{"public_base_url":"https://auth.example.com"}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)

	srv.UpdateExternalAuthPlatformSettings(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.ExternalAuthPlatformSettings
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Source != generated.PlatformSetting {
		t.Fatalf("source = %q, want %q", resp.Source, generated.PlatformSetting)
	}
	if resp.PublicBaseUrl != "https://auth.example.com" {
		t.Fatalf("public_base_url = %q", resp.PublicBaseUrl)
	}

	row, err := client.PlatformSetting.Query().Only(t.Context())
	if err != nil {
		t.Fatalf("query platform setting: %v", err)
	}
	if got, _ := row.Value["public_base_url"].(string); got != "https://auth.example.com" {
		t.Fatalf("stored public_base_url = %q", got)
	}
}

func TestUpdateExternalAuthPlatformSettings_RejectsInvalidURL(t *testing.T) {
	t.Parallel()

	srv, _ := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "")
	c, w := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/platform-settings/external-auth",
		`{"public_base_url":"auth.example.com/callback"}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)

	srv.UpdateExternalAuthPlatformSettings(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "INVALID_REQUEST")
}
