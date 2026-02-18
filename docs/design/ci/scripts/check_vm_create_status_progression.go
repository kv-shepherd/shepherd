//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	const path = "internal/jobs/vm_create.go"

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", path, err)
		os.Exit(1)
	}

	hasVMStatusSuccess := false
	hasVMStatusFailed := false

	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		setStatus, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || setStatus.Sel == nil || setStatus.Sel.Name != "SetStatus" {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}
		if !isVMUpdateChain(setStatus.X) {
			return true
		}

		switch arg := call.Args[0].(type) {
		case *ast.Ident:
			if arg.Name == "targetVMStatus" {
				hasVMStatusSuccess = true
			}
		case *ast.SelectorExpr:
			if ident, ok := arg.X.(*ast.Ident); ok && ident.Name == "vm" && arg.Sel != nil && arg.Sel.Name == "StatusFAILED" {
				hasVMStatusFailed = true
			}
		}
		return true
	})

	if !hasVMStatusSuccess || !hasVMStatusFailed {
		fmt.Println("FAIL: vm_create status progression check failed")
		if !hasVMStatusSuccess {
			fmt.Println(" - missing VM status update on success path (expected SetStatus(targetVMStatus))")
		}
		if !hasVMStatusFailed {
			fmt.Println(" - missing VM status update on failure path (expected SetStatus(vm.StatusFAILED))")
		}
		fmt.Println("Rule: Stage 5.C must persist VM row status progression (CREATING -> RUNNING|FAILED).")
		os.Exit(1)
	}

	fmt.Println("OK: vm_create persists VM status on both success and failure paths")
}

func isVMUpdateChain(expr ast.Expr) bool {
	inner, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	updateSel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || updateSel.Sel == nil || updateSel.Sel.Name != "UpdateOneID" {
		return false
	}
	vmSel, ok := updateSel.X.(*ast.SelectorExpr)
	if !ok || vmSel.Sel == nil || vmSel.Sel.Name != "VM" {
		return false
	}
	return true
}
