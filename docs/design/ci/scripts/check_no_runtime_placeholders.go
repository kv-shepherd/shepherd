//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	commentPattern = regexp.MustCompile(`(?i)\b(todo|fixme|xxx|placeholder|stub|not implemented)\b`)
	stringPattern  = regexp.MustCompile(`(?i)\b(placeholder|stub|not implemented)\b`)
)

func main() {
	roots := []string{"internal", "cmd"}
	var violations []string

	for _, root := range roots {
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if shouldSkipDir(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fileViolations, err := checkFile(path)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s: parse failed: %v", path, err))
				return nil
			}
			violations = append(violations, fileViolations...)
			return nil
		}); err != nil {
			fmt.Printf("FAIL: walk %s: %v\n", root, err)
			os.Exit(1)
		}
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: runtime placeholder marker check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: runtime code must not contain TODO/FIXME/placeholder/stub markers.")
		os.Exit(1)
	}

	fmt.Println("OK: no runtime placeholder markers detected")
}

func shouldSkipDir(path string) bool {
	clean := filepath.Clean(path)
	switch clean {
	case filepath.Clean("internal/api/generated"), filepath.Clean("internal/repository/sqlc"):
		return true
	default:
		return false
	}
}

func checkFile(path string) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var violations []string

	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*"))
			if commentPattern.MatchString(text) {
				pos := fset.Position(c.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d: comment contains blocked marker: %q", path, pos.Line, text))
			}
		}
	}

	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		if stringPattern.MatchString(unquoted) {
			pos := fset.Position(lit.Pos())
			violations = append(violations, fmt.Sprintf("%s:%d: string literal contains blocked marker: %q", path, pos.Line, unquoted))
		}
		return true
	})

	return violations, nil
}
