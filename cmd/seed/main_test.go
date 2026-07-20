package main

import (
	"slices"
	"strings"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"
)

func TestMain(m *testing.M) {
	if err := logger.Init("error", "console"); err != nil {
		panic(err)
	}
	testutil.MustStartDockerPG(m)
}

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

func TestDefaultAdminSeedSpec_Stage15BootstrapContract(t *testing.T) {
	t.Parallel()

	spec := defaultAdminSeedSpec()
	if spec.id != "user-default-admin" {
		t.Fatalf("default admin ID = %q, want user-default-admin", spec.id)
	}
	if spec.username != "admin" {
		t.Fatalf("default admin username = %q, want admin", spec.username)
	}
	if spec.email != "admin@localhost" {
		t.Fatalf("default admin email = %q, want admin@localhost", spec.email)
	}
	if spec.displayName != "Default Administrator" {
		t.Fatalf("default admin display name = %q, want Default Administrator", spec.displayName)
	}
	if strings.TrimSpace(spec.password) == "" {
		t.Fatal("default admin password must not be empty")
	}
	if spec.password != "admin" {
		t.Fatalf("default admin password = %q, want admin", spec.password)
	}
	if !spec.forcePasswordChange {
		t.Fatal("default admin must force password change on first login")
	}
	if spec.roleID != "role-platform-admin" {
		t.Fatalf("default admin role ID = %q, want role-platform-admin", spec.roleID)
	}
	if spec.roleBindingScopeType != "global" {
		t.Fatalf("default admin role binding scope = %q, want global", spec.roleBindingScopeType)
	}
	if spec.roleBindingCreatedBy != "system-seed" {
		t.Fatalf("default admin role binding creator = %q, want system-seed", spec.roleBindingCreatedBy)
	}
}

func TestDefaultAdminSeedSpec_BindsExistingPlatformAdminRole(t *testing.T) {
	t.Parallel()

	spec := defaultAdminSeedSpec()
	var platformAdmin *builtInRole
	for _, role := range builtInRoles() {
		if role.ID == spec.roleID {
			roleRecord := role
			platformAdmin = &roleRecord
			break
		}
	}
	if platformAdmin == nil {
		t.Fatalf("default admin role %s is not in built-in role catalog", spec.roleID)
	}
	if !slices.Contains(platformAdmin.Permissions, "platform:admin") {
		t.Fatalf("default admin role %s does not include platform:admin", spec.roleID)
	}
}

func TestEffectiveSeedTimeout(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr string
	}{
		{
			name: "default",
			want: defaultSeedTimeout,
		},
		{
			name: "explicit duration",
			env:  "45s",
			want: 45 * time.Second,
		},
		{
			name:    "invalid duration",
			env:     "soon",
			wantErr: "parse SEED_TIMEOUT",
		},
		{
			name:    "zero duration",
			env:     "0s",
			wantErr: "SEED_TIMEOUT must be > 0",
		},
		{
			name:    "negative duration",
			env:     "-1s",
			wantErr: "SEED_TIMEOUT must be > 0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SEED_TIMEOUT", tc.env)

			got, err := effectiveSeedTimeout()
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("effectiveSeedTimeout() error = nil, want %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("effectiveSeedTimeout() error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("effectiveSeedTimeout() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("effectiveSeedTimeout() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestSeedBuiltInRoles_ReconcilesCatalogAndIsIdempotent(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "seed_reconciles_builtin_roles")

	if _, err := client.Role.Create().
		SetID("role-platform-admin").
		SetName("StalePlatformAdmin").
		SetDisplayName("Stale").
		SetDescription("stale role").
		SetPermissions([]string{"legacy:permission"}).
		SetBuiltIn(true).
		SetEnabled(false).
		Save(t.Context()); err != nil {
		t.Fatalf("create stale platform admin role: %v", err)
	}

	if err := seedBuiltInRoles(t.Context(), client); err != nil {
		t.Fatalf("seedBuiltInRoles() initial error = %v", err)
	}

	roles, err := client.Role.Query().
		Where(role.BuiltIn(true)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query built-in roles: %v", err)
	}
	if len(roles) != len(builtInRoles()) {
		t.Fatalf("built-in role count = %d, want %d", len(roles), len(builtInRoles()))
	}

	platformAdmin, err := client.Role.Get(t.Context(), "role-platform-admin")
	if err != nil {
		t.Fatalf("query platform admin role: %v", err)
	}
	if platformAdmin.Name != "PlatformAdmin" || platformAdmin.DisplayName != "Platform Administrator" {
		t.Fatalf("platform admin metadata = %q/%q, want canonical values", platformAdmin.Name, platformAdmin.DisplayName)
	}
	if !platformAdmin.Enabled {
		t.Fatal("platform admin role remains disabled after reconciliation")
	}
	if !slices.Equal(platformAdmin.Permissions, []string{"platform:admin"}) {
		t.Fatalf("platform admin permissions = %#v, want platform:admin", platformAdmin.Permissions)
	}

	if rerunErr := seedBuiltInRoles(t.Context(), client); rerunErr != nil {
		t.Fatalf("seedBuiltInRoles() rerun error = %v", rerunErr)
	}
	count, err := client.Role.Query().
		Where(role.BuiltIn(true)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count built-in roles after rerun: %v", err)
	}
	if count != len(builtInRoles()) {
		t.Fatalf("built-in role count after rerun = %d, want %d", count, len(builtInRoles()))
	}
}

func TestSeedBuiltInRoles_RemovesMappingsForObsoleteBuiltInRoles(t *testing.T) {
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

func TestSeedDefaultAdminCreatesPlatformAdminBindingAndIsIdempotent(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "seed_default_admin")
	if err := seedBuiltInRoles(t.Context(), client); err != nil {
		t.Fatalf("seedBuiltInRoles() error = %v", err)
	}

	if err := seedDefaultAdmin(t.Context(), client); err != nil {
		t.Fatalf("seedDefaultAdmin() initial error = %v", err)
	}
	if err := seedDefaultAdmin(t.Context(), client); err != nil {
		t.Fatalf("seedDefaultAdmin() rerun error = %v", err)
	}

	spec := defaultAdminSeedSpec()
	admin, err := client.User.Query().
		Where(entuser.IDEQ(spec.id)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query default admin: %v", err)
	}
	if admin.Username != spec.username || admin.Email != spec.email || admin.DisplayName != spec.displayName {
		t.Fatalf("default admin = username %q email %q display %q, want seed spec", admin.Username, admin.Email, admin.DisplayName)
	}
	if !admin.ForcePasswordChange {
		t.Fatal("default admin ForcePasswordChange = false, want true")
	}
	if compareErr := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(spec.password)); compareErr != nil {
		t.Fatalf("default admin password hash does not match seed password: %v", compareErr)
	}

	userCount, err := client.User.Query().
		Where(entuser.UsernameEQ(spec.username)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count admin users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("admin user count = %d, want 1", userCount)
	}
	bindingCount, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(admin.ID)),
			rolebinding.HasRoleWith(role.IDEQ(spec.roleID)),
			rolebinding.ScopeTypeEQ(spec.roleBindingScopeType),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count admin role bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("admin platform role binding count = %d, want 1", bindingCount)
	}
}

