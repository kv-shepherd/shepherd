//go:build ignore

// scripts/ci/check_repository_tests.go

/*
Repository test coverage check - CI enforced

Rules:
1. Every exported method in internal/repository/*.go must have a corresponding test.
2. Preferred test naming format: TestXxxRepository_MethodName

Detection strategy:
- Scan exported methods from repository package files.
- Check whether matching tests exist in _test.go files.
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
	"unicode"
)

type method struct {
	receiver string
	name     string
	file     string
	line     int
}

func main() {
	repoDir := "internal/repository"

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Println("WARN: internal/repository/ does not exist, skipping check")
		os.Exit(0)
	}

	// Collect all exported methods.
	methods := make(map[string]method)   // key: "ReceiverType.MethodName"
	testMethods := make(map[string]bool) // existing test names

	fset := token.NewFileSet()

	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		isTestFile := strings.HasSuffix(path, "_test.go")

		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			if isTestFile {
				// Collect test function names.
				if strings.HasPrefix(funcDecl.Name.Name, "Test") {
					testMethods[funcDecl.Name.Name] = true
				}
			} else {
				// Collect exported receiver methods.
				if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
					recvType := getReceiverTypeName(funcDecl.Recv.List[0].Type)
					if recvType != "" && isExported(funcDecl.Name.Name) {
						key := recvType + "." + funcDecl.Name.Name
						methods[key] = method{
							receiver: recvType,
							name:     funcDecl.Name.Name,
							file:     path,
							line:     fset.Position(funcDecl.Pos()).Line,
						}
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("ERROR: failed to walk repository directory: %v\n", err)
		os.Exit(1)
	}

	// Verify test coverage for each method.
	var missing []string
	for key, m := range methods {
		expectedTestName := fmt.Sprintf("Test%s_%s", m.receiver, m.name)

		// Also accept alternative test naming patterns.
		altTestName1 := fmt.Sprintf("Test%s%s", m.receiver, m.name)
		altTestName2 := fmt.Sprintf("Test_%s_%s", m.receiver, m.name)

		if !testMethods[expectedTestName] && !testMethods[altTestName1] && !testMethods[altTestName2] {
			missing = append(missing, fmt.Sprintf(
				"%s:%d: %s is missing test coverage (expected: %s)",
				m.file, m.line, key, expectedTestName,
			))
		}
	}

	if len(missing) > 0 {
		fmt.Println("ERROR: found untested Repository methods:")
		for _, m := range missing {
			fmt.Printf("  %s\n", m)
		}
		fmt.Printf("\n%d method(s) missing tests (total exported methods: %d)\n", len(missing), len(methods))
		os.Exit(1)
	}

	fmt.Printf("OK: repository test coverage check passed (%d exported method(s) covered)\n", len(methods))
}

func getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func isExported(name string) bool {
	return len(name) > 0 && unicode.IsUpper(rune(name[0]))
}
