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
	openAPIPath                 = "api/openapi.yaml"
	systemHandlerPath           = "internal/api/handlers/server_system.go"
	memberHandlerPath           = "internal/api/handlers/member.go"
	systemStageTestPath         = "internal/api/handlers/server_system_stage_test.go"
	systemBehaviorTestPath      = "internal/api/handlers/server_system_behavior_test.go"
	permissionTestPath          = "internal/api/handlers/permission_enforcement_test.go"
	frontendSystemHookTestPath  = "web/src/features/systems-management/hooks/useSystemsManagementController.test.tsx"
	frontendServiceHookTestPath = "web/src/features/services-management/hooks/useServicesManagementController.test.tsx"
	frontendMemberHookTestPath  = "web/src/features/systems-management/hooks/useSystemMembersController.test.tsx"
	frontendLiveE2EPath         = "web/tests/e2e/master-flow-live.spec.ts"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkFragments(&violations, systemHandlerPath, []string{
		"func (s *Server) ListSystems(",
		"func (s *Server) CreateSystem(",
		"func (s *Server) UpdateSystem(",
		"func (s *Server) ListServices(",
		"func (s *Server) CreateService(",
		"func (s *Server) UpdateService(",
		`requireGlobalPermission(c, "system:read")`,
		`requireGlobalPermission(c, "system:write")`,
		`requireGlobalPermission(c, "service:read")`,
		`requireGlobalPermission(c, "service:create")`,
		`requireGlobalPermission(c, "service:delete")`,
	})
	checkFragments(&violations, memberHandlerPath, []string{
		"func (s *Server) ListSystemMembers(",
		"func (s *Server) AddSystemMember(",
		"func (s *Server) UpdateSystemMemberRole(",
		"func (s *Server) DeleteSystemMember(",
		`requireGlobalPermission(c, "rbac:manage")`,
	})
	checkFragments(&violations, systemStageTestPath, []string{
		"TestStage4A_SystemCreationOwnerBindingContract",
		"TestStage4Hierarchy_AccessGuardsContract",
		"TestStage4C_UpdateDescriptionOnlyContract",
	})
	checkFragments(&violations, systemBehaviorTestPath, []string{
		"TestSystemHandler_ListSystems_RespectsResourceBindings",
		"TestSystemHandler_UpdateSystem_DescriptionOnly",
		"TestSystemHandler_UpdateService_DescriptionOnly",
	})
	checkFragments(&violations, permissionTestPath, []string{
		"TestPermissionEnforcement_ListServices_RequiresServiceRead",
		"TestPermissionEnforcement_CreateService_RequiresServiceCreate",
		"TestPermissionEnforcement_UpdateService_RequiresServiceCreate",
	})
	checkFragments(&violations, frontendSystemHookTestPath, []string{
		"useSystemsManagementController",
		"submits create and delete operations with expected payload",
		"submits description-only edit with selected system id",
	})
	checkFragments(&violations, frontendServiceHookTestPath, []string{
		"useServicesManagementController",
		"submits create request with split system/body payload",
		"submits update and delete operations for selected service",
	})
	checkFragments(&violations, frontendMemberHookTestPath, []string{
		"useSystemMembersController",
		"submits add-member payload and closes modal state",
		"dispatches remove/update role operations with user identity",
	})
	checkFragments(&violations, frontendLiveE2EPath, []string{
		"system/service create-update-delete follows Stage 4 + Stage 5.D success paths",
		"system delete enforces confirm_name and calls real Stage 5.D API",
		"service delete sends confirm=true and returns conflict when child VMs exist",
	})

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: Stage 4 system/service baseline check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 4 hierarchy + member + service behaviors must remain implemented across API/runtime/tests.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 4 system/service baseline check passed")
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
		{path: "/systems", op: "get", id: "listSystems"},
		{path: "/systems", op: "post", id: "createSystem"},
		{path: "/systems/{system_id}", op: "patch", id: "updateSystem"},
		{path: "/systems/{system_id}/members", op: "get", id: "listSystemMembers"},
		{path: "/systems/{system_id}/members", op: "post", id: "addSystemMember"},
		{path: "/systems/{system_id}/members/{user_id}", op: "patch", id: "updateSystemMemberRole"},
		{path: "/systems/{system_id}/members/{user_id}", op: "delete", id: "deleteSystemMember"},
		{path: "/systems/{system_id}/services", op: "get", id: "listServices"},
		{path: "/systems/{system_id}/services", op: "post", id: "createService"},
		{path: "/systems/{system_id}/services/{service_id}", op: "get", id: "getService"},
		{path: "/systems/{system_id}/services/{service_id}", op: "patch", id: "updateService"},
		{path: "/systems/{system_id}/services/{service_id}", op: "delete", id: "deleteService"},
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
