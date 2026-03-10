package service

import (
	"strings"

	"kv-shepherd.io/shepherd/ent/namespaceregistry"
)

const (
	CatalogScopeUnclassified = "unclassified"
	CatalogScopeTest         = "test"
	CatalogScopeProd         = "prod"
	CatalogScopeAll          = "all"
)

// NormalizeCatalogScope trims and lowercases a catalog scope value.
func NormalizeCatalogScope(scope string) string {
	return strings.TrimSpace(strings.ToLower(scope))
}

// IsValidCatalogScope returns true when scope is one of the supported values.
func IsValidCatalogScope(scope string) bool {
	switch NormalizeCatalogScope(scope) {
	case CatalogScopeUnclassified, CatalogScopeTest, CatalogScopeProd, CatalogScopeAll:
		return true
	default:
		return false
	}
}

// CatalogScopeMatchesEnvironment returns true when the catalog scope is visible
// to the given namespace environment. Unclassified never matches user request
// flows; "all" matches both test and prod.
func CatalogScopeMatchesEnvironment(scope string, env namespaceregistry.Environment) bool {
	switch NormalizeCatalogScope(scope) {
	case CatalogScopeAll:
		return true
	case CatalogScopeTest, CatalogScopeProd:
		return NormalizeCatalogScope(scope) == strings.ToLower(string(env))
	default:
		return false
	}
}
