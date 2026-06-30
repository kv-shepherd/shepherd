package service

import (
	"errors"
	"strings"
	"testing"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func createExternalAuthProviderForTest(t *testing.T, client *ent.Client, id string) {
	t.Helper()
	createExternalAuthProviderForTestWithEnabled(t, client, id, true)
}

func createExternalAuthProviderForTestWithEnabled(t *testing.T, client *ent.Client, id string, enabled bool) {
	t.Helper()
	if _, err := client.AuthProvider.Create().
		SetID(id).
		SetName(id).
		SetAuthType("test").
		SetConfig(map[string]interface{}{}).
		SetEnabled(enabled).
		SetCreatedBy("test").
		Save(t.Context()); err != nil {
		t.Fatalf("create auth provider %q: %v", id, err)
	}
}

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
	if !result.RBACChanged {
		t.Fatal("upsert external user RBACChanged = false, want true for managed binding creation")
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

	updateResult, updateErr := service.UpsertExternalUser(t.Context(), "provider-1", runtimecontract.AuthResult{
		ExternalID:  "ext-user-1",
		Username:    "alice.external",
		DisplayName: "Alice External",
		Email:       "alice.external@example.com",
		Enabled:     true,
	})
	if updateErr != nil {
		t.Fatalf("upsert external user without cohorts: %v", updateErr)
	}
	if !updateResult.RBACChanged {
		t.Fatal("upsert external user without cohorts RBACChanged = false, want true for managed binding deletion")
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

	stableResult, updateErr := service.UpsertExternalUser(t.Context(), "provider-1", runtimecontract.AuthResult{
		ExternalID:  "ext-user-1",
		Username:    "alice.external",
		DisplayName: "Alice External",
		Email:       "alice.external@example.com",
		Enabled:     true,
	})
	if updateErr != nil {
		t.Fatalf("repeat upsert external user without cohorts: %v", updateErr)
	}
	if stableResult.RBACChanged {
		t.Fatal("repeat upsert external user without cohorts RBACChanged = true, want false without RBAC changes")
	}
}

func TestExternalAuthService_ReconcileManagedBindingSkipsDisabledRoleMapping(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_disabled_role_mapping_skip")
	service := NewExternalAuthService(client)

	disabledRole, err := client.Role.Create().
		SetID("role-ext-auth-disabled-mapping").
		SetName("external_auth_disabled_mapping").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-disabled-role").
		SetProviderID("provider-disabled-role").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(disabledRole.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-disabled-role", runtimecontract.AuthResult{
		ExternalID: "ext-disabled-role-mapping",
		Username:   "disabled.role.mapping",
		Enabled:    true,
		Cohorts: []runtimecontract.ExternalCohort{
			{Kind: "group", Key: "engineering"},
		},
	})
	if err != nil {
		t.Fatalf("upsert external user: %v", err)
	}
	if result.RBACChanged {
		t.Fatal("upsert external user RBACChanged = true, want false for disabled-role mapping")
	}

	bindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(user.IDEQ(result.User.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("role binding count = %d, want 0", bindingCount)
	}
	grantCount, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(result.User.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count external cohort grants: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("external cohort grant count = %d, want 0", grantCount)
	}
}

func TestExternalAuthService_ReconcileManagedBindingDeletesGrantForDisabledRoleMapping(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_disabled_role_mapping_delete")
	service := NewExternalAuthService(client)

	disabledRole, err := client.Role.Create().
		SetID("role-ext-auth-disabled-mapping-delete").
		SetName("external_auth_disabled_mapping_delete").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-disabled-mapping-delete").
		SetUsername("disabled.mapping.delete").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-disabled-role-delete").
		SetProviderID("provider-disabled-role-delete").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(disabledRole.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-disabled-role-delete").
		SetUserID(userEnt.ID).
		SetRoleID(disabledRole.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create managed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-disabled-role-delete").
		SetUserID(userEnt.ID).
		SetProviderID("provider-disabled-role-delete").
		SetBindingKey(externalCohortBindingKey(disabledRole.ID, "global", "", nil)).
		SetRoleBindingID(bindingEnt.ID).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	rbacChanged, err := reconcileExternalCohortRBACChangedTxForTest(t, client, service, userEnt.ID, "provider-disabled-role-delete", []runtimecontract.ExternalCohort{
		{Kind: "group", Key: "engineering"},
	})
	if err != nil {
		t.Fatalf("reconcileExternalCohortRBAC() error = %v", err)
	}
	if !rbacChanged {
		t.Fatal("reconcileExternalCohortRBAC() RBACChanged = false, want true for stale disabled-role grant cleanup")
	}
	if _, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("external cohort grant should be deleted, got err %v", err)
	}
	if _, err := client.RoleBinding.Get(t.Context(), bindingEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("managed role binding should be deleted, got err %v", err)
	}
}

func TestExternalAuthService_UpsertExternalUserSkipsRBACForDisabledUser(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_disabled_user_skip_rbac")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-disabled-user-skip").
		SetName("external_auth_disabled_user_skip").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-disabled-user-skip").
		SetProviderID("provider-disabled-user-skip").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-disabled-user-skip", runtimecontract.AuthResult{
		ExternalID: "ext-disabled-user-skip",
		Username:   "disabled.user.skip",
		Enabled:    false,
		Cohorts: []runtimecontract.ExternalCohort{
			{Kind: "group", Key: "engineering", DisplayName: "Engineering"},
		},
	})
	if err != nil {
		t.Fatalf("upsert disabled external user: %v", err)
	}
	if !result.Created {
		t.Fatal("Created = false, want true")
	}
	if result.User.Enabled {
		t.Fatal("created user Enabled = true, want false")
	}
	if result.RBACChanged {
		t.Fatal("RBACChanged = true, want false when disabled user has no existing managed RBAC")
	}

	bindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(user.IDEQ(result.User.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("role binding count = %d, want 0", bindingCount)
	}
	grantCount, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(result.User.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count external cohort grants: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("external cohort grant count = %d, want 0", grantCount)
	}
	observedCount, err := client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ("provider-disabled-user-skip")).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count observed external cohorts: %v", err)
	}
	if observedCount != 1 {
		t.Fatalf("observed external cohort count = %d, want 1", observedCount)
	}
}

