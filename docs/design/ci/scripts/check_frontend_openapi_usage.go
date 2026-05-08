//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath                            = "api/openapi.yaml"
	frontendSrcDir                         = "web/src"
	allowlistPath                          = "docs/design/ci/allowlists/frontend_openapi_unused.txt"
	frontendASTScannerScript               = "docs/design/ci/scripts/frontend_api_ast_scan.mjs"
	frontendConsumptionExtension           = "x-frontend-consumption"
	frontendConsumptionModeRestClient      = "rest_client"
	frontendConsumptionModeBrowserSession  = "browser_session"
	frontendConsumptionModeBrowserCallback = "browser_callback"
	frontendConsumptionModeServerToServer  = "server_to_server"
)

var supportedMethods = []string{"get", "post", "put", "patch", "delete"}

var supportedFrontendConsumptionModes = map[string]struct{}{
	frontendConsumptionModeRestClient:      {},
	frontendConsumptionModeBrowserSession:  {},
	frontendConsumptionModeBrowserCallback: {},
	frontendConsumptionModeServerToServer:  {},
}

type operation struct {
	Method                  string
	Path                    string
	FrontendConsumptionMode string
	FrontendReason          string
}

type frontendASTScanResult struct {
	UsedOperations             []string `json:"usedOperations"`
	SystemDeleteHasConfirmName bool     `json:"systemDeleteHasConfirmName"`
	SystemDeleteSourceFile     string   `json:"systemDeleteSourceFile"`
}

func (o operation) key() string {
	return o.Method + " " + o.Path
}

func (o operation) requiresFrontendRESTClientCaller() bool {
	return o.FrontendConsumptionMode == "" || o.FrontendConsumptionMode == frontendConsumptionModeRestClient
}

func main() {
	ops, err := collectOpenAPIOperations(openAPIPath)
	if err != nil {
		fmt.Printf("FAIL: collect OpenAPI operations: %v\n", err)
		os.Exit(1)
	}

	scan, err := runFrontendASTScan(frontendSrcDir)
	if err != nil {
		fmt.Printf("FAIL: frontend AST usage scan: %v\n", err)
		os.Exit(1)
	}

	usage := usageIndex(ops, scan.UsedOperations)

	allowlist, err := loadAllowlist(allowlistPath)
	if err != nil {
		fmt.Printf("FAIL: load allowlist: %v\n", err)
		os.Exit(1)
	}

	var violations []string
	opIndex := make(map[string]operation, len(ops))
	usedCount := 0
	nonRESTClientCount := 0

	for _, op := range ops {
		key := op.key()
		opIndex[key] = op
		if !op.requiresFrontendRESTClientCaller() {
			nonRESTClientCount++
			continue
		}
		if usage[key] {
			usedCount++
			continue
		}
		if _, ok := allowlist[key]; ok {
			continue
		}
		violations = append(violations, fmt.Sprintf("missing frontend caller for %s", key))
	}

	for key := range allowlist {
		if _, ok := opIndex[key]; !ok {
			violations = append(violations, fmt.Sprintf("stale allowlist entry (operation not in OpenAPI): %s", key))
			continue
		}
		if usage[key] {
			violations = append(violations, fmt.Sprintf("stale allowlist entry (already used in frontend): %s", key))
			continue
		}
		if !opIndex[key].requiresFrontendRESTClientCaller() {
			violations = append(
				violations,
				fmt.Sprintf(
					"stale allowlist entry (operation declares %s.mode=%s): %s",
					frontendConsumptionExtension,
					opIndex[key].FrontendConsumptionMode,
					key,
				),
			)
		}
	}

	if !scan.SystemDeleteHasConfirmName {
		violations = append(
			violations,
			"system delete UI is missing params.query.confirm_name on DELETE /systems/{system_id} (ADR-0015 §13)",
		)
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: frontend/OpenAPI usage check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Printf(
			"Rule: each REST-client OpenAPI operation must be consumed by frontend or be explicitly deferred in allowlist; non-REST-client operations must declare %s with supported mode and reason.\n",
			frontendConsumptionExtension,
		)
		fmt.Println("Rule: system delete UI must send confirm_name query parameter (ADR-0015 §13).")
		os.Exit(1)
	}

	fmt.Printf(
		"OK: frontend/OpenAPI usage check passed (operations=%d used=%d nonRESTClient=%d allowlisted=%d systemDelete=%s)\n",
		len(ops),
		usedCount,
		nonRESTClientCount,
		len(allowlist),
		confirmSource(scan.SystemDeleteSourceFile),
	)
}

