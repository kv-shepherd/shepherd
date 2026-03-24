package provider

func oidcAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"issuer_url", "client_id", "client_secret"},
		"properties": map[string]interface{}{
			"issuer_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Issuer URL",
				"description": "OIDC issuer discovery URL (e.g. https://accounts.google.com)",
			},
			"client_id": map[string]interface{}{
				"type":        "string",
				"title":       "Client ID",
				"description": "OAuth 2.0 client identifier",
			},
			"client_secret": map[string]interface{}{
				"type":        "string",
				"format":      "password",
				"title":       "Client Secret",
				"description": "OAuth 2.0 client secret",
			},
			"scopes": map[string]interface{}{
				"type":        "array",
				"title":       "Scopes",
				"description": "OAuth 2.0 scopes to request (default: openid, profile, email)",
				"items":       map[string]interface{}{"type": "string"},
				"default":     []string{"openid", "profile", "email"},
			},
			"redirect_uri": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Redirect URI",
				"description": "OAuth 2.0 callback URL",
			},
			"claims_mapping": map[string]interface{}{
				"type":                 "object",
				"title":                "Claims Mapping",
				"description":          "Map OIDC claims to Shepherd user fields (e.g. {\"email\": \"email\", \"name\": \"display_name\"})",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
			"test_endpoint": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Test Endpoint",
				"description": "URL for connectivity testing",
			},
			"sample_users": map[string]interface{}{
				"type":  "array",
				"title": "Sample Users",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
		},
	}
}
