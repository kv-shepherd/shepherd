package service

import (
	"testing"

	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
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

func TestExternalAuthService_UpsertExternalUser_ClaimsExistingImportedUserByEmail(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_claims_imported_user")
	service := NewExternalAuthService(client)

	importedUser, err := client.User.Create().
		SetID("user-imported-1").
		SetUsername("alice@example.com").
		SetEmail("alice@example.com").
		SetDisplayName("Alice Imported").
		SetAuthProviderID("provider-directory").
		SetExternalID("alice@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create imported user: %v", err)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-sso", runtimecontract.AuthResult{
		ExternalID:  "alice@example.com",
		Username:    "alice@example.com",
		DisplayName: "Alice SSO",
		Email:       "alice@example.com",
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("UpsertExternalUser() error = %v", err)
	}
	if result.Created {
		t.Fatal("Created = true, want false")
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.User.ID != importedUser.ID {
		t.Fatalf("user id = %q, want %q", result.User.ID, importedUser.ID)
	}
	if result.User.AuthProviderID != "provider-sso" {
		t.Fatalf("auth_provider_id = %q, want provider-sso", result.User.AuthProviderID)
	}
	if result.User.ExternalID != "alice@example.com" {
		t.Fatalf("external_id = %q, want alice@example.com", result.User.ExternalID)
	}
}

func TestExternalAuthService_UpsertExternalUser_LoginOnlyClaimPreservesDirectoryOwnershipAndProfile(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_preserves_directory_owner")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-directory-user").
		SetName("DirectoryUser").
		SetPermissions([]string{"system:read", "service:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	importedUser, err := client.User.Create().
		SetID("user-imported-2").
		SetUsername("alice@example.com").
		SetEmail("alice@example.com").
		SetDisplayName("Alice Imported").
		SetAuthProviderID("provider-directory").
		SetExternalID("alice@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create imported user: %v", err)
	}
	if _, createErr := client.UserDirectoryProfile.Create().
		SetID("profile-imported-2").
		SetUserID(importedUser.ID).
		SetAttributes(map[string]interface{}{
			"department": "Engineering",
			"section":    "Platform",
		}).
		SetLastSyncedAt(importedUser.CreatedAt).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create directory profile: %v", createErr)
	}
	roleBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-imported-2").
		SetUserID(importedUser.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("system:external-cohort-mapper").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	if _, createErr := client.ExternalCohortGrant.Create().
		SetID("grant-imported-2").
		SetUserID(importedUser.ID).
		SetProviderID("provider-directory").
		SetBindingKey("role-directory-user|global||").
		SetRoleBindingID(roleBindingEnt.ID).
		SetLastAppliedAt(importedUser.CreatedAt).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create external cohort grant: %v", createErr)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-sso", runtimecontract.AuthResult{
		ExternalID:         "alice@example.com",
		Username:           "alice@example.com",
		DisplayName:        "Alice SSO",
		Email:              "alice@example.com",
		Enabled:            true,
		DirectoryAuthority: runtimecontract.AuthDirectoryAuthorityLoginOnly,
	})
	if err != nil {
		t.Fatalf("UpsertExternalUser() error = %v", err)
	}
	if result.Created {
		t.Fatal("Created = true, want false")
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.User.ID != importedUser.ID {
		t.Fatalf("user id = %q, want %q", result.User.ID, importedUser.ID)
	}
	if result.User.AuthProviderID != "provider-directory" {
		t.Fatalf("auth_provider_id = %q, want provider-directory", result.User.AuthProviderID)
	}
	if result.User.ExternalID != "alice@example.com" {
		t.Fatalf("external_id = %q, want alice@example.com", result.User.ExternalID)
	}

	profile, err := client.UserDirectoryProfile.Query().
		Where(userdirectoryprofile.UserIDEQ(importedUser.ID)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query directory profile: %v", err)
	}
	if profile.Attributes["department"] != "Engineering" {
		t.Fatalf("profile department = %#v, want Engineering", profile.Attributes["department"])
	}
	if profile.Attributes["section"] != "Platform" {
		t.Fatalf("profile section = %#v, want Platform", profile.Attributes["section"])
	}

	grantCount, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(importedUser.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count external cohort grants: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("external cohort grant count = %d, want 1", grantCount)
	}
}

func TestExternalAuthService_UpsertExternalUser_DoesNotClaimLocalPasswordUser(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_rejects_local_claim")
	service := NewExternalAuthService(client)

	if _, err := client.User.Create().
		SetID("user-local-1").
		SetUsername("alice@example.com").
		SetEmail("alice@example.com").
		SetDisplayName("Alice Local").
		SetPasswordHash("bcrypt-hash").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create local user: %v", err)
	}

	_, err := service.UpsertExternalUser(t.Context(), "provider-sso", runtimecontract.AuthResult{
		ExternalID:  "alice@example.com",
		Username:    "alice@example.com",
		DisplayName: "Alice SSO",
		Email:       "alice@example.com",
		Enabled:     true,
	})
	if err == nil {
		t.Fatal("UpsertExternalUser() error = nil, want non-nil")
	}
	if got := err.Error(); got != "external identity already belongs to another user" {
		t.Fatalf("error = %q", got)
	}
}
