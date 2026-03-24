package provider

func newOIDCBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return &genericAuthProviderAdminAdapter{
		typeKey:      "oidc",
		displayName:  "OIDC",
		description:  "OpenID Connect provider via standardized adapter contract",
		builtIn:      true,
		configSchema: oidcAuthProviderSchema(),
	}
}
