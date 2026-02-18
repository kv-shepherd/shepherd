//go:build ignore

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath                      = "api/openapi.yaml"
	adminCatalogHandlerPath          = "internal/api/handlers/server_admin_catalog.go"
	adminCatalogBehaviorTestPath     = "internal/api/handlers/server_admin_catalog_test.go"
	adminPermissionTestPath          = "internal/api/handlers/permission_enforcement_test.go"
	frontendTemplateHookTestPath     = "web/src/features/admin-templates/hooks/useAdminTemplatesController.test.tsx"
	frontendInstanceSizeHookTestPath = "web/src/features/admin-instance-sizes/hooks/useAdminInstanceSizesController.test.tsx"
	frontendLiveE2EPath              = "web/tests/e2e/master-flow-live.spec.ts"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkFragments(&violations, adminCatalogHandlerPath, []string{
		"func (s *Server) ListAdminTemplates(",
		`requireActorWithAnyGlobalPermission(c, "template:read", "template:manage")`,
		"func (s *Server) CreateAdminTemplate(",
		`requireActorWithAnyGlobalPermission(c, "template:write", "template:manage")`,
		"func (s *Server) ListAdminInstanceSizes(",
		`requireActorWithAnyGlobalPermission(c, "instance_size:read")`,
		"func (s *Server) CreateAdminInstanceSize(",
		`requireActorWithAnyGlobalPermission(c, "instance_size:write")`,
	})
	checkFragments(&violations, adminCatalogBehaviorTestPath, []string{
		"TestAdminTemplateCRUD",
		"TestAdminInstanceSizeCRUD",
	})
	checkFragments(&violations, adminPermissionTestPath, []string{
		"TestPermissionEnforcement_ListAdminTemplates_RequiresTemplateRead",
		"TestPermissionEnforcement_CreateAdminTemplate_RequiresTemplateWrite",
		"TestPermissionEnforcement_ListAdminInstanceSizes_RequiresInstanceSizeRead",
		"TestPermissionEnforcement_CreateAdminInstanceSize_RequiresInstanceSizeWrite",
	})
	checkFragments(&violations, frontendTemplateHookTestPath, []string{
		"useAdminTemplatesController",
		"submits create payload with parsed spec JSON",
		"rejects invalid create spec JSON and does not mutate",
		"templates.spec_invalid",
	})
	checkFragments(&violations, frontendInstanceSizeHookTestPath, []string{
		"useAdminInstanceSizesController",
		"submits create payload with parsed spec_overrides JSON",
		"rejects invalid spec_overrides JSON and does not mutate",
		"instanceSizes.spec_overrides_invalid",
	})
	checkFragments(&violations, frontendLiveE2EPath, []string{
		"admin template flow performs create/delete against real Stage 3 API",
		"admin instance-size flow performs create/delete against real Stage 3 API",
		"/api/v1/admin/templates",
		"/api/v1/admin/instance-sizes",
	})

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: Stage 3 admin catalog baseline check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 3 admin catalog (template/instance-size) CRUD and RBAC gates must remain implemented across API/runtime/tests.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 3 admin catalog baseline check passed")
}

func checkOpenAPI(violations *[]string) {
	specBytes, err := os.ReadFile(openAPIPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", openAPIPath, err))
		return
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(specBytes, &doc); err != nil {
		*violations = append(*violations, fmt.Sprintf("parse %s: %v", openAPIPath, err))
		return
	}

	root := documentRoot(&doc)
	paths, ok := mapValue(root, "paths")
	if !ok {
		*violations = append(*violations, "missing root.paths")
		return
	}

	required := []struct {
		path string
		op   string
		id   string
	}{
		{path: "/admin/templates", op: "get", id: "listAdminTemplates"},
		{path: "/admin/templates", op: "post", id: "createAdminTemplate"},
		{path: "/admin/templates/{template_id}", op: "patch", id: "updateAdminTemplate"},
		{path: "/admin/templates/{template_id}", op: "delete", id: "deleteAdminTemplate"},
		{path: "/admin/instance-sizes", op: "get", id: "listAdminInstanceSizes"},
		{path: "/admin/instance-sizes", op: "post", id: "createAdminInstanceSize"},
		{path: "/admin/instance-sizes/{instance_size_id}", op: "patch", id: "updateAdminInstanceSize"},
		{path: "/admin/instance-sizes/{instance_size_id}", op: "delete", id: "deleteAdminInstanceSize"},
	}

	for _, r := range required {
		pathNode, ok := mapValue(paths, r.path)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("missing OpenAPI path %s", r.path))
			continue
		}
		opNode, ok := mapValue(pathNode, r.op)
		if !ok {
			*violations = append(*violations, fmt.Sprintf("missing OpenAPI operation %s.%s", r.path, r.op))
			continue
		}
		id, ok := scalarValueByKey(opNode, "operationId")
		if !ok || id != r.id {
			*violations = append(*violations, fmt.Sprintf("%s.%s operationId must be %s", r.path, r.op, r.id))
		}
	}
}

func checkFragments(violations *[]string, path string, needles []string) {
	content, err := os.ReadFile(path)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", path, err))
		return
	}

	text := string(content)
	for _, n := range needles {
		if !strings.Contains(text, n) {
			*violations = append(*violations, fmt.Sprintf("%s missing fragment %q", path, n))
		}
	}
}

func documentRoot(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	node = documentRoot(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i]
		v := node.Content[i+1]
		if strings.TrimSpace(k.Value) == key {
			return v, true
		}
	}
	return nil, false
}

func scalarValueByKey(node *yaml.Node, key string) (string, bool) {
	valNode, ok := mapValue(node, key)
	if !ok {
		return "", false
	}
	valNode = documentRoot(valNode)
	if valNode == nil || valNode.Kind != yaml.ScalarNode {
		return "", false
	}
	return strings.TrimSpace(valNode.Value), true
}
