// scripts/ci/check_test_assertions.go

/*
测试断言检查 - CI 强制执行

🛑 检查规则：
1. 测试函数必须包含断言调用
2. 禁止空测试、虚假覆盖

检测模式：
- 扫描 _test.go 中的 Test* 函数
- 检查是否包含 assert.*, require.*, t.Error, t.Fatal 等调用
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

var assertionCalls = map[string]bool{
	// testify
	"Equal":         true,
	"NotEqual":      true,
	"Nil":           true,
	"NotNil":        true,
	"True":          true,
	"False":         true,
	"Error":         true,
	"NoError":       true,
	"Contains":      true,
	"NotContains":   true,
	"Len":           true,
	"Empty":         true,
	"NotEmpty":      true,
	"Greater":       true,
	"Less":          true,
	"GreaterOrEqual": true,
	"LessOrEqual":   true,
	"Panics":        true,
	"NotPanics":     true,
	"Eventually":    true,
	"Never":         true,

	// testing.T
	"Errorf":  true,
	"Fatalf":  true,
	"Fail":    true,
	"FailNow": true,
	"Fatal":   true,
}

func main() {
	var emptyTests []string

	for _, dir := range []string{"internal", "pkg", "cmd"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, "_test.go") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}

			for _, decl := range node.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}

				// 只检查 Test* 函数
				if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
					continue
				}

				// 检查函数体是否包含断言
				if !hasAssertion(funcDecl.Body) {
					pos := fset.Position(funcDecl.Pos())
					emptyTests = append(emptyTests, fmt.Sprintf(
						"%s:%d: %s() 没有断言调用 - 可能是空测试",
						path, pos.Line, funcDecl.Name.Name,
					))
				}
			}

			return nil
		})

		if err != nil {
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
		}
	}

	if len(emptyTests) > 0 {
		fmt.Println("❌ 发现没有断言的测试函数:")
		for _, t := range emptyTests {
			fmt.Printf("  %s\n", t)
		}
		fmt.Println("\n📋 测试必须包含断言，如:")
		fmt.Println("  assert.NoError(t, err)")
		fmt.Println("  assert.Equal(t, expected, actual)")
		fmt.Println("  require.NotNil(t, result)")
		os.Exit(1)
	}

	fmt.Println("✅ 测试断言检查通过")
}

func hasAssertion(body *ast.BlockStmt) bool {
	if body == nil {
		return false
	}

	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// 检查方法调用 (assert.Equal, t.Error, etc.)
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if assertionCalls[fn.Sel.Name] {
				found = true
				return false
			}
		}

		return true
	})

	return found
}
