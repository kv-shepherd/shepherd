//go:build ignore

// scripts/ci/check_river_job_args.go

/*
River Job Arguments Claim Check 验证 (ADR-0009)

规则：
1. River Job Args 结构体只应包含 EventID 字段
2. 禁止在 Job Args 中传递 vm_id, ticket_id 或其他业务 ID
3. Worker 通过 EventID 查询 DomainEvent 获取完整数据

误报处理：
- 某些 Job 可能有合理理由包含其他字段（如 batch_id）
- 使用 //nolint:river-claim-check 注释跳过检查
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

// 禁止在 Job Args 中出现的字段名
var forbiddenJobArgFields = map[string]bool{
	"VMID":       true,
	"VmID":       true,
	"VMId":       true,
	"vm_id":      true,
	"TicketID":   true,
	"ticket_id":  true,
	"ServiceID":  true,
	"service_id": true,
	"SystemID":   true,
	"system_id":  true,
	"ClusterID":  true,
	"cluster_id": true,
}

// 允许的字段名
var allowedJobArgFields = map[string]bool{
	"EventID":  true,
	"event_id": true,
	"BatchID":  true, // For batch operations
	"batch_id": true,
	"Metadata": true, // Generic metadata allowed
	"TraceID":  true, // Observability
	"trace_id": true,
}

type jobArgsVisitor struct {
	fset       *token.FileSet
	path       string
	violations []string
}

func (v *jobArgsVisitor) Visit(n ast.Node) ast.Visitor {
	// 查找以 "JobArgs" 或 "Args" 结尾的结构体定义
	ts, ok := n.(*ast.TypeSpec)
	if !ok {
		return v
	}

	name := ts.Name.Name
	if !strings.HasSuffix(name, "JobArgs") && !strings.HasSuffix(name, "Args") {
		return v
	}

	st, ok := ts.Type.(*ast.StructType)
	if !ok || st.Fields == nil {
		return v
	}

	// 检查结构体字段
	for _, field := range st.Fields.List {
		for _, ident := range field.Names {
			fieldName := ident.Name
			if forbiddenJobArgFields[fieldName] {
				pos := v.fset.Position(field.Pos())
				v.violations = append(v.violations, fmt.Sprintf(
					"%s:%d: River Job Args %s 包含禁止的字段 '%s' (ADR-0009 Claim Check 要求只传递 EventID)",
					v.path, pos.Line, name, fieldName,
				))
			}
		}
	}

	return v
}

func main() {
	var violations []string

	// 扫描 usecase 和 worker 目录
	for _, dir := range []string{"internal/usecase", "internal/worker", "internal/jobs"} {
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

			// 检查是否有 nolint 注释
			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if strings.Contains(string(content), "//nolint:river-claim-check") {
				return nil
			}

			fset := token.NewFileSet()
			node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return nil
			}

			visitor := &jobArgsVisitor{
				fset: fset,
				path: path,
			}
			ast.Walk(visitor, node)
			violations = append(violations, visitor.violations...)

			return nil
		})

		if err != nil {
			fmt.Printf("❌ 遍历目录 %s 失败: %v\n", dir, err)
		}
	}

	if len(violations) > 0 {
		fmt.Println("❌ River Job Args Claim Check 检查失败:")
		for _, v := range violations {
			fmt.Printf("  %s\n", v)
		}
		fmt.Println("\n📋 规则 (ADR-0009): River Job Args 只应包含 EventID")
		fmt.Println("📋 正确做法: Worker 通过 EventID 查询 DomainEvent 获取完整数据")
		fmt.Println("📋 跳过检查: 使用 //nolint:river-claim-check 注释")
		os.Exit(1)
	} else {
		fmt.Println("✅ River Job Args Claim Check 检查通过")
	}
}
