package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestNamespaceVisibilityFromRoleBindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		bindings         []*ent.RoleBinding
		wantRestricted   bool
		wantEnvironments []namespaceregistry.Environment
	}{
		{
			name:           "no bindings means no visibility",
			bindings:       nil,
			wantRestricted: true,
		},
		{
			name: "empty allowed environments means unrestricted",
			bindings: []*ent.RoleBinding{
				{AllowedEnvironments: []string{}},
			},
			wantRestricted: false,
		},
		{
			name: "explicit test environment restriction",
			bindings: []*ent.RoleBinding{
				{AllowedEnvironments: []string{"test"}},
			},
			wantRestricted:   true,
			wantEnvironments: []namespaceregistry.Environment{namespaceregistry.EnvironmentTest},
		},
		{
			name: "union and normalization across bindings",
			bindings: []*ent.RoleBinding{
				{AllowedEnvironments: []string{"  TEST "}},
				{AllowedEnvironments: []string{"prod"}},
			},
			wantRestricted: true,
			wantEnvironments: []namespaceregistry.Environment{
				namespaceregistry.EnvironmentProd,
				namespaceregistry.EnvironmentTest,
			},
		},
		{
			name: "unknown explicit environments fail closed",
			bindings: []*ent.RoleBinding{
				{AllowedEnvironments: []string{"staging"}},
			},
			wantRestricted: true,
		},
		{
			name: "disabled unrestricted binding does not widen enabled restricted binding",
			bindings: []*ent.RoleBinding{
				{
					AllowedEnvironments: []string{},
					Edges: ent.RoleBindingEdges{
						Role: &ent.Role{Enabled: false},
					},
				},
				{
					AllowedEnvironments: []string{"test"},
					Edges: ent.RoleBindingEdges{
						Role: &ent.Role{Enabled: true},
					},
				},
			},
			wantRestricted:   true,
			wantEnvironments: []namespaceregistry.Environment{namespaceregistry.EnvironmentTest},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := namespaceVisibilityFromRoleBindings(tc.bindings)
			if got.restricted != tc.wantRestricted {
				t.Fatalf("restricted mismatch: got %v want %v", got.restricted, tc.wantRestricted)
			}
			if len(got.envs) != len(tc.wantEnvironments) {
				t.Fatalf("env count mismatch: got %d want %d (%v)", len(got.envs), len(tc.wantEnvironments), got.envs)
			}
			for i := range tc.wantEnvironments {
				if got.envs[i] != tc.wantEnvironments[i] {
					t.Fatalf("env[%d] mismatch: got %s want %s", i, got.envs[i], tc.wantEnvironments[i])
				}
			}
		})
	}
}

func TestResolveNamespaceVisibility_IgnoresDisabledRoles(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "namespace_visibility_disabled_role")
	srv := NewServer(ServerDeps{EntClient: client})
	const userID = "user-visibility-disabled-role"

	userEnt, err := client.User.Create().
		SetID(userID).
		SetUsername("visibility-disabled-role").
		SetPasswordHash("hash").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	enabledRole, err := client.Role.Create().
		SetID("role-visibility-enabled").
		SetName("visibility_enabled").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create enabled role: %v", err)
	}
	disabledRole, err := client.Role.Create().
		SetID("role-visibility-disabled").
		SetName("visibility_disabled").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}
	if _, bindingErr := client.RoleBinding.Create().
		SetID("binding-visibility-enabled").
		SetUserID(userEnt.ID).
		SetRoleID(enabledRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("test").
		Save(t.Context()); bindingErr != nil {
		t.Fatalf("create enabled role binding: %v", bindingErr)
	}
	if _, bindingErr := client.RoleBinding.Create().
		SetID("binding-visibility-disabled").
		SetUserID(userEnt.ID).
		SetRoleID(disabledRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("test").
		Save(t.Context()); bindingErr != nil {
		t.Fatalf("create disabled role binding: %v", bindingErr)
	}

	c, _ := newAuthedGinContext(t, http.MethodGet, "/vms", "", userEnt.ID, []string{"vm:read"})
	got, err := srv.resolveNamespaceVisibility(c)
	if err != nil {
		t.Fatalf("resolve namespace visibility: %v", err)
	}
	if !got.restricted {
		t.Fatal("visibility is unrestricted, want test-only restriction")
	}
	if len(got.envs) != 1 || got.envs[0] != namespaceregistry.EnvironmentTest {
		t.Fatalf("envs = %#v, want [test]", got.envs)
	}
}
