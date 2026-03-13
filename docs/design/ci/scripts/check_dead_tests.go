//go:build ignore

// docs/design/ci/scripts/check_dead_tests.go

/*
Dead test detection - CI warning (non-blocking)

Rules:
1. Detect tests containing only t.Skip().
2. Detect tests with commented-out/placeholder-only logic.
3. Detect tests with empty function bodies.

This check is warning-level and does not block CI.
*/

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type deadTest struct {
	file   string
	line   int
	name   string
	reason string
}

func main() {
	var warnings []deadTest

	for _, dir := range []string{"internal", "pkg", "cmd"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}

			for _, decl := range node.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
					continue
				}

				// Empty function body.
				if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "empty function body",
					})
					continue
				}

				// t.Skip-only test.
				if isOnlySkip(funcDecl.Body) {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "contains only t.Skip()",
					})
					continue
				}

				// TODO-only placeholder test.
				if hasOnlyTODO(funcDecl.Body) {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "TODO placeholder without actual assertions",
					})
				}
			}

			return nil
		})
	}

	if len(warnings) > 0 {
		fmt.Println("WARNING: found potential dead tests (manual review required):")
		for _, w := range warnings {
			fmt.Printf("  %s:%d: %s - %s\n", w.file, w.line, w.name, w.reason)
		}
		fmt.Println("\nThese tests may need implementation or removal.")
		// Warning only; do not exit non-zero.
	} else {
		fmt.Println("OK: dead test check passed")
	}
}

func isOnlySkip(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}

	// Return true only when every statement is a Skip/SkipNow call.
	for _, stmt := range body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			return false
		}
		call, ok := exprStmt.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Skip" && sel.Sel.Name != "SkipNow") {
			return false
		}
	}
	return true
}

func hasOnlyTODO(body *ast.BlockStmt) bool {
	// Simplified heuristic: one literal-only statement often indicates placeholder TODO text.
	if len(body.List) == 1 {
		if exprStmt, ok := body.List[0].(*ast.ExprStmt); ok {
			if _, ok := exprStmt.X.(*ast.BasicLit); ok {
				return true // Could be a bare string literal like "TODO".
			}
		}
	}
	return false
}
