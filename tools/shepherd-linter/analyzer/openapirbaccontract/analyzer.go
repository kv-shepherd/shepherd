// Package openapirbaccontract implements a go/analysis Analyzer that enforces
// explicit RBAC semantics on every OpenAPI operation.
package openapirbaccontract

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
	"gopkg.in/yaml.v3"
)

const (
	anchorPackagePath = "kv-shepherd.io/shepherd/internal/api/handlers"
	openAPISpecPath   = "api/openapi.yaml"
)

var httpMethods = map[string]struct{}{
	"get":    {},
	"post":   {},
	"put":    {},
	"patch":  {},
	"delete": {},
}

var validAuthModes = map[string]struct{}{
	"public":           {},
	"bearer":           {},
	"bootstrap_cookie": {},
}

var validDeniedModes = map[string]struct{}{
	"forbidden": {},
	"not_found": {},
}

// Analyzer is the exported analyzer for OpenAPI RBAC contract validation.
var Analyzer = &analysis.Analyzer{
	Name: "openapirbaccontract",
	Doc:  "Enforces explicit OpenAPI x-rbac semantics, aligned auth schemes, and 401/403 responses",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	if pass.Pkg == nil || pass.Pkg.Path() != anchorPackagePath || len(pass.Files) == 0 {
		return nil, nil
	}

	anchorPos := pass.Files[0].Package
	anchorFile := pass.Fset.File(anchorPos)
	if anchorFile == nil {
		pass.Reportf(anchorPos, "openapi RBAC contract: unable to resolve package filename")
		return nil, nil
	}

	repoRoot, err := findRepoRoot(filepath.Dir(anchorFile.Name()))
	if err != nil {
		pass.Reportf(anchorPos, "openapi RBAC contract: %v", err)
		return nil, nil
	}

	doc, err := loadSpec(filepath.Join(repoRoot, openAPISpecPath))
	if err != nil {
		pass.Reportf(anchorPos, "openapi RBAC contract: %v", err)
		return nil, nil
	}

	violations := checkDocument(doc)
	for _, violation := range violations {
		pass.Reportf(anchorPos, "openapi RBAC contract: %s", violation)
	}

	return nil, nil
}

func findRepoRoot(startDir string) (string, error) {
	current := startDir
	for {
		candidate := filepath.Join(current, openAPISpecPath)
		if _, err := os.Stat(candidate); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot find %s from %s", openAPISpecPath, startDir)
		}
		current = parent
	}
}

func loadSpec(specPath string) (map[string]any, error) {
	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", specPath, err)
	}

	var doc map[string]any
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", specPath, err)
	}
	return doc, nil
}

