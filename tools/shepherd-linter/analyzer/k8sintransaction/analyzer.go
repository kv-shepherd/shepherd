// Package k8sintransaction implements a go/analysis Analyzer that detects
// suspicious K8s provider method calls inside transaction callbacks (ADR-0006 / ADR-0012).
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy CI script `check_k8s_in_transaction.go`.
//
// Rule enforced (advisory — emits warnings, not hard errors):
//   - K8s provider methods (CreateVM, DeleteVM, UpdateVM…) must NOT be called
//     inside a WithTx(func(tx *ent.Tx){...}) or Tx(func(){...}) callback closure.
//   - Mixing external I/O (K8s API) with DB transactions violates ADR-0006:
//     a network timeout inside a transaction holds DB locks until rollback.
//
// Detection strategy (AST-level, conservative):
//   - Find CallExpr where Fun.Sel.Name ∈ {"WithTx","Tx"}.
//   - Identify the last argument as a *ast.FuncLit (the callback closure).
//   - Walk the closure body for CallExpr where Sel.Name ∈ k8sProviderMethods.
//   - Report any matches.
//
// Limitations (acknowledged, same as the original script):
//   - This is pattern-based, not a full control-flow / call-graph analysis.
//   - False positives are possible (different WithTx callers, method name collision).
//   - All reports require manual confirmation before fixing.
//
// Applies to: internal/api/handlers and internal/service packages (non-test files).
package k8sintransaction

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for K8s-in-transaction detection.
var Analyzer = &analysis.Analyzer{
	Name:     "k8sintransaction",
	Doc:      "ADR-0006/ADR-0012: detects K8s provider method calls inside DB transaction callbacks (advisory warning)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// k8sProviderMethods are K8s-side operations flagged when found inside a transaction closure.
// Source: same set as the legacy check_k8s_in_transaction.go script.
var k8sProviderMethods = map[string]bool{
	"CreateVM":              true,
	"DeleteVM":              true,
	"UpdateVM":              true,
	"StartVM":               true,
	"StopVM":                true,
	"RestartVM":             true,
	"PauseVM":               true,
	"UnpauseVM":             true,
	"CreateResource":        true,
	"DeleteResource":        true,
	"UpdateResource":        true,
	"PerformAction":         true,
	"CreateVMSnapshot":      true,
	"DeleteVMSnapshot":      true,
	"RestoreVMFromSnapshot": true,
	"CloneVM":               true,
	"MigrateVM":             true,
}

// txEntryPoints are method names that open a transaction callback.
// A transaction callback is the last argument *ast.FuncLit to these methods.
var txEntryPoints = map[string]bool{
	"WithTx": true, // helper wrapper (ADR-0012 recommended pattern)
	"Tx":     true, // ent.Client.Tx() (direct usage, discouraged in service layer)
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside handler and service packages.
	if !isTargetPkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// Walk all CallExpr nodes looking for transaction entry points.
	insp.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// Skip test files.
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		// Check if this call is a transaction entry point: WithTx(...) or Tx(...).
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		if !txEntryPoints[sel.Sel.Name] {
			return
		}

		// Find the last argument — it should be the callback FuncLit.
		if len(call.Args) == 0 {
			return
		}
		lastArg := call.Args[len(call.Args)-1]
		funcLit, ok := lastArg.(*ast.FuncLit)
		if !ok {
			return
		}

		// Walk the closure body for forbidden K8s provider method calls.
		ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
			if inner == nil {
				return false
			}
			innerCall, ok := inner.(*ast.CallExpr)
			if !ok {
				return true
			}
			innerSel, ok := innerCall.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if k8sProviderMethods[innerSel.Sel.Name] {
				pass.Reportf(innerCall.Pos(),
					"suspicious K8s API call %s() inside transaction callback (ADR-0006/ADR-0012): "+
						"K8s calls inside DB transactions hold locks on network timeout; "+
						"split into: DB write inside transaction, K8s call outside transaction",
					innerSel.Sel.Name)
			}
			return true
		})
	})

	return nil, nil
}

// isTargetPkg returns true if this package is under internal/api/handlers or internal/service.
// go/analysis provides pass.Pkg.Path() — we use exact path-segment boundary checks ("/handlers/"
// or suffix "/handlers") to prevent false matches on names like "/external-handlers/".
func isTargetPkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}
	pkgPath := pass.Pkg.Path()
	// Check for exact path segment: "/handlers" as suffix or "/handlers/" as infix.
	isHandlers := strings.HasSuffix(pkgPath, "/handlers") ||
		strings.Contains(pkgPath, "/handlers/")
	// Check for exact path segment: "/service" as suffix or "/service/" as infix.
	isService := strings.HasSuffix(pkgPath, "/service") ||
		strings.Contains(pkgPath, "/service/")
	return isHandlers || isService
}
