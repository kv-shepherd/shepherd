// Package semaphoreusage implements a go/analysis Analyzer that enforces
// paired Acquire/defer Release patterns for semaphores.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_semaphore_usage.go` (removed post-ADR-0039 migration).
//
// Rule enforced (ADR-0031):
//   - Any function in internal/ that calls sem.Acquire() must have a paired
//     defer Release() to prevent resource leaks on early returns or panics.
//
// Applies to: internal/ packages (non-test files).
package semaphoreusage

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for semaphore usage checking.
var Analyzer = &analysis.Analyzer{
	Name:     "semaphoreusage",
	Doc:      "Enforces that semaphore Acquire() calls have a paired defer Release() (ADR-0031)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	if pass.Pkg == nil {
		return nil, nil
	}

	// Only enforce inside internal/ packages.
	pkgPath := pass.Pkg.Path()
	if !strings.Contains(pkgPath, "internal") &&
		!strings.Contains(pkgPath, "/internal/") &&
		!strings.HasSuffix(pkgPath, "/internal") {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Visit every function declaration.
	nodeFilter := []ast.Node{(*ast.FuncDecl)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		funcDecl, ok := n.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			return
		}

		// Skip test files.
		pos := pass.Fset.Position(funcDecl.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		acquireNode, hasAcquire := findAcquireCall(funcDecl.Body)
		hasDeferRelease := findDeferRelease(funcDecl.Body)

		if hasAcquire && !hasDeferRelease {
			pass.Reportf(acquireNode.Pos(),
				"func %s calls Acquire() without a paired defer Release(); add defer to prevent resource leak (ADR-0031)",
				funcDecl.Name.Name)
		}
	})

	return nil, nil
}

// findAcquireCall returns the first Acquire() call node in the body, if any.
// Note: ast.Inspect is intentionally used here (not insp.Preorder from inspect.Analyzer)
// because we are traversing a sub-tree (FuncDecl.Body) that has already been selected
// by the outer insp.Preorder call. Mixing insp.Preorder for inner traversals would
// require a second Inspector pass over the same nodes, which is unnecessary.
func findAcquireCall(body *ast.BlockStmt) (*ast.CallExpr, bool) {
	var result *ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		if result != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Acquire" {
			result = call
		}
		return true
	})
	return result, result != nil
}

// findDeferRelease returns true if the function body contains a defer Release() statement.
// Matches both: defer x.Release() and defer func() { x.Release() }().
// Note: ast.Inspect is used intentionally for sub-tree traversal (see findAcquireCall).
func findDeferRelease(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		ds, ok := n.(*ast.DeferStmt)
		if !ok {
			return true
		}
		// Pattern: defer x.Release(...)
		if sel, ok := ds.Call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Release" {
			found = true
			return false
		}
		// Pattern: defer func() { x.Release() }()
		if funcLit, ok := ds.Call.Fun.(*ast.FuncLit); ok {
			ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
				if found {
					return false
				}
				call, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Release" {
					found = true
				}
				return true
			})
		}
		return true
	})
	return found
}
