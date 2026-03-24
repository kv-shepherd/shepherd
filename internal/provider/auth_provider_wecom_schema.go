package provider

func weComAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"corp_id", "agent_id", "agent_secret"},
		"properties": map[string]interface{}{
			"corp_id": map[string]interface{}{
				"type":        "string",
				"title":       "Corp ID",
				"description": "WeCom CorpID",
			},
			"agent_id": map[string]interface{}{
				"type":        "string",
				"title":       "Agent ID",
				"description": "WeCom application AgentID",
			},
			"agent_secret": map[string]interface{}{
				"type":        "string",
				"format":      "password",
				"title":       "Agent Secret",
				"description": "WeCom application secret",
			},
			"enrichment_enabled": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enable Scheduled Enrichment",
				"description": "Enable scheduled enrichment for existing canonical users using WeCom directory data",
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
				"description": "Explicit join key used to match WeCom records to canonical users",
				"enum":        []string{string(DirectoryJoinKeyUsername)},
				"default":     string(DirectoryJoinKeyUsername),
			},
			"scheduled_department_ids": map[string]interface{}{
				"type":        "array",
				"title":       "Scheduled Department IDs",
				"description": "WeCom department IDs included in scheduled enrichment",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"scheduled_include_nested": map[string]interface{}{
				"type":        "boolean",
				"title":       "Include Nested Departments",
				"description": "Whether scheduled enrichment should include descendant departments",
				"default":     false,
			},
		},
	}
}
