package runtimecontract

import (
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
