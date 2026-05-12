// Package k8spollingrv implements a go/analysis Analyzer that enforces ADR-0038's
// mandatory ResourceVersion requirement on K8s VM List/Get requests.
//
// ADR-0038 requires all K8s VM status polling requests to include the
// resourceVersion from the previous API response. This routes the request
// through the K8s watch cache instead of penetrating etcd.
//
// Violations detected:
//   - k8smetav1.ListOptions{} without ResourceVersion in status-sync context
//   - k8smetav1.GetOptions{} without ResourceVersion in status-sync context
//
// The analyzer uses AST inspection plus type information to find literal struct
// initializations of metav1.ListOptions and metav1.GetOptions that do not set a
// usable ResourceVersion field.
//
// Scope: only files whose name matches polling/sync/health patterns are checked.
// Test files (_test.go) are excluded to avoid false positives from test fixtures.
package k8spollingrv

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported golangci-lint Analyzer for ADR-0038 ResourceVersion enforcement.
var Analyzer = &analysis.Analyzer{
	Name:     "k8spollingrv",
	Doc:      "ADR-0038: enforces ResourceVersion field in K8s ListOptions/GetOptions struct literals to prevent etcd penetration",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// targetTypes maps the short struct names we look for.
// Both are from k8s.io/apimachinery/pkg/apis/meta/v1.
var targetTypes = map[string]bool{
	"ListOptions": true,
	"GetOptions":  true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CompositeLit)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return
		}

		typeName := resolveMetav1OptionsType(pass, lit.Type)
		if typeName == "" {
			return
		}

		// Check if this file is in a provider/polling/sync context.
		// We only flag files that are clearly part of the K8s polling path.
		fileName := pass.Fset.File(lit.Pos()).Name()
		if !isPollingRelatedFile(fileName) {
			return
		}

		rvSet, rvZero := resourceVersionStatus(pass, lit)
		if rvZero {
			pass.Reportf(lit.Pos(),
				"ADR-0038: %s literal uses ResourceVersion \"0\"; "+
					"K8s polling requests MUST use the previous response resourceVersion, "+
					"or an empty resourceVersion only when establishing a baseline",
				typeName,
			)
			return
		}

		if rvSet {
			return
		}

		pass.Reportf(lit.Pos(),
			"ADR-0038: %s literal missing ResourceVersion field; "+
				"all K8s polling requests MUST include resourceVersion "+
				"to route through the watch cache and avoid etcd penetration",
			typeName,
		)
	})

	return nil, nil
}

// resolveMetav1OptionsType returns the K8s options type name for real
// k8s.io/apimachinery/pkg/apis/meta/v1 ListOptions/GetOptions literals.
func resolveMetav1OptionsType(pass *analysis.Pass, expr ast.Expr) string {
	if pass.TypesInfo == nil {
		return ""
	}
	typ := pass.TypesInfo.TypeOf(expr)
	named, ok := typ.(*types.Named)
	if !ok {
		return ""
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	if obj.Pkg().Path() != "k8s.io/apimachinery/pkg/apis/meta/v1" {
		return ""
	}
	name := obj.Name()
	if !targetTypes[name] {
		return ""
	}
	return name
}

// resourceVersionStatus returns whether ResourceVersion is present and whether
// it is explicitly set to the forbidden Kubernetes "0" sentinel.
func resourceVersionStatus(pass *analysis.Pass, lit *ast.CompositeLit) (bool, bool) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		ident, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		if ident.Name != "ResourceVersion" {
			continue
		}
		if isZeroResourceVersion(pass, kv.Value) {
			return true, true
		}
		return true, false
	}
	return false, false
}

func isZeroResourceVersion(pass *analysis.Pass, expr ast.Expr) bool {
	if pass.TypesInfo != nil {
		if tv, ok := pass.TypesInfo.Types[expr]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			return constant.StringVal(tv.Value) == "0"
		}
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(lit.Value)
	return err == nil && value == "0"
}

// isPollingRelatedFile returns true if the **filename** (not full path) suggests
// it is part of the K8s VM polling/sync infrastructure where ResourceVersion
// is mandatory. Only the base filename is checked to avoid false positives from
// directory names (e.g., the analyzer's own directory "k8spollingrv").
//
// Test files (_test.go) are excluded: test fixtures often construct bare
// ListOptions/GetOptions for mock setup where ResourceVersion is irrelevant.
func isPollingRelatedFile(path string) bool {
	// Extract base filename only.
	base := path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		base = path[idx+1:]
	}

	// Exclude test files: test fixtures construct bare ListOptions for mocks.
	if strings.HasSuffix(base, "_test.go") {
		return false
	}

	lower := strings.ToLower(base)

	// Files explicitly in the polling/sync/health-check path.
	// Maintenance: add new patterns if the project introduces new polling-related
	// file naming conventions. Prefer over-matching here — the analyzer's error
	// message clearly explains what to fix.
	pollingPatterns := []string{
		"status_sync",
		"polling",
		"poll_",
		"_poll",
		"sync_status",
		"health_check",
		"healthcheck",
		"reconcile",
		"watcher",
		"refresh",
		"heartbeat",
	}
	for _, p := range pollingPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
