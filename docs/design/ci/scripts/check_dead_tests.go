// scripts/ci/check_dead_tests.go

/*
死测试检测 - CI 警告（不阻断）

检查规则：
1. 检测只有 t.Skip() 的测试
2. 检测被注释掉的测试逻辑
3. 检测空函数体的测试

这是警告级别，不会阻断 CI。
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

type deadTest struct {
	file   string
	line   int
	name   string
	reason string
}

func main() {
	var warnings []deadTest

	for _, dir := range []string{"internal", "pkg", "cmd"} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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

				if !strings.HasPrefix(funcDecl.Name.Name, "Test") {
					continue
				}

				// 检查空函数体
				if funcDecl.Body == nil || len(funcDecl.Body.List) == 0 {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "空函数体",
					})
					continue
				}

				// 检查只有 t.Skip 的测试
				if isOnlySkip(funcDecl.Body) {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "只有 t.Skip()",
					})
					continue
				}

				// 检查只有 TODO 注释的测试
				if hasOnlyTODO(funcDecl.Body) {
					warnings = append(warnings, deadTest{
						file:   path,
						line:   fset.Position(funcDecl.Pos()).Line,
						name:   funcDecl.Name.Name,
						reason: "只有 TODO 注释，无实际测试",
					})
				}
			}

			return nil
		})
	}

	if len(warnings) > 0 {
		fmt.Println("⚠️ 发现可能的死测试（需人工确认）:")
		for _, w := range warnings {
			fmt.Printf("  %s:%d: %s - %s\n", w.file, w.line, w.name, w.reason)
		}
		fmt.Println("\n这些测试可能需要补充实现或删除。")
		// 不退出，只警告
	} else {
		fmt.Println("✅ 死测试检测通过")
	}
}

func isOnlySkip(body *ast.BlockStmt) bool {
	if len(body.List) == 0 {
		return false
	}

	// 检查是否所有语句都是 Skip
	for _, stmt := range body.List {
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			return false
		}
		call, ok := exprStmt.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (sel.Sel.Name != "Skip" && sel.Sel.Name != "SkipNow") {
			return false
		}
	}
	return true
}

func hasOnlyTODO(body *ast.BlockStmt) bool {
	// 简化检查：如果函数体只有一个语句且是空语句或 TODO 相关
	if len(body.List) == 1 {
		if exprStmt, ok := body.List[0].(*ast.ExprStmt); ok {
			if _, ok := exprStmt.X.(*ast.BasicLit); ok {
				return true // 可能是字符串字面量如 "TODO"
			}
		}
	}
	return false
}
