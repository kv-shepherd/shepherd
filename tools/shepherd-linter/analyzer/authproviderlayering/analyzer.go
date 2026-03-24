package authproviderlayering

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "authproviderlayering",
	Doc:  "Enforce core/edge/provider import and provider-specific branching boundaries for auth-provider runtime and directory sync code",
	Run:  run,
}

var coreFileSuffixes = []string{
	"/internal/service/external_auth.go",
	"/internal/service/directory_sync.go",
	"/internal/jobs/directory_sync_worker.go",
	"/internal/api/handlers/server_auth_external.go",
}

var edgeFileSuffixes = []string{
	"/internal/api/handlers/server_admin_directory_sync.go",
	"/internal/api/handlers/server_admin_runtime.go",
}

var edgeDirSuffixes = []string{
	"/internal/edge/",
}

var providerDirSuffixes = []string{
	"/internal/provider/",
}

var coreForbiddenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`case\s+"(wecom|oidc|ldap|azure|feishu)"`),
	regexp.MustCompile(`provider(ID|Type|Ent)?\s*==\s*"(wecom|oidc|ldap|azure|feishu)"`),
	regexp.MustCompile(`providerRequest\s*\[\s*"(departments|base_dn|selected_fields|groups|include_nested)"\s*\]`),
	regexp.MustCompile(`RequestSnapshot\s*\[\s*"(departments|base_dn|selected_fields|groups|include_nested)"\s*\]`),
}

var edgeForbiddenImportPatterns = []string{
	"/internal/repository",
	"/internal/usecase",
}

var providerForbiddenImportPatterns = []string{
	"/internal/api",
	"/internal/jobs",
	"/internal/service",
	"/internal/repository",
	"/internal/usecase",
	"/ent",
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		filename := filepath.ToSlash(pass.Fset.File(file.Pos()).Name())
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		switch {
		case hasSuffix(filename, coreFileSuffixes):
			checkCoreFile(pass, file, filename)
		case hasSuffix(filename, edgeFileSuffixes), containsAny(filename, edgeDirSuffixes):
			checkImports(pass, file, filename, edgeForbiddenImportPatterns, "edge workspace must not import repository/usecase packages directly")
		case containsAny(filename, providerDirSuffixes):
			checkImports(pass, file, filename, providerForbiddenImportPatterns, "provider implementation must not import API/service/jobs/repository/usecase/ent packages directly")
		}
	}

	return nil, nil
}

func checkCoreFile(pass *analysis.Pass, file *ast.File, filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	text := string(content)
	for _, pattern := range coreForbiddenPatterns {
		if match := pattern.FindString(text); match != "" {
			pass.Reportf(file.Package, "core auth-provider file must stay provider-neutral; found provider-specific fragment %q", match)
		}
	}
}

func checkImports(pass *analysis.Pass, file *ast.File, filename string, forbiddenPatterns []string, reason string) {
	for _, imp := range file.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		for _, pattern := range forbiddenPatterns {
			if strings.Contains(importPath, pattern) {
				pass.Reportf(imp.Pos(), "%s: forbidden import %q", reason, importPath)
			}
		}
	}
}

func hasSuffix(value string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func containsAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(value, pattern) {
			return true
		}
	}
	return false
}
