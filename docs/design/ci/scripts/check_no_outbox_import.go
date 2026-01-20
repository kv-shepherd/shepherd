/*
CI 检查脚本: 禁止 Outbox 导入和引用

🛑 禁止规则（ADR-006）：
1. 禁止导入任何 outbox 相关包
2. 禁止使用 OutboxWorker、OutboxTask 等类型
3. 禁止创建 outbox_tasks 表

使用 River Queue 替代：
- github.com/riverqueue/river
- internal/governance/river/
*/

package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	dirs := []string{"internal", "cmd"}
	var errors []string

	forbiddenPatterns := []string{
		"outbox",
		"OutboxWorker",
		"OutboxTask",
		"outbox_tasks",
	}

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				return nil
			}

			// 检查导入
			for _, imp := range node.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				for _, pattern := range forbiddenPatterns {
					if strings.Contains(strings.ToLower(importPath), strings.ToLower(pattern)) {
						pos := fset.Position(imp.Pos())
						errors = append(errors, fmt.Sprintf(
							"%s:%d: 禁止导入 outbox 相关包 '%s' - 使用 River Queue 替代 (ADR-006)",
							path, pos.Line, importPath,
						))
					}
				}
			}

			return nil
		})

		if err != nil {
			fmt.Printf("❌ 遍历目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	// 检查是否存在 outbox 目录
	outboxDirs := []string{
		"internal/governance/outbox",
		"internal/outbox",
	}
	for _, dir := range outboxDirs {
		if _, err := os.Stat(dir); err == nil {
			errors = append(errors, fmt.Sprintf(
				"目录 %s 存在 - 自建 Outbox 已废弃，应使用 River Queue (ADR-006)",
				dir,
			))
		}
	}

	if len(errors) > 0 {
		fmt.Println("❌ 发现禁止的 Outbox 引用:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 正确做法: 使用 github.com/riverqueue/river 和 internal/governance/river/")
		fmt.Println("📖 参考: decisions/ADR-006-unified-async-model.md")
		os.Exit(1)
	}

	fmt.Println("✅ Outbox 检查通过 - 未发现禁止的 Outbox 引用")
}
