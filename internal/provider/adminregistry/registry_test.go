package adminregistry

import (
	"context"
	"testing"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
)

type registryAdapter struct{ typeKey string }

func (a registryAdapter) Type() string                              { return a.typeKey }
func (registryAdapter) ValidateConfig(map[string]interface{}) error { return nil }
func (registryAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}
func (registryAdapter) SampleFields(context.Context, map[string]interface{}) ([]admincontract.AuthProviderSampleField, error) {
	return nil, nil
}
func (a registryAdapter) Describe() admincontract.AuthProviderTypeDescriptor {
	return admincontract.AuthProviderTypeDescriptor{Type: a.typeKey, DisplayName: a.typeKey}
}

func TestRegistryRegisterResolveAndList(t *testing.T) {
	registry := New()
	if err := registry.Register(registryAdapter{typeKey: "stub"}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if got := registry.Resolve("stub"); got == nil {
		t.Fatal("Resolve() = nil, want adapter")
	}
	if items := registry.List(); len(items) != 1 || items[0].Type != "stub" {
		t.Fatalf("List() = %#v, want one stub descriptor", items)
	}
}
