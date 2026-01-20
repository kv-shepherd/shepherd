// scripts/ci/check_repository_tests.go

/*
Repository 测试覆盖检查 - CI 强制执行

🛑 检查规则：
1. internal/repository/*.go 中所有导出方法必须有对应测试
2. 测试方法名格式: TestXxxRepository_MethodName

检测模式：
- 扫描 repository 包的所有导出方法
- 检查 _test.go 中是否有对应测试
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
	"unicode"
)

type method struct {
	receiver string
	name     string
	file     string
	line     int
}

func main() {
	repoDir := "internal/repository"

	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Println("⚠️ internal/repository/ 目录不存在，跳过检查")
		os.Exit(0)
	}

	// 收集所有导出方法
	methods := make(map[string]method)     // key: "ReceiverType.MethodName"
	testMethods := make(map[string]bool)   // 存在的测试

	fset := token.NewFileSet()

	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		node, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		isTestFile := strings.HasSuffix(path, "_test.go")

		for _, decl := range node.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			if isTestFile {
				// 收集测试方法
				if strings.HasPrefix(funcDecl.Name.Name, "Test") {
					testMethods[funcDecl.Name.Name] = true
				}
			} else {
				// 收集导出方法
				if funcDecl.Recv != nil && len(funcDecl.Recv.List) > 0 {
					// 获取接收器类型
					recvType := getReceiverTypeName(funcDecl.Recv.List[0].Type)
					if recvType != "" && isExported(funcDecl.Name.Name) {
						key := recvType + "." + funcDecl.Name.Name
						methods[key] = method{
							receiver: recvType,
							name:     funcDecl.Name.Name,
							file:     path,
							line:     fset.Position(funcDecl.Pos()).Line,
						}
					}
				}
			}
		}
		return nil
	})

	if err != nil {
		fmt.Printf("❌ 遍历目录失败: %v\n", err)
		os.Exit(1)
	}

	// 检查每个方法是否有测试
	var missing []string
	for key, m := range methods {
		// 生成预期的测试名
		expectedTestName := fmt.Sprintf("Test%s_%s", m.receiver, m.name)
		
		// 也接受其他格式
		altTestName1 := fmt.Sprintf("Test%s%s", m.receiver, m.name)
		altTestName2 := fmt.Sprintf("Test_%s_%s", m.receiver, m.name)

		if !testMethods[expectedTestName] && !testMethods[altTestName1] && !testMethods[altTestName2] {
			missing = append(missing, fmt.Sprintf(
				"%s:%d: %s 缺少测试 (预期: %s)",
				m.file, m.line, key, expectedTestName,
			))
		}
	}

	if len(missing) > 0 {
		fmt.Println("❌ 发现未测试的 Repository 方法:")
		for _, m := range missing {
			fmt.Printf("  %s\n", m)
		}
		fmt.Printf("\n共 %d 个方法缺少测试 (总计 %d 个导出方法)\n", len(missing), len(methods))
		os.Exit(1)
	}

	fmt.Printf("✅ Repository 测试覆盖检查通过 (%d 个方法均有测试)\n", len(methods))
}

func getReceiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func isExported(name string) bool {
	return len(name) > 0 && unicode.IsUpper(rune(name[0]))
}