func TestExternalAuthService_UpsertExternalUserDeletesManagedRBACWhenUserDisabled(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_disabled_user_deletes_rbac")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-disabled-user-delete").
		SetName("external_auth_disabled_user_delete").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-disabled-user-delete").
		SetUsername("disabled.user.delete").
		SetAuthProviderID("provider-disabled-user-delete").
		SetExternalID("ext-disabled-user-delete").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external user: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-disabled-user-delete").
		SetProviderID("provider-disabled-user-delete").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-disabled-user-delete").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create managed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-disabled-user-delete").
		SetUserID(userEnt.ID).
		SetProviderID("provider-disabled-user-delete").
		SetBindingKey(externalCohortBindingKey(roleEnt.ID, "global", "", nil)).
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{"cohort-mapping-disabled-user-delete"}).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-disabled-user-delete", runtimecontract.AuthResult{
		ExternalID: "ext-disabled-user-delete",
		Username:   "disabled.user.delete",
		Enabled:    false,
		Cohorts: []runtimecontract.ExternalCohort{
			{Kind: "group", Key: "engineering"},
		},
	})
	if err != nil {
		t.Fatalf("upsert disabled external user: %v", err)
	}
	if !result.Updated {
		t.Fatal("Updated = false, want true")
	}
	if result.User.Enabled {
		t.Fatal("updated user Enabled = true, want false")
	}
	if !result.RBACChanged {
		t.Fatal("RBACChanged = false, want true for disabled user managed RBAC cleanup")
	}
	if _, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("external cohort grant should be deleted, got err %v", err)
	}
	if _, err := client.RoleBinding.Get(t.Context(), bindingEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("managed role binding should be deleted, got err %v", err)
	}
}

