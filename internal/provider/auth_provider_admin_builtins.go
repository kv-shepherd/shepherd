package provider

func builtInAuthProviderAdapters() []AuthProviderAdminAdapter {
	return []AuthProviderAdminAdapter{
		newGenericBuiltInAuthProviderAdapter(),
		newOIDCBuiltInAuthProviderAdapter(),
		newLDAPBuiltInAuthProviderAdapter(),
		newSSOBuiltInAuthProviderAdapter(),
		newWeComBuiltInAuthProviderAdapter(),
	}
}
