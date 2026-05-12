package runtimeview

import (
	"errors"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	_ "kv-shepherd.io/shepherd/internal/provider"
)

func TestBuildRuntimeDescriptor_ReturnsErrorWhenAdapterMissing(t *testing.T) {
	_, err := BuildRuntimeDescriptor(&ent.AuthProvider{
		AuthType: "missing-provider",
		Name:     "Missing",
	})
	if !errors.Is(err, ErrAuthProviderAdapterNotFound) {
		t.Fatalf("BuildRuntimeDescriptor() error = %v, want ErrAuthProviderAdapterNotFound", err)
	}
}

func TestBuildLoginProvider_ReturnsCredentialModeForLDAP(t *testing.T) {
	item, supported := BuildLoginProvider(&ent.AuthProvider{
		ID:       "provider-1",
		Name:     "Corporate LDAP",
		AuthType: "ldap",
	})
	if !supported {
		t.Fatal("BuildLoginProvider() supported = false, want true")
	}
	if item.Id != "provider-1" {
		t.Fatalf("BuildLoginProvider() id = %q, want provider-1", item.Id)
	}
	if !containsInteraction(item.LoginModes, "credentials") {
		t.Fatalf("BuildLoginProvider() login modes = %#v, want credentials interaction", item.LoginModes)
	}
}

func TestBuildLoginProvider_ReturnsUnsupportedForMissingType(t *testing.T) {
	_, supported := BuildLoginProvider(&ent.AuthProvider{
		ID:       "provider-2",
		Name:     "Missing",
		AuthType: "missing-provider",
	})
	if supported {
		t.Fatal("BuildLoginProvider() supported = true, want false")
	}
}

func containsInteraction(modes []generated.AuthLoginMode, interaction string) bool {
	for _, mode := range modes {
		if string(mode.Interaction) == interaction {
			return true
		}
	}
	return false
}
