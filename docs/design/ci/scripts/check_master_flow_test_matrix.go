//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	matrixPath            = "docs/design/traceability/master-flow-tests.json"
	deferredAllowlistPath = "docs/design/ci/allowlists/master_flow_test_deferred.txt"
	defaultMasterFlowPath = "docs/design/interaction-flows/master-flow.md"
)

var (
	stageAnchorRe     = regexp.MustCompile(`\{#([a-z0-9][a-z0-9-]*)\}`)
	stageHTMLAnchorRe = regexp.MustCompile(`(?i)<a\s+id\s*=\s*["']([a-z0-9][a-z0-9-]*)["']\s*>`)
	stageTokenRe      = regexp.MustCompile(`(?i)\bStages?\s+([0-9]+(?:\.[0-9A-Za-z]+)?)`)
	jsTestRe          = regexp.MustCompile(`(?m)\btest\s*\(|\bit\s*\(|\btest\.describe\s*\(`)
)

type stageTestMatrix struct {
	Version        int                `json:"version"`
	MasterFlow     string             `json:"master_flow"`
	RequiredStages []string           `json:"required_stages"`
	Stages         []stageTestMapping `json:"stages"`
}

type stageTestMapping struct {
	ID    string   `json:"id"`
	Tests []string `json:"tests"`
}

func main() {
	var violations []string

	matrix, err := loadMatrix(matrixPath)
	if err != nil {
		fmt.Printf("FAIL: load %s: %v\n", matrixPath, err)
		os.Exit(1)
	}

	masterFlowPath := strings.TrimSpace(matrix.MasterFlow)
	if masterFlowPath == "" {
		masterFlowPath = defaultMasterFlowPath
	}

	stageCatalog, err := collectMasterFlowStages(masterFlowPath)
	if err != nil {
		fmt.Printf("FAIL: collect master-flow stages: %v\n", err)
		os.Exit(1)
	}
	if len(stageCatalog) == 0 {
		fmt.Printf("FAIL: no stage identifiers discovered in %s\n", masterFlowPath)
		os.Exit(1)
	}

	deferredSet, deferredList, err := parseDeferredAllowlist(deferredAllowlistPath)
	if err != nil {
		fmt.Printf("FAIL: parse deferred allowlist: %v\n", err)
		os.Exit(1)
	}

	if matrix.Version <= 0 {
		violations = append(violations, fmt.Sprintf("%s: version must be > 0", matrixPath))
	}

	requiredSet := make(map[string]struct{}, len(matrix.RequiredStages))
	for _, id := range matrix.RequiredStages {
		stageID := strings.TrimSpace(id)
		if stageID == "" {
			violations = append(violations, "required_stages contains empty stage id")
			continue
		}
		if _, ok := requiredSet[stageID]; ok {
			violations = append(violations, fmt.Sprintf("required_stages has duplicate stage id %q", stageID))
			continue
		}
		requiredSet[stageID] = struct{}{}
		if _, ok := stageCatalog[stageID]; !ok {
			violations = append(violations, fmt.Sprintf("required stage %q not found in %s (anchor or stage token)", stageID, masterFlowPath))
		}
	}

	mappingByID := make(map[string]stageTestMapping, len(matrix.Stages))
	stageExecutableCoverage := make(map[string]bool, len(matrix.Stages))
	for _, mapping := range matrix.Stages {
		stageID := strings.TrimSpace(mapping.ID)
		if stageID == "" {
			violations = append(violations, "stages[] contains empty id")
			continue
		}
		if _, exists := mappingByID[stageID]; exists {
			violations = append(violations, fmt.Sprintf("stages[] contains duplicate id %q", stageID))
			continue
		}
		mappingByID[stageID] = mapping

		if _, ok := stageCatalog[stageID]; !ok {
			violations = append(violations, fmt.Sprintf("mapped stage %q not found in %s (anchor or stage token)", stageID, masterFlowPath))
		}
		if len(mapping.Tests) == 0 {
			violations = append(violations, fmt.Sprintf("stage %q has no tests[] entries", stageID))
			continue
		}

		seen := make(map[string]struct{}, len(mapping.Tests))
		for _, path := range mapping.Tests {
			testPath := strings.TrimSpace(path)
			if testPath == "" {
				violations = append(violations, fmt.Sprintf("stage %q has empty test path", stageID))
				continue
			}
			if _, ok := seen[testPath]; ok {
				violations = append(violations, fmt.Sprintf("stage %q has duplicate test path %q", stageID, testPath))
				continue
			}
			seen[testPath] = struct{}{}

			executable, reason := validateExecutableTest(testPath)
			if !executable {
				violations = append(violations, fmt.Sprintf("stage %q test %q is not executable: %s", stageID, testPath, reason))
				continue
			}
			stageExecutableCoverage[stageID] = true
		}
	}

	deferredSeen := make(map[string]struct{}, len(deferredList))
	for _, rawID := range deferredList {
		stageID := strings.TrimSpace(rawID)
		if stageID == "" {
			continue
		}
		if _, ok := deferredSeen[stageID]; ok {
			violations = append(violations, fmt.Sprintf("deferred allowlist has duplicate stage id %q", stageID))
			continue
		}
		deferredSeen[stageID] = struct{}{}

		if _, ok := stageCatalog[stageID]; !ok {
			violations = append(violations, fmt.Sprintf("deferred stage %q not found in %s (anchor or stage token)", stageID, masterFlowPath))
		}
		if _, ok := requiredSet[stageID]; !ok {
			violations = append(violations, fmt.Sprintf("deferred stage %q is not listed in required_stages", stageID))
		}
		if stageExecutableCoverage[stageID] {
			violations = append(violations, fmt.Sprintf("deferred stage %q is stale: executable tests already mapped", stageID))
		}
	}

	for _, stageID := range matrix.RequiredStages {
		if stageExecutableCoverage[stageID] {
			continue
		}
		if _, deferred := deferredSet[stageID]; deferred {
			continue
		}
		violations = append(violations, fmt.Sprintf("required stage %q has no executable mapped test and is not deferred", stageID))
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: master-flow test matrix check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Printf("Rule: Every required master-flow stage must be covered by executable tests in %s or explicitly deferred in %s\n", matrixPath, deferredAllowlistPath)
		os.Exit(1)
	}

	fmt.Printf(
		"OK: master-flow test matrix check passed (required=%d, mapped=%d, covered=%d, deferred=%d)\n",
		len(matrix.RequiredStages),
		len(mappingByID),
		len(stageExecutableCoverage),
		len(deferredSet),
	)
}

