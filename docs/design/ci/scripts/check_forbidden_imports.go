// scripts/ci/check_forbidden_imports.go

/*
禁止导入检查 - CI 强制执行

🛑 禁止规则：
1. 禁止导入 fake client 相关包（测试文件除外）
2. 禁止硬编码 kubeconfig 路径
3. 禁止导入已弃用的包

这是架构治理的一部分，确保代码质量和安全性。
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
	"k8s.io/client-go/kubernetes/fake":   "使用 Mock Provider 替代 fake client",
	"kubevirt.io/client-go/kubevirt/fake": "使用 Mock Provider 替代 fake client",
	"gorm.io/gorm":                        "已切换到 Ent ORM，禁止使用 GORM",
	"gorm.io/driver/mysql":                "已切换到 PostgreSQL，禁止使用 MySQL",
	"gorm.io/driver/sqlite":               "已切换到 PostgreSQL，禁止使用 SQLite",
}

// 禁止的硬编码字符串模式
var forbiddenPatterns = []string{
	"/root/.kube/config",
	"/home/",
	"~/.kube/config",
}

func main() {
	var errors []string

	// 遍历代码目录
	for _, dir := range []string{"cmd", "internal", "pkg"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// 跳过目录、测试文件
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}

			// 检查导入
			for _, imp := range node.Imports {
				importPath := strings.Trim(imp.Path.Value, `"`)
				if reason, forbidden := forbiddenImports[importPath]; forbidden {
					pos := fset.Position(imp.Pos())
					errors = append(errors, fmt.Sprintf(
						"%s:%d: 禁止导入 %s - %s",
						path, pos.Line, importPath, reason,
					))
				}
			}

			// 检查硬编码字符串
			ast.Inspect(node, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind.String() != "STRING" {
					return true
				}

				value := strings.Trim(lit.Value, `"`)
				for _, pattern := range forbiddenPatterns {
					if strings.Contains(value, pattern) {
						pos := fset.Position(lit.Pos())
						errors = append(errors, fmt.Sprintf(
							"%s:%d: 禁止硬编码路径 %s - 使用环境变量或配置文件",
							path, pos.Line, pattern,
						))
					}
				}
				return true
			})

			return nil
		})

		if err != nil {
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
			os.Exit(1)
		}
	}

	if len(errors) > 0 {
		fmt.Println("❌ 发现禁止的导入或硬编码:")
		for _, e := range errors {
			fmt.Printf("  %s\n", e)
		}
		os.Exit(1)
	}

	fmt.Println("✅ 禁止导入检查通过")
}
