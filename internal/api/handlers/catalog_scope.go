package handlers

import (
	"fmt"

	"kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	enttemplate "kv-shepherd.io/shepherd/ent/template"
	"kv-shepherd.io/shepherd/internal/service"
)

func normalizeCatalogScopeInput(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	scope := service.NormalizeCatalogScope(*raw)
	if scope == "" {
		return "", fmt.Errorf("catalog_scope cannot be empty")
	}
	if !service.IsValidCatalogScope(scope) {
		return "", fmt.Errorf("catalog_scope must be one of unclassified, test, prod, all")
	}
	return scope, nil
}

func visibleTemplateCatalogScopes(vis namespaceVisibility) []enttemplate.CatalogScope {
	if !vis.restricted {
		return []enttemplate.CatalogScope{
			enttemplate.CatalogScopeTest,
			enttemplate.CatalogScopeProd,
			enttemplate.CatalogScopeAll,
		}
	}
	if len(vis.envs) == 0 {
		return nil
	}

	scopes := []enttemplate.CatalogScope{enttemplate.CatalogScopeAll}
	for _, env := range vis.envs {
		switch env {
		case namespaceregistry.EnvironmentTest:
			scopes = append(scopes, enttemplate.CatalogScopeTest)
		case namespaceregistry.EnvironmentProd:
			scopes = append(scopes, enttemplate.CatalogScopeProd)
		}
	}
	return dedupeTemplateScopes(scopes)
}

func visibleInstanceSizeCatalogScopes(vis namespaceVisibility) []instancesize.CatalogScope {
	if !vis.restricted {
		return []instancesize.CatalogScope{
			instancesize.CatalogScopeTest,
			instancesize.CatalogScopeProd,
			instancesize.CatalogScopeAll,
		}
	}
	if len(vis.envs) == 0 {
		return nil
	}

	scopes := []instancesize.CatalogScope{instancesize.CatalogScopeAll}
	for _, env := range vis.envs {
		switch env {
		case namespaceregistry.EnvironmentTest:
			scopes = append(scopes, instancesize.CatalogScopeTest)
		case namespaceregistry.EnvironmentProd:
			scopes = append(scopes, instancesize.CatalogScopeProd)
		}
	}
	return dedupeInstanceSizeScopes(scopes)
}

func dedupeTemplateScopes(scopes []enttemplate.CatalogScope) []enttemplate.CatalogScope {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[enttemplate.CatalogScope]struct{}, len(scopes))
	out := make([]enttemplate.CatalogScope, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}

func dedupeInstanceSizeScopes(scopes []instancesize.CatalogScope) []instancesize.CatalogScope {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[instancesize.CatalogScope]struct{}, len(scopes))
	out := make([]instancesize.CatalogScope, 0, len(scopes))
	for _, scope := range scopes {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	return out
}
