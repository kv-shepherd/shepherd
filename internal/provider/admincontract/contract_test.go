package admincontract

import (
	"context"
	"testing"
)

type stubAdminAdapter struct{}

func (stubAdminAdapter) Type() string                                { return "stub" }
func (stubAdminAdapter) ValidateConfig(map[string]interface{}) error { return nil }
func (stubAdminAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}
func (stubAdminAdapter) SampleFields(context.Context, map[string]interface{}) ([]AuthProviderSampleField, error) {
	return nil, nil
}
func (stubAdminAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{Type: "stub", DisplayName: "Stub"}
}

func TestStubAdminAdapterImplementsContracts(t *testing.T) {
	adapter := stubAdminAdapter{}

	if _, ok := any(adapter).(AuthProviderAdminAdapter); !ok {
		t.Fatal("stub adapter does not implement AuthProviderAdminAdapter")
	}
	if _, ok := any(adapter).(AuthProviderAdminAdapterDescriber); !ok {
		t.Fatal("stub adapter does not implement AuthProviderAdminAdapterDescriber")
	}
	if got := adapter.Describe().Type; got != "stub" {
		t.Fatalf("Describe().Type = %q, want %q", got, "stub")
	}
}
