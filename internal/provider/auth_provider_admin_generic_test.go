package provider

import (
	"strings"
	"testing"
)

func TestValidateGenericProviderHealthcheckEndpoint_RejectsPrivateTargets(t *testing.T) {
	t.Parallel()

	_, err := validateGenericProviderHealthcheckEndpoint(t.Context(), "http://127.0.0.1:8080/health")
	if err == nil {
		t.Fatal("validateGenericProviderHealthcheckEndpoint() expected private-target rejection")
	}
	if !strings.Contains(err.Error(), "private or reserved") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestValidateGenericProviderHealthcheckEndpoint_RejectsNonHTTP(t *testing.T) {
	t.Parallel()

	_, err := validateGenericProviderHealthcheckEndpoint(t.Context(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("validateGenericProviderHealthcheckEndpoint() expected scheme rejection")
	}
}

func TestValidateGenericProviderHealthcheckEndpoint_AllowsPublicHTTPS(t *testing.T) {
	t.Parallel()

	parsed, err := validateGenericProviderHealthcheckEndpoint(t.Context(), "https://8.8.8.8/health")
	if err != nil {
		t.Fatalf("validateGenericProviderHealthcheckEndpoint() error = %v", err)
	}
	if got := parsed.Hostname(); got != "8.8.8.8" {
		t.Fatalf("Hostname() = %q, want 8.8.8.8", got)
	}
}
