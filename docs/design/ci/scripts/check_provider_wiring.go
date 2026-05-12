//go:build ignore

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
)

const (
	infraFile = "internal/app/modules/infrastructure.go"
	vmFile    = "internal/app/modules/vm.go"
)

func main() {
	var violations []string

	infraAST, err := parseGoFile(infraFile)
	if err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", infraFile, err)
		os.Exit(1)
	}

	if !hasSelectorCall(infraAST, "provider", "NewKubeVirtProvider") {
		violations = append(violations, infraFile+": missing provider.NewKubeVirtProvider() wiring")
	}
	if hasSelectorCall(infraAST, "provider", "NewMockProvider") || hasIdentCall(infraAST, "NewMockProvider") {
		violations = append(violations, infraFile+": runtime infrastructure must not wire NewMockProvider()")
	}
	if !hasCompositeField(infraAST, "VMProvider") {
		violations = append(violations, infraFile+": Infrastructure struct return must assign VMProvider")
	}

	vmAST, err := parseGoFile(vmFile)
	if err != nil {
		fmt.Printf("FAIL: parse %s: %v\n", vmFile, err)
		os.Exit(1)
	}

	if !hasNilCheck(vmAST, "infra.VMProvider") {
		violations = append(violations, vmFile+": missing nil-check for infra.VMProvider")
	}
	if !hasMockTypeRejection(vmAST) {
		violations = append(violations, vmFile+`: missing explicit rejection for infra.VMProvider.Type() == "mock"`)
	}
	if !hasNewVMServiceFromInfraProvider(vmAST) {
		violations = append(violations, vmFile+": vm service must be wired from infra.VMProvider")
	}

	if len(violations) > 0 {
		fmt.Println("FAIL: provider wiring check failed")
		for _, v := range violations {
			fmt.Println(" -", v)
		}
		fmt.Println("Rule: runtime must wire real KubeVirt provider and reject mock provider in module composition root.")
		os.Exit(1)
	}

	fmt.Println("OK: provider wiring check passed")
}

func parseGoFile(path string) (*ast.File, error) {
	fset := token.NewFileSet()
	return parser.ParseFile(fset, path, nil, 0)
}

func hasSelectorCall(file *ast.File, qualifier, name string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != name {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == qualifier {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasIdentCall(file *ast.File, name string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasCompositeField(file *ast.File, fieldName string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == fieldName {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasNilCheck(file *ast.File, selector string) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(*ast.BinaryExpr)
		if !ok || expr.Op != token.EQL {
			return true
		}
		if (exprText(expr.X) == selector && exprText(expr.Y) == "nil") ||
			(exprText(expr.Y) == selector && exprText(expr.X) == "nil") {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasMockTypeRejection(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		expr, ok := n.(*ast.BinaryExpr)
		if !ok || expr.Op != token.EQL {
			return true
		}
		if (isInfraProviderTypeCall(expr.X) && stringLiteralValue(expr.Y) == "mock") ||
			(isInfraProviderTypeCall(expr.Y) && stringLiteralValue(expr.X) == "mock") {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasNewVMServiceFromInfraProvider(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "NewVMService" {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "service" {
			return true
		}
		if exprText(call.Args[0]) == "infra.VMProvider" {
			found = true
			return false
		}
		return true
	})
	return found
}

func isInfraProviderTypeCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Type" && exprText(sel.X) == "infra.VMProvider"
}

func stringLiteralValue(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return value
}

func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		left := exprText(e.X)
		if left == "" {
			return e.Sel.Name
		}
		return left + "." + e.Sel.Name
	}
	return ""
}
