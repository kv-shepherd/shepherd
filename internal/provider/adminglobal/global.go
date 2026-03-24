package adminglobal

import (
	"fmt"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminregistry "kv-shepherd.io/shepherd/internal/provider/adminregistry"
)

var globalRegistry = adminregistry.New()

// Register registers an adapter globally.
func Register(adapter admincontract.AuthProviderAdminAdapter) error {
	return globalRegistry.Register(adapter)
}

// MustRegister panics when a global adapter registration fails.
func MustRegister(adapters ...admincontract.AuthProviderAdminAdapter) {
	for _, adapter := range adapters {
		if err := Register(adapter); err != nil {
			panic(fmt.Sprintf("auth provider admin register failed: %v", err))
		}
	}
}

// Resolve resolves an adapter from the global registry.
func Resolve(authType string) admincontract.AuthProviderAdminAdapter {
	return globalRegistry.Resolve(authType)
}

// List returns all registered provider type descriptors.
func List() []admincontract.AuthProviderTypeDescriptor {
	return globalRegistry.List()
}
