package authproviderplugin_test

import (
	"slices"
	"testing"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
	_ "kv-shepherd.io/shepherd/plugins/authprovider/autoreg"
)

func TestListRegisteredAdminTypes_IncludesAutoRegisteredExample(t *testing.T) {
	t.Parallel()

	types := authproviderplugin.ListRegisteredAdminTypes()
	if len(types) == 0 {
		t.Fatal("expected non-empty registered auth provider types")
	}

	keys := make([]string, 0, len(types))
	for _, item := range types {
		keys = append(keys, item.Type)
	}

	if !slices.Contains(keys, "example-sso") {
		t.Fatalf("expected auto-registered plugin type example-sso, got %#v", keys)
	}
}