func TestExternalAuthService_ReconcileManagedBindingRollsBackRoleBindingOnGrantCreateFailure(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_reconcile_grant_create_rollback")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-create-rollback").
		SetName("external_auth_create_rollback").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-create-rollback").
		SetUsername("create.rollback").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-create-rollback").
		SetProviderID("provider-create-rollback").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}
	client.ExternalCohortGrant.Use(enthook.On(
		enthook.FixedError(errors.New("external cohort grant create unavailable")),
		ent.OpCreate,
	))

	err = reconcileExternalCohortRBACTxForTest(t, client, service, userEnt.ID, "provider-create-rollback", []runtimecontract.ExternalCohort{
		{Kind: "group", Key: "engineering"},
	})
	if err == nil {
		t.Fatal("reconcileExternalCohortRBAC() error = nil, want non-nil")
	}
	if got := err.Error(); !strings.Contains(got, "external cohort grant create unavailable") {
		t.Fatalf("error = %q, want grant create failure", got)
	}

	bindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(user.IDEQ(userEnt.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("role binding count after rollback = %d, want 0", bindingCount)
	}
	grantCount, err := client.ExternalCohortGrant.Query().
		Where(externalcohortgrant.UserIDEQ(userEnt.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count external cohort grants: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("external cohort grant count after rollback = %d, want 0", grantCount)
	}
}

func TestExternalAuthService_ReconcileManagedBindingRollsBackGrantDeleteOnRoleBindingDeleteFailure(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_reconcile_grant_delete_rollback")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-delete-rollback").
		SetName("external_auth_delete_rollback").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-delete-rollback").
		SetUsername("delete.rollback").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-delete-rollback").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-delete-rollback").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-rollback").
		SetBindingKey("stale-binding").
		SetRoleBindingID(bindingEnt.ID).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}
	client.RoleBinding.Use(enthook.On(
		enthook.FixedError(errors.New("managed role binding delete unavailable")),
		ent.OpDeleteOne,
	))

	err = reconcileExternalCohortRBACTxForTest(t, client, service, userEnt.ID, "provider-delete-rollback", nil)
	if err == nil {
		t.Fatal("reconcileExternalCohortRBAC() error = nil, want non-nil")
	}
	if got := err.Error(); !strings.Contains(got, "managed role binding delete unavailable") {
		t.Fatalf("error = %q, want role binding delete failure", got)
	}

	if _, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID); err != nil {
		t.Fatalf("external cohort grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(t.Context(), bindingEnt.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
}

func TestExternalAuthService_ReconcileManagedBindingPreservesUnmanagedRoleBindingOnGrantDelete(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_preserve_unmanaged_binding_delete")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-preserve-unmanaged-delete").
		SetName("external_auth_preserve_unmanaged_delete").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-preserve-unmanaged-delete").
		SetUsername("preserve.unmanaged.delete").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-preserve-unmanaged-delete").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create unmanaged role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-preserve-unmanaged-delete").
		SetUserID(userEnt.ID).
		SetProviderID("provider-preserve-unmanaged-delete").
		SetBindingKey("stale-binding").
		SetRoleBindingID(bindingEnt.ID).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	if err := reconcileExternalCohortRBACTxForTest(t, client, service, userEnt.ID, "provider-preserve-unmanaged-delete", nil); err != nil {
		t.Fatalf("reconcileExternalCohortRBAC() error = %v", err)
	}

	if _, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("external cohort grant should be deleted, got err %v", err)
	}
	if _, err := client.RoleBinding.Get(t.Context(), bindingEnt.ID); err != nil {
		t.Fatalf("unmanaged role binding should remain: %v", err)
	}
}

