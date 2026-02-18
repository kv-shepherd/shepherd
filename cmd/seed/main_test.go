package main

import (
	"slices"
	"strings"
	"testing"
)

func TestBuiltInRoles_Stage2Baseline(t *testing.T) {
	t.Parallel()

	roles := builtInRoles()
	if len(roles) != 6 {
		t.Fatalf("builtInRoles count = %d, want 6", len(roles))
	}

	byID := make(map[string]builtInRole, len(roles))
	for _, role := range roles {
		if _, exists := byID[role.ID]; exists {
			t.Fatalf("duplicate role id: %s", role.ID)
		}
		byID[role.ID] = role
	}

	requiredRoleIDs := []string{
		"role-bootstrap",
		"role-platform-admin",
		"role-system-admin",
		"role-approver",
		"role-operator",
		"role-viewer",
	}
	for _, roleID := range requiredRoleIDs {
		if _, ok := byID[roleID]; !ok {
			t.Fatalf("missing required built-in role: %s", roleID)
		}
	}
}

func TestBuiltInRoles_NoWildcardPermissions(t *testing.T) {
	t.Parallel()

	for _, role := range builtInRoles() {
		for _, perm := range role.Permissions {
			if strings.Contains(perm, "*") {
				t.Fatalf("role %s contains wildcard permission %q", role.ID, perm)
			}
		}
	}
}

func TestBuiltInRoles_CanonicalPermissionSets(t *testing.T) {
	t.Parallel()

	byID := make(map[string]builtInRole, 6)
	for _, role := range builtInRoles() {
		byID[role.ID] = role
	}

	assertHasPerm := func(roleID string, required ...string) {
		t.Helper()
		role, ok := byID[roleID]
		if !ok {
			t.Fatalf("missing role %s", roleID)
		}
		for _, perm := range required {
			if !slices.Contains(role.Permissions, perm) {
				t.Fatalf("role %s missing permission %s", roleID, perm)
			}
		}
	}

	assertHasPerm("role-bootstrap", "platform:admin")
	assertHasPerm("role-platform-admin", "platform:admin")
	assertHasPerm("role-system-admin",
		"system:read", "system:write", "system:delete",
		"service:read", "service:create", "service:delete",
		"vm:read", "vm:create", "vm:operate", "vm:delete",
		"vnc:access", "rbac:manage",
	)
	assertHasPerm("role-approver", "approval:approve", "approval:view", "vm:read", "service:read", "system:read")
	assertHasPerm("role-operator", "vm:operate", "vm:create", "vm:read", "vnc:access")
	assertHasPerm("role-viewer", "vm:read", "system:read", "service:read")
}
