//go:build ignore

// docs/design/ci/scripts/check_test_assertions.go

/*
Test assertion check - CI enforced

Rules:
1. Test functions must contain assertion calls.
2. Go tests must contain at least one behavior assertion, not only object
   existence checks such as Nil or NotNil.
3. Frontend Vitest files must contain expect/assert calls.
4. Empty tests and fake coverage are forbidden.

Detection strategy:
- Scan Test* functions in _test.go files.
- Require assertion-like calls (assert.*, require.*, t.Error, t.Fatal, etc.).
- Scan frontend test files with a conservative text check for Vitest assertions.
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

var weakAssertionCalls = map[string]bool{
	"Nil":    true,
	"NotNil": true,
}

func main() {
	var emptyTests []string
	var weakTests []string
	var frontendTests []string

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

				stats := assertionStats(funcDecl.Body)
				if stats.total == 0 {
					pos := fset.Position(funcDecl.Pos())
					emptyTests = append(emptyTests, fmt.Sprintf(
						"%s:%d: %s() has no assertion call (possible empty test)",
						path, pos.Line, funcDecl.Name.Name,
					))
					continue
				}
				if stats.strong == 0 {
					pos := fset.Position(funcDecl.Pos())
					weakTests = append(weakTests, fmt.Sprintf(
						"%s:%d: %s() only has setup-level assertions; add behavior/state assertions",
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

	if _, err := os.Stat("web/src"); err == nil {
		err := filepath.Walk("web/src", func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !isFrontendTestFile(path) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if !hasFrontendAssertion(string(content)) {
				frontendTests = append(frontendTests, fmt.Sprintf(
					"%s: frontend test file has no Vitest expect/assert call",
					path,
				))
			}
			return nil
		})
		if err != nil {
			fmt.Printf("ERROR: failed to walk directory web/src: %v\n", err)
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

	if len(weakTests) > 0 {
		fmt.Println("ERROR: found test functions with only setup-level assertions:")
		for _, t := range weakTests {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("\nAdd assertions that verify behavior or state, for example:")
		fmt.Println("  assert.Equal(t, expected, actual)")
		fmt.Println("  assert.Contains(t, got, expectedItem)")
		fmt.Println("  assert.ErrorContains(t, err, expectedMessage)")
		os.Exit(1)
	}

	if len(frontendTests) > 0 {
		fmt.Println("ERROR: found frontend test files without assertions:")
		for _, t := range frontendTests {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("\nVitest tests must include expect(...) or assert.* calls.")
		os.Exit(1)
	}

	fmt.Println("OK: test assertion check passed")
}

type assertionCounters struct {
	total  int
	strong int
}

func assertionStats(body *ast.BlockStmt) assertionCounters {
	var stats assertionCounters
	if body == nil {
		return stats
	}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check method calls (assert.Equal, t.Error, etc.).
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if assertionCalls[fn.Sel.Name] {
				stats.total++
				if !weakAssertionCalls[fn.Sel.Name] {
					stats.strong++
				}
			}
		}

		return true
	})

	return stats
}

func isFrontendTestFile(path string) bool {
	for _, suffix := range []string{".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func hasFrontendAssertion(content string) bool {
	return strings.Contains(content, "expect(") ||
		strings.Contains(content, "expect.") ||
		strings.Contains(content, "assert.")
}
