// Package k8stimeout implements a go/analysis Analyzer that enforces bounded
// contexts on direct Kubernetes client calls in the provider layer.
//
// Scope is intentionally narrow:
//   - provider-owned K8s client interface calls must pass the provider
//     operation-timeout context (`opCtx`) as the first argument.
//   - ClusterHealthChecker.CheckCluster must pass its bounded probe context
//     (`opCtx`) to KubeVirt probing, capability detection, and storage-class
//     detection.
package k8stimeout

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for K8s operation-timeout checks.
var Analyzer = &analysis.Analyzer{
	Name:     "k8stimeout",
	Doc:      "enforces bounded operation-timeout contexts on direct provider-layer Kubernetes client calls",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

var k8sClientAccessors = map[string]bool{
	"Authorization":  true,
	"DataVolume":     true,
	"Events":         true,
	"KubeVirt":       true,
	"Namespaces":     true,
	"Nodes":          true,
	"Pods":           true,
	"PVC":            true,
	"SSA":            true,
	"StorageClass":   true,
	"StorageProfile": true,
	"VM":             true,
	"VMI":            true,
}

var k8sContextMethods = map[string]bool{
	"ApplyClusterScopedYAML":        true,
	"ApplyYAML":                     true,
	"CreateSelfSubjectAccessReview": true,
	"Delete":                        true,
	"DryRunApplyYAML":               true,
	"Get":                           true,
	"GetFeatureGates":               true,
	"GetVersion":                    true,
	"List":                          true,
	"ListClusterInstanceTypes":      true,
	"ListClusterPreferences":        true,
	"ListInstanceTypes":             true,
	"ListPreferences":               true,
	"Patch":                         true,
	"Pause":                         true,
	"Restart":                       true,
	"Start":                         true,
	"Stop":                          true,
	"Unpause":                       true,
}

var providerK8sClientInterfaces = map[string]bool{
	"AuthorizationClient":          true,
	"DataVolumeClient":             true,
	"DynamicSSAClient":             true,
	"EventClient":                  true,
	"InstanceTypeCatalogClient":    true,
	"KubeVirtCRClient":             true,
	"NamespaceClient":              true,
	"NodeClient":                   true,
	"PersistentVolumeClaimClient":  true,
	"PodClient":                    true,
	"StorageClassClient":           true,
	"StorageProfileClient":         true,
	"VirtualMachineClient":         true,
	"VirtualMachineInstanceClient": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	if !isProviderPackage(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	insp.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		if fn.Body == nil || isTestFile(pass, fn.Pos()) {
			return
		}

		receiver := receiverTypeName(fn)
		checkHealthChecker := receiver == "ClusterHealthChecker" && fn.Name.Name == "CheckCluster"

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			if node == nil {
				return false
			}
			if _, ok := node.(*ast.FuncLit); ok {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}

			if callName, ok := providerK8sClientCall(pass, call); ok {
				requireOpCtx(pass, call, callName)
				return true
			}

			if checkHealthChecker && healthDelegationCall(call) {
				requireOpCtx(pass, call, renderedCallName(call))
			}
			return true
		})
	})

	return nil, nil
}

func providerK8sClientCall(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !k8sContextMethods[sel.Sel.Name] || len(call.Args) == 0 {
		return "", false
	}
	if iface, ok := providerK8sClientInterfaceName(pass, sel.X); ok {
		if accessor, ok := providerClusterClientAccessor(pass, sel.X); ok {
			return fmt.Sprintf("%s().%s", accessor, sel.Sel.Name), true
		}
		return fmt.Sprintf("%s.%s", iface, sel.Sel.Name), true
	}
	return "", false
}

func providerClusterClientAccessor(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	receiverCall, ok := expr.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	receiverSel, ok := receiverCall.Fun.(*ast.SelectorExpr)
	if !ok || !k8sClientAccessors[receiverSel.Sel.Name] {
		return "", false
	}
	if iface, ok := providerNamedTypeName(pass, receiverSel.X); !ok || iface != "KubeVirtClusterClient" {
		return "", false
	}
	return receiverSel.Sel.Name, true
}

func providerK8sClientInterfaceName(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	name, ok := providerNamedTypeName(pass, expr)
	if !ok || !providerK8sClientInterfaces[name] {
		return "", false
	}
	return name, true
}

func providerNamedTypeName(pass *analysis.Pass, expr ast.Expr) (string, bool) {
	if pass.TypesInfo == nil {
		return "", false
	}
	typ := pass.TypesInfo.TypeOf(expr)
	if typ == nil {
		return "", false
	}
	if ptr, ok := typ.(*types.Pointer); ok {
		typ = ptr.Elem()
	}
	named, ok := typ.(*types.Named)
	if !ok {
		return "", false
	}
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil || !isProviderPackagePath(obj.Pkg().Path()) {
		return "", false
	}
	return obj.Name(), true
}

func healthDelegationCall(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name == "Detect"
	case *ast.Ident:
		return fun.Name == "detectStorageClasses"
	default:
		return false
	}
}

func requireOpCtx(pass *analysis.Pass, call *ast.CallExpr, callName string) {
	if len(call.Args) == 0 {
		return
	}
	if ident, ok := call.Args[0].(*ast.Ident); ok && ident.Name == "opCtx" {
		return
	}
	pass.Reportf(call.Args[0].Pos(),
		"K8s operation %s must use bounded operation-timeout context opCtx, not raw caller context",
		callName,
	)
}

func renderedCallName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name
	case *ast.Ident:
		return fun.Name
	default:
		return "call"
	}
}

func isProviderPackage(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	return isProviderPackagePath(pass.Pkg.Path())
}

func isProviderPackagePath(pkgPath string) bool {
	return strings.HasSuffix(pkgPath, "/internal/provider") || strings.Contains(pkgPath, "/internal/provider/")
}

func isTestFile(pass *analysis.Pass, pos token.Pos) bool {
	file := pass.Fset.File(pos)
	if file == nil {
		return false
	}
	return strings.HasSuffix(file.Name(), "_test.go")
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return receiverExprName(fn.Recv.List[0].Type)
}

func receiverExprName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverExprName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	default:
		return ""
	}
}