func TestExternalAuthService_ReconcileManagedBindingReplacesUnmanagedGrantBinding(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_replace_unmanaged_binding")
	service := NewExternalAuthService(client)

	roleEnt, err := client.Role.Create().
		SetID("role-ext-auth-replace-unmanaged").
		SetName("external_auth_replace_unmanaged").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-replace-unmanaged").
		SetUsername("replace.unmanaged").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-replace-unmanaged").
		SetProviderID("provider-replace-unmanaged").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}
	unmanagedBinding, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-replace-unmanaged").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create unmanaged role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-replace-unmanaged").
		SetUserID(userEnt.ID).
		SetProviderID("provider-replace-unmanaged").
		SetBindingKey(externalCohortBindingKey(roleEnt.ID, "global", "", []string{"prod"})).
		SetRoleBindingID(unmanagedBinding.ID).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	reconcileErr := reconcileExternalCohortRBACTxForTest(t, client, service, userEnt.ID, "provider-replace-unmanaged", []runtimecontract.ExternalCohort{
		{Kind: "group", Key: "engineering"},
	})
	if reconcileErr != nil {
		t.Fatalf("reconcileExternalCohortRBAC() error = %v", reconcileErr)
	}

	reloadedGrant, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID)
	if err != nil {
		t.Fatalf("query external cohort grant: %v", err)
	}
	if reloadedGrant.RoleBindingID == unmanagedBinding.ID {
		t.Fatal("external cohort grant still points at unmanaged role binding")
	}
	if _, getErr := client.RoleBinding.Get(t.Context(), unmanagedBinding.ID); getErr != nil {
		t.Fatalf("unmanaged role binding should remain: %v", getErr)
	}
	managedBinding, err := client.RoleBinding.Query().
		Where(
			rolebinding.IDEQ(reloadedGrant.RoleBindingID),
			rolebinding.CreatedByEQ(externalCohortRoleBindingActor),
			rolebinding.HasUserWith(user.IDEQ(userEnt.ID)),
			rolebinding.HasRoleWith(role.IDEQ(roleEnt.ID)),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query replacement managed role binding: %v", err)
	}
	if managedBinding.ScopeType != "global" {
		t.Fatalf("replacement scope_type = %q, want global", managedBinding.ScopeType)
	}
	if got := strings.Join(managedBinding.AllowedEnvironments, ","); got != "prod" {
		t.Fatalf("replacement allowed_environments = %q, want prod", got)
	}
}

