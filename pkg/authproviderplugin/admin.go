package authproviderplugin

import (
	"fmt"

	internalprovider "kv-shepherd.io/shepherd/internal/provider"
)

// AdminSampleField is the standardized sample-field contract for auth-provider plugins.
type AdminSampleField = internalprovider.AuthProviderSampleField

// AdminTypeDescriptor is the discoverable plugin type metadata returned by registry/API.
type AdminTypeDescriptor = internalprovider.AuthProviderTypeDescriptor

// AdminAdapter is the admin-side plugin contract.
type AdminAdapter = internalprovider.AuthProviderAdminAdapter

// AdminAdapterDescriber allows plugins to expose type metadata and config schema.
type AdminAdapterDescriber = internalprovider.AuthProviderAdminAdapterDescriber

// RegisterAdminAdapter registers a provider plugin adapter.
func RegisterAdminAdapter(adapter AdminAdapter) error {
	return internalprovider.RegisterAuthProviderAdminAdapter(adapter)
}

// MustRegisterAdminAdapter registers a provider plugin adapter and panics on failure.
func MustRegisterAdminAdapter(adapter AdminAdapter) {
	if err := RegisterAdminAdapter(adapter); err != nil {
		panic(fmt.Sprintf("auth provider plugin register failed: %v", err))
	}
}

// ListRegisteredAdminTypes returns current registered plugin types.
func ListRegisteredAdminTypes() []AdminTypeDescriptor {
	return internalprovider.ListAuthProviderAdminAdapterTypes()
}