func loadMatrix(path string) (stageTestMatrix, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return stageTestMatrix{}, err
	}

	var matrix stageTestMatrix
	if err := json.Unmarshal(b, &matrix); err != nil {
		return stageTestMatrix{}, err
	}
	return matrix, nil
}

func collectMasterFlowStages(path string) (map[string]struct{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(b)

	out := make(map[string]struct{})

	anchorMatches := stageAnchorRe.FindAllStringSubmatch(content, -1)
	for _, m := range anchorMatches {
		if len(m) != 2 {
			continue
		}
		id := strings.TrimSpace(strings.ToLower(m[1]))
		if id != "" {
			out[id] = struct{}{}
		}
	}

	htmlAnchorMatches := stageHTMLAnchorRe.FindAllStringSubmatch(content, -1)
	for _, m := range htmlAnchorMatches {
		if len(m) != 2 {
			continue
		}
		id := strings.TrimSpace(strings.ToLower(m[1]))
		if id != "" {
			out[id] = struct{}{}
		}
	}

	tokenMatches := stageTokenRe.FindAllStringSubmatch(content, -1)
	for _, m := range tokenMatches {
		if len(m) != 2 {
			continue
		}
		if id := stageTokenToID(m[1]); id != "" {
			out[id] = struct{}{}
		}
	}

	return out, nil
}

func stageTokenToID(token string) string {
	t := strings.TrimSpace(token)
	if t == "" {
		return ""
	}
	t = strings.Trim(t, ".,;:()[]{}")
	if t == "" {
		return ""
	}
	t = strings.ReplaceAll(t, ".", "-")
	t = strings.ToLower(t)
	return "stage-" + t
}

func parseDeferredAllowlist(path string) (map[string]struct{}, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	set := make(map[string]struct{})
	list := make([]string, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		set[line] = struct{}{}
		list = append(list, line)
	}
	if err := s.Err(); err != nil {
		return nil, nil, err
	}
	return set, list, nil
}

func validateExecutableTest(path string) (bool, string) {
	if _, err := os.Stat(path); err != nil {
		return false, err.Error()
	}

	switch {
	case strings.HasSuffix(path, "_test.go"):
		ok, err := hasGoTestFunction(path)
		if err != nil {
			return false, fmt.Sprintf("parse failed: %v", err)
		}
		if !ok {
			return false, "missing func TestXxx(t *testing.T)"
		}
		return true, ""
	case isJSTestFile(path):
		ok, err := hasJSTestMarker(path)
		if err != nil {
			return false, err.Error()
		}
		if !ok {
			return false, "missing test()/it()/test.describe() marker"
		}
		return true, ""
	default:
		return false, fmt.Sprintf("unsupported test file type %q", filepath.Ext(path))
	}
}

func isJSTestFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return true
	default:
		return false
	}
}

func hasJSTestMarker(path string) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return jsTestRe.Match(b), nil
}

func hasGoTestFunction(path string) (bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return false, err
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Recv != nil {
			continue
		}
		if !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		if fn.Type == nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
			continue
		}
		param := fn.Type.Params.List[0]
		if len(param.Names) != 1 {
			continue
		}
		if isTestingTStar(param.Type) {
			return true, nil
		}
	}
	return false, nil
}

func isTestingTStar(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}

	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		pkg, ok := x.X.(*ast.Ident)
		if !ok || x.Sel == nil {
			return false
		}
		return pkg.Name == "testing" && x.Sel.Name == "T"
	case *ast.Ident:
		return x.Name == "T"
	default:
		return false
	}
}
