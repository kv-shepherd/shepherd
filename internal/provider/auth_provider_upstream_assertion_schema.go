package provider

func upstreamAssertionAuthProviderSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"login_entry_url", "trust_mode"},
		"properties": map[string]interface{}{
			"login_entry_url": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Login Entry URL",
				"description": "Upstream entry URL that receives callback/state query parameters and initiates upstream authentication",
			},
			"callback_param_name": map[string]interface{}{
				"type":        "string",
				"title":       "Callback Param Name",
				"description": "Query parameter name used to forward Shepherd callback URL to the upstream entry URL",
				"default":     "redirect_uri",
			},
			"state_param_name": map[string]interface{}{
				"type":        "string",
				"title":       "State Param Name",
				"description": "Query parameter name used to forward Shepherd state to the upstream entry URL",
				"default":     "state",
			},
			"return_to_param_name": map[string]interface{}{
				"type":        "string",
				"title":       "Return-To Param Name",
				"description": "Optional query parameter name used to forward the desired final return URL to the upstream entry URL",
			},
			"trust_mode": map[string]interface{}{
				"type":        "string",
				"title":       "Trust Mode",
				"description": "How this provider validates the upstream identity assertion",
				"enum": []string{
					upstreamAssertionTrustModeUserInfo,
					upstreamAssertionTrustModeIntrospect,
					upstreamAssertionTrustModeHeaders,
				},
			},
			"incoming_token_transport": map[string]interface{}{
				"type":        "string",
				"title":       "Incoming Token Transport",
				"description": "How Shepherd should read the upstream assertion token from the callback request",
				"enum": []string{
					upstreamTokenTransportBearer,
					upstreamTokenTransportQuery,
					upstreamTokenTransportHeader,
					upstreamTokenTransportCookie,
				},
				"default": upstreamTokenTransportQuery,
			},
			"incoming_token_name": map[string]interface{}{
				"type":        "string",
				"title":       "Incoming Token Name",
				"description": "Query/header/cookie name for the upstream assertion token",
				"default":     "token",
			},
			"userinfo_endpoint": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "UserInfo Endpoint",
				"description": "Endpoint used in token_userinfo mode",
			},
			"introspection_endpoint": map[string]interface{}{
				"type":        "string",
				"format":      "uri",
				"title":       "Introspection Endpoint",
				"description": "Endpoint used in token_introspection mode",
			},
			"upstream_token_transport": map[string]interface{}{
				"type":        "string",
				"title":       "Upstream Token Transport",
				"description": "How the token should be forwarded to the upstream endpoint",
				"enum": []string{
					upstreamTokenTransportBearer,
					upstreamTokenTransportQuery,
					upstreamTokenTransportHeader,
					upstreamTokenTransportForm,
				},
				"default": upstreamTokenTransportBearer,
			},
			"upstream_token_name": map[string]interface{}{
				"type":        "string",
				"title":       "Upstream Token Name",
				"description": "Parameter or header name used when forwarding the token upstream",
				"default":     "token",
			},
			"external_id_path": map[string]interface{}{
				"type":        "string",
				"title":       "External ID Path",
				"description": "Dot path in the upstream JSON payload for the canonical external_id",
			},
			"username_path": map[string]interface{}{
				"type":        "string",
				"title":       "Username Path",
				"description": "Dot path in the upstream JSON payload for the canonical username",
			},
			"display_name_path": map[string]interface{}{
				"type":        "string",
				"title":       "Display Name Path",
				"description": "Dot path in the upstream JSON payload for the canonical display_name",
			},
			"email_path": map[string]interface{}{
				"type":        "string",
				"title":       "Email Path",
				"description": "Dot path in the upstream JSON payload for the canonical email",
			},
			"enabled_path": map[string]interface{}{
				"type":        "string",
				"title":       "Enabled Path",
				"description": "Optional dot path in the upstream JSON payload for the canonical enabled flag",
			},
			"active_path": map[string]interface{}{
				"type":        "string",
				"title":       "Active Path",
				"description": "Boolean dot path checked in token_introspection mode",
				"default":     "active",
			},
			"cohort_path": map[string]interface{}{
				"type":        "string",
				"title":       "Cohort Path",
				"description": "Optional dot path to an array of strings used as external cohorts",
			},
			"cohort_kind": map[string]interface{}{
				"type":        "string",
				"title":       "Cohort Kind",
				"description": "Canonical cohort kind used when mapping string-array cohorts",
				"default":     "group",
			},
			"profile_attribute_paths": map[string]interface{}{
				"type":                 "object",
				"title":                "Profile Attribute Paths",
				"description":          "Map display-only profile attribute keys to upstream JSON dot paths",
				"additionalProperties": map[string]interface{}{"type": "string"},
			},
			"trusted_gateway_cidrs": map[string]interface{}{
				"type":        "array",
				"title":       "Trusted Gateway CIDRs",
				"description": "CIDR ranges that may assert trusted gateway headers",
				"items":       map[string]interface{}{"type": "string"},
			},
			"trusted_header_external_id": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header External ID",
				"description": "Header name for canonical external_id in trusted_gateway_headers mode",
			},
			"trusted_header_username": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Username",
				"description": "Header name for canonical username in trusted_gateway_headers mode",
			},
			"trusted_header_display_name": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Display Name",
				"description": "Header name for canonical display_name in trusted_gateway_headers mode",
			},
			"trusted_header_email": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Email",
				"description": "Header name for canonical email in trusted_gateway_headers mode",
			},
			"trusted_header_enabled": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Enabled",
				"description": "Optional header name for canonical enabled in trusted_gateway_headers mode",
			},
			"trusted_header_cohorts": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Cohorts",
				"description": "Optional header name for comma-separated external cohorts in trusted_gateway_headers mode",
			},
			"trusted_header_cohort_kind": map[string]interface{}{
				"type":        "string",
				"title":       "Trusted Header Cohort Kind",
				"description": "Canonical cohort kind for trusted_header_cohorts",
				"default":     "group",
			},
			"request_timeout_seconds": map[string]interface{}{
				"type":        "integer",
				"title":       "Request Timeout Seconds",
				"description": "HTTP timeout used for upstream calls",
				"default":     10,
			},
		},
	}
}