func checkDocument(doc map[string]any) []string {
	var violations []string

	components := getMap(doc, "components")
	securitySchemes := getMap(components, "securitySchemes")
	if _, ok := securitySchemes["BearerAuth"]; !ok {
		violations = append(violations, "components.securitySchemes.BearerAuth is required")
	}
	if _, ok := securitySchemes["VNCBootstrapCookie"]; !ok {
		violations = append(violations, "components.securitySchemes.VNCBootstrapCookie is required")
	}

	rootSecurity := doc["security"]
	paths := getMap(doc, "paths")
	for path, rawPath := range paths {
		pathItem, ok := rawPath.(map[string]any)
		if !ok {
			violations = append(violations, fmt.Sprintf("paths.%s must be a mapping", path))
			continue
		}
		for method, rawOperation := range pathItem {
			if _, tracked := httpMethods[strings.ToLower(method)]; !tracked {
				continue
			}
			operation, ok := rawOperation.(map[string]any)
			if !ok {
				violations = append(violations, fmt.Sprintf("paths.%s.%s must be a mapping", path, method))
				continue
			}

			operationID, _ := operation["operationId"].(string)
			location := fmt.Sprintf("%s %s (%s)", strings.ToUpper(method), path, operationID)

			if _, legacy := operation["x-required-permissions"]; legacy {
				violations = append(violations, fmt.Sprintf("%s still uses legacy x-required-permissions", location))
			}

			xrbac, ok := operation["x-rbac"].(map[string]any)
			if !ok {
				violations = append(violations, fmt.Sprintf("%s missing x-rbac", location))
				continue
			}

			authMode, _ := xrbac["authentication"].(string)
			if _, ok := validAuthModes[authMode]; !ok {
				violations = append(violations, fmt.Sprintf("%s has invalid x-rbac.authentication %q", location, authMode))
			}

			if _, ok := xrbac["platformAdminOverride"].(bool); !ok {
				violations = append(violations, fmt.Sprintf("%s missing boolean x-rbac.platformAdminOverride", location))
			}

			validateGlobalPermissions(location, "x-rbac.globalPermissions", xrbac["globalPermissions"], &violations)
			validateConditions(location, xrbac["conditions"], &violations)
			validateResourceRoles(location, xrbac["resourceRoles"], &violations)
			validateScopeFilters(location, "x-rbac.scopeFilters", xrbac["scopeFilters"], &violations)
			validateFieldVisibility(location, xrbac["fieldVisibility"], &violations)
			validateBootstrapSession(location, authMode, xrbac["bootstrapSession"], &violations)

			responses := getMap(operation, "responses")

			switch authMode {
			case "public":
				if !isExplicitEmptySecurity(operation["security"]) {
					violations = append(violations, fmt.Sprintf("%s public operation must declare security: []", location))
				}
			case "bearer":
				if !securityIncludesScheme(operation["security"], "BearerAuth") && !securityIncludesScheme(rootSecurity, "BearerAuth") {
					violations = append(violations, fmt.Sprintf("%s bearer operation must inherit or declare BearerAuth", location))
				}
				if _, ok := responses["401"]; !ok {
					violations = append(violations, fmt.Sprintf("%s bearer operation missing 401 response", location))
				}
			case "bootstrap_cookie":
				if !securityIncludesScheme(operation["security"], "VNCBootstrapCookie") {
					violations = append(violations, fmt.Sprintf("%s bootstrap-cookie operation must declare VNCBootstrapCookie security", location))
				}
				if _, ok := responses["401"]; !ok {
					violations = append(violations, fmt.Sprintf("%s bootstrap-cookie operation missing 401 response", location))
				}
			}

			if hasGlobalPermissions(xrbac) {
				if _, ok := responses["403"]; !ok {
					violations = append(violations, fmt.Sprintf("%s with globalPermissions must declare 403 response", location))
				}
			}
		}
	}

	sort.Strings(violations)
	return violations
}

func getMap(parent map[string]any, key string) map[string]any {
	if parent == nil {
		return map[string]any{}
	}
	if value, ok := parent[key].(map[string]any); ok {
		return value
	}
	return map[string]any{}
}

func validateGlobalPermissions(location, field string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("%s %s must be a mapping", location, field))
		return
	}
	validateAnyOf(location, field+".anyOf", entry["anyOf"], violations)
}

func validateConditions(location string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.conditions must be a non-empty sequence when present", location))
		return
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.conditions[%d] must be a mapping", location, index))
			continue
		}
		if when, ok := item["when"].(string); !ok || when == "" {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.conditions[%d].when must be a non-empty string", location, index))
		}
		validateGlobalPermissions(location, fmt.Sprintf("x-rbac.conditions[%d].globalPermissions", index), item["globalPermissions"], violations)
		validateScopeFilters(location, fmt.Sprintf("x-rbac.conditions[%d].scopeFilters", index), item["scopeFilters"], violations)
		validateResourceRoles(location, item["resourceRoles"], violations)
	}
}

