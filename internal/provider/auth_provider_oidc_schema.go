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
			"claims_mapping": map[string]interface{}{
				"type":                 "object",
				"title":                "Claims Mapping",
				"description":          "Optional claim-to-Shepherd field mapping (e.g. {\"preferred_username\":\"username\",\"name\":\"display_name\"})",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
			"groups_claim": map[string]interface{}{
				"type":        "string",
				"title":       "Groups Claim",
				"description": "OIDC claim containing group/cohort values (default: groups)",
				"default":     "groups",
			},
			"request_timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"title":       "Request Timeout Seconds",
				"description": "Network timeout used for discovery, token exchange, and userinfo",
				"default":     10,
			},
		},
	}
}
