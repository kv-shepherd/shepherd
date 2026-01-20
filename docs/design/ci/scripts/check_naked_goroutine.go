// scripts/ci/check_naked_goroutine.go

/*
裸 goroutine 检查 - CI 强制执行

🛑 禁止规则：
1. 禁止在非测试代码中使用 `go func()` 或 `go someFunc()`
2. 所有并发必须通过 Worker Pool 提交

例外情况（需代码审查批准）：
- 内部包中的基础设施代码（如 Worker Pool 实现本身）
- River Worker（底层组件，使用 sync.WaitGroup 管理生命周期）
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

func main() {
	internalDir := "internal"
	var errors []string

	// 豁免路径
	// 🏙️ 说明：
	// - Worker Pool 实现本身需要创建 goroutine
	// - River Worker 是底层基础组件，其内部 goroutine 由 sync.WaitGroup 保障
	// - 新增豁免需经过代码审查并更新此列表
	exemptPaths := map[string]bool{
		"internal/pkg/worker":       true, // Worker Pool 实现本身
		"internal/governance/river": true, // River Worker（底层组件）
	}

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录、测试文件、豁免路径
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for exempt := range exemptPaths {
			if strings.HasPrefix(path, exempt) {
				return nil
			}
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		// 检测 go 语句
		ast.Inspect(node, func(n ast.Node) bool {
			if goStmt, ok := n.(*ast.GoStmt); ok {
				pos := fset.Position(goStmt.Pos())
				errors = append(errors, fmt.Sprintf(
					"%s:%d: 禁止使用裸 goroutine - 请使用 Worker Pool (pools.General.Submit())",
					path, pos.Line,
				))
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
		fmt.Println("❌ 发现裸 goroutine 使用:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 正确做法: 使用 pools.General.Submit(func() { ... })")
		os.Exit(1)
	}

	fmt.Println("✅ 裸 goroutine 检查通过")
}
