package provider

func builtInAuthProviderAdapters() []AuthProviderAdminAdapter {
	return []AuthProviderAdminAdapter{
		newOIDCBuiltInAuthProviderAdapter(),
		newLDAPBuiltInAuthProviderAdapter(),
		newWeComBuiltInAuthProviderAdapter(),
	}
}
