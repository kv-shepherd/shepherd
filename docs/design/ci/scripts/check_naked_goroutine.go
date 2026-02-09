// check_naked_goroutine.go enforces ADR-0031 (Concurrency Safety and Worker Pool Standard).
//
// Rule: no naked `go` statements under internal/ (non-test) code.
// All in-process concurrency must go through a worker pool submission API.
//
// Exemptions are deliberately small and must be reviewed.

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

func main() {
	internalDir := "internal"
	var errors []string

	if _, err := os.Stat(internalDir); os.IsNotExist(err) {
		fmt.Println("[naked-goroutine] SKIP: internal/ not present")
		return
	}

	// Exempt paths:
	// - worker pool implementation itself may spawn goroutines
	// - River worker infrastructure owns its own lifecycle management
	exemptPaths := map[string]bool{
		"internal/pkg/worker":       true,
		"internal/governance/river": true,
	}

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and test files.
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Skip exempt paths (prefix match).
		slashPath := filepath.ToSlash(path)
		for exempt := range exemptPaths {
			exempt = filepath.ToSlash(exempt)
			if slashPath == exempt || strings.HasPrefix(slashPath, exempt+"/") {
				return nil
			}
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		// Detect any "go" statement.
		ast.Inspect(node, func(n ast.Node) bool {
			if goStmt, ok := n.(*ast.GoStmt); ok {
				pos := fset.Position(goStmt.Pos())
				errors = append(errors, fmt.Sprintf(
					"%s:%d: naked goroutine is forbidden; use worker pool submission (e.g. pools.General.Submit())",
					path, pos.Line,
				))
			}
			return true
		})

		return nil
	})

	if err != nil {
		fmt.Printf("[naked-goroutine] FAIL: walk internal/: %v\n", err)
		os.Exit(1)
	}

	if len(errors) > 0 {
		fmt.Println("[naked-goroutine] FAIL: naked goroutines found")
		for _, e := range errors {
			fmt.Printf("%s\n", e)
		}
		os.Exit(1)
	}

	fmt.Println("[naked-goroutine] OK")
}
