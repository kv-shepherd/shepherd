// scripts/ci/check_legacy_orm.go

/*
遗留 ORM 检测 - CI 强制执行

🛑 禁止规则：
1. 禁止导入 gorm.io/gorm
2. 禁止使用任何 GORM 相关类型

本项目已迁移到 Ent ORM + PostgreSQL，禁止使用 GORM。
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

// 禁止导入的包
var forbiddenImports = map[string]string{
	"gorm.io/gorm":         "❌ 禁止使用 GORM - 本项目使用 Ent ORM",
	"gorm.io/driver/mysql": "❌ 禁止使用 MySQL 驱动 - 本项目使用 PostgreSQL + pgx",
	"gorm.io/driver/postgres": "❌ 禁止使用 GORM PostgreSQL 驱动 - 本项目使用 Ent + pgx",
	"github.com/go-gorm/gorm": "❌ 禁止使用 GORM - 本项目使用 Ent ORM",
}

func main() {
	dirs := []string{"internal", "cmd"}
	var errors []string

	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil // 目录不存在时跳过
				}
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

			checkForbiddenImports(fset, node, path, &errors)
			return nil
		})

		if err != nil {
			fmt.Printf("⚠️  遍历目录 %s 时出错: %v\n", dir, err)
		}
	}

	if len(errors) > 0 {
		fmt.Println("❌ 发现禁止的遗留 ORM 导入:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		fmt.Println("\n📋 本项目已迁移到 Ent ORM + PostgreSQL")
		fmt.Println("   请使用 ent.Client 替代 gorm.DB")
		os.Exit(1)
	}

	fmt.Println("✅ 遗留 ORM 检测通过")
}

func checkForbiddenImports(fset *token.FileSet, node *ast.File, path string, errors *[]string) {
	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"")

		if reason, forbidden := forbiddenImports[importPath]; forbidden {
			pos := fset.Position(imp.Pos())
			*errors = append(*errors, fmt.Sprintf(
				"%s:%d: %s (import: %s)",
				path, pos.Line, reason, importPath,
			))
		}
	}
}
