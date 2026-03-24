package provider

func newSSOBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return &genericAuthProviderAdminAdapter{
		typeKey:      "sso",
		displayName:  "SSO",
		description:  "Enterprise SSO provider via standardized adapter contract",
		builtIn:      true,
		configSchema: ssoAuthProviderSchema(),
	}
}
