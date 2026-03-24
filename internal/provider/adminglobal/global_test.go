package adminglobal

import (
	"context"
	"testing"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminregistry "kv-shepherd.io/shepherd/internal/provider/adminregistry"
)

type globalAdapter struct{}

func (globalAdapter) Type() string                                { return "global-test" }
func (globalAdapter) ValidateConfig(map[string]interface{}) error { return nil }
func (globalAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}
func (globalAdapter) SampleFields(context.Context, map[string]interface{}) ([]admincontract.AuthProviderSampleField, error) {
	return nil, nil
}
func (globalAdapter) Describe() admincontract.AuthProviderTypeDescriptor {
	return admincontract.AuthProviderTypeDescriptor{Type: "global-test", DisplayName: "Global Test"}
}

func TestRegisterResolveAndList(t *testing.T) {
	previous := globalRegistry
	globalRegistry = adminregistry.New()
	t.Cleanup(func() { globalRegistry = previous })

	if err := Register(globalAdapter{}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if Resolve("global-test") == nil {
		t.Fatal("Resolve() = nil, want adapter")
	}
	if items := List(); len(items) != 1 || items[0].Type != "global-test" {
		t.Fatalf("List() = %#v, want one global-test descriptor", items)
	}
}
