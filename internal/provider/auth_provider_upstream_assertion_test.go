package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestUpstreamAssertionStartLoginBuildsRedirect(t *testing.T) {
	t.Parallel()

	adapter := newUpstreamAssertionAuthProviderAdapter()
	resp, err := adapter.StartLogin(context.Background(), map[string]interface{}{
		"login_entry_url":      "https://legacy.example.com/login?client=shepherd",
		"trust_mode":           upstreamAssertionTrustModeUserInfo,
		"userinfo_endpoint":    "https://legacy.example.com/userinfo",
		"external_id_path":     "sub",
		"username_path":        "username",
		"callback_param_name":  "redirect_uri",
		"state_param_name":     "state",
		"return_to_param_name": "return_to",
	}, AuthStartRequest{
		CallbackURL: "https://shepherd.example.com/api/v1/auth/providers/p1/callback",
		State:       "state-token",
		ReturnTo:    "https://shepherd.example.com/dashboard",
	})
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	redirectURL, err := url.Parse(resp.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect url: %v", err)
	}
	if redirectURL.Host != "legacy.example.com" {
		t.Fatalf("redirect host = %q, want legacy.example.com", redirectURL.Host)
	}
	if got := redirectURL.Query().Get("redirect_uri"); got != "https://shepherd.example.com/api/v1/auth/providers/p1/callback" {
		t.Fatalf("redirect_uri = %q", got)
	}
	if got := redirectURL.Query().Get("state"); got != "state-token" {
		t.Fatalf("state = %q", got)
	}
	if got := redirectURL.Query().Get("return_to"); got != "https://shepherd.example.com/dashboard" {
		t.Fatalf("return_to = %q", got)
	}
}

func TestUpstreamAssertionCompleteLoginTokenUserInfoMapsCanonicalResult(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer callback-token" {
			t.Fatalf("authorization header = %q, want Bearer callback-token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"sub":"user-1",
			"profile":{"username":"alice","display_name":"Alice Example","email":"alice@example.com"},
			"groups":["ops","dev"],
			"enabled":true
		}`))
	}))
	defer upstream.Close()

	adapter := newUpstreamAssertionAuthProviderAdapter()
	result, err := adapter.CompleteLogin(context.Background(), map[string]interface{}{
		"login_entry_url":          "https://legacy.example.com/login",
		"trust_mode":               upstreamAssertionTrustModeUserInfo,
		"userinfo_endpoint":        upstream.URL,
		"external_id_path":         "sub",
		"username_path":            "profile.username",
		"display_name_path":        "profile.display_name",
		"email_path":               "profile.email",
		"enabled_path":             "enabled",
		"cohort_path":              "groups",
		"profile_attribute_paths":  map[string]interface{}{"mail_alias": "profile.email"},
		"incoming_token_transport": upstreamTokenTransportQuery,
		"incoming_token_name":      "token",
		"upstream_token_transport": upstreamTokenTransportBearer,
	}, AuthCallbackRequest{
		Query: map[string][]string{
			"token": {"callback-token"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteLogin() error = %v", err)
	}
	if result.ExternalID != "user-1" || result.Username != "alice" || result.DisplayName != "Alice Example" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Cohorts) != 2 || result.Cohorts[0].Kind != "group" {
		t.Fatalf("cohorts = %#v", result.Cohorts)
	}
	if got := result.ProfileAttributes["mail_alias"]; got != "alice@example.com" {
		t.Fatalf("mail_alias = %#v, want alice@example.com", got)
	}
}

func TestUpstreamAssertionCompleteLoginTrustedHeadersRejectsUntrustedRemote(t *testing.T) {
	t.Parallel()

	adapter := newUpstreamAssertionAuthProviderAdapter()
	_, err := adapter.CompleteLogin(context.Background(), map[string]interface{}{
		"login_entry_url":             "https://gateway.example.com/login",
		"trust_mode":                  upstreamAssertionTrustModeHeaders,
		"trusted_gateway_cidrs":       []interface{}{"10.0.0.0/24"},
		"trusted_header_external_id":  "X-User-ID",
		"trusted_header_username":     "X-Username",
		"trusted_header_display_name": "X-Display-Name",
	}, AuthCallbackRequest{
		Header: map[string][]string{
			"X-User-ID":      {"u-1"},
			"X-Username":     {"alice"},
			"X-Display-Name": {"Alice"},
		},
		RemoteAddr: "192.168.1.10:12345",
	})
	if err == nil {
		t.Fatal("expected untrusted remote error, got nil")
	}
}
