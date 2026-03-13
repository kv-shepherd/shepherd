//go:build ignore

// docs/design/ci/scripts/check_validate_spec.go

/*
ValidateSpec transaction check - CI enforced

Rule:
ValidateSpec()/ValidateAndPrepare() must not be called inside transaction callbacks.

Rationale:
- Validation may invoke K8s API calls.
- Running validation inside transactions can create long transactions and connection pressure.
- Validation should complete before transaction start.

Recommended pattern:
  // 1. Validate outside transaction
  if err := service.ValidateAndPrepare(ctx, spec); err != nil {
      return err
  }

  // 2. Persist inside transaction only
  err := WithTx(ctx, client, func(tx *ent.Tx) error {
      return service.CreateVMRecord(ctx, tx, spec)
  })
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

type txVisitor struct {
	fset     *token.FileSet
	path     string
	errors   []string
	inTxFunc bool
}

func (v *txVisitor) Visit(n ast.Node) ast.Visitor {
	switch node := n.(type) {
	case *ast.CallExpr:
		// Detect transaction callback boundaries.
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "WithTx" || sel.Sel.Name == "Transaction" {
				if len(node.Args) > 0 {
					if funcLit, ok := node.Args[len(node.Args)-1].(*ast.FuncLit); ok {
						innerVisitor := &txVisitor{
							fset:     v.fset,
							path:     v.path,
							inTxFunc: true,
						}
						ast.Walk(innerVisitor, funcLit.Body)
						v.errors = append(v.errors, innerVisitor.errors...)
						return nil
					}
				}
			}
		}

		// If inside transaction callback, flag validation calls.
		if v.inTxFunc {
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "ValidateSpec" || sel.Sel.Name == "ValidateAndPrepare" {
					pos := v.fset.Position(node.Pos())
					v.errors = append(v.errors, fmt.Sprintf(
						"%s:%d: %s() must not be called inside a transaction callback; validate before transaction",
						v.path, pos.Line, sel.Sel.Name,
					))
				}
			}
		}
	}
	return v
}

func main() {
	var allErrors []string

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

			visitor := &txVisitor{fset: fset, path: path}
			ast.Walk(visitor, node)
			allErrors = append(allErrors, visitor.errors...)

			return nil
		})

		if err != nil {
			fmt.Printf("ERROR: failed to walk directory %s: %v\n", dir, err)
		}
	}

	if len(allErrors) > 0 {
		fmt.Println("ERROR: found validation calls inside transaction callbacks:")
		for _, e := range allErrors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\nRecommended pattern: complete ValidateAndPrepare() before WithTx()")
		os.Exit(1)
	}

	fmt.Println("OK: ValidateSpec transaction check passed")
}
