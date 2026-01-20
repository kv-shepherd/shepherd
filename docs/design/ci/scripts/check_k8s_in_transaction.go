// scripts/ci/check_k8s_in_transaction.go

/*
K8s 事务调用检查 - 代码审查辅助工具

🛑 说明：
此检查需要控制流分析（CFG），无法通过简单 AST 完全验证。
此脚本列出所有可疑的 K8s API 调用位置供人工审查。

禁区规则（需人工确认）：
1. 事务回调函数内禁止调用 KubeVirtProvider 方法
2. WithTx(func(tx *ent.Tx) { ... }) 内禁止 K8s 操作
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

// K8s Provider 方法（需要检测的调用）
var k8sProviderMethods = map[string]bool{
	"CreateVM":            true,
	"DeleteVM":            true,
	"UpdateVM":            true,
	"StartVM":             true,
	"StopVM":              true,
	"RestartVM":           true,
	"PauseVM":             true,
	"UnpauseVM":           true,
	"CreateResource":      true,
	"DeleteResource":      true,
	"UpdateResource":      true,
	"PerformAction":       true,
	"CreateVMSnapshot":    true,
	"DeleteVMSnapshot":    true,
	"RestoreVMFromSnapshot": true,
	"CloneVM":             true,
	"MigrateVM":           true,
}

// 检测是否在事务回调中
type inTransactionVisitor struct {
	fset            *token.FileSet
	path            string
	suspiciousCalls []string
	inTxCallback    bool
}

func (v *inTransactionVisitor) Visit(n ast.Node) ast.Visitor {
	switch node := n.(type) {
	case *ast.CallExpr:
		// 检查是否是事务调用
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "WithTx" || sel.Sel.Name == "Tx" {
				// 进入事务回调
				if len(node.Args) > 0 {
					if funcLit, ok := node.Args[len(node.Args)-1].(*ast.FuncLit); ok {
						innerVisitor := &inTransactionVisitor{
							fset:         v.fset,
							path:         v.path,
							inTxCallback: true,
						}
						ast.Walk(innerVisitor, funcLit.Body)
						v.suspiciousCalls = append(v.suspiciousCalls, innerVisitor.suspiciousCalls...)
						return nil // 不再递归处理这个节点
					}
				}
			}
		}

		// 如果在事务回调中，检查是否调用了 K8s 方法
		if v.inTxCallback {
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if k8sProviderMethods[sel.Sel.Name] {
					pos := v.fset.Position(node.Pos())
					v.suspiciousCalls = append(v.suspiciousCalls, fmt.Sprintf(
						"%s:%d: 疑似事务内调用 K8s API: %s()",
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
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
		}
	}

	if len(warnings) > 0 {
		fmt.Println("⚠️ 发现可疑的事务内 K8s 调用（需人工确认）:")
		for _, w := range warnings {
			fmt.Printf("  %s\n", w)
		}
		fmt.Println("\n📋 规则: 事务回调内禁止调用 Provider 方法")
		fmt.Println("📋 正确做法: 分离为两阶段 - 事务内只写 DB，事务外调用 K8s")
		// 不退出，只警告
	} else {
		fmt.Println("✅ K8s 事务调用检查通过（未发现可疑调用）")
	}
}
