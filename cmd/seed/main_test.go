package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
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
		"role-platform-admin",
		"role-approval-admin",
		"role-development-engineer",
		"role-test-engineer",
		"role-system-operator",
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

	assertHasPerm("role-platform-admin", "platform:admin")
	assertHasPerm("role-approval-admin",
		"builtin_approval:approve", "builtin_approval:view",
		"ticket:view", "vm:read", "service:read", "system:read",
	)
	assertHasPerm("role-development-engineer",
		"service:create", "service:read",
		"system:read", "system:write",
		"vm:create", "vm:delete", "vm:operate", "vm:read",
		"vnc:access",
	)
	assertHasPerm("role-test-engineer",
		"service:create", "service:read",
		"system:read", "system:write",
		"vm:create", "vm:delete", "vm:operate", "vm:read",
		"vnc:access",
	)
	assertHasPerm("role-system-operator",
		"service:create", "service:read",
		"system:read", "system:write",
		"vm:create", "vm:delete", "vm:operate", "vm:read",
		"vnc:access",
	)
	assertHasPerm("role-viewer", "vm:read", "system:read", "service:read")
}

func TestBuiltInRoles_EngineerRolesExcludePlatformAndApprovalAdmin(t *testing.T) {
	t.Parallel()

	roleIDs := []string{
		"role-development-engineer",
		"role-test-engineer",
		"role-system-operator",
	}

	for _, roleID := range roleIDs {
		var roleRecord *builtInRole
		for _, role := range builtInRoles() {
			if role.ID == roleID {
				roleRecord = &role
				break
			}
		}
		if roleRecord == nil {
			t.Fatalf("missing role %s", roleID)
			continue
		}
		if slices.Contains(roleRecord.Permissions, "platform:admin") {
			t.Fatalf("role %s unexpectedly includes platform:admin", roleID)
		}
		if slices.Contains(roleRecord.Permissions, "builtin_approval:approve") || slices.Contains(roleRecord.Permissions, "builtin_approval:view") {
			t.Fatalf("role %s unexpectedly includes approval administration permissions", roleID)
		}
	}
}

func TestBuiltInRoles_MetadataCompleteness(t *testing.T) {
	t.Parallel()

	for _, role := range builtInRoles() {
		if strings.TrimSpace(role.Name) == "" {
			t.Fatalf("role %s has empty Name", role.ID)
		}
		if strings.TrimSpace(role.DisplayName) == "" {
			t.Fatalf("role %s has empty DisplayName", role.ID)
		}
		if strings.TrimSpace(role.Description) == "" {
			t.Fatalf("role %s has empty Description", role.ID)
		}
		if len(role.Permissions) == 0 {
			t.Fatalf("role %s has no permissions", role.ID)
		}
	}
}

func TestSeedBuiltInRoles_RemovesMappingsForObsoleteBuiltInRoles(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	if err := logger.Init("error", "console"); err != nil {
		t.Fatalf("init logger: %v", err)
	}
	client := testutil.OpenEntPostgres(t, "seed_removes_obsolete_role_mappings")

	obsoleteRole, err := client.Role.Create().
		SetID("role-approver").
		SetName("Approver").
		SetDisplayName("Approver").
		SetDescription("obsolete built-in role").
		SetPermissions([]string{"builtin_approval:approve"}).
		SetBuiltIn(true).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create obsolete role: %v", err)
	}

	if _, createErr := client.ExternalCohortMapping.Create().
		SetID("mapping-obsolete-role").
		SetProviderID("provider-1").
		SetCohortKind("department").
		SetCohortKey("2955").
		SetRoleID(obsoleteRole.ID).
		SetScopeType("global").
		SetCreatedBy("test").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create mapping for obsolete role: %v", createErr)
	}

	if seedErr := seedBuiltInRoles(t.Context(), client); seedErr != nil {
		t.Fatalf("seedBuiltInRoles() error = %v", seedErr)
	}

	if _, getErr := client.Role.Get(t.Context(), obsoleteRole.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("obsolete role still exists, err = %v", getErr)
	}

	count, err := client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.RoleIDEQ(obsoleteRole.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count mappings for obsolete role: %v", err)
	}
	if count != 0 {
		t.Fatalf("obsolete role mappings count = %d, want 0", count)
	}
}
