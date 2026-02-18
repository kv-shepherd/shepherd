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
	vmHandlerPath               = "internal/api/handlers/server_vm.go"
	deleteUseCaseTestPath       = "internal/usecase/delete_vm_test.go"
	systemBehaviorTestPath      = "internal/api/handlers/server_system_behavior_test.go"
	frontendVMHookTestPath      = "web/src/features/vm-management/hooks/useVMManagementController.test.tsx"
	frontendSystemHookTestPath  = "web/src/features/systems-management/hooks/useSystemsManagementController.test.tsx"
	frontendServiceHookTestPath = "web/src/features/services-management/hooks/useServicesManagementController.test.tsx"
	frontendLiveE2EPath         = "web/tests/e2e/master-flow-live.spec.ts"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkFragments(&violations, systemHandlerPath, []string{
		"func (s *Server) DeleteSystem(",
		"func (s *Server) DeleteService(",
		"SYSTEM_HAS_SERVICES",
		"SERVICE_HAS_VMS",
		"DELETE_CONFIRMATION_REQUIRED",
		"confirm_name query parameter must match system name exactly",
		"confirm=true query parameter is required",
	})
	checkFragments(&violations, vmHandlerPath, []string{
		"func (s *Server) DeleteVM(",
		"input := usecase.DeleteVMInput{",
		"Confirm:     params.Confirm",
		"ConfirmName: params.ConfirmName",
	})
	checkFragments(&violations, deleteUseCaseTestPath, []string{
		"TestValidateDeleteConfirmationByEnvironment",
		"DELETE_CONFIRMATION_REQUIRED",
		"CONFIRMATION_NAME_MISMATCH",
	})
	checkFragments(&violations, systemBehaviorTestPath, []string{
		"TestSystemHandler_DeleteSystem_RequiresConfirmNameMatch",
		"TestSystemHandler_DeleteSystem_ConflictWhenServicesExist",
		"TestSystemHandler_DeleteSystem_Success",
		"TestSystemHandler_DeleteService_RequiresConfirmTrue",
		"TestSystemHandler_DeleteService_ConflictWhenVMsExist",
		"TestSystemHandler_DeleteService_Success",
	})
	checkFragments(&violations, frontendVMHookTestPath, []string{
		"deleteVM('vm-2', 'vm-two')",
		"expect(deleteMutate).toHaveBeenCalledWith({ vmId: 'vm-2', vmName: 'vm-two' });",
	})
	checkFragments(&violations, frontendSystemHookTestPath, []string{
		"setDeleteConfirmName('System A')",
		"expect(deleteMutate).toHaveBeenCalledWith({ id: 'sys-1', confirmName: 'System A' });",
	})
	checkFragments(&violations, frontendServiceHookTestPath, []string{
		"expect(deleteMutate).toHaveBeenCalledWith({ systemId: 'sys-1', serviceId: 'svc-1' });",
	})
	checkFragments(&violations, frontendLiveE2EPath, []string{
		"system delete enforces confirm_name and calls real Stage 5.D API",
		"service delete sends confirm=true and returns conflict when child VMs exist",
		"confirm_name=",
		"SERVICE_HAS_VMS",
	})

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: Stage 5.D delete baseline check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 5.D delete confirmation/cascade semantics must stay enforced in API, runtime, and tests.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 5.D delete baseline check passed")
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
		path      string
		op        string
		id        string
		paramName string
		paramRefs []string
	}{
		{path: "/systems/{system_id}", op: "delete", id: "deleteSystem", paramName: "confirm_name", paramRefs: []string{"ConfirmName"}},
		{path: "/systems/{system_id}/services/{service_id}", op: "delete", id: "deleteService", paramName: "confirm", paramRefs: []string{"Confirm"}},
		{path: "/vms/{vm_id}", op: "delete", id: "deleteVM", paramName: "confirm", paramRefs: []string{"Confirm"}},
		{path: "/vms/{vm_id}", op: "delete", id: "deleteVM", paramName: "confirm_name", paramRefs: []string{"ConfirmName"}},
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
		if !hasParameter(opNode, r.paramName, r.paramRefs) {
			*violations = append(*violations, fmt.Sprintf("%s.%s missing required delete confirmation parameter %q", r.path, r.op, r.paramName))
		}
	}
}

func hasParameter(opNode *yaml.Node, expectedName string, refHints []string) bool {
	params, ok := mapValue(opNode, "parameters")
	if !ok {
		return false
	}
	params = documentRoot(params)
	if params == nil || params.Kind != yaml.SequenceNode {
		return false
	}

	for _, item := range params.Content {
		item = documentRoot(item)
		if item == nil || item.Kind != yaml.MappingNode {
			continue
		}
		if name, ok := scalarValueByKey(item, "name"); ok && strings.TrimSpace(name) == expectedName {
			return true
		}
		if ref, ok := scalarValueByKey(item, "$ref"); ok {
			for _, hint := range refHints {
				if strings.Contains(ref, hint) {
					return true
				}
			}
		}
	}
	return false
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
