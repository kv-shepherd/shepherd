package provider

func ssoAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"metadata_url", "entity_id"},
		"properties": map[string]interface{}{
			"metadata_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Metadata URL",
				"description": "SAML IdP metadata endpoint URL",
			},
			"entity_id": map[string]interface{}{
				"type":        "string",
				"title":       "Entity ID",
				"description": "SAML Service Provider entity ID",
			},
			"certificate": map[string]interface{}{
				"type":        "string",
				"format":      "password",
				"title":       "Certificate",
				"description": "X.509 certificate (PEM) for signature verification",
			},
			"name_id_format": map[string]interface{}{
				"type":        "string",
				"title":       "NameID Format",
				"description": "SAML NameID format (e.g. urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress)",
				"default":     "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress",
			},
			"assertion_consumer_service_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "ACS URL",
				"description": "Assertion Consumer Service callback URL",
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
