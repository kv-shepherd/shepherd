package openapirbaccontract

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCheckDocumentValidSpec(t *testing.T) {
	t.Parallel()

	doc := parseYAML(t, `
openapi: 3.1.0
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
    VNCBootstrapCookie:
      type: apiKey
      in: cookie
      name: kvs_vnc_token
security:
  - BearerAuth: []
paths:
  /healthz:
    get:
      operationId: healthz
      security: []
      responses:
        "200":
          description: ok
      x-rbac:
        authentication: public
        platformAdminOverride: false
  /admin/clusters:
    get:
      operationId: listClusters
      responses:
        "200":
          description: ok
        "401":
          description: unauthenticated
        "403":
          description: forbidden
      x-rbac:
        authentication: bearer
        platformAdminOverride: true
        globalPermissions:
          anyOf:
            - cluster:read
  /vms/{vm_id}/vnc:
    get:
      operationId: createVncSession
      security:
        - VNCBootstrapCookie: []
      responses:
        "200":
          description: ok
        "401":
          description: unauthenticated
      x-rbac:
        authentication: bootstrap_cookie
        platformAdminOverride: false
        bootstrapSession:
          securityScheme: VNCBootstrapCookie
          cookieName: kvs_vnc_token
          bindsToPathParam: vm_id
          requiresVmRunning: true
`)

	if violations := checkDocument(doc); len(violations) != 0 {
		t.Fatalf("expected no violations, got %v", violations)
	}
}

func TestCheckDocumentRejectsMissingXRBAC(t *testing.T) {
	t.Parallel()

	doc := parseYAML(t, `
openapi: 3.1.0
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
    VNCBootstrapCookie:
      type: apiKey
      in: cookie
      name: kvs_vnc_token
security:
  - BearerAuth: []
paths:
  /users/me:
    get:
      operationId: getCurrentUser
      responses:
        "200":
          description: ok
        "401":
          description: unauthenticated
`)

	violations := checkDocument(doc)
	if len(violations) == 0 {
		t.Fatal("expected violations, got none")
	}
	if !containsViolation(violations, "GET /users/me (getCurrentUser) missing x-rbac") {
		t.Fatalf("expected missing x-rbac violation, got %v", violations)
	}
}

func TestCheckDocumentRejectsInvalidConditionalPermissions(t *testing.T) {
	t.Parallel()

	doc := parseYAML(t, `
openapi: 3.1.0
components:
  securitySchemes:
    BearerAuth:
      type: http
      scheme: bearer
    VNCBootstrapCookie:
      type: apiKey
      in: cookie
      name: kvs_vnc_token
security:
  - BearerAuth: []
paths:
  /tickets:
    get:
      operationId: listTickets
      responses:
        "200":
          description: ok
        "401":
          description: unauthenticated
      x-rbac:
        authentication: bearer
        platformAdminOverride: true
        conditions:
          - when: query.mine != true
            globalPermissions:
              anyOf:
                - ticket:view
`)

	violations := checkDocument(doc)
	if len(violations) == 0 {
		t.Fatal("expected violations, got none")
	}
	if !containsViolation(violations, "GET /tickets (listTickets) with globalPermissions must declare 403 response") {
		t.Fatalf("expected missing 403 violation, got %v", violations)
	}
}

func TestRepositorySpecHasNoRBACContractViolations(t *testing.T) {
	t.Parallel()

	doc := loadRepositorySpec(t)
	if violations := checkDocument(doc); len(violations) != 0 {
		t.Fatalf("repository OpenAPI spec has RBAC contract violations: %v", violations)
	}
}

