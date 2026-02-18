package handlers

import (
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/namespaceregistry"
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
