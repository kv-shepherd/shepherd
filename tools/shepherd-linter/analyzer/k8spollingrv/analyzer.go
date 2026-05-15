// Package k8spollingrv implements a go/analysis Analyzer that enforces ADR-0038's
// mandatory ResourceVersion requirement on K8s VM List/Get requests.
//
// ADR-0038 requires all K8s VM status polling requests to include the
// resourceVersion from the previous API response. This routes the request
// through the K8s watch cache instead of penetrating etcd.
//
// Violations detected in polling contexts:
//   - k8smetav1.ListOptions{} without ResourceVersion
//   - k8smetav1.GetOptions{} without ResourceVersion
//   - infracontract.ListOptions{} without ResourceVersion
//
// The analyzer uses AST inspection plus type information to find literal struct
// initializations of metav1.ListOptions, metav1.GetOptions, and Shepherd
// provider ListOptions that do not set a usable ResourceVersion field.
//
// Scope: files or functions whose names match polling/sync/health/live-status
// patterns are checked. Test files (_test.go) are excluded to avoid false
// positives from test fixtures.
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
	Doc:      "ADR-0038: enforces ResourceVersion field in K8s and Shepherd provider ListOptions/GetOptions struct literals to prevent etcd penetration",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// targetTypes maps the short struct names we look for.
// Both are from k8s.io/apimachinery/pkg/apis/meta/v1.
var targetTypes = map[string]bool{
	"ListOptions": true,
	"GetOptions":  true,
}

type functionScope struct {
	start token.Pos
	end   token.Pos
	name  string
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	scopes := collectFunctionScopes(pass.Files)

	nodeFilter := []ast.Node{
		(*ast.CompositeLit)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return
		}

		typeName := resolveOptionsType(pass, lit.Type)
		if typeName == "" {
			return
		}

		if !isPollingRelatedContext(pass, scopes, lit.Pos()) {
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

// resolveOptionsType returns the relevant options type name for real Kubernetes
// metav1 options or Shepherd provider contract ListOptions literals.
func resolveOptionsType(pass *analysis.Pass, expr ast.Expr) string {
	if pass.TypesInfo == nil {
		return ""
	}
	typ := pass.TypesInfo.TypeOf(expr)
	return resolveOptionsTypeFromType(typ)
}

func resolveOptionsTypeFromType(typ types.Type) string {
	switch t := typ.(type) {
	case *types.Named:
		return resolveOptionsTypeName(t.Obj())
	case *types.Alias:
		if name := resolveOptionsTypeName(t.Obj()); name != "" {
			return name
		}
		return resolveOptionsTypeFromType(types.Unalias(t))
	default:
		return ""
	}
}

func resolveOptionsTypeName(obj *types.TypeName) string {
	if obj == nil || obj.Pkg() == nil {
		return ""
	}

	pkgPath := obj.Pkg().Path()
	name := obj.Name()
	switch pkgPath {
	case "k8s.io/apimachinery/pkg/apis/meta/v1":
		if !targetTypes[name] {
			return ""
		}
	case "kv-shepherd.io/shepherd/internal/provider/infracontract",
		"kv-shepherd.io/shepherd/internal/provider":
		if name != "ListOptions" {
			return ""
		}
	default:
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

func collectFunctionScopes(files []*ast.File) []functionScope {
	var scopes []functionScope
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			scopes = append(scopes, functionScope{
				start: fn.Pos(),
				end:   fn.End(),
				name:  functionScopeName(fn),
			})
		}
	}
	return scopes
}

func functionScopeName(fn *ast.FuncDecl) string {
	name := fn.Name.Name
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return name
	}
	return receiverTypeName(fn.Recv.List[0].Type) + "." + name
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}

func isPollingRelatedContext(pass *analysis.Pass, scopes []functionScope, pos token.Pos) bool {
	fileName := pass.Fset.File(pos).Name()
	if isGoTestFile(fileName) {
		return false
	}
	if isPollingRelatedFile(fileName) {
		return true
	}
	for _, scope := range scopes {
		if pos >= scope.start && pos <= scope.end && isPollingRelatedIdentifier(scope.name) {
			return true
		}
	}
	return false
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
	return isPollingRelatedIdentifier(lower)
}

func isGoTestFile(path string) bool {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	return strings.HasSuffix(path, "_test.go")
}

func isPollingRelatedIdentifier(value string) bool {
	lower := strings.ToLower(value)
	pollingPatterns := []string{
		"live_status",
		"livestatus",
		"observed_live",
		"status_sync",
		"statussync",
		"polling",
		"poll_",
		"_poll",
		"sync_status",
		"syncstatus",
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