func TestExternalAuthService_ReconcileManagedBindingRepairsManagedRoleBindingDrift(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_repair_managed_binding")
	service := NewExternalAuthService(client)

	oldRole, err := client.Role.Create().
		SetID("role-ext-auth-repair-managed-old").
		SetName("external_auth_repair_managed_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	desiredRole, err := client.Role.Create().
		SetID("role-ext-auth-repair-managed-desired").
		SetName("external_auth_repair_managed_desired").
		SetPermissions([]string{"vm:read", "vm:update"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create desired role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-ext-auth-repair-managed").
		SetUsername("repair.managed").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-repair-managed").
		SetProviderID("provider-repair-managed").
		SetCohortKind("group").
		SetCohortKey("engineering").
		SetRoleID(desiredRole.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create mapping: %v", mappingErr)
	}
	driftedBinding, err := client.RoleBinding.Create().
		SetID("rb-ext-auth-repair-managed").
		SetUserID(userEnt.ID).
		SetRoleID(oldRole.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create drifted managed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-ext-auth-repair-managed").
		SetUserID(userEnt.ID).
		SetProviderID("provider-repair-managed").
		SetBindingKey(externalCohortBindingKey(desiredRole.ID, "global", "", []string{"prod"})).
		SetRoleBindingID(driftedBinding.ID).
		SetLastAppliedAt(userEnt.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	reconcileErr := reconcileExternalCohortRBACTxForTest(t, client, service, userEnt.ID, "provider-repair-managed", []runtimecontract.ExternalCohort{
		{Kind: "group", Key: "engineering"},
	})
	if reconcileErr != nil {
		t.Fatalf("reconcileExternalCohortRBAC() error = %v", reconcileErr)
	}

	reloadedGrant, err := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID)
	if err != nil {
		t.Fatalf("query external cohort grant: %v", err)
	}
	if reloadedGrant.RoleBindingID != driftedBinding.ID {
		t.Fatalf("grant role_binding_id = %q, want repaired binding %q", reloadedGrant.RoleBindingID, driftedBinding.ID)
	}
	repairedBinding, err := client.RoleBinding.Query().
		Where(rolebinding.IDEQ(driftedBinding.ID)).
		WithRole().
		Only(t.Context())
	if err != nil {
		t.Fatalf("query repaired role binding: %v", err)
	}
	repairedRole, err := repairedBinding.Edges.RoleOrErr()
	if err != nil {
		t.Fatalf("query repaired role: %v", err)
	}
	if repairedRole.ID != desiredRole.ID {
		t.Fatalf("repaired role id = %q, want %q", repairedRole.ID, desiredRole.ID)
	}
	if repairedBinding.ScopeType != "global" {
		t.Fatalf("repaired scope_type = %q, want global", repairedBinding.ScopeType)
	}
	if repairedBinding.ScopeID != "" {
		t.Fatalf("repaired scope_id = %q, want empty", repairedBinding.ScopeID)
	}
	if got := strings.Join(repairedBinding.AllowedEnvironments, ","); got != "prod" {
		t.Fatalf("repaired allowed_environments = %q, want prod", got)
	}
}

func reconcileExternalCohortRBACTxForTest(
	t *testing.T,
	client *ent.Client,
	service *ExternalAuthService,
	userID string,
	providerID string,
	cohorts []runtimecontract.ExternalCohort,
) error {
	t.Helper()
	_, err := reconcileExternalCohortRBACChangedTxForTest(t, client, service, userID, providerID, cohorts)
	return err
}

func reconcileExternalCohortRBACChangedTxForTest(
	t *testing.T,
	client *ent.Client,
	service *ExternalAuthService,
	userID string,
	providerID string,
	cohorts []runtimecontract.ExternalCohort,
) (bool, error) {
	t.Helper()

	tx, err := client.Tx(t.Context())
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	txService := service.WithClient(tx.Client())
	rbacChanged, err := txService.reconcileExternalCohortRBAC(t.Context(), userID, providerID, cohorts)
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			t.Fatalf("rollback tx: %v", rollbackErr)
		}
		return false, err
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}
	return rbacChanged, nil
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
	createExternalAuthProviderForTest(t, client, "provider-directory")

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

func TestExternalAuthService_UpsertExternalUser_LoginOnlyClaimRepairsMissingDirectoryOwner(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_repairs_missing_owner")
	service := NewExternalAuthService(client)
	createExternalAuthProviderForTest(t, client, "provider-sso")

	importedUser, err := client.User.Create().
		SetID("user-imported-missing-provider").
		SetUsername("alice@example.com").
		SetEmail("alice@example.com").
		SetDisplayName("Alice Imported").
		SetAuthProviderID("provider-deleted").
		SetExternalID("alice@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create imported user: %v", err)
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
	if result.User.AuthProviderID != "provider-sso" {
		t.Fatalf("auth_provider_id = %q, want provider-sso", result.User.AuthProviderID)
	}
	if result.User.ExternalID != "alice@example.com" {
		t.Fatalf("external_id = %q, want alice@example.com", result.User.ExternalID)
	}
}

func TestExternalAuthService_UpsertExternalUser_LoginOnlyClaimRepairsDisabledDirectoryOwner(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "external_auth_service_repairs_disabled_owner")
	service := NewExternalAuthService(client)
	createExternalAuthProviderForTest(t, client, "provider-sso")
	createExternalAuthProviderForTestWithEnabled(t, client, "provider-directory-disabled", false)

	roleEnt, err := client.Role.Create().
		SetID("role-disabled-owner-grant").
		SetName("disabled_owner_grant").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	importedUser, err := client.User.Create().
		SetID("user-imported-disabled-provider").
		SetUsername("alice-disabled@example.com").
		SetEmail("alice-disabled@example.com").
		SetDisplayName("Alice Disabled Provider").
		SetAuthProviderID("provider-directory-disabled").
		SetExternalID("alice-disabled@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create imported user: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-disabled-owner-grant").
		SetUserID(importedUser.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create managed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-disabled-owner").
		SetUserID(importedUser.ID).
		SetProviderID("provider-directory-disabled").
		SetBindingKey("role-disabled-owner-grant|global||").
		SetRoleBindingID(bindingEnt.ID).
		SetLastAppliedAt(importedUser.CreatedAt).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}

	result, err := service.UpsertExternalUser(t.Context(), "provider-sso", runtimecontract.AuthResult{
		ExternalID:         "alice-disabled@example.com",
		Username:           "alice-disabled@example.com",
		DisplayName:        "Alice SSO",
		Email:              "alice-disabled@example.com",
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
	if result.User.AuthProviderID != "provider-sso" {
		t.Fatalf("auth_provider_id = %q, want provider-sso", result.User.AuthProviderID)
	}
	if result.User.ExternalID != "alice-disabled@example.com" {
		t.Fatalf("external_id = %q, want alice-disabled@example.com", result.User.ExternalID)
	}
	if !result.RBACChanged {
		t.Fatal("RBACChanged = false, want true after stale external cohort grant cleanup")
	}
	if _, getErr := client.ExternalCohortGrant.Get(t.Context(), grantEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("stale external cohort grant should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RoleBinding.Get(t.Context(), bindingEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("stale managed role binding should be deleted, err=%v", getErr)
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
