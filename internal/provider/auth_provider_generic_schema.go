package provider

func genericAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]interface{}{
			"test_endpoint": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Test Endpoint",
				"description": "URL for connectivity testing (healthcheck)",
			},
			"healthcheck_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Healthcheck URL",
				"description": "Alternative healthcheck endpoint",
			},
			"sample_users": map[string]interface{}{
				"type":        "array",
				"title":       "Sample Users",
				"description": "Sample user objects for field discovery and RBAC mapping",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"enrichment_enabled": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enable Scheduled Enrichment",
				"description": "Enable scheduled enrichment for already existing canonical users",
				"default":     false,
			},
			"enrichment_mode": map[string]interface{}{
				"type":        "string",
				"title":       "Enrichment Mode",
				"description": "Canonical enrichment policy applied by the scheduler",
				"enum":        []string{string(DirectoryEnrichmentModeEnrichExistingOnly)},
				"default":     string(DirectoryEnrichmentModeEnrichExistingOnly),
			},
			"schedule_cron": map[string]interface{}{
				"type":        "string",
				"title":       "Schedule Cron",
				"description": "Standard 5-field cron expression used for scheduled enrichment",
				"default":     "0 * * * *",
			},
			"schedule_timezone": map[string]interface{}{
				"type":        "string",
				"title":       "Schedule Timezone",
				"description": "IANA timezone used when evaluating the cron schedule",
				"default":     "UTC",
			},
			"join_key_type": map[string]interface{}{
				"type":        "string",
				"title":       "Join Key Type",
				"description": "Explicit join key used to match external records to canonical users",
				"enum":        []string{string(DirectoryJoinKeyUsername)},
				"default":     string(DirectoryJoinKeyUsername),
			},
			"scheduled_provider_request": map[string]interface{}{
				"type":                 "object",
				"title":                "Scheduled Provider Request",
				"description":          "Opaque provider-owned request payload frozen into each scheduled enrichment job",
				"additionalProperties": true,
			},
		},
	}
}
