// Package manualdi implements a go/analysis Analyzer that enforces the
// project's centralized hand-written dependency injection policy.
//
// Rules enforced:
//   - Forbid service/repository struct wiring outside internal/app/
//   - Forbid constructor-style dependency wiring outside the composition root
//   - Forbid Redis and Wire imports in runtime packages
//
// This replaces the blocking portions of docs/design/ci/scripts/check_manual_di.sh
// with syntax-aware analysis.
package manualdi

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var constructorPattern = regexp.MustCompile(`^New[A-Z][A-Za-z0-9]*(Service|Repository|UseCase|Gateway|Sender)$`)

// Analyzer is the exported go/analysis analyzer for centralized manual DI rules.
var Analyzer = &analysis.Analyzer{
	Name:     "manualdi",
	Doc:      "Enforces centralized hand-written dependency injection and forbids Wire/Redis imports",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	for _, file := range pass.Files {
		filename := normalizedFilename(pass, file.Pos())
		if isSkippedFile(filename) || !isRuntimeFile(filename) {
			continue
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch {
			case isRedisImport(importPath):
				pass.Reportf(
					imp.Pos(),
					"manual DI policy: Redis import %q is forbidden; keep runtime dependencies aligned with platform-approved primitives",
					importPath,
				)
			case isWireImport(importPath):
				pass.Reportf(
					imp.Pos(),
					"manual DI policy: Wire import %q is forbidden; keep dependency wiring centralized in internal/app",
					importPath,
				)
			}
		}
	}

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
		(*ast.UnaryExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		filename := normalizedFilename(pass, n.Pos())
		if isSkippedFile(filename) {
			return
		}

		switch node := n.(type) {
		case *ast.CallExpr:
			if !isConstructorScope(filename) || isCompositionRootFile(filename) {
				return
			}
			name := calledName(node.Fun)
			if constructorPattern.MatchString(name) {
				pass.Reportf(
					node.Pos(),
					"manual DI policy: constructor call %s() must stay in internal/app composition root",
					name,
				)
			}
		case *ast.UnaryExpr:
			if !isInternalFile(filename) || isCompositionRootFile(filename) {
				return
			}
			if node.Op != token.AND {
				return
			}
			lit, ok := node.X.(*ast.CompositeLit)
			if !ok {
				return
			}
			pkgAlias, typeName, ok := selectorType(lit.Type)
			if !ok {
				return
			}
			if pkgAlias == "service" || pkgAlias == "repository" {
				pass.Reportf(
					node.Pos(),
					"manual DI policy: decentralized %s.%s struct wiring is forbidden outside internal/app",
					pkgAlias,
					typeName,
				)
			}
		}
	})

	return nil, nil
}

func normalizedFilename(pass *analysis.Pass, pos token.Pos) string {
	file := pass.Fset.File(pos)
	if file == nil {
		return ""
	}
	return filepath.ToSlash(file.Name())
}

func isSkippedFile(filename string) bool {
	return strings.HasSuffix(filename, "_test.go") ||
		strings.HasSuffix(filename, "/mock.go") ||
		strings.HasSuffix(filename, "/testmain_test.go") ||
		strings.Contains(filename, "/testutil/")
}

func isRuntimeFile(filename string) bool {
	return isInternalFile(filename) || hasPathSegment(filename, "cmd")
}

func isInternalFile(filename string) bool {
	return hasPathSegment(filename, "internal")
}

func isConstructorScope(filename string) bool {
	return hasSubtree(filename, "internal/api") ||
		hasSubtree(filename, "internal/jobs") ||
		hasSubtree(filename, "internal/domain") ||
		hasSubtree(filename, "internal/infrastructure") ||
		hasPathSegment(filename, "cmd")
}

func isCompositionRootFile(filename string) bool {
	return hasSubtree(filename, "internal/app")
}

func hasPathSegment(filename, segment string) bool {
	return filename == segment ||
		strings.HasPrefix(filename, segment+"/") ||
		strings.Contains(filename, "/"+segment+"/")
}

func hasSubtree(filename, subtree string) bool {
	return filename == subtree ||
		strings.HasPrefix(filename, subtree+"/") ||
		strings.Contains(filename, "/"+subtree+"/")
}

func isRedisImport(importPath string) bool {
	return strings.Contains(importPath, "go-redis") || strings.HasPrefix(importPath, "github.com/redis")
}

func isWireImport(importPath string) bool {
	return strings.Contains(importPath, "google/wire") || strings.Contains(importPath, "goforj/wire")
}

func calledName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	default:
		return ""
	}
}

func selectorType(expr ast.Expr) (pkgAlias, typeName string, ok bool) {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return pkg.Name, sel.Sel.Name, true
}
