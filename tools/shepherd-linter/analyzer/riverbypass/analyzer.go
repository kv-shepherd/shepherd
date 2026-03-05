// Package riverbypass implements a go/analysis Analyzer that detects
// River Queue bypass violations per ADR-0006.
//
// This Analyzer is a go/analysis-compatible re-implementation of the
// legacy Batch1 CI script `check_river_bypass.go` (removed post-ADR-0039 migration).
//
// Rule: Write operations (Create, Update, Delete) for protected entities
// in the UseCase layer MUST be inserted as River Jobs, not direct DB writes.
//
// Applies to: internal/usecase/ packages (non-test files).
// Exempted: Notification, DomainEvent, AuditLog, Session, ApprovalTicket writes.
package riverbypass

import (
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer is the exported go/analysis.Analyzer for River bypass detection.
var Analyzer = &analysis.Analyzer{
	Name:     "riverbypass",
	Doc:      "ADR-0006: detects direct DB writes to protected entities in UseCase layer (must use River Queue)",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

// protectedEntities are entities whose writes MUST go through River Queue.
var protectedEntities = map[string]bool{
	"VM":             true,
	"VirtualMachine": true,
	"Service":        true,
	"System":         true,
	"Cluster":        true,
}

// writeMethods are method names that indicate direct DB write operations.
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

// exemptedEntities are allowed to bypass the River Queue (per ADR-0006 §exception table).
var exemptedEntities = map[string]bool{
	"Notification":   true, // synchronous, per 04-governance.md §6.3
	"DomainEvent":    true, // part of the claim-check transaction (ADR-0009)
	"AuditLog":       true, // synchronous audit trail
	"Session":        true, // auth session management, no K8s interaction
	"ApprovalTicket": true, // ADR-0006 lines 133-134: River Job inserted after approval
	"Ticket":         true,
}

func run(pass *analysis.Pass) (interface{}, error) {
	// Only enforce inside internal/usecase/ packages.
	if !isUsecasePkg(pass) {
		return nil, nil
	}

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}
	insp.Preorder(nodeFilter, func(n ast.Node) {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return
		}

		// Require a selector expression: receiver.Method(...)
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}

		methodName := sel.Sel.Name
		if !writeMethods[methodName] {
			return
		}

		// Skip test files.
		pos := pass.Fset.Position(call.Pos())
		if strings.HasSuffix(pos.Filename, "_test.go") {
			return
		}

		// Extract entity name from the method chain.
		entityName := extractEntityName(sel.X)
		if entityName == "" {
			return
		}

		// Not a protected entity — ignore.
		if !protectedEntities[entityName] {
			return
		}

		// Exempted entity — allowed to bypass.
		if exemptedEntities[entityName] {
			return
		}

		pass.Reportf(call.Pos(),
			"direct write to %s.%s() in UseCase layer (ADR-0006): all writes to protected entities must go through River Queue; use river.InsertTx()",
			entityName, methodName)
	})

	return nil, nil
}

// isUsecasePkg returns true if the package is under internal/usecase.
func isUsecasePkg(pass *analysis.Pass) bool {
	if pass.Pkg == nil {
		return false
	}

	pkgPath := pass.Pkg.Path()
	return strings.Contains(pkgPath, "/usecase") ||
		strings.HasSuffix(pkgPath, "/usecase") ||
		strings.HasPrefix(pkgPath, "usecase")
}

// extractEntityName attempts to find an entity name from a call chain like
// entClient.VM.Create() or client.VirtualMachineClient.Update().
func extractEntityName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		name := e.Sel.Name
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
		return extractEntityName(e.X)
	case *ast.CallExpr:
		return extractEntityName(e.Fun)
	case *ast.Ident:
		// Check identifier itself.
		for entity := range protectedEntities {
			if strings.Contains(e.Name, entity) {
				return entity
			}
		}
		for entity := range exemptedEntities {
			if strings.Contains(e.Name, entity) {
				return entity
			}
		}
		return ""
	}
	return ""
}
