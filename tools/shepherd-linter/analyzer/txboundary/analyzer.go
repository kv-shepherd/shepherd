// Package txboundary implements a go/analysis Analyzer that enforces
// transaction boundary rules in the Ent ORM service layer.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_transaction_boundary.go` (removed post-ADR-0039 migration).
//
// Rule enforced:
//   - The service layer (internal/service/) must not call Tx(), Commit(), or Rollback().
//   - Only the handler layer (internal/api/handlers/) may manage transactions.
//   - Use the WithTx() helper in handlers to scope transactions correctly.
//
// Applies to: internal/service/ packages (non-test files).
package txboundary

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for transaction boundary checking.
var Analyzer = &analysis.Analyzer{
	Name:     "txboundary",
	Doc:      "Enforces that service layer code does not call Tx/Commit/Rollback; transactions belong in handlers",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// transactionMethods are Ent ORM methods that manage transactions.
var transactionMethods = map[string]bool{
	"Tx":       true, // entClient.Tx(ctx)
	"Commit":   true,
	"Rollback": true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside internal/service packages.
	if !isServicePkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		// Skip test files.
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}

		if transactionMethods[sel.Sel.Name] {
			pass.Reportf(call.Pos(),
				"service layer must not call %s(): transactions must be managed in the handler layer using WithTx()",
				sel.Sel.Name)
		}
	})

	return nil, nil
}

// isServicePkg returns true if the package is under internal/service.
func isServicePkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	return strings.Contains(pkgPath, "/service") ||
		strings.Contains(pkgPath, "/service/") ||
		strings.HasSuffix(pkgPath, "/service")
}