func TestRepositorySpecHighRiskOperationsLockRBACSemantics(t *testing.T) {
	t.Parallel()

	doc := loadRepositorySpec(t)

	tickets := getOperation(t, doc, "/tickets", "get")
	ticketsRBAC := getMap(tickets, "x-rbac")
	ticketConditions := getSequence(ticketsRBAC, "conditions")
	if len(ticketConditions) != 2 {
		t.Fatalf("ticket conditions len = %d, want 2", len(ticketConditions))
	}
	firstCondition := asMap(t, ticketConditions[0], "tickets.conditions[0]")
	if firstCondition["when"] != "query.mine != true" {
		t.Fatalf("tickets first condition when = %#v, want %q", firstCondition["when"], "query.mine != true")
	}
	assertAnyOfEquals(
		t,
		getMap(firstCondition, "globalPermissions"),
		"anyOf",
		[]string{"ticket:view"},
	)
	secondCondition := asMap(t, ticketConditions[1], "tickets.conditions[1]")
	if secondCondition["when"] != "query.mine == true" {
		t.Fatalf("tickets second condition when = %#v, want %q", secondCondition["when"], "query.mine == true")
	}
	assertScopeTypesEqual(
		t,
		getSequence(secondCondition, "scopeFilters"),
		[]string{"ticket_requester_is_actor"},
	)

	authProviders := getOperation(t, doc, "/admin/auth-providers", "get")
	authProvidersRBAC := getMap(authProviders, "x-rbac")
	fieldVisibility := getSequence(authProvidersRBAC, "fieldVisibility")
	if len(fieldVisibility) != 1 {
		t.Fatalf("auth provider fieldVisibility len = %d, want 1", len(fieldVisibility))
	}
	authProviderFieldVisibility := asMap(t, fieldVisibility[0], "authProviders.fieldVisibility[0]")
	assertAnyOfEquals(
		t,
		authProviderFieldVisibility,
		"fields",
		[]string{"items[].config"},
	)
	assertAnyOfEquals(
		t,
		getMap(authProviderFieldVisibility, "globalPermissions"),
		"anyOf",
		[]string{"auth_provider:configure", "auth_provider:update"},
	)

	for _, operationPath := range []string{"/vms/{vm_id}/vnc", "/vms/{vm_id}/serial"} {
		operation := getOperation(t, doc, operationPath, "get")
		xrbac := getMap(operation, "x-rbac")
		if got := xrbac["authentication"]; got != "bootstrap_cookie" {
			t.Fatalf("%s authentication = %#v, want %q", operationPath, got, "bootstrap_cookie")
		}
		if got := xrbac["platformAdminOverride"]; got != false {
			t.Fatalf("%s platformAdminOverride = %#v, want false", operationPath, got)
		}
		bootstrap := getMap(xrbac, "bootstrapSession")
		if got := bootstrap["securityScheme"]; got != "VNCBootstrapCookie" {
			t.Fatalf("%s bootstrap securityScheme = %#v, want %q", operationPath, got, "VNCBootstrapCookie")
		}
		if got := bootstrap["cookieName"]; got != "vnc_bootstrap" {
			t.Fatalf("%s bootstrap cookieName = %#v, want %q", operationPath, got, "vnc_bootstrap")
		}
		if got := bootstrap["bindsToPathParam"]; got != "vm_id" {
			t.Fatalf("%s bootstrap bindsToPathParam = %#v, want %q", operationPath, got, "vm_id")
		}
		if got := bootstrap["requiresVmRunning"]; got != true {
			t.Fatalf("%s bootstrap requiresVmRunning = %#v, want true", operationPath, got)
		}
	}

	for _, method := range []string{"put", "delete"} {
		operation := getOperation(t, doc, "/auth/preferences/{key}", method)
		xrbac := getMap(operation, "x-rbac")
		scopeFilters := getSequence(xrbac, "scopeFilters")
		assertScopeTypesEqual(t, scopeFilters, []string{"current_user_preferences"})
		conditions := getSequence(xrbac, "conditions")
		if len(conditions) != 1 {
			t.Fatalf("auth preferences %s conditions len = %d, want 1", method, len(conditions))
		}
		condition := asMap(t, conditions[0], "authPreferences.conditions[0]")
		if got := condition["when"]; got != "path.key == admin.users.columns.v4" {
			t.Fatalf("auth preferences %s when = %#v, want %q", method, got, "path.key == admin.users.columns.v4")
		}
		assertAnyOfEquals(
			t,
			getMap(condition, "globalPermissions"),
			"anyOf",
			[]string{"user:manage", "rbac:manage"},
		)
	}
}

func parseYAML(t *testing.T, raw string) map[string]any {
	t.Helper()

	var doc map[string]any
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}
	return doc
}

func containsViolation(violations []string, want string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return true
		}
	}
	return false
}

func loadRepositorySpec(t *testing.T) map[string]any {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	doc, err := loadSpec(filepath.Join(repoRoot, "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load repository spec: %v", err)
	}
	return doc
}

func getOperation(t *testing.T, doc map[string]any, path, method string) map[string]any {
	t.Helper()

	paths := getMap(doc, "paths")
	pathItem := asMap(t, paths[path], "paths."+path)
	return asMap(t, pathItem[method], "paths."+path+"."+method)
}

func getSequence(parent map[string]any, key string) []any {
	if parent == nil {
		return nil
	}
	if value, ok := parent[key].([]any); ok {
		return value
	}
	return nil
}

func asMap(t *testing.T, raw any, label string) map[string]any {
	t.Helper()

	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("%s is not a map: %#v", label, raw)
	}
	return value
}

func assertAnyOfEquals(t *testing.T, parent map[string]any, key string, want []string) {
	t.Helper()

	items := getSequence(parent, key)
	if len(items) != len(want) {
		t.Fatalf("%s len = %d, want %d", key, len(items), len(want))
	}
	for index, wantValue := range want {
		got, ok := items[index].(string)
		if !ok {
			t.Fatalf("%s[%d] is not a string: %#v", key, index, items[index])
		}
		if got != wantValue {
			t.Fatalf("%s[%d] = %q, want %q", key, index, got, wantValue)
		}
	}
}

func assertScopeTypesEqual(t *testing.T, filters []any, want []string) {
	t.Helper()

	if len(filters) != len(want) {
		t.Fatalf("scopeFilters len = %d, want %d", len(filters), len(want))
	}
	for index, wantType := range want {
		filter := asMap(t, filters[index], "scopeFilters")
		got, ok := filter["type"].(string)
		if !ok {
			t.Fatalf("scopeFilters[%d].type is not a string: %#v", index, filter["type"])
		}
		if got != wantType {
			t.Fatalf("scopeFilters[%d].type = %q, want %q", index, got, wantType)
		}
	}
}
