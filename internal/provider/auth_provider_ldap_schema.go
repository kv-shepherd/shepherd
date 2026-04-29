package provider

func ldapAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"required":             []string{"server_url", "bind_dn", "base_dn"},
		"properties": map[string]interface{}{
			"server_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Server URL",
				"description": "LDAP server URL (prefer ldaps://; ldap:// requires StartTLS with tls_enabled=true)",
			},
			"bind_dn": map[string]interface{}{
				"type":        "string",
				"title":       "Bind DN",
				"description": "Distinguished name for LDAP bind (e.g. cn=admin,dc=example,dc=com)",
			},
			"bind_password": map[string]interface{}{
				"type":        "string",
				"format":      "password",
				"title":       "Bind Password",
				"description": "Password for LDAP bind authentication",
			},
			"base_dn": map[string]interface{}{
				"type":        "string",
				"title":       "Base DN",
				"description": "Base distinguished name for user searches (e.g. ou=users,dc=example,dc=com)",
			},
			"user_filter": map[string]interface{}{
				"type":        "string",
				"title":       "User Filter",
				"description": "LDAP filter for user lookup (e.g. (uid=%s))",
				"default":     "(uid=%s)",
			},
			"group_filter": map[string]interface{}{
				"type":        "string",
				"title":       "Group Filter",
				"description": "LDAP filter for group membership (e.g. (memberUid=%s))",
			},
			"group_base_dn": map[string]interface{}{
				"type":        "string",
				"title":       "Group Base DN",
				"description": "Optional search base for LDAP group lookups; defaults to Base DN",
			},
			"directory_base_dn": map[string]interface{}{
				"type":        "string",
				"title":       "Directory Base DN",
				"description": "Optional LDAP search base for directory sync and enrichment; defaults to Base DN",
			},
			"directory_filter": map[string]interface{}{
				"type":        "string",
				"title":       "Directory Filter",
				"description": "LDAP filter used for directory sync and enrichment",
				"default":     "(objectClass=person)",
			},
			"username_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "Username Attribute",
				"description": "LDAP attribute mapped to canonical username",
				"default":     "uid",
			},
			"display_name_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "Display Name Attribute",
				"description": "LDAP attribute mapped to canonical display name",
				"default":     "cn",
			},
			"email_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "Email Attribute",
				"description": "LDAP attribute mapped to canonical email",
				"default":     "mail",
			},
			"external_id_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "External ID Attribute",
				"description": "LDAP attribute mapped to canonical external_id; use dn to keep the entry DN",
				"default":     "dn",
			},
			"member_of_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "Member Of Attribute",
				"description": "LDAP attribute that carries direct group memberships",
				"default":     "memberOf",
			},
			"group_name_attribute": map[string]interface{}{
				"type":        "string",
				"title":       "Group Name Attribute",
				"description": "LDAP group attribute used when resolving groups through group_filter",
				"default":     "cn",
			},
			"tls_enabled": map[string]interface{}{
				"type":        "boolean",
				"title":       "Enable TLS",
				"description": "Use TLS/STARTTLS for LDAP connection; ldap:// without StartTLS is rejected",
				"default":     true,
			},
			"test_endpoint": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Test Endpoint",
				"description": "Alternative healthcheck URL",
			},
			"sample_users": map[string]interface{}{
				"type":  "array",
				"title": "Sample Users",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
			"request_timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"title":       "Request Timeout Seconds",
				"description": "LDAP network timeout used for bind and search operations",
				"default":     10,
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
				"default":     defaultScheduledEnrichmentCron,
			},
			"schedule_timezone": map[string]interface{}{
				"type":        "string",
				"title":       "Schedule Timezone",
				"description": "IANA timezone used when evaluating the cron schedule",
				"default":     defaultScheduleTimezoneUTC,
			},
			"join_key_type": map[string]interface{}{
				"type":        "string",
				"title":       "Join Key Type",
				"description": "Explicit join key used to match LDAP records to canonical users",
				"enum":        []string{string(DirectoryJoinKeyUsername)},
				"default":     string(DirectoryJoinKeyUsername),
			},
			"scheduled_provider_request": map[string]interface{}{
				"type":                 "object",
				"title":                "Scheduled Provider Request",
				"description":          "Opaque LDAP request payload frozen into each scheduled enrichment job",
				"additionalProperties": true,
			},
		},
	}
}
