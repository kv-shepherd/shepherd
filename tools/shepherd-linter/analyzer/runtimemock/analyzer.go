// Package runtimemock implements a go/analysis Analyzer that detects
// runtime wiring of MockProvider — a test-only construct.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_no_runtime_mock.go` (removed post-ADR-0039 migration).
//
// Rule enforced:
//   - Production code (cmd/, internal/) must not call NewMockProvider().
//   - MockProvider is strictly test-only; runtime must wire real implementations.
//
// Exemption: internal/provider/mock.go is excluded (it is the definition file).
package runtimemock

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for runtime mock detection.
var Analyzer = &analysis.Analyzer{
	Name:     "runtimemock",
	Doc:      "Detects runtime wiring of MockProvider in production code (test-only construct)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// targetPkgPrefixes constrains the analyzer to production packages.
var targetPkgPrefixes = []string{"cmd", "internal", "pkg"}

// mockConstructors are the function names that are test-only.
var mockConstructors = map[string]bool{
	"NewMockProvider": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce in production packages.
	if !isTargetPkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		// Skip test files.
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		// Exempt the mock definition file itself.
		if strings.HasSuffix(pos.Filename, "internal/provider/mock.go") {
			return
		}

		// Match NewMockProvider() as bare name or selector (pkg.NewMockProvider).
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if mockConstructors[fun.Name] {
				pass.Reportf(call.Pos(),
					"runtime wiring must not call %s(): MockProvider is test-only; wire real provider implementations",
					fun.Name)
			}
		case *ast.SelectorExpr:
			if fun.Sel != nil && mockConstructors[fun.Sel.Name] {
				pass.Reportf(call.Pos(),
					"runtime wiring must not call %s(): MockProvider is test-only; wire real provider implementations",
					fun.Sel.Name)
			}
		}
	})

	return nil, nil
}

// isTargetPkg returns true if the package is under cmd/, internal/, or pkg/.
func isTargetPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	for _, prefix := range targetPkgPrefixes {
		if strings.HasPrefix(pkgPath, prefix+"/") ||
			pkgPath == prefix ||
			strings.Contains(pkgPath, "/"+prefix+"/") {
			return true
		}
	}
	return false
}
