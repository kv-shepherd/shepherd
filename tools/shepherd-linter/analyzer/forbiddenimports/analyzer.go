// Package forbiddenimports implements a go/analysis Analyzer that detects
// forbidden package imports and hardcoded paths.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_forbidden_imports.go` (removed post-ADR-0039 migration).
//
// Rules enforced:
//  1. Forbidden package imports (fake clients, GORM, etc.)
//  2. Hardcoded kubeconfig path literals
//  3. Forbidden outbox package paths in production code
//
// Applies to: cmd/, internal/, pkg/ packages (non-test files).
package forbiddenimports

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for forbidden import detection.
var Analyzer = &analysis.Analyzer{
	Name:     "forbiddenimports",
	Doc:      "Detects forbidden package imports and hardcoded kubeconfig paths in production code",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// forbiddenImports maps import paths to the reason they are forbidden.
// Covers: check_forbidden_imports.go, check_no_gorm_import.go, check_no_outbox_import.go.
var forbiddenImports = map[string]string{
	// Fake/mock k8s clients — must use Mock Provider (ADR-0001)
	"k8s.io/client-go/kubernetes/fake":    "use Mock Provider instead of fake client (ADR-0001)",
	"kubevirt.io/client-go/kubevirt/fake": "use Mock Provider instead of fake client (ADR-0001)",
	// GORM is forbidden — project uses Ent ORM + PostgreSQL (check_no_gorm_import.go)
	"gorm.io/gorm":            "project uses Ent ORM; GORM is forbidden",
	"gorm.io/driver/mysql":    "project uses PostgreSQL; MySQL driver is forbidden",
	"gorm.io/driver/sqlite":   "project uses PostgreSQL; SQLite driver is forbidden",
	"gorm.io/driver/postgres": "use Ent + pgx instead of GORM PostgreSQL driver",
	"github.com/go-gorm/gorm": "project uses Ent ORM; GORM is forbidden",
	// Outbox pattern is forbidden — use River Queue (ADR-0006) (check_no_outbox_import.go)
	"kv-shepherd.io/shepherd/internal/governance/outbox": "use River Queue (github.com/riverqueue/river) instead of outbox pattern (ADR-0006)",
	"kv-shepherd.io/shepherd/internal/outbox":            "use River Queue instead of outbox pattern (ADR-0006)",
}

// forbiddenImportSubstrings are fuzzy-matched against any import path.
// Restores the detection capability of the original check_no_outbox_import.go script,
// which used strings.Contains(importPath, "outbox") for broad matching.
var forbiddenImportSubstrings = map[string]string{
	"outbox": "outbox pattern is forbidden; use River Queue (ADR-0006)",
}

// forbiddenPathPatterns are hardcoded path fragments that must not appear in string literals.
var forbiddenPathPatterns = []string{
	"/root/.kube/config",
	"~/.kube/config",
}

// targetPkgPrefixes constrains the analyzer to relevant packages.
var targetPkgPrefixes = []string{"cmd", "internal", "pkg"}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce in cmd/, internal/, pkg/ packages.
	if !isTargetPkg(pass) {
		return nil, nil
	}

	pkgPath := pass.Pkg.Path()

	// check_no_outbox_import.go also forbids keeping outbox packages in-tree.
	// Keep this behavior by rejecting package paths with an "outbox" segment.
	if isOutboxPkg(pkgPath) {
		reportOutboxPackage(pass, pkgPath)
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// --- Check 1: Forbidden import paths (exact + substring) ---
	for _, file := range pass.Files {
		// Skip test files.
		filename := pass.Fset.File(file.Pos()).Name()
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)

			// Exact match
			if reason, forbidden := forbiddenImports[importPath]; forbidden {
				pass.Reportf(imp.Pos(), "forbidden import %q: %s", importPath, reason)
				continue
			}

			// Substring/fuzzy match (e.g. any import containing "outbox")
			lowerPath := strings.ToLower(importPath)
			for substr, reason := range forbiddenImportSubstrings {
				if strings.Contains(lowerPath, strings.ToLower(substr)) {
					pass.Reportf(imp.Pos(), "forbidden import %q: %s", importPath, reason)
					break
				}
			}
		}
	}

	// --- Check 2: Hardcoded kubeconfig path string literals ---
	nodeFilter := []ast.Node{(*ast.BasicLit)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		lit, ok := n.(*ast.BasicLit)
		if !ok {
			return
		}
		// Only check string literals.
		if lit.Kind.String() != "STRING" {
			return
		}

		// Skip if this node is in a test file.
		pos := pass.Fset.Position(lit.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		value := strings.Trim(lit.Value, `"`)
		for _, pattern := range forbiddenPathPatterns {
			if strings.Contains(value, pattern) {
				pass.Reportf(lit.Pos(),
					"hardcoded path %q detected: use environment variables or config files instead", pattern)
			}
		}
	})

	return nil, nil
}

// isTargetPkg returns true if the package is under cmd/, internal/, or pkg/.
func isTargetPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	for _, prefix := range targetPkgPrefixes {
		if strings.HasPrefix(pkgPath, prefix+"/") ||
			pkgPath == prefix ||
			strings.Contains(pkgPath, "/"+prefix+"/") {
			return true
		}
	}
	return false
}

func isOutboxPkg(pkgPath string) bool {
	for _, seg := range strings.Split(strings.ToLower(pkgPath), "/") {
		if seg == "outbox" {
			return true
		}
	}
	return false
}

func reportOutboxPackage(pass *analysis.Pass, pkgPath string) {
	for _, file := range pass.Files {
		filename := pass.Fset.File(file.Pos()).Name()
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		pass.Reportf(file.Package,
			"forbidden package path %q: outbox pattern is deprecated; use River Queue (ADR-0006)",
			pkgPath)
		return
	}
}
