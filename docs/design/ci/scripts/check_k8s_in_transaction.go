//go:build ignore

// scripts/ci/check_k8s_in_transaction.go

/*
K8s-in-transaction check - code review assistant

Note:
This check requires control-flow-level reasoning and cannot be fully validated with
simple AST pattern matching. It reports suspicious K8s API calls for manual review.

Restricted pattern (manual confirmation required):
1. KubeVirtProvider method calls inside transaction callbacks.
2. K8s operations inside WithTx(func(tx *ent.Tx) { ... }).
*/

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// K8s provider methods to flag when called inside a transaction callback.
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

// Tracks whether traversal is currently inside a transaction callback.
type inTransactionVisitor struct {
	fset            *token.FileSet
	path            string
	suspiciousCalls []string
	inTxCallback    bool
}

func (v *inTransactionVisitor) Visit(n ast.Node) ast.Visitor {
	switch node := n.(type) {
	case *ast.CallExpr:
		// Detect transaction entry points.
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "WithTx" || sel.Sel.Name == "Tx" {
				if len(node.Args) > 0 {
					if funcLit, ok := node.Args[len(node.Args)-1].(*ast.FuncLit); ok {
						innerVisitor := &inTransactionVisitor{
							fset:         v.fset,
							path:         v.path,
							inTxCallback: true,
						}
						ast.Walk(innerVisitor, funcLit.Body)
						v.suspiciousCalls = append(v.suspiciousCalls, innerVisitor.suspiciousCalls...)
						return nil // Skip recursive traversal of this node; handled by inner visitor.
					}
				}
			}
		}

		// If inside a transaction callback, check K8s provider calls.
		if v.inTxCallback {
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if k8sProviderMethods[sel.Sel.Name] {
					pos := v.fset.Position(node.Pos())
					v.suspiciousCalls = append(v.suspiciousCalls, fmt.Sprintf(
						"%s:%d: suspicious K8s API call inside transaction: %s()",
						v.path, pos.Line, sel.Sel.Name,
					))
				}
			}
		}
	}
	return v
}

func main() {
	var warnings []string

	for _, dir := range []string{"internal/api/handlers", "internal/service"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}

			visitor := &inTransactionVisitor{
				fset: fset,
				path: path,
			}
			ast.Walk(visitor, node)
			warnings = append(warnings, visitor.suspiciousCalls...)

			return nil
		})

		if err != nil {
			fmt.Printf("ERROR: failed to walk directory %s: %v\n", dir, err)
		}
	}

	if len(warnings) > 0 {
		fmt.Println("WARNING: found suspicious K8s calls inside transaction callbacks (manual review required):")
		for _, w := range warnings {
			fmt.Printf("  %s\n", w)
		}
		fmt.Println("\nRule: provider calls are forbidden inside transaction callbacks")
		fmt.Println("Recommended pattern: split into two phases - DB write inside transaction, K8s call outside transaction")
		// Warning only; do not fail.
	} else {
		fmt.Println("OK: K8s-in-transaction check passed (no suspicious calls found)")
	}
}
