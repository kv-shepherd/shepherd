// scripts/ci/check_river_bypass.go

/*
River Queue Bypass Detection (ADR-0006)

Rule:
1. Write operations (Create, Update, Delete) for entities subject to ADR-0006
   MUST be inserted as River Jobs, not direct DB writes in UseCase layer.
2. UseCase layer should only call InsertTx for River Jobs within transactions.
3. Direct entity mutations in UseCase layer are violations.

Exemptions:
- Notification writes (synchronous, per 04-governance.md §6.3)
- DomainEvent creation (part of the transaction)
- Status updates within workers (after commit)

Use //nolint:river-bypass to skip checks for legitimate exemptions.
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

// Entities that MUST go through River Queue for write operations
var protectedEntities = map[string]bool{
	"VM":             true,
	"VirtualMachine": true,
	"ApprovalTicket": true,
	"Ticket":         true,
	"Service":        true,
	"System":         true,
	"Cluster":        true,
}

// Methods that indicate direct write operations
var writeMethods = map[string]bool{
	"Create":      true,
	"Update":      true,
	"UpdateOne":   true,
	"UpdateOneID": true,
	"Delete":      true,
	"DeleteOne":   true,
	"DeleteOneID": true,
	"Save":        true,
	"Exec":        true,
}

// Exempted entities (synchronous writes allowed per ADR exception)
var exemptedEntities = map[string]bool{
	"Notification": true,
	"DomainEvent":  true,
	"AuditLog":     true,
	"Session":      true,
}

type bypassVisitor struct {
	fset       *token.FileSet
	path       string
	violations []string
	inWorker   bool // Workers are allowed to do direct writes
}

func (v *bypassVisitor) Visit(n ast.Node) ast.Visitor {
	// Check for call expressions
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return v
	}

	// Check for selector expressions (method calls)
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return v
	}

	methodName := sel.Sel.Name

	// Check if this is a write method
	if !writeMethods[methodName] {
		return v
	}

	// Try to determine the entity type from the chain
	entityName := extractEntityName(sel.X)
	if entityName == "" {
		return v
	}

	// Check if entity is protected
	if !protectedEntities[entityName] {
		return v
	}

	// Check if entity is exempted
	if exemptedEntities[entityName] {
		return v
	}

	// This is a violation - direct write to protected entity in usecase
	pos := v.fset.Position(call.Pos())
	v.violations = append(v.violations, fmt.Sprintf(
		"%s:%d: Direct write to %s.%s() detected (ADR-0006: use River Queue)",
		v.path, pos.Line, entityName, methodName,
	))

	return v
}

// extractEntityName attempts to find the entity name from a chain like entClient.VM.Create()
func extractEntityName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		name := e.Sel.Name
		// Check if this looks like an entity name
		for entity := range protectedEntities {
			if strings.Contains(name, entity) {
				return entity
			}
		}
		for entity := range exemptedEntities {
			if strings.Contains(name, entity) {
				return entity
			}
		}
		// Recurse into the expression
		return extractEntityName(e.X)
	case *ast.CallExpr:
		return extractEntityName(e.Fun)
	case *ast.Ident:
		return e.Name
	}
	return ""
}

func main() {
	var violations []string

	// Only scan usecase directory - workers are exempt
	dir := "internal/usecase"
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		fmt.Println("✅ River Bypass Check: no usecase directory found (skipping)")
		os.Exit(0)
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Check for nolint comment
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(content), "//nolint:river-bypass") {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		visitor := &bypassVisitor{
			fset: fset,
			path: path,
		}
		ast.Walk(visitor, node)
		violations = append(violations, visitor.violations...)

		return nil
	})

	if err != nil {
		fmt.Printf("❌ Failed to walk directory %s: %v\n", dir, err)
		os.Exit(1)
	}

	if len(violations) > 0 {
		fmt.Println("❌ River Bypass Check FAILED:")
		for _, v := range violations {
			fmt.Printf("  %s\n", v)
		}
		fmt.Println("\n📋 Rule (ADR-0006): All write operations MUST go through River Queue")
		fmt.Println("📋 Correct Pattern: UseCase inserts River Job → Worker performs actual write")
		fmt.Println("📋 Exemptions: Notification, DomainEvent, AuditLog (see 04-governance.md §6.3)")
		fmt.Println("📋 Skip Check: Use //nolint:river-bypass comment at file level")
		os.Exit(1)
	} else {
		fmt.Println("✅ River Bypass Check PASSED")
	}
}
