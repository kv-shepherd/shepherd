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
		},
	}
}
