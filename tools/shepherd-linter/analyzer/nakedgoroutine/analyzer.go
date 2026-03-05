// Package nakedgoroutine implements a go/analysis Analyzer that enforces ADR-0031:
// no naked `go` statements in internal/ (non-test) code.
// All in-process concurrency must go through a worker pool submission API.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_naked_goroutine.go` (removed post-ADR-0039 migration).
//
// Suppression:
//   - File-level:   //nolint:nakedgoroutine or //nolint:naked-goroutine in the first 20 lines
//   - Function-level: doc comment containing nolint:nakedgoroutine or nolint:naked-goroutine
//   - Inline:       //nolint:nakedgoroutine or //nolint:naked-goroutine on the same or preceding line
//   - Recommended for golangci-lint integration: //nolint:shepherd-arch
package nakedgoroutine

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for naked goroutine detection.
var Analyzer = &analysis.Analyzer{
	Name:     "nakedgoroutine",
	Doc:      "ADR-0031: detects naked 'go' statements outside approved worker-pool directories",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// exemptPkgPaths are import path suffixes that are allowed to spawn goroutines.
// These correspond to the worker pool implementation and River worker infrastructure.
var exemptPkgPaths = []string{
	"internal/pkg/worker",
	"internal/governance/river",
	// The server main entry point is handled via issues excludes in .golangci.yml
}

var suppressionTags = []string{
	"nolint:nakedgoroutine",
	// Backward compatibility with legacy script tag spelling.
	"nolint:naked-goroutine",
	"nolint:shepherd-arch",
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside internal/ packages.
	if !isInternalPkg(pass) {
		return nil, nil
	}

	// Skip exempt packages (worker pool, River infrastructure).
	if isExemptPkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Build suppressed location set from nolint comments.
	// Key format: "filename:line" to avoid cross-file false suppression.
	suppressedLocs := buildSuppressedLocs(pass)

	// Walk only GoStmt nodes.
	nodeFilter := []ast.Node{(*ast.GoStmt)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return
		}
		pos := pass.Fset.Position(goStmt.Pos())
		key := fmt.Sprintf("%s:%d", pos.Filename, pos.Line)
		if suppressedLocs[key] {
			return
		}
		pass.Reportf(goStmt.Pos(),
			"naked goroutine is forbidden (ADR-0031); use worker pool submission (e.g. pools.General.Submit())")
	})

	return nil, nil
}

// isInternalPkg returns true if the package is under an "internal" path segment.
func isInternalPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	return strings.Contains(pkgPath, "/internal/") ||
		strings.HasSuffix(pkgPath, "/internal") ||
		strings.HasPrefix(pkgPath, "internal/") ||
		pkgPath == "internal"
}

// isExemptPkg returns true if the package path matches an exempt suffix.
func isExemptPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	for _, exempt := range exemptPkgPaths {
		if strings.HasSuffix(pkgPath, exempt) || strings.Contains(pkgPath, exempt) {
			return true
		}
	}
	return false
}

// buildSuppressedLocs collects all "filename:line" keys that should be skipped due to
// suppression annotations (file-level, function-level, or inline).
// Uses "filename:line" composite keys to prevent cross-file false suppression
// (different files in the same package may share line numbers).
func buildSuppressedLocs(pass *analysis.Pass) map[string]bool {
	suppressed := make(map[string]bool)

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		// Check file-level suppression in first 20 lines.
		if hasFileSuppression(file, pass) {
			// Suppress entire file — mark all lines of THIS file only.
			start := pass.Fset.Position(file.Pos()).Line
			end := pass.Fset.Position(file.End()).Line
			for i := start; i <= end; i++ {
				suppressed[fmt.Sprintf("%s:%d", filename, i)] = true
			}
			continue // move on to the next file in this package
		}

		// Check function-level suppression in doc comments.
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Body == nil {
				continue
			}
			if funcDecl.Doc != nil {
				for _, c := range funcDecl.Doc.List {
					if hasSuppressionTag(c.Text) {
						start := pass.Fset.Position(funcDecl.Body.Pos()).Line
						end := pass.Fset.Position(funcDecl.Body.End()).Line
						for i := start; i <= end; i++ {
							suppressed[fmt.Sprintf("%s:%d", filename, i)] = true
						}
					}
				}
			}
		}

		// Check inline suppression comments.
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				if hasSuppressionTag(c.Text) {
					line := pass.Fset.Position(c.Pos()).Line
					suppressed[fmt.Sprintf("%s:%d", filename, line)] = true
					suppressed[fmt.Sprintf("%s:%d", filename, line+1)] = true
				}
			}
		}
	}

	return suppressed
}

// hasFileSuppression checks whether the file has a suppression tag in the first 20 lines.
func hasFileSuppression(file *ast.File, pass *analysis.Pass) bool {
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			pos := pass.Fset.Position(c.Pos())
			line := pos.Line
			if line > 20 {
				break
			}
			// File-level suppressions should be standalone comments.
			// Ignore trailing inline comments on declarations/statements.
			if pos.Column != 1 {
				continue
			}
			if hasSuppressionTag(c.Text) {
				return true
			}
		}
	}
	return false
}

func hasSuppressionTag(comment string) bool {
	for _, tag := range suppressionTags {
		if strings.Contains(comment, tag) {
			return true
		}
	}
	return false
}
