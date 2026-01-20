// scripts/ci/check_semaphore_usage.go

/*
信号量使用检查 - CI 强制执行

🛑 检查规则：
1. semaphore.Acquire() 必须配对 Release()
2. Release 必须使用 defer（防止 panic 导致泄漏）
3. 检测可能的信号量泄漏

检测模式：
- 搜索 Acquire 调用
- 验证同一函数内有配对的 defer ... Release()
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

type funcInfo struct {
	name          string
	hasAcquire    bool
	hasDefer      bool
	acquireLine   int
	releaseLine   int
	hasDeferRelease bool
}

func main() {
	var errors []string

	for _, dir := range []string{"internal"} {
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

			// 遍历所有函数
			ast.Inspect(node, func(n ast.Node) bool {
				funcDecl, ok := n.(*ast.FuncDecl)
				if !ok {
					return true
				}

				info := analyzeFuncForSemaphore(funcDecl, fset)
				if info.hasAcquire && !info.hasDeferRelease {
					errors = append(errors, fmt.Sprintf(
						"%s:%d: 函数 %s() 调用了 Acquire() 但未使用 defer Release()",
						path, info.acquireLine, info.name,
					))
				}

				return true
			})

			return nil
		})

		if err != nil {
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
		}
	}

	if len(errors) > 0 {
		fmt.Println("❌ 发现信号量使用问题:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 正确模式:")
		fmt.Println("  if err := sem.Acquire(ctx, 1); err != nil { return err }")
		fmt.Println("  defer sem.Release(1)")
		os.Exit(1)
	}

	fmt.Println("✅ 信号量使用检查通过")
}

func analyzeFuncForSemaphore(funcDecl *ast.FuncDecl, fset *token.FileSet) funcInfo {
	info := funcInfo{name: funcDecl.Name.Name}

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
				if sel.Sel.Name == "Acquire" {
					info.hasAcquire = true
					info.acquireLine = fset.Position(node.Pos()).Line
				}
				if sel.Sel.Name == "Release" {
					info.releaseLine = fset.Position(node.Pos()).Line
				}
			}
		case *ast.DeferStmt:
			info.hasDefer = true
			// 检查 defer 的是否是 Release
			if call, ok := node.Call.Fun.(*ast.SelectorExpr); ok {
				if call.Sel.Name == "Release" {
					info.hasDeferRelease = true
				}
			}
			// 也检查 defer func() { ... Release() }
			if funcLit, ok := node.Call.Fun.(*ast.FuncLit); ok {
				ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
					if call, ok := inner.(*ast.CallExpr); ok {
						if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
							if sel.Sel.Name == "Release" {
								info.hasDeferRelease = true
							}
						}
					}
					return true
				})
			}
		}
		return true
	})

	return info
}
