// check_semaphore_usage.go enforces ADR-0031 (Concurrency Safety and Worker Pool Standard).
//
// Rule: any semaphore Acquire() in a function must have a paired defer Release()
// within the same function to avoid leaks on early returns/panics.

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

type funcInfo struct {
	name            string
	hasAcquire      bool
	hasDefer        bool
	acquireLine     int
	releaseLine     int
	hasDeferRelease bool
}

func main() {
	var errors []string

	for _, dir := range []string{"internal"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}

			// Inspect all function bodies.
			ast.Inspect(node, func(n ast.Node) bool {
				funcDecl, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}

				info := analyzeFuncForSemaphore(funcDecl, fset)
				if info.hasAcquire && !info.hasDeferRelease {
					errors = append(errors, fmt.Sprintf(
						"%s:%d: func %s() calls Acquire() without a paired defer Release()",
						path, info.acquireLine, info.name,
					))
				}

				return true
			})

			return nil
		})

		if err != nil {
			fmt.Printf("[semaphore] FAIL: walk %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	if len(errors) > 0 {
		fmt.Println("[semaphore] FAIL: semaphore Acquire/Release issues found")
		for _, e := range errors {
			fmt.Printf("%s\n", e)
		}
		os.Exit(1)
	}

	fmt.Println("[semaphore] OK")
}

func analyzeFuncForSemaphore(funcDecl *ast.FuncDecl, fset *token.FileSet) funcInfo {
	info := funcInfo{name: funcDecl.Name.Name}
	if funcDecl.Body == nil {
		return info
	}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Acquire" {
					info.hasAcquire = true
					info.acquireLine = fset.Position(node.Pos()).Line
				}
				if sel.Sel.Name == "Release" {
					info.releaseLine = fset.Position(node.Pos()).Line
				}
			}
		case *ast.DeferStmt:
			info.hasDefer = true
			// Check "defer x.Release(...)" and "defer func(){ ... Release(...) }()".
			if call, ok := node.Call.Fun.(*ast.SelectorExpr); ok {
				if call.Sel.Name == "Release" {
					info.hasDeferRelease = true
				}
			}
			if funcLit, ok := node.Call.Fun.(*ast.FuncLit); ok {
				ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
					if call, ok := inner.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Release" {
								info.hasDeferRelease = true
							}
						}
					}
					return true
				})
			}
		}
		return true
	})

	return info
}
