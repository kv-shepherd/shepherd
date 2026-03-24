package admincontract

import "context"

// AuthProviderTypeDescriptor describes a provider type exposed to admin UI/API.
type AuthProviderTypeDescriptor struct {
	Type         string
	DisplayName  string
	Description  string
	BuiltIn      bool
	ConfigSchema map[string]interface{}
}

// AuthProviderSampleField is the normalized sample-field contract exposed by plugins.
type AuthProviderSampleField struct {
	Field       string
	ValueType   string
	UniqueCount int
	Sample      []string
}

// AuthProviderAdminAdapter defines the plugin contract for auth provider management endpoints.
type AuthProviderAdminAdapter interface {
	// Type returns the provider type key used by auth_providers.auth_type.
	Type() string
	// ValidateConfig checks whether the provider config is structurally valid.
	ValidateConfig(config map[string]interface{}) error
	// TestConnection performs a provider-specific connectivity check.
	TestConnection(ctx context.Context, config map[string]interface{}) (bool, string, error)
	// SampleFields extracts sample fields for RBAC mapping configuration.
	SampleFields(ctx context.Context, config map[string]interface{}) ([]AuthProviderSampleField, error)
}

// AuthProviderAdminAdapterDescriber is an optional adapter extension for metadata exposure.
type AuthProviderAdminAdapterDescriber interface {
	Describe() AuthProviderTypeDescriptor
}
