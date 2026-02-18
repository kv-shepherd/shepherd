package handlers

import (
	"os"
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
	required := []string{
		`s.requireSystemRole(c, systemId, "view")`,
		`s.requireSystemRole(c, systemId, "create")`,
		`s.requireSystemRole(c, systemId, "update")`,
		`s.requireSystemRole(c, systemId, "delete")`,
		`rrb.ResourceTypeEQ("system")`,
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Fatalf("missing Stage 4 hierarchy guard fragment %q in %s", fragment, serverSystemSourcePath)
		}
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