func collectOpenAPIOperations(path string) ([]operation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	doc := root.Content[0]
	paths, ok := mapValue(doc, "paths")
	if !ok {
		return nil, fmt.Errorf("missing paths node")
	}

	ops := make([]operation, 0, 32)
	for i := 0; i+1 < len(paths.Content); i += 2 {
		pathKey := paths.Content[i]
		pathNode := paths.Content[i+1]
		for _, method := range supportedMethods {
			opNode, ok := mapValue(pathNode, method)
			if !ok {
				continue
			}
			mode, reason, err := parseFrontendConsumption(opNode, strings.TrimSpace(pathKey.Value), strings.ToUpper(method))
			if err != nil {
				return nil, err
			}
			ops = append(ops, operation{
				Method:                  strings.ToUpper(method),
				Path:                    strings.TrimSpace(pathKey.Value),
				FrontendConsumptionMode: mode,
				FrontendReason:          reason,
			})
		}
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].Path == ops[j].Path {
			return ops[i].Method < ops[j].Method
		}
		return ops[i].Path < ops[j].Path
	})
	return ops, nil
}

func parseFrontendConsumption(opNode *yaml.Node, path, method string) (string, string, error) {
	extensionNode, ok := mapValue(opNode, frontendConsumptionExtension)
	if !ok {
		return frontendConsumptionModeRestClient, "", nil
	}
	if extensionNode.Kind != yaml.MappingNode {
		return "", "", fmt.Errorf("%s %s: %s must be a mapping", method, path, frontendConsumptionExtension)
	}

	modeNode, ok := mapValue(extensionNode, "mode")
	if !ok || modeNode.Kind != yaml.ScalarNode || strings.TrimSpace(modeNode.Value) == "" {
		return "", "", fmt.Errorf("%s %s: %s.mode is required", method, path, frontendConsumptionExtension)
	}
	mode := strings.TrimSpace(modeNode.Value)
	if _, ok := supportedFrontendConsumptionModes[mode]; !ok {
		return "", "", fmt.Errorf("%s %s: unsupported %s.mode %q", method, path, frontendConsumptionExtension, mode)
	}

	reasonNode, ok := mapValue(extensionNode, "reason")
	reason := ""
	if ok {
		if reasonNode.Kind != yaml.ScalarNode {
			return "", "", fmt.Errorf("%s %s: %s.reason must be a scalar", method, path, frontendConsumptionExtension)
		}
		reason = strings.TrimSpace(reasonNode.Value)
	}
	if mode != frontendConsumptionModeRestClient && reason == "" {
		return "", "", fmt.Errorf("%s %s: %s.reason is required for mode %s", method, path, frontendConsumptionExtension, mode)
	}

	return mode, reason, nil
}

func runFrontendASTScan(root string) (frontendASTScanResult, error) {
	cmd := exec.Command("node", frontendASTScannerScript, root)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		return frontendASTScanResult{}, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	var scan frontendASTScanResult
	if err := json.Unmarshal(output, &scan); err != nil {
		return frontendASTScanResult{}, fmt.Errorf("decode AST scan output: %w", err)
	}
	return scan, nil
}

func usageIndex(ops []operation, usedOps []string) map[string]bool {
	usage := make(map[string]bool, len(ops))
	for _, op := range ops {
		usage[op.key()] = false
	}
	for _, key := range usedOps {
		if _, ok := usage[key]; ok {
			usage[key] = true
		}
	}
	return usage
}

func loadAllowlist(path string) (map[string]struct{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]struct{})
	lines := strings.Split(string(b), "\n")
	for idx, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hash := strings.Index(line, "#"); hash >= 0 {
			line = strings.TrimSpace(line[:hash])
		}
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			return nil, fmt.Errorf("%s:%d: invalid allowlist format", path, idx+1)
		}
		method := strings.ToUpper(parts[0])
		pathPart := parts[1]
		key := method + " " + pathPart
		if _, exists := allowed[key]; exists {
			return nil, fmt.Errorf("%s:%d: duplicate allowlist entry %q", path, idx+1, key)
		}
		allowed[key] = struct{}{}
	}
	return allowed, nil
}

func confirmSource(path string) string {
	if strings.TrimSpace(path) == "" {
		return "missing"
	}
	return path
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}
