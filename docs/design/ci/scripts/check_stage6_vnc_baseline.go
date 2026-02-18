//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath      = "api/openapi.yaml"
	handlerPath      = "internal/api/handlers/server_vm_console.go"
	gatewayPath      = "internal/governance/approval/gateway.go"
	tokenServicePath = "internal/service/vnc_token.go"
	serverPath       = "internal/api/handlers/server.go"
	allowlistPath    = "docs/design/ci/allowlists/master_flow_api_deferred.txt"
	handlerTestPath  = "internal/api/handlers/server_vm_console_behavior_test.go"
	tokenTestPath    = "internal/service/vnc_token_test.go"
	frontendTestPath = "web/src/features/vm-management/hooks/useVMManagementController.test.tsx"
)

func main() {
	var violations []string

	checkOpenAPI(&violations)
	checkFragments(&violations, handlerPath, []string{
		"func (s *Server) RequestVMConsoleAccess(",
		"func (s *Server) GetVMConsoleStatus(",
		"func (s *Server) OpenVMVNC(",
		"service.EvaluateVNCRequest(",
		"s.vncTokens.ValidateAndConsume(",
		"createVNCApprovalRequest(",
		"hasPendingVNCRequest(",
		"vncBootstrapCookieName",
		"setVNCBootstrapCookie(",
		"clearVNCBootstrapCookie(",
		"c.Cookie(vncBootstrapCookieName)",
	})
	checkNoLegacyQueryToken(&violations, handlerPath)
	checkFragments(&violations, serverPath, []string{
		"service.NewPostgresVNCReplayStore(",
		"service.NewVNCTokenManager(",
	})
	checkFragments(&violations, tokenServicePath, []string{
		"type VNCTokenManager struct",
		"type PostgresVNCReplayStore struct",
		"func NewPostgresVNCReplayStore(",
		"func (m *VNCTokenManager) Issue(",
		"func (m *VNCTokenManager) ValidateAndConsume(",
		"ErrVNCTokenReplayed",
		"SingleUse",
	})
	checkFragments(&violations, gatewayPath, []string{
		"case approvalticket.OperationTypeVNC_ACCESS:",
		"approveVNC(",
		"EventVNCAccessRequested",
	})
	checkFragments(&violations, handlerTestPath, []string{
		"TestVMConsole_Request_TestEnvironmentIssuesDirectVNCURL",
		"TestVMConsole_Request_ProductionCreatesPendingApprovalTicket",
		"TestVMConsole_OpenVNC_RejectsTokenReplay",
	})
	checkFragments(&violations, tokenTestPath, []string{
		"TestVNCTokenManager_IssueAndValidateSingleUse",
		"TestVNCTokenManager_ValidateRejectsVMMismatch",
		"TestPostgresVNCReplayStore_ConsumeSingleUseAcrossInstances",
		"TestVNCTokenManager_ValidateAndConsume_UsesPostgresReplayStore",
	})
	checkFragments(&violations, frontendTestPath, []string{
		"requestConsole",
		"requestConsoleMutate",
	})
	checkAllowlist(&violations)

	if len(violations) > 0 {
		fmt.Println("FAIL: Stage 6 VNC baseline check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: Stage 6 VNC API, runtime baseline, and behavior tests must remain implemented once introduced.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 6 VNC baseline check passed")
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
		{path: "/vms/{vm_id}/console/request", op: "post", id: "requestVMConsoleAccess"},
		{path: "/vms/{vm_id}/console/status", op: "get", id: "getVMConsoleStatus"},
		{path: "/vms/{vm_id}/vnc", op: "get", id: "openVMVNC"},
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
		if r.path == "/vms/{vm_id}/vnc" && r.op == "get" {
			if paramsNode, ok := mapValue(opNode, "parameters"); ok {
				if hasLegacyVNCQueryToken(paramsNode) {
					*violations = append(*violations, "/vms/{vm_id}/vnc.get must not expose legacy query token parameter")
				}
			}
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

func checkAllowlist(violations *[]string) {
	content, err := os.ReadFile(allowlistPath)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", allowlistPath, err))
		return
	}

	lines := parseAllowlistLines(string(content))
	blocked := []string{
		"/vms/{}/console/request",
		"/vms/{}/console/status",
		"/vms/{}/vnc",
	}
	for _, b := range blocked {
		if _, ok := lines[b]; ok {
			*violations = append(*violations, fmt.Sprintf("allowlist must not contain implemented path %s", b))
		}
	}
}

func checkNoLegacyQueryToken(violations *[]string, path string) {
	content, err := os.ReadFile(path)
	if err != nil {
		*violations = append(*violations, fmt.Sprintf("read %s: %v", path, err))
		return
	}
	text := string(content)

	blocked := []string{
		"/vnc?token=",
		"Query(\"token\")",
		"OpenVMVNCParams",
	}
	for _, needle := range blocked {
		if strings.Contains(text, needle) {
			*violations = append(*violations, fmt.Sprintf("%s contains forbidden legacy token transport fragment %q", path, needle))
		}
	}
}

func hasLegacyVNCQueryToken(node *yaml.Node) bool {
	node = documentRoot(node)
	if node == nil {
		return false
	}

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			k := strings.TrimSpace(node.Content[i].Value)
			v := node.Content[i+1]
			if k == "$ref" && strings.Contains(strings.TrimSpace(v.Value), "VNCToken") {
				return true
			}
			if k == "name" && strings.TrimSpace(v.Value) == "token" {
				return true
			}
			if hasLegacyVNCQueryToken(v) {
				return true
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if hasLegacyVNCQueryToken(child) {
				return true
			}
		}
	}
	return false
}

func parseAllowlistLines(content string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		out[line] = struct{}{}
	}
	return out
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
