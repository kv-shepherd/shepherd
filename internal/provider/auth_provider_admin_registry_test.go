package provider

import (
	"context"
	"slices"
	"testing"
)

type testAuthProviderAdapter struct {
	typeKey string
}

func (a *testAuthProviderAdapter) Type() string { return a.typeKey }

func (a *testAuthProviderAdapter) ValidateConfig(config map[string]interface{}) error {
	return nil
}

func (a *testAuthProviderAdapter) TestConnection(_ context.Context, _ map[string]interface{}) (bool, string, error) {
	return true, "ok", nil
}

func (a *testAuthProviderAdapter) SampleFields(_ context.Context, _ map[string]interface{}) ([]AuthProviderSampleField, error) {
	return nil, nil
}

func TestAuthProviderAdminRegistryBuiltinsAndStrictRegistration(t *testing.T) {
	t.Parallel()

	r := newAuthProviderAdminRegistry()
	types := r.List()
	if len(types) < 4 {
		t.Fatalf("expected built-in provider types, got %d", len(types))
	}

	keys := make([]string, 0, len(types))
	for _, item := range types {
		keys = append(keys, item.Type)
	}
	for _, expected := range []string{"generic", "oidc", "ldap", "sso"} {
		if !slices.Contains(keys, expected) {
			t.Fatalf("missing built-in auth provider type %q in %#v", expected, keys)
		}
	}

	if adapter := r.Resolve("unknown-provider"); adapter != nil {
		t.Fatal("expected unknown provider type to resolve to nil adapter")
	}

	if err := r.Register(&testAuthProviderAdapter{typeKey: "custom"}); err != nil {
		t.Fatalf("register custom adapter failed: %v", err)
	}
	if adapter := r.Resolve("custom"); adapter == nil {
		t.Fatal("expected registered custom adapter to resolve")
	}

	if err := r.Register(&testAuthProviderAdapter{typeKey: "custom"}); err == nil {
		t.Fatal("expected duplicate adapter registration to fail")
	}
}
