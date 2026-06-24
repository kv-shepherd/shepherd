package handlers

import (
	"slices"
	"testing"

	"kv-shepherd.io/shepherd/ent"
)

func TestLoadRoleNames_IgnoresDisabledRoles(t *testing.T) {
	t.Parallel()

	got := loadRoleNames([]*ent.RoleBinding{
		{
			Edges: ent.RoleBindingEdges{
				Role: &ent.Role{Name: "disabled-admin", Enabled: false},
			},
		},
		{
			Edges: ent.RoleBindingEdges{
				Role: &ent.Role{Name: "enabled-operator", Enabled: true},
			},
		},
		{
			Edges: ent.RoleBindingEdges{
				Role: &ent.Role{Name: "enabled-viewer", Enabled: true},
			},
		},
	})

	want := []string{"enabled-operator", "enabled-viewer"}
	if !slices.Equal(got, want) {
		t.Fatalf("roles = %#v, want %#v", got, want)
	}
}
