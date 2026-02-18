package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

type fileRule struct {
	path           string
	required       []string
	forbiddenRegex []*regexp.Regexp
	forbiddenText  []string
}

func main() {
	rules := []fileRule{
		{
			path: "internal/api/handlers/server_admin_catalog.go",
			required: []string{
				"providerregistry.ResolveAuthProviderAdminAdapter(",
			},
			forbiddenRegex: []*regexp.Regexp{
				regexp.MustCompile(`authType\s*==\s*"(oidc|ldap|sso)"`),
				regexp.MustCompile(`case\s+"(oidc|ldap|sso)"`),
			},
		},
		{
			path: "web/src/features/admin-auth-providers/hooks/useAdminAuthProvidersController.ts",
			required: []string{
				"api.GET('/admin/auth-provider-types')",
			},
			forbiddenText: []string{
				"auth_type: 'oidc'",
				"auth_type: \"oidc\"",
				"AUTH_PROVIDER_TYPE_OPTIONS",
			},
		},
		{
			path: "web/src/features/admin-auth-providers/types.ts",
			forbiddenText: []string{
				"AUTH_PROVIDER_TYPE_OPTIONS",
			},
		},
		{
			path: "web/tests/e2e/master-flow-live.spec.ts",
			required: []string{
				"auth provider flow uses discovered types and performs create/delete",
				"/api/v1/admin/auth-provider-types",
				"/api/v1/admin/auth-providers",
			},
		},
		{
			path: "api/openapi.yaml",
			forbiddenText: []string{
				"OIDC/LDAP authentication provider management",
				"Create authentication provider (OIDC/LDAP)",
				"Update authentication provider (OIDC/LDAP)",
			},
		},
	}

	var failures []string
	for _, rule := range rules {
		content, err := os.ReadFile(rule.path)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: read failed: %v", rule.path, err))
			continue
		}
		text := string(content)

		for _, req := range rule.required {
			if !strings.Contains(text, req) {
				failures = append(failures, fmt.Sprintf("%s: missing required fragment %q", rule.path, req))
			}
		}
		for _, forbidden := range rule.forbiddenText {
			if strings.Contains(text, forbidden) {
				failures = append(failures, fmt.Sprintf("%s: found forbidden text %q", rule.path, forbidden))
			}
		}
		for _, re := range rule.forbiddenRegex {
			if match := re.FindString(text); match != "" {
				failures = append(failures, fmt.Sprintf("%s: found forbidden provider-specific branch %q", rule.path, match))
			}
		}
	}

	if len(failures) > 0 {
		fmt.Println("FAIL: auth provider plugin boundary check failed")
		for _, item := range failures {
			fmt.Printf(" - %s\n", item)
		}
		fmt.Println("Rule: auth-provider core must stay plugin-standard and must not hardcode provider-specific branches in runtime paths.")
		os.Exit(1)
	}

	fmt.Println("OK: auth provider plugin boundary check passed")
}
