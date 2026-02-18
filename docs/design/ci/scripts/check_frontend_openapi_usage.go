//go:build ignore

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	openAPIPath    = "api/openapi.yaml"
	frontendSrcDir = "web/src"
	allowlistPath  = "docs/design/ci/allowlists/frontend_openapi_unused.txt"
)

var supportedMethods = []string{"get", "post", "put", "patch", "delete"}

type operation struct {
	Method string
	Path   string
}

func (o operation) key() string {
	return o.Method + " " + o.Path
}

func main() {
	ops, err := collectOpenAPIOperations(openAPIPath)
	if err != nil {
		fmt.Printf("FAIL: collect OpenAPI operations: %v\n", err)
		os.Exit(1)
	}

	usage, err := collectFrontendUsage(frontendSrcDir, ops)
	if err != nil {
		fmt.Printf("FAIL: collect frontend usage: %v\n", err)
		os.Exit(1)
	}

	allowlist, err := loadAllowlist(allowlistPath)
	if err != nil {
		fmt.Printf("FAIL: load allowlist: %v\n", err)
		os.Exit(1)
	}

	var violations []string
	opIndex := make(map[string]operation, len(ops))
	usedCount := 0

	for _, op := range ops {
		key := op.key()
		opIndex[key] = op
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
		}
	}

	if err := checkSystemDeleteConfirmGate(frontendSrcDir); err != nil {
		violations = append(violations, err.Error())
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: frontend/OpenAPI usage check failed")
		for _, v := range violations {
			fmt.Printf(" - %s\n", v)
		}
		fmt.Println("Rule: each OpenAPI operation must be consumed by frontend or be explicitly deferred in allowlist.")
		fmt.Println("Rule: system delete UI must send confirm_name query parameter (ADR-0015 §13).")
		os.Exit(1)
	}

	fmt.Printf(
		"OK: frontend/OpenAPI usage check passed (operations=%d used=%d allowlisted=%d)\n",
		len(ops),
		usedCount,
		len(allowlist),
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
			if _, ok := mapValue(pathNode, method); !ok {
				continue
			}
			ops = append(ops, operation{
				Method: strings.ToUpper(method),
				Path:   strings.TrimSpace(pathKey.Value),
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

func collectFrontendUsage(root string, ops []operation) (map[string]bool, error) {
	usage := make(map[string]bool, len(ops))
	for _, op := range ops {
		usage[op.key()] = false
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
		case ".ts", ".tsx", ".js", ".jsx":
		default:
			return nil
		}

		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		for _, op := range ops {
			key := op.key()
			if usage[key] {
				continue
			}
			if containsAPICall(text, op) {
				usage[key] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return usage, nil
}

func containsAPICall(text string, op operation) bool {
	patterns := []string{
		fmt.Sprintf("api.%s('%s'", op.Method, op.Path),
		fmt.Sprintf("api.%s(\"%s\"", op.Method, op.Path),
		fmt.Sprintf("api.%s(`%s`", op.Method, op.Path),
	}
	for _, p := range patterns {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
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

func checkSystemDeleteConfirmGate(root string) error {
	deleteFragments := []string{
		"api.DELETE('/systems/{system_id}'",
		"confirm_name",
	}
	var matchedFile string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".ts" && ext != ".tsx" && ext != ".js" && ext != ".jsx" {
			return nil
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(b)
		for _, frag := range deleteFragments {
			if !strings.Contains(text, frag) {
				return nil
			}
		}
		matchedFile = path
		return fs.SkipAll
	})
	if err != nil && err != fs.SkipAll {
		return fmt.Errorf("scan frontend for system delete confirm gate: %w", err)
	}
	if matchedFile == "" {
		return fmt.Errorf(
			"missing system delete confirm_name flow in frontend source: require fragments %q + %q",
			deleteFragments[0],
			deleteFragments[1],
		)
	}
	return nil
}

func mapValue(node *yaml.Node, key string) (*yaml.Node, bool) {
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
