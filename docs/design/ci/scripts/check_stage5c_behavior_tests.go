//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

type behaviorTestRequirement struct {
	file             string
	requiredTests    []string
	requiredSnippets []string
}

var requirements = []behaviorTestRequirement{
	{
		file: "internal/provider/kubevirt_test.go",
		requiredTests: []string{
			"TestBuildVMFromSpec_AppliesSpecOverrides",
			"TestBuildVMFromSpec_RejectsInvalidSpecOverridePath",
		},
		requiredSnippets: []string{
			"spec.template.spec.domain.memory.hugepages.pageSize",
			"spec.template.spec.domain.devices.gpus",
			"spec.template.spec.domain.cpu.dedicatedCpuPlacement",
			"metadata.labels.foo",
			"expected dedicatedCpuPlacement=true from spec_overrides",
		},
	},
	{
		file: "internal/jobs/vm_create_test.go",
		requiredTests: []string{
			"TestApplyModifiedSpecOverrides",
			"TestResolveInstanceSizeSpecOverrides",
			"TestResolveInstanceSizeSpecOverrides_BackwardCompatibleFlatSnapshot",
		},
		requiredSnippets: []string{
			"spec_overrides",
			"spec.template.spec.domain.cpu.cores",
			"spec.template.spec.domain.memory.hugepages.pageSize",
			"expected snapshot overrides",
		},
	},
}

var assertionCalls = map[string]struct{}{
	"Error": {}, "Errorf": {}, "Fatal": {}, "Fatalf": {}, "Fail": {}, "FailNow": {},
	"Equal": {}, "NotEqual": {}, "Nil": {}, "NotNil": {}, "True": {}, "False": {},
	"NoError": {}, "Contains": {}, "NotContains": {}, "Len": {}, "Empty": {}, "NotEmpty": {},
}

func main() {
	var violations []string

	for _, req := range requirements {
		content, err := os.ReadFile(req.file)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: read failed: %v", req.file, err))
			continue
		}
		text := string(content)
		for _, snippet := range req.requiredSnippets {
			if !strings.Contains(text, snippet) {
				violations = append(violations, fmt.Sprintf("%s: missing scenario snippet %q", req.file, snippet))
			}
		}

		testStatus, err := collectTestAssertionStatus(req.file)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s: parse failed: %v", req.file, err))
			continue
		}
		for _, testName := range req.requiredTests {
			hasAssertions, ok := testStatus[testName]
			if !ok {
				violations = append(violations, fmt.Sprintf("%s: missing required behavior test %s", req.file, testName))
				continue
			}
			if !hasAssertions {
				violations = append(violations, fmt.Sprintf("%s: %s has no assertions", req.file, testName))
			}
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: Stage 5.C behavior test check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: critical Stage 5.C tests must exist with assertions and cover key spec_overrides scenarios.")
		os.Exit(1)
	}

	fmt.Println("OK: Stage 5.C behavior test check passed")
}

func collectTestAssertionStatus(path string) (map[string]bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Recv != nil {
			continue
		}
		if !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		out[fn.Name.Name] = hasAssertion(fn.Body)
	}
	return out, nil
}

func hasAssertion(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		if _, ok := assertionCalls[sel.Sel.Name]; ok {
			found = true
			return false
		}
		return true
	})

	return found
}