func validateResourceRoles(location string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles must be a non-empty sequence when present", location))
		return
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles[%d] must be a mapping", location, index))
			continue
		}
		if resourceType, ok := item["resourceType"].(string); !ok || resourceType == "" {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles[%d].resourceType must be a non-empty string", location, index))
		}
		if action, ok := item["action"].(string); !ok || action == "" {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles[%d].action must be a non-empty string", location, index))
		}
		_, hasParam := item["resourceIdParam"].(string)
		_, hasBodyField := item["resourceIdBodyField"].(string)
		if !hasParam && !hasBodyField {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles[%d] must declare resourceIdParam or resourceIdBodyField", location, index))
		}
		if denied, ok := item["onDenied"].(string); ok {
			if _, valid := validDeniedModes[denied]; !valid {
				*violations = append(*violations, fmt.Sprintf("%s x-rbac.resourceRoles[%d].onDenied must be one of forbidden/not_found", location, index))
			}
		}
	}
}

func validateScopeFilters(location, field string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		*violations = append(*violations, fmt.Sprintf("%s %s must be a non-empty sequence when present", location, field))
		return
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("%s %s[%d] must be a mapping", location, field, index))
			continue
		}
		if scopeType, ok := item["type"].(string); !ok || scopeType == "" {
			*violations = append(*violations, fmt.Sprintf("%s %s[%d].type must be a non-empty string", location, field, index))
		}
	}
}

func validateFieldVisibility(location string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.fieldVisibility must be a non-empty sequence when present", location))
		return
	}
	for index, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.fieldVisibility[%d] must be a mapping", location, index))
			continue
		}
		validateAnyOf(location, fmt.Sprintf("x-rbac.fieldVisibility[%d].fields", index), item["fields"], violations)
		validateGlobalPermissions(location, fmt.Sprintf("x-rbac.fieldVisibility[%d].globalPermissions", index), item["globalPermissions"], violations)
		validateScopeFilters(location, fmt.Sprintf("x-rbac.fieldVisibility[%d].scopeFilters", index), item["scopeFilters"], violations)
	}
}

func validateBootstrapSession(location, authMode string, raw any, violations *[]string) {
	if raw == nil {
		return
	}
	if authMode != "bootstrap_cookie" {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.bootstrapSession is only valid for bootstrap_cookie operations", location))
		return
	}
	entry, ok := raw.(map[string]any)
	if !ok {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.bootstrapSession must be a mapping", location))
		return
	}
	requiredStrings := []string{"securityScheme", "cookieName", "bindsToPathParam"}
	for _, key := range requiredStrings {
		if value, ok := entry[key].(string); !ok || value == "" {
			*violations = append(*violations, fmt.Sprintf("%s x-rbac.bootstrapSession.%s must be a non-empty string", location, key))
		}
	}
	if _, ok := entry["requiresVmRunning"].(bool); !ok {
		*violations = append(*violations, fmt.Sprintf("%s x-rbac.bootstrapSession.requiresVmRunning must be boolean", location))
	}
}

func validateAnyOf(location, field string, raw any, violations *[]string) {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		*violations = append(*violations, fmt.Sprintf("%s %s must be a non-empty sequence", location, field))
		return
	}
	for index, rawItem := range items {
		if value, ok := rawItem.(string); !ok || value == "" {
			*violations = append(*violations, fmt.Sprintf("%s %s[%d] must be a non-empty string", location, field, index))
		}
	}
}

func securityIncludesScheme(raw any, scheme string) bool {
	items, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := item[scheme]; ok {
			return true
		}
	}
	return false
}

func isExplicitEmptySecurity(raw any) bool {
	items, ok := raw.([]any)
	return ok && len(items) == 0
}

func hasGlobalPermissions(xrbac map[string]any) bool {
	if anyOf := getMap(xrbac, "globalPermissions")["anyOf"]; anyOf != nil {
		if items, ok := anyOf.([]any); ok && len(items) > 0 {
			return true
		}
	}
	if conditions, ok := xrbac["conditions"].([]any); ok {
		for _, rawCondition := range conditions {
			condition, ok := rawCondition.(map[string]any)
			if !ok {
				continue
			}
			if anyOf := getMap(condition, "globalPermissions")["anyOf"]; anyOf != nil {
				if items, ok := anyOf.([]any); ok && len(items) > 0 {
					return true
				}
			}
		}
	}
	return false
}
