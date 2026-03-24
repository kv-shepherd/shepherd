package service

import (
	"testing"

	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestExternalAuthService_UpsertExternalUser_ReconcilesManagedBindings(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_reconcile")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-1").
		SetName("external_auth_viewer").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-1").
		SetProviderID("provider-1").
		SetCohortKind("department").
		SetCohortKey("2").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping 1: %v", mappingErr)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-2").
		SetProviderID("provider-1").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping 2: %v", mappingErr)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-1", runtimecontract.AuthResult{
		ExternalID:  "ext-user-1",
		Username:    "alice.external",
		DisplayName: "Alice External",
		Email:       "alice.external@example.com",
		Enabled:     true,
		Cohorts: []runtimecontract.ExternalCohort{
			{Kind: "department", Key: "2", DisplayName: "Engineering"},
			{Kind: "group", Key: "engineering", DisplayName: "Engineering"},
		},
	})
	if err != nil {
		t.Fatalf("upsert external user: %v", err)
	}

	bindings, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(user.IDEQ(result.User.ID)),
			rolebinding.HasRoleWith(role.IDEQ(roleEnt.ID)),
		).
		All(t.Context())
	if err != nil {
		t.Fatalf("query managed bindings: %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("managed binding count = %d, want 1", len(bindings))
	}

	grants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(result.User.ID)).
		All(t.Context())
	if err != nil {
		t.Fatalf("query external cohort grants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("managed grant count = %d, want 1", len(grants))
	}
	if got := grants[0].SourceMappingIds; len(got) != 2 {
		t.Fatalf("source_mapping_ids len = %d, want 2", len(got))
	}
	observedCohorts, err := client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ("provider-1")).
		All(t.Context())
	if err != nil {
		t.Fatalf("query observed external cohorts: %v", err)
	}
	if len(observedCohorts) != 2 {
		t.Fatalf("observed external cohort count = %d, want 2", len(observedCohorts))
	}

	if _, updateErr := service.UpsertExternalUser(t.Context(), "provider-1", runtimecontract.AuthResult{
		ExternalID:  "ext-user-1",
		Username:    "alice.external",
		DisplayName: "Alice External",
		Email:       "alice.external@example.com",
		Enabled:     true,
	}); updateErr != nil {
		t.Fatalf("upsert external user without cohorts: %v", updateErr)
	}

	remainingBindings, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(user.IDEQ(result.User.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count remaining bindings: %v", err)
	}
	if remainingBindings != 0 {
		t.Fatalf("remaining managed binding count = %d, want 0", remainingBindings)
	}
	remainingGrants, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(result.User.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count remaining grants: %v", err)
	}
	if remainingGrants != 0 {
		t.Fatalf("remaining managed grant count = %d, want 0", remainingGrants)
	}
	remainingCohorts, err := client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ("provider-1")).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count observed external cohorts: %v", err)
	}
	if remainingCohorts != 2 {
		t.Fatalf("observed external cohort count after login without cohorts = %d, want 2", remainingCohorts)
	}
}
