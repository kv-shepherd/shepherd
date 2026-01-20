// scripts/ci/check_validate_spec.go

/*
ValidateSpec 事务检查 - CI 强制执行

🛑 检查规则：
事务回调内禁止调用 ValidateSpec() 方法

原因：
- ValidateSpec 可能调用 K8s API 验证资源
- 事务内调用会导致长事务、连接占用
- 应在事务开启前完成验证

正确模式：
  // 1. 事务外验证
  if err := service.ValidateAndPrepare(ctx, spec); err != nil {
      return err
  }
  
  // 2. 事务内只写数据库
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
		// 检查是否进入事务回调
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

		// 如果在事务内，检查是否调用 ValidateSpec
		if v.inTxFunc {
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "ValidateSpec" || sel.Sel.Name == "ValidateAndPrepare" {
					pos := v.fset.Position(node.Pos())
					v.errors = append(v.errors, fmt.Sprintf(
						"%s:%d: 事务内禁止调用 %s() - 验证应在事务外完成",
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
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
		}
	}

	if len(allErrors) > 0 {
		fmt.Println("❌ 发现事务内调用验证方法:")
		for _, e := range allErrors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 正确做法: 在 WithTx() 调用之前完成 ValidateAndPrepare()")
		os.Exit(1)
	}

	fmt.Println("✅ ValidateSpec 事务检查通过")
}
