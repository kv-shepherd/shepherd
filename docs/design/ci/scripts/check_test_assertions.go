//go:build ignore

// docs/design/ci/scripts/check_test_assertions.go

/*
Test assertion check - CI enforced

Rules:
1. Test functions must contain assertion calls.
2. Empty tests and fake coverage are forbidden.

Detection strategy:
- Scan Test* functions in _test.go files.
- Require assertion-like calls (assert.*, require.*, t.Error, t.Fatal, etc.).
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

var assertionCalls = map[string]bool{
	// testify
	"Equal":          true,
	"NotEqual":       true,
	"EqualValues":    true,
	"Exactly":        true,
	"Nil":            true,
	"NotNil":         true,
	"True":           true,
	"False":          true,
	"Error":          true,
	"NoError":        true,
	"ErrorIs":        true,
	"ErrorAs":        true,
	"ErrorContains":  true,
	"Contains":       true,
	"NotContains":    true,
	"ElementsMatch":  true,
	"Subset":         true,
	"NotSubset":      true,
	"JSONEq":         true,
	"Regexp":         true,
	"NotRegexp":      true,
	"Len":            true,
	"Empty":          true,
	"NotEmpty":       true,
	"Greater":        true,
	"Less":           true,
	"GreaterOrEqual": true,
	"LessOrEqual":    true,
	"Panics":         true,
	"NotPanics":      true,
	"Eventually":     true,
	"Never":          true,

	// testing.T
	"Errorf":  true,
	"Fatalf":  true,
	"Fail":    true,
	"FailNow": true,
	"Fatal":   true,
}

func main() {
	var emptyTests []string

	for _, dir := range []string{"internal", "pkg", "cmd"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
				// Methods on test doubles (for example TestConnection on a stub adapter)
				// are not test functions and should not be treated as such.
				if funcDecl.Recv != nil {
					continue
				}

				// Only check Test* functions.
				if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
					continue
				}
				// TestMain is process-level setup/teardown and typically has no assertions.
				if funcDecl.Name.Name == "TestMain" {
					continue
				}

				// Ensure test body contains at least one assertion call.
				if !hasAssertion(funcDecl.Body) {
					pos := fset.Position(funcDecl.Pos())
					emptyTests = append(emptyTests, fmt.Sprintf(
						"%s:%d: %s() has no assertion call (possible empty test)",
						path, pos.Line, funcDecl.Name.Name,
					))
				}
			}

			return nil
		})

		if err != nil {
			fmt.Printf("ERROR: failed to walk directory %s: %v\n", dir, err)
		}
	}

	if len(emptyTests) > 0 {
		fmt.Println("ERROR: found test functions without assertions:")
		for _, t := range emptyTests {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("\nTests must include assertions, for example:")
		fmt.Println("  assert.NoError(t, err)")
		fmt.Println("  assert.Equal(t, expected, actual)")
		fmt.Println("  require.NotNil(t, result)")
		os.Exit(1)
	}

	fmt.Println("OK: test assertion check passed")
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

		// Check method calls (assert.Equal, t.Error, etc.).
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if assertionCalls[fn.Sel.Name] {
				found = true
				return false
			}
		}

		return true
	})

	return found
}
