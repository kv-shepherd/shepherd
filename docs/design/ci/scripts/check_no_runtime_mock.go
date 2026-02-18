//go:build ignore

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

const runtimeMockNolint = "//nolint:runtime-mock"

func main() {
	var violations []string

	targetDirs := []string{"cmd", "internal"}
	for _, dir := range targetDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if filepath.ToSlash(path) == "internal/provider/mock.go" {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if strings.Contains(string(content), runtimeMockNolint) {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
			if err != nil {
				return nil
			}

			ast.Inspect(node, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				isMockCtor := false
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					isMockCtor = fun.Name == "NewMockProvider"
				case *ast.SelectorExpr:
					isMockCtor = fun.Sel != nil && fun.Sel.Name == "NewMockProvider"
				}

				if !isMockCtor {
					return true
				}

				pos := fset.Position(call.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d: runtime wiring must not call NewMockProvider()", path, pos.Line))
				return true
			})

			return nil
		})
	}

	if len(violations) == 0 {
		fmt.Println("OK: no runtime MockProvider wiring found")
		return
	}

	fmt.Println("FAIL: runtime MockProvider wiring detected")
	for _, v := range violations {
		fmt.Println(" -", v)
	}
	fmt.Println("Rule: MockProvider is test-only. Runtime must wire real provider implementations.")
	os.Exit(1)
}
