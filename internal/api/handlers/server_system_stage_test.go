package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

const serverSystemSourcePath = "server_system.go"

func TestStage4A_SystemCreationOwnerBindingContract(t *testing.T) {
	t.Parallel()

	source := mustReadSource(t, serverSystemSourcePath)
	required := []string{
		`SetResourceType("system")`,
		`SetRole("owner")`,
		`SetCreatedBy(actor)`,
		`"system.create"`,
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("missing Stage 4.A contract fragment %q in %s", fragment, serverSystemSourcePath)
		}
	}
}

func TestStage4Hierarchy_AccessGuardsContract(t *testing.T) {
	t.Parallel()

	source := mustReadSource(t, serverSystemSourcePath)
	file := mustParseSource(t, serverSystemSourcePath, source)

	requiredActions := []string{"view", "create", "update", "delete"}
	for _, action := range requiredActions {
		if !hasRequireSystemRoleAction(file, action) {
			t.Fatalf("missing Stage 4 hierarchy guard requireSystemRole(..., %q) in %s", action, serverSystemSourcePath)
		}
	}
	if !strings.Contains(source, `rrb.ResourceTypeEQ("system")`) {
		t.Fatalf("missing Stage 4 hierarchy guard fragment %q in %s", `rrb.ResourceTypeEQ("system")`, serverSystemSourcePath)
	}
}

func TestStage4C_UpdateDescriptionOnlyContract(t *testing.T) {
	t.Parallel()

	source := mustReadSource(t, serverSystemSourcePath)
	required := []string{
		"func (s *Server) UpdateSystem(",
		"func (s *Server) UpdateService(",
		"generated.SystemUpdateRequest",
		"generated.ServiceUpdateRequest",
		"SetDescription(req.Description)",
		`"system.update"`,
		`"service.update"`,
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("missing Stage 4.C contract fragment %q in %s", fragment, serverSystemSourcePath)
		}
	}
}

func mustReadSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func mustParseSource(t *testing.T, path, source string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func hasRequireSystemRoleAction(file *ast.File, wantAction string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil || sel.Sel.Name != "requireSystemRole" {
			return true
		}
		if len(call.Args) < 3 {
			return true
		}
		actionLit, ok := call.Args[2].(*ast.BasicLit)
		if !ok || actionLit.Kind != token.STRING {
			return true
		}
		action, err := strconv.Unquote(actionLit.Value)
		if err != nil {
			return true
		}
		if action == wantAction {
			found = true
			return false
		}
		return true
	})
	return found
}
