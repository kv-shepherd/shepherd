package runtimecontract

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewAuthStartError(t *testing.T) {
	err := NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "unavailable")
	var startErr *AuthStartError
	if !errors.As(err, &startErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if startErr.Code != "AUTH_LOGIN_MODE_UNAVAILABLE" {
		t.Fatalf("startErr.Code = %q, want AUTH_LOGIN_MODE_UNAVAILABLE", startErr.Code)
	}
}

func TestNewAuthCredentialError(t *testing.T) {
	err := NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials")
	var credentialErr *AuthCredentialError
	if !errors.As(err, &credentialErr) {
		t.Fatalf("errors.As() = false, want true")
	}
	if credentialErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("credentialErr.Code = %q, want INVALID_CREDENTIALS", credentialErr.Code)
	}
}

func TestAuthResultDirectoryAuthorityJSONRoundTrip(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(AuthResult{
		ExternalID:         "user-1",
		Username:           "user@example.com",
		DisplayName:        "User One",
		Enabled:            true,
		DirectoryAuthority: AuthDirectoryAuthorityLoginOnly,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded AuthResult
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.DirectoryAuthority != AuthDirectoryAuthorityLoginOnly {
		t.Fatalf("decoded.DirectoryAuthority = %q, want %q", decoded.DirectoryAuthority, AuthDirectoryAuthorityLoginOnly)
	}
}

func TestAuthCallbackRequestCallbackURLJSONRoundTrip(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(AuthCallbackRequest{
		Method:      "GET",
		CallbackURL: "https://console.example.com/api/v1/auth/providers/oidc/callback",
		Query: map[string][]string{
			"code":  {"auth-code"},
			"state": {"state-token"},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded AuthCallbackRequest
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.CallbackURL != "https://console.example.com/api/v1/auth/providers/oidc/callback" {
		t.Fatalf("decoded.CallbackURL = %q", decoded.CallbackURL)
	}
}

type testCallbackOriginDescriber struct{}

func (testCallbackOriginDescriber) AllowedCallbackOrigins(config map[string]interface{}) []string {
	origin, _ := config["origin"].(string)
	if origin == "" {
		return nil
	}
	return []string{origin}
}

func TestAuthCallbackOriginDescriber(t *testing.T) {
	t.Parallel()

	var describer AuthCallbackOriginDescriber = testCallbackOriginDescriber{}
	got := describer.AllowedCallbackOrigins(map[string]interface{}{
		"origin": "https://login.example.com",
	})
	if len(got) != 1 || got[0] != "https://login.example.com" {
		t.Fatalf("AllowedCallbackOrigins() = %#v, want login origin", got)
	}
}
