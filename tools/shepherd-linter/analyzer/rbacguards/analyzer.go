// Package rbacguards implements a go/analysis Analyzer for high-risk RBAC gate
// invariants that must remain explicit and fail-closed.
//
// The original governance scripts used file-content matching. This analyzer keeps
// the same policy surface, but matches real call expressions and string literal
// arguments instead of raw text snippets.
package rbacguards

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type callPattern struct {
	Callee     string
	StringArgs []string
	Label      string
	Message    string
}

type fileRule struct {
	Required  []callPattern
	Forbidden []callPattern
}

var rules = map[string]fileRule{
	"internal/app/router.go": {
		Forbidden: []callPattern{
			{
				Callee:     "RequirePermission",
				StringArgs: []string{"platform:admin"},
				Message:    "RBAC policy: route-level global platform:admin gate is forbidden; keep admin authorization handler-level and permission-granular",
			},
			{
				Callee:  "rbacAdminRoutes",
				Message: "RBAC policy: legacy rbacAdminRoutes middleware is forbidden; keep admin authorization explicit in handlers",
			},
		},
	},
	"internal/api/handlers/server_admin_rate_limit.go": {
		Forbidden: []callPattern{
			{
				Callee:  "requirePlatformAdminActor",
				Message: "RBAC policy: legacy requirePlatformAdminActor helper is forbidden; use explicit granular permissions instead",
			},
		},
	},
	"internal/api/handlers/member.go": {
		Required: []callPattern{
			{
				Callee:     "requireActorWithAnyGlobalPermission",
				StringArgs: []string{"user:manage", "rbac:read", "rbac:manage"},
				Label:      `requireActorWithAnyGlobalPermission(..., "user:manage", "rbac:read", "rbac:manage")`,
			},
		},
	},
	"internal/api/handlers/server_namespace.go": {
		Required: []callPattern{
			{
				Callee:     "requireActorWithAnyGlobalPermission",
				StringArgs: []string{"namespace:read", "namespace:write"},
				Label:      `requireActorWithAnyGlobalPermission(..., "namespace:read", "namespace:write")`,
			},
			{
				Callee:     "requireActorWithAnyGlobalPermission",
				StringArgs: []string{"namespace:write"},
				Label:      `requireActorWithAnyGlobalPermission(..., "namespace:write")`,
			},
		},
		Forbidden: []callPattern{
			{
				Callee:  "middleware.GetUserID",
				Message: "RBAC policy: legacy middleware.GetUserID access is forbidden in server_namespace.go; keep authorization explicit and actor-aware",
			},
		},
	},
	"internal/api/handlers/server_admin.go": {
		Required: []callPattern{
			{
				Callee:     "requireAnyGlobalPermission",
				StringArgs: []string{"vm:create", "template:read", "template:write"},
				Label:      `requireAnyGlobalPermission(..., "vm:create", "template:read", "template:write")`,
			},
			{
				Callee:     "requireAnyGlobalPermission",
				StringArgs: []string{"vm:create", "instance_size:read", "instance_size:write"},
				Label:      `requireAnyGlobalPermission(..., "vm:create", "instance_size:read", "instance_size:write")`,
			},
		},
	},
}

// Analyzer is the exported go/analysis analyzer for RBAC guard invariants.
var Analyzer = &analysis.Analyzer{
	Name: "rbacguards",
	Doc:  "Enforces explicit high-risk RBAC guard invariants using AST call matching",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		filename := normalizedFilename(pass, file.Pos())
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		rule, ok := matchingRule(filename)
		if !ok {
			continue
		}

		foundRequired := make([]bool, len(rule.Required))

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			callee := callName(call.Fun)
			stringArgs := collectStringArgs(call.Args)

			for _, forbidden := range rule.Forbidden {
				if forbidden.matches(callee, stringArgs) {
					pass.Reportf(call.Pos(), "%s", forbidden.Message)
				}
			}
			for i, required := range rule.Required {
				if required.matches(callee, stringArgs) {
					foundRequired[i] = true
				}
			}
			return true
		})

		for i, required := range rule.Required {
			if !foundRequired[i] {
				pass.Reportf(
					file.Package,
					"RBAC policy: %s missing required guard %s",
					filepath.Base(filename),
					required.Label,
				)
			}
		}
	}

	return nil, nil
}

func (p callPattern) matches(callee string, stringArgs []string) bool {
	if p.Callee != "" && !matchesCallee(callee, p.Callee) {
		return false
	}
	if p.StringArgs == nil {
		return true
	}
	if len(stringArgs) != len(p.StringArgs) {
		return false
	}
	for i := range p.StringArgs {
		if stringArgs[i] != p.StringArgs[i] {
			return false
		}
	}
	return true
}

func matchingRule(filename string) (fileRule, bool) {
	for rel, rule := range rules {
		if filename == rel || strings.HasSuffix(filename, "/"+rel) {
			return rule, true
		}
	}
	return fileRule{}, false
}

func normalizedFilename(pass *analysis.Pass, pos token.Pos) string {
	file := pass.Fset.File(pos)
	if file == nil {
		return ""
	}
	return filepath.ToSlash(file.Name())
}

func callName(expr ast.Expr) string {
	switch fn := expr.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		left := callName(fn.X)
		if left == "" {
			return fn.Sel.Name
		}
		return left + "." + fn.Sel.Name
	default:
		return ""
	}
}

func matchesCallee(actual, expected string) bool {
	return actual == expected || strings.HasSuffix(actual, "."+expected)
}

func collectStringArgs(args []ast.Expr) []string {
	values := make([]string, 0, len(args))
	for _, arg := range args {
		if s, ok := stringLiteral(arg); ok {
			values = append(values, s)
		}
	}
	return values
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, `"`), true
}
