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

type criticalTarget struct {
	sourceFile string
	testFiles  []string
}

var criticalTargets = []criticalTarget{
	{
		sourceFile: "internal/jobs/vm_create.go",
		testFiles:  []string{"internal/jobs/vm_create_test.go"},
	},
	{
		sourceFile: "internal/provider/kubevirt.go",
		testFiles:  []string{"internal/provider/kubevirt_test.go"},
	},
	{
		sourceFile: "internal/usecase/create_vm.go",
		testFiles:  []string{"internal/usecase/create_vm_test.go"},
	},
	{
		sourceFile: "internal/usecase/delete_vm.go",
		testFiles:  []string{"internal/usecase/delete_vm_test.go"},
	},
	{
		sourceFile: "internal/governance/approval/gateway.go",
		testFiles:  []string{"internal/governance/approval/gateway_test.go"},
	},
	{
		sourceFile: "internal/api/middleware/openapi_validator.go",
		testFiles:  []string{"internal/api/middleware/openapi_validator_test.go"},
	},
}

func main() {
	var violations []string

	for _, target := range criticalTargets {
		if _, err := os.Stat(target.sourceFile); err != nil {
			violations = append(violations, fmt.Sprintf("missing critical source file: %s", target.sourceFile))
			continue
		}

		var sourceHasTest bool
		for _, testFile := range target.testFiles {
			if _, err := os.Stat(testFile); err != nil {
				violations = append(violations, fmt.Sprintf("%s: missing required test file %s", target.sourceFile, testFile))
				continue
			}
			ok, err := hasGoTestFunction(testFile)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s: parse failed: %v", testFile, err))
				continue
			}
			if !ok {
				violations = append(violations, fmt.Sprintf("%s: no Go test function (func TestXxx(t *testing.T)) found", testFile))
				continue
			}
			sourceHasTest = true
		}
		if !sourceHasTest {
			violations = append(violations, fmt.Sprintf("%s: no valid test coverage found", target.sourceFile))
		}
	}

	if len(violations) > 0 {
		sort.Strings(violations)
		fmt.Println("FAIL: critical test presence check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: critical runtime paths must keep at least one valid Go test in their paired *_test.go files.")
		os.Exit(1)
	}

	fmt.Println("OK: critical test presence check passed")
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
		// Support uncommon alias/import forms where type resolves to local T.
		return x.Name == "T"
	default:
		return false
	}
}
