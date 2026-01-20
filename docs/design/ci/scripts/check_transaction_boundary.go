// scripts/ci/check_transaction_boundary.go

/*
事务边界检查 - CI 强制执行

🛑 禁区规则：
1. Service 层（internal/service/）禁止调用 client.Tx()
2. Service 层禁止直接使用事务 API
3. 只有 Handler 层（internal/api/handlers/）可以管理事务

适用于 Ent ORM 事务模式。
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

// Ent 事务相关方法
var transactionMethods = map[string]bool{
	"Tx":       true, // client.Tx(ctx)
	"Commit":   true,
	"Rollback": true,
}

func main() {
	serviceDir := "internal/service"
	var errors []string

	err := filepath.Walk(serviceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil // 跳过解析失败的文件
		}

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if transactionMethods[sel.Sel.Name] {
					pos := fset.Position(call.Pos())
					errors = append(errors, fmt.Sprintf(
						"%s:%d: Service 层禁止调用 %s() - 事务应在 Handler 层管理",
						path, pos.Line, sel.Sel.Name,
					))
				}
			}
			return true
		})
		return nil
	})

	if err != nil {
		fmt.Printf("❌ 遍历目录失败: %v\n", err)
		os.Exit(1)
	}

	if len(errors) > 0 {
		fmt.Println("❌ 发现事务边界违规:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 正确做法: 在 Handler 层使用 WithTx() 辅助函数管理事务")
		os.Exit(1)
	}

	fmt.Println("✅ 事务边界检查通过")
}
