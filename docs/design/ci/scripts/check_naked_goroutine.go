//go:build ignore

// check_naked_goroutine.go enforces ADR-0031 (Concurrency Safety and Worker Pool Standard).
//
// Rule: no naked `go` statements under internal/ (non-test) code.
// All in-process concurrency must go through a worker pool submission API.
//
// Exemptions are deliberately small and must be reviewed.

package main

import (
	"bufio"
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

		// Check for file-level nolint comment.
		if hasFileNolint(path, "naked-goroutine") {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		// Build set of function positions suppressed by nolint.
		type lineRange struct{ start, end int }
		var suppressedRanges []lineRange
		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			// Check doc comments on the function.
			if funcDecl.Doc != nil {
				for _, c := range funcDecl.Doc.List {
					if strings.Contains(c.Text, "nolint:naked-goroutine") {
						bodyStart := fset.Position(funcDecl.Body.Pos()).Line
						bodyEnd := fset.Position(funcDecl.Body.End()).Line
						suppressedRanges = append(suppressedRanges, lineRange{bodyStart, bodyEnd})
					}
				}
			}
		}
		// Also check standalone inline nolint comments (not attached to functions).
		for _, cg := range node.Comments {
			for _, c := range cg.List {
				if strings.Contains(c.Text, "nolint:naked-goroutine") {
					commentLine := fset.Position(c.Pos()).Line
					// Suppress same line and next line.
					suppressedRanges = append(suppressedRanges, lineRange{commentLine, commentLine + 1})
				}
			}
		}
		isSuppressed := func(line int) bool {
			for _, r := range suppressedRanges {
				if line >= r.start && line <= r.end {
					return true
				}
			}
			return false
		}

		// Detect any "go" statement.
		ast.Inspect(node, func(n ast.Node) bool {
			if goStmt, ok := n.(*ast.GoStmt); ok {
				pos := fset.Position(goStmt.Pos())
				if isSuppressed(pos.Line) {
					return true // Suppressed by nolint
				}
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

// hasFileNolint scans the first 20 lines of a file for a file-level nolint comment.
func hasFileNolint(path, tag string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; i < 20 && scanner.Scan(); i++ {
		line := scanner.Text()
		if strings.Contains(line, "nolint:"+tag) {
			return true
		}
	}
	return false
}