func TestSeedDefaultAdminConcurrentRunsCreateOneBinding(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "seed_default_admin_concurrent")
	if err := seedBuiltInRoles(t.Context(), client); err != nil {
		t.Fatalf("seedBuiltInRoles() error = %v", err)
	}

	var group errgroup.Group
	for range 2 {
		group.Go(func() error {
			return seedDefaultAdmin(t.Context(), client)
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("concurrent seedDefaultAdmin() error = %v", err)
	}

	spec := defaultAdminSeedSpec()
	admin, err := client.User.Query().
		Where(entuser.UsernameEQ(spec.username)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query concurrently seeded admin: %v", err)
	}
	bindingCount, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(admin.ID)),
			rolebinding.HasRoleWith(role.IDEQ(spec.roleID)),
			rolebinding.ScopeTypeEQ(spec.roleBindingScopeType),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count concurrently seeded admin role bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("concurrent admin role binding count = %d, want 1", bindingCount)
	}
}

func TestSeedDefaultAdminBindsExistingUsernameConflict(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "seed_default_admin_username_conflict")
	if err := seedBuiltInRoles(t.Context(), client); err != nil {
		t.Fatalf("seedBuiltInRoles() error = %v", err)
	}

	spec := defaultAdminSeedSpec()
	existing, err := client.User.Create().
		SetID("existing-admin-id").
		SetUsername(spec.username).
		SetEmail("existing-admin@example.com").
		SetDisplayName("Existing Admin").
		SetPasswordHash("already-set").
		SetForcePasswordChange(false).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing admin username: %v", err)
	}

	if seedErr := seedDefaultAdmin(t.Context(), client); seedErr != nil {
		t.Fatalf("seedDefaultAdmin() error = %v", seedErr)
	}

	userCount, err := client.User.Query().
		Where(entuser.UsernameEQ(spec.username)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count admin users: %v", err)
	}
	if userCount != 1 {
		t.Fatalf("admin user count = %d, want 1", userCount)
	}
	bindingExists, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(existing.ID)),
			rolebinding.HasRoleWith(role.IDEQ(spec.roleID)),
			rolebinding.ScopeTypeEQ(spec.roleBindingScopeType),
		).
		Exist(t.Context())
	if err != nil {
		t.Fatalf("query existing admin role binding: %v", err)
	}
	if !bindingExists {
		t.Fatalf("default admin username conflict user %s was not bound to %s", existing.ID, spec.roleID)
	}
}
