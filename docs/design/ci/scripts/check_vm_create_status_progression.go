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
	const (
		createPath = "internal/jobs/vm_create.go"
		helperPath = "internal/jobs/helpers.go"
	)

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, createPath, nil, 0)
	if err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", createPath, err)
		os.Exit(1)
	}
	helperNode, err := parser.ParseFile(fset, helperPath, nil, 0)
	if err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", helperPath, err)
		os.Exit(1)
	}

	hasVMStatusSuccess := hasCompletedCreateVMStatusHelper(node) && helperPersistsCompletedVMStatus(helperNode)
	hasVMStatusFailed := hasFinalCreateFailureVMHelper(node) && helperPersistsFailedVMStatus(helperNode)

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
		}
		return true
	})

	if !hasVMStatusSuccess || !hasVMStatusFailed {
		fmt.Println("FAIL: vm_create status progression check failed")
		if !hasVMStatusSuccess {
			fmt.Println(" - missing VM status update on success path (expected SetStatus(targetVMStatus) or transactional completed-state helper with targetVMStatus)")
		}
		if !hasVMStatusFailed {
			fmt.Println(" - missing VM status update on failure path (expected persistFinalCreateFailure to call the transactional VM/Event/Ticket failure helper with entvm.StatusFAILED)")
		}
		fmt.Println("Rule: Stage 5.C must persist VM row status progression (CREATING -> RUNNING|FAILED).")
		os.Exit(1)
	}

	fmt.Println("OK: vm_create persists VM status on both success and failure paths")
}

func hasCompletedCreateVMStatusHelper(node *ast.File) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "persistCompletedEventTicketAndVMStatusByEvent" || len(call.Args) < 5 {
			return true
		}
		arg, ok := call.Args[4].(*ast.Ident)
		if ok && arg.Name == "targetVMStatus" {
			found = true
			return false
		}
		return true
	})
	return found
}

func helperPersistsCompletedVMStatus(node *ast.File) bool {
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "persistCompletedEventTicketAndVMStatusByEvent" || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			setStatus, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || setStatus.Sel == nil || setStatus.Sel.Name != "SetStatus" || len(call.Args) != 1 {
				return true
			}
			arg, ok := call.Args[0].(*ast.Ident)
			if ok && arg.Name == "vmStatus" && isVMUpdateChain(setStatus.X) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}

func hasFinalCreateFailureVMHelper(node *ast.File) bool {
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "persistFinalCreateFailure" || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if ok && (ident.Name == "persistFailedEventTicketAndVMByEvent" ||
				ident.Name == "persistFailedEventTicketAndVMByEventUnlessDeleting") {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}

func helperPersistsFailedVMStatus(node *ast.File) bool {
	if helperSetsFailedVMStatus(node, "persistFailedEventTicketAndVMByEventUnlessDeleting") {
		return true
	}
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != "persistFailedEventTicketAndVMByEvent" || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "persistFailedEventTicketAndMaybeVMByEvent" {
				return true
			}
			for _, arg := range call.Args {
				sel, ok := arg.(*ast.SelectorExpr)
				if !ok || sel.Sel == nil || sel.Sel.Name != "StatusFAILED" {
					continue
				}
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "entvm" {
					found = true
					return false
				}
			}
			return true
		})
		return found
	}
	return false
}

func helperSetsFailedVMStatus(node *ast.File, helperName string) bool {
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil || fn.Name.Name != helperName || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			setStatus, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || setStatus.Sel == nil || setStatus.Sel.Name != "SetStatus" || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Args[0].(*ast.SelectorExpr)
			if !ok || sel.Sel == nil || sel.Sel.Name != "StatusFAILED" {
				return true
			}
			if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "entvm" && isVMUpdateChain(setStatus.X) {
				found = true
				return false
			}
			return true
		})
		return found
	}
	return false
}

func isVMUpdateChain(expr ast.Expr) bool {
	inner, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	updateSel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || updateSel.Sel == nil {
		return false
	}
	if updateSel.Sel.Name != "UpdateOneID" {
		return isVMUpdateChain(updateSel.X)
	}
	vmSel, ok := updateSel.X.(*ast.SelectorExpr)
	if !ok || vmSel.Sel == nil || vmSel.Sel.Name != "VM" {
		return false
	}
	return true
}
