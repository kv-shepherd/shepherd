// Package entquerysafety enforces the ADR-0003 dynamic query boundary.
//
// Generated Ent predicates should be the default for runtime filtering,
// ordering, and pagination. Raw ent/dialect/sql and sqljson usage is allowed
// only in reviewed helper files for JSONB predicates or in database/test
// integration setup.
package entquerysafety

import (
	"go/ast"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer is the exported go/analysis.Analyzer for Ent query safety checks.
var Analyzer = &analysis.Analyzer{
	Name: "entquerysafety",
	Doc:  "ADR-0003: keeps raw Ent SQL predicates isolated in reviewed helpers; use generated Ent predicates for dynamic queries",
	Run:  run,
}

var restrictedEntSQLImports = map[string]bool{
	"entgo.io/ent/dialect/sql":         true,
	"entgo.io/ent/dialect/sql/sqljson": true,
}

var allowedRawEntSQLFileSuffixes = []string{
	"internal/api/handlers/audit_ent_predicates.go",
	"internal/api/handlers/member_ent_predicates.go",
	"internal/api/handlers/ticket_ent_predicates.go",
	"internal/jobs/helpers_ent_predicates.go",
	"internal/infrastructure/database.go",
	"internal/testutil/postgres_ent.go",
}

func run(pass *analysis.Pass) (interface{}, error) {
	if !isTargetPkg(pass) {
		return nil, nil
	}

	for _, file := range pass.Files {
		filename := normalizedFilename(pass, file)
		if strings.HasSuffix(filename, "_test.go") || isAllowedRawEntSQLFile(filename) {
			continue
		}

		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if restrictedEntSQLImports[importPath] {
				pass.Reportf(imp.Pos(),
					"raw Ent SQL import %q is restricted to reviewed *_ent_predicates.go helpers or database/test integration; use generated Ent predicates for dynamic queries",
					importPath)
			}
		}
	}

	return nil, nil
}

func normalizedFilename(pass *analysis.Pass, file *ast.File) string {
	return filepath.ToSlash(pass.Fset.File(file.Pos()).Name())
}

func isAllowedRawEntSQLFile(filename string) bool {
	for _, suffix := range allowedRawEntSQLFileSuffixes {
		if strings.HasSuffix(filename, suffix) {
			return true
		}
	}
	return false
}

func isTargetPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	pkgPath := pass.Pkg.Path()
	return strings.Contains(pkgPath, "/cmd/") ||
		strings.Contains(pkgPath, "/internal/") ||
		strings.Contains(pkgPath, "/pkg/") ||
		strings.HasPrefix(pkgPath, "cmd/") ||
		strings.HasPrefix(pkgPath, "internal/") ||
		strings.HasPrefix(pkgPath, "pkg/")
}
