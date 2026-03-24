package provider

import adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"

func init() {
	adminglobal.MustRegister(builtInAuthProviderAdapters()...)
}

// RegisterAuthProviderAdminAdapter registers an adapter globally.
func RegisterAuthProviderAdminAdapter(adapter AuthProviderAdminAdapter) error {
	return adminglobal.Register(adapter)
}

// ResolveAuthProviderAdminAdapter resolves an adapter from global registry.
func ResolveAuthProviderAdminAdapter(authType string) AuthProviderAdminAdapter {
	return adminglobal.Resolve(authType)
}

// ListAuthProviderAdminAdapterTypes returns all registered provider type descriptors.
func ListAuthProviderAdminAdapterTypes() []AuthProviderTypeDescriptor {
	return adminglobal.List()
}
