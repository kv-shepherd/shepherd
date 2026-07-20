package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/notification"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	entrole "kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type genericStorageProfileProvider struct {
	*provider.MockProvider
}

func newGenericStorageProfileProvider() *genericStorageProfileProvider {
	return &genericStorageProfileProvider{MockProvider: provider.NewMockProvider()}
}

func (p *genericStorageProfileProvider) GetStorageProfile(
	ctx context.Context,
	cluster,
	name string,
) (*domain.StorageProfile, error) {
	if profile, err := p.MockProvider.GetStorageProfile(ctx, cluster, name); err == nil {
		return profile, nil
	}
	return &domain.StorageProfile{
		Name: name,
		ClaimPropertySets: []domain.StorageClaimPropertySet{
			{
				AccessModes: []string{"ReadWriteOnce"},
				VolumeMode:  "Filesystem",
			},
		},
		DefaultVolumeMode: "Filesystem",
	}, nil
}

func TestAdminUserRoleBindingAndAuthProviderCRUD(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	createRoleCtx, createRoleW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/roles",
		`{"name":"DevLead","display_name":"Dev Lead","permissions":["system:read","vm:read","vm:operate"],"enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateRole(createRoleCtx)
	if createRoleW.Code != http.StatusCreated {
		t.Fatalf("create role status = %d, want %d, body=%s", createRoleW.Code, http.StatusCreated, createRoleW.Body.String())
	}
	var createdRole generated.Role
	mustDecodeJSON(t, createRoleW.Body.Bytes(), &createdRole)
	if createdRole.Id == "" {
		t.Fatal("created role id is empty")
	}

	createUserCtx, createUserW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users",
		`{"username":"dev.user","password":"dev-user-123","display_name":"Dev User","enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateUser(createUserCtx)
	if createUserW.Code != http.StatusCreated {
		t.Fatalf("create user status = %d, want %d, body=%s", createUserW.Code, http.StatusCreated, createUserW.Body.String())
	}
	var createdUser generated.User
	mustDecodeJSON(t, createUserW.Body.Bytes(), &createdUser)
	if createdUser.Id == "" {
		t.Fatal("created user id is empty")
	}

	bindCtx, bindW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+createdUser.Id+"/role-bindings",
		`{"role_id":"`+createdRole.Id+`","scope_type":"global","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateUserRoleBinding(bindCtx, createdUser.Id)
	if bindW.Code != http.StatusCreated {
		t.Fatalf("create role binding status = %d, want %d, body=%s", bindW.Code, http.StatusCreated, bindW.Body.String())
	}
	var createdBinding generated.GlobalRoleBinding
	mustDecodeJSON(t, bindW.Body.Bytes(), &createdBinding)
	if createdBinding.Id == "" {
		t.Fatal("created role binding id is empty")
	}

	listBindingsCtx, listBindingsW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users/"+createdUser.Id+"/role-bindings",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListUserRoleBindings(listBindingsCtx, createdUser.Id)
	if listBindingsW.Code != http.StatusOK {
		t.Fatalf("list role bindings status = %d, want %d, body=%s", listBindingsW.Code, http.StatusOK, listBindingsW.Body.String())
	}

	createProviderCtx, createProviderW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{"name":"Corp SSO","auth_type":"oidc","enabled":true,"config":{"issuer":"https://sso.example.com","client_id":"shepherd","client_secret":"secret"}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createProviderCtx)
	if createProviderW.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, want %d, body=%s", createProviderW.Code, http.StatusCreated, createProviderW.Body.String())
	}
	var createdProvider generated.AuthProvider
	mustDecodeJSON(t, createProviderW.Body.Bytes(), &createdProvider)
	if createdProvider.Id == "" {
		t.Fatal("created provider id is empty")
	}

	updateProviderCtx, updateProviderW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+createdProvider.Id,
		`{"enabled":false}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAuthProvider(updateProviderCtx, createdProvider.Id)
	if updateProviderW.Code != http.StatusOK {
		t.Fatalf("update provider status = %d, want %d, body=%s", updateProviderW.Code, http.StatusOK, updateProviderW.Body.String())
	}

	deleteBindingCtx, deleteBindingW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+createdUser.Id+"/role-bindings/"+createdBinding.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteUserRoleBinding(deleteBindingCtx, createdUser.Id, createdBinding.Id)
	if got := deleteBindingCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete role binding status = %d, want %d, body=%s", got, http.StatusNoContent, deleteBindingW.Body.String())
	}

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+createdProvider.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAuthProvider(deleteProviderCtx, createdProvider.Id)
	if got := deleteProviderCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete provider status = %d, want %d, body=%s", got, http.StatusNoContent, deleteProviderW.Body.String())
	}

	deleteUserCtx, deleteUserW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+createdUser.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteUser(deleteUserCtx, createdUser.Id)
	if got := deleteUserCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete user status = %d, want %d, body=%s", got, http.StatusNoContent, deleteUserW.Body.String())
	}

	deleteRoleCtx, deleteRoleW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+createdRole.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteRole(deleteRoleCtx, createdRole.Id)
	if got := deleteRoleCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete role status = %d, want %d, body=%s", got, http.StatusNoContent, deleteRoleW.Body.String())
	}

	if _, err := client.User.Get(t.Context(), createdUser.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected user deleted, err=%v", err)
	}
	if _, err := client.Role.Get(t.Context(), createdRole.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected role deleted, err=%v", err)
	}
	if _, err := client.AuthProvider.Get(t.Context(), createdProvider.Id); !ent.IsNotFound(err) {
		t.Fatalf("expected auth provider deleted, err=%v", err)
	}
}

func TestDeleteAuthProviderReturnsConflictWhenUsersRemainLinked(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	createProviderCtx, createProviderW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{"name":"Legacy Corp SSO","auth_type":"oidc","enabled":true,"config":{"issuer":"https://sso.example.com","client_id":"shepherd","client_secret":"secret"}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createProviderCtx)
	if createProviderW.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, want %d, body=%s", createProviderW.Code, http.StatusCreated, createProviderW.Body.String())
	}
	var providerRecord generated.AuthProvider
	mustDecodeJSON(t, createProviderW.Body.Bytes(), &providerRecord)

	if _, err := client.User.Create().
		SetID("linked-user-auth-provider-conflict").
		SetUsername("linked.user").
		SetDisplayName("Linked User").
		SetPasswordHash("not-used-in-this-test").
		SetEnabled(true).
		SetAuthProviderID(providerRecord.Id).
		Save(t.Context()); err != nil {
		t.Fatalf("create linked user: %v", err)
	}

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerRecord.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAuthProvider(deleteProviderCtx, providerRecord.Id)
	if got := deleteProviderCtx.Writer.Status(); got != http.StatusConflict {
		t.Fatalf("delete provider status = %d, want %d, body=%s", got, http.StatusConflict, deleteProviderW.Body.String())
	}

	var apiErr generated.Error
	mustDecodeJSON(t, deleteProviderW.Body.Bytes(), &apiErr)
	if apiErr.Code != "AUTH_PROVIDER_IN_USE" {
		t.Fatalf("delete provider error code = %q, want %q", apiErr.Code, "AUTH_PROVIDER_IN_USE")
	}
}

func TestDeleteAuthProvider_CleansExternalCohortState(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-delete-cohort-cleanup").
		SetName("Delete Cohort Cleanup").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer": "https://sso.example.com"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	roleEnt, err := client.Role.Create().
		SetID("role-delete-provider-cleanup").
		SetName("delete_provider_cleanup").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-delete-provider-cleanup").
		SetUsername("delete.provider.cleanup").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	cohortEnt, err := client.ExternalCohort.Create().
		SetID("cohort-delete-provider-cleanup").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetDisplayName("Ops").
		SetSourceField("groups").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-provider-cleanup").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-provider-cleanup").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-provider-cleanup").
		SetUserID(userEnt.ID).
		SetProviderID(providerEnt.ID).
		SetBindingKey("delete-provider-cleanup").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	srv.DeleteAuthProvider(deleteProviderCtx, providerEnt.ID)
	if got := deleteProviderCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete provider status = %d, want %d, body=%s", got, http.StatusNoContent, deleteProviderW.Body.String())
	}
	if _, err := client.AuthProvider.Get(ctx, providerEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("provider should be deleted, err=%v", err)
	}
	if _, err := client.ExternalCohort.Get(ctx, cohortEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("cohort should be deleted, err=%v", err)
	}
	if _, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("mapping should be deleted, err=%v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("grant should be deleted, err=%v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingEnt.ID); !ent.IsNotFound(err) {
		t.Fatalf("managed role binding should be deleted, err=%v", err)
	}
	if _, err := client.User.Get(ctx, userEnt.ID); err != nil {
		t.Fatalf("user should remain: %v", err)
	}
	if _, err := client.Role.Get(ctx, roleEnt.ID); err != nil {
		t.Fatalf("role should remain: %v", err)
	}
}

func TestDeleteAuthProvider_RevokesAffectedExternalCohortGrantSessions(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "admin_identity_delete_provider_cohort_revoke")
	ctx := t.Context()

	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-delete-cohort-revoke").
		SetName("Delete Cohort Revoke").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer": "https://sso.example.com"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	roleEnt, err := client.Role.Create().
		SetID("role-delete-provider-revoke").
		SetName("delete_provider_revoke").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-delete-provider-revoke").
		SetUsername("delete.provider.revoke").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-provider-revoke").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-provider-revoke").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	_, err = client.ExternalCohortGrant.Create().
		SetID("grant-delete-provider-revoke").
		SetUserID(userEnt.ID).
		SetProviderID(providerEnt.ID).
		SetBindingKey("delete-provider-revoke").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	srv.DeleteAuthProvider(deleteProviderCtx, providerEnt.ID)
	if got := deleteProviderCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete provider status = %d, want %d, body=%s", got, http.StatusNoContent, deleteProviderW.Body.String())
	}
	afterVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("read session version after provider delete: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after provider delete = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func TestDeleteAuthProvider_RollsBackWhenCohortGrantCleanupFails(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-delete-cohort-rollback").
		SetName("Delete Cohort Rollback").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer": "https://sso.example.com"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	roleEnt, err := client.Role.Create().
		SetID("role-delete-provider-rollback").
		SetName("delete_provider_rollback").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-delete-provider-rollback").
		SetUsername("delete.provider.rollback").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	cohortEnt, err := client.ExternalCohort.Create().
		SetID("cohort-delete-provider-rollback").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetDisplayName("Ops").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-provider-rollback").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-provider-rollback").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-provider-rollback").
		SetUserID(userEnt.ID).
		SetProviderID(providerEnt.ID).
		SetBindingKey("delete-provider-rollback").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}
	client.RoleBinding.Use(enthook.On(
		enthook.FixedError(errors.New("role binding delete unavailable")),
		ent.OpDeleteOne,
	))

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	srv.DeleteAuthProvider(deleteProviderCtx, providerEnt.ID)
	if deleteProviderW.Code != http.StatusInternalServerError {
		t.Fatalf("delete provider status = %d, want %d, body=%s", deleteProviderW.Code, http.StatusInternalServerError, deleteProviderW.Body.String())
	}
	if _, err := client.AuthProvider.Get(ctx, providerEnt.ID); err != nil {
		t.Fatalf("provider should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohort.Get(ctx, cohortEnt.ID); err != nil {
		t.Fatalf("cohort should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID); err != nil {
		t.Fatalf("mapping should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); err != nil {
		t.Fatalf("grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingEnt.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
}

func TestListPermissions_ExcludesUnknownPermissionsStoredInRoles(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	if _, err := client.Role.Create().
		SetID("role-legacy-compat").
		SetName("LegacyCompatRole").
		SetDisplayName("Legacy Compat Role").
		SetPermissions([]string{"legacy:compat"}).
		SetBuiltIn(false).
		Save(t.Context()); err != nil {
		t.Fatalf("create legacy role: %v", err)
	}

	c, w := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/permissions",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListPermissions(c)
	if w.Code != http.StatusOK {
		t.Fatalf("list permissions status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var response generated.PermissionList
	mustDecodeJSON(t, w.Body.Bytes(), &response)
	for _, permission := range response.Items {
		if permission.Key == "legacy:compat" {
			t.Fatalf("unexpected legacy permission in catalog: %s", permission.Key)
		}
	}
}

func TestCreateRole_RejectsUnsupportedPermissionKeys(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)

	c, w := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/roles",
		`{"name":"CompatRole","permissions":["cluster:manage"],"enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateRole(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("create role status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var apiErr generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &apiErr)
	if apiErr.Message != "unsupported permission key: cluster:manage" {
		t.Fatalf("unexpected error message = %q", apiErr.Message)
	}
}

func TestUserRoleBinding_IncludesRoleAndScopeDisplayNames(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	systemEnt := mustCreateSystem(t, client, "sys-rbac-1", "commerce", "owner-1")
	serviceEnt := mustCreateService(t, client, "svc-rbac-1", "billing", systemEnt.ID, "billing service")

	roleEnt, err := client.Role.Create().
		SetID("role-rbac-1").
		SetName("billing_admin").
		SetDisplayName("Billing Admin").
		SetPermissions([]string{"service:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-rbac-1").
		SetUsername("finance.alice").
		SetDisplayName("Alice Finance").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	bindCtx, bindW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userEnt.ID+"/role-bindings",
		`{"role_id":"`+roleEnt.ID+`","scope_type":"service","scope_id":"`+serviceEnt.ID+`","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateUserRoleBinding(bindCtx, userEnt.ID)
	if bindW.Code != http.StatusCreated {
		t.Fatalf("create role binding status = %d, want %d, body=%s", bindW.Code, http.StatusCreated, bindW.Body.String())
	}
	var createdBinding generated.GlobalRoleBinding
	mustDecodeJSON(t, bindW.Body.Bytes(), &createdBinding)
	if createdBinding.RoleDisplayName != "Billing Admin" {
		t.Fatalf("role_display_name = %q, want %q", createdBinding.RoleDisplayName, "Billing Admin")
	}
	if createdBinding.ScopeDisplayName != "commerce / billing" {
		t.Fatalf("scope_display_name = %q, want %q", createdBinding.ScopeDisplayName, "commerce / billing")
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users/"+userEnt.ID+"/role-bindings",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListUserRoleBindings(listCtx, userEnt.ID)
	if listW.Code != http.StatusOK {
		t.Fatalf("list role bindings status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}
	var list generated.GlobalRoleBindingList
	mustDecodeJSON(t, listW.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("bindings len = %d, want 1", len(list.Items))
	}
	if list.Items[0].ScopeDisplayName != "commerce / billing" {
		t.Fatalf("scope_display_name = %q, want %q", list.Items[0].ScopeDisplayName, "commerce / billing")
	}
}

func TestCreateUserRoleBinding_RejectsDisabledRole(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()
	roleEnt, err := client.Role.Create().
		SetID("role-disabled-user-binding").
		SetName("disabled_user_binding").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(false).
		Save(ctx)
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-disabled-role-binding").
		SetUsername("disabled.role.binding").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	bindCtx, bindW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userEnt.ID+"/role-bindings",
		`{"role_id":"`+roleEnt.ID+`","scope_type":"global"}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.CreateUserRoleBinding(bindCtx, userEnt.ID)
	if bindW.Code != http.StatusConflict {
		t.Fatalf("create role binding status = %d, want %d, body=%s", bindW.Code, http.StatusConflict, bindW.Body.String())
	}
	assertErrorCode(t, bindW.Body.Bytes(), "ROLE_DISABLED")

	count, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(userEnt.ID)),
			rolebinding.HasRoleWith(entrole.IDEQ(roleEnt.ID)),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count disabled role bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled role binding count = %d, want 0", count)
	}
}

func TestCreateUserRoleBinding_RejectsDisabledUser(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()
	roleEnt, err := client.Role.Create().
		SetID("role-disabled-user-target").
		SetName("disabled_user_target_role").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-disabled-role-target").
		SetUsername("disabled.role.target").
		SetEnabled(false).
		Save(ctx)
	if err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	bindCtx, bindW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userEnt.ID+"/role-bindings",
		`{"role_id":"`+roleEnt.ID+`","scope_type":"global"}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.CreateUserRoleBinding(bindCtx, userEnt.ID)
	if bindW.Code != http.StatusConflict {
		t.Fatalf("create disabled-user role binding status = %d, want %d, body=%s", bindW.Code, http.StatusConflict, bindW.Body.String())
	}
	assertErrorCode(t, bindW.Body.Bytes(), "USER_DISABLED")

	count, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(userEnt.ID)),
			rolebinding.HasRoleWith(entrole.IDEQ(roleEnt.ID)),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count disabled user role bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled user role binding count = %d, want 0", count)
	}
}

func TestListUserRoleBindings_ManagedStateRequiresMapperOwnedGrant(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-rbac-managed-state").
		SetName("rbac_managed_state").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-rbac-managed-state").
		SetUsername("managed.state").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	manualBinding, err := client.RoleBinding.Create().
		SetID("rb-rbac-managed-state-manual").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create manual role binding: %v", err)
	}
	mapperBinding, err := client.RoleBinding.Create().
		SetID("rb-rbac-managed-state-mapper").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-rbac-managed-state").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapper role binding: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Create().
		SetID("grant-rbac-managed-state-manual").
		SetUserID(userEnt.ID).
		SetProviderID("provider-rbac-managed-state").
		SetBindingKey("manual-corrupt-grant").
		SetRoleBindingID(manualBinding.ID).
		SetLastAppliedAt(time.Now()).
		Save(ctx); err != nil {
		t.Fatalf("create manual external cohort grant: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Create().
		SetID("grant-rbac-managed-state-mapper").
		SetUserID(userEnt.ID).
		SetProviderID("provider-rbac-managed-state").
		SetBindingKey("mapper-grant").
		SetRoleBindingID(mapperBinding.ID).
		SetLastAppliedAt(time.Now()).
		Save(ctx); err != nil {
		t.Fatalf("create mapper external cohort grant: %v", err)
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users/"+userEnt.ID+"/role-bindings",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUserRoleBindings(listCtx, userEnt.ID)
	if listW.Code != http.StatusOK {
		t.Fatalf("list role bindings status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}
	var list generated.GlobalRoleBindingList
	mustDecodeJSON(t, listW.Body.Bytes(), &list)
	bindingsByID := make(map[string]generated.GlobalRoleBinding, len(list.Items))
	for _, item := range list.Items {
		bindingsByID[item.Id] = item
	}

	manualItem, ok := bindingsByID[manualBinding.ID]
	if !ok {
		t.Fatalf("manual role binding %q missing from list", manualBinding.ID)
	}
	if manualItem.Managed {
		t.Fatal("manual role binding with corrupt grant was reported managed")
	}
	if manualItem.ManagedSource != "" {
		t.Fatalf("manual role binding managed_source = %q, want empty", manualItem.ManagedSource)
	}

	mapperItem, ok := bindingsByID[mapperBinding.ID]
	if !ok {
		t.Fatalf("mapper role binding %q missing from list", mapperBinding.ID)
	}
	if !mapperItem.Managed {
		t.Fatal("mapper-owned role binding with matching grant was not reported managed")
	}
	if mapperItem.ManagedSource != "external_cohort" {
		t.Fatalf("mapper role binding managed_source = %q, want external_cohort", mapperItem.ManagedSource)
	}
}

func TestListSystemMemberCandidates_ExcludesExistingMembersAndSupportsSearch(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	systemID := "11111111-2222-3333-4444-555555555555"
	mustCreateSystem(t, client, systemID, "shop-members", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", systemID, "owner")

	for _, user := range []struct {
		id          string
		username    string
		displayName string
		email       string
		enabled     bool
	}{
		{id: "user-alice", username: "alice", displayName: "Alice Zhang", email: "alice@example.com", enabled: true},
		{id: "user-bob", username: "bob", displayName: "Bob Platform", email: "bob@example.com", enabled: true},
		{id: "user-carol", username: "carol", displayName: "Carol Ops", email: "ops@example.com", enabled: true},
		{id: "user-disabled-candidate", username: "disabled-candidate", displayName: "Disabled Candidate", email: "disabled@example.com", enabled: false},
		{id: "user-existing", username: "existing", displayName: "Existing Member", email: "existing@example.com", enabled: true},
	} {
		_, err := client.User.Create().
			SetID(user.id).
			SetUsername(user.username).
			SetDisplayName(user.displayName).
			SetEmail(user.email).
			SetEnabled(user.enabled).
			Save(t.Context())
		if err != nil {
			t.Fatalf("create user %s: %v", user.id, err)
		}
	}
	if _, err := client.UserDirectoryProfile.Create().
		SetID("profile-user-bob").
		SetUserID("user-bob").
		SetAttributes(map[string]interface{}{
			"department": "Finance",
			"section":    "Ledger",
			"position":   "Engineer",
		}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(t.Context()); err != nil {
		t.Fatalf("create bob directory profile: %v", err)
	}
	mustCreateSystemBinding(t, client, "user-existing", systemID, "viewer")

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+systemID+"/member-candidates?page=1&per_page=20",
		"",
		"owner-1",
		[]string{"rbac:manage"},
	)
	srv.ListSystemMemberCandidates(listCtx, systemID, generated.ListSystemMemberCandidatesParams{
		Page:    1,
		PerPage: 20,
	})
	if listW.Code != http.StatusOK {
		t.Fatalf("list member candidates status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}
	var candidates generated.UserList
	mustDecodeJSON(t, listW.Body.Bytes(), &candidates)
	if len(candidates.Items) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates.Items))
	}
	if got := []string{candidates.Items[0].Username, candidates.Items[1].Username, candidates.Items[2].Username}; !slices.Equal(got, []string{"alice", "bob", "carol"}) {
		t.Fatalf("candidate usernames = %v, want [alice bob carol]", got)
	}

	searchCtx, searchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+systemID+"/member-candidates?page=1&per_page=20&search=ops",
		"",
		"owner-1",
		[]string{"rbac:manage"},
	)
	srv.ListSystemMemberCandidates(searchCtx, systemID, generated.ListSystemMemberCandidatesParams{
		Page:    1,
		PerPage: 20,
		Search:  "ops",
	})
	if searchW.Code != http.StatusOK {
		t.Fatalf("search member candidates status = %d, want %d, body=%s", searchW.Code, http.StatusOK, searchW.Body.String())
	}
	mustDecodeJSON(t, searchW.Body.Bytes(), &candidates)
	if len(candidates.Items) != 1 || candidates.Items[0].Username != "carol" {
		t.Fatalf("search candidates = %+v, want only carol", candidates.Items)
	}

	profileSearchCtx, profileSearchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/systems/"+systemID+"/member-candidates?page=1&per_page=20&search=finance",
		"",
		"owner-1",
		[]string{"rbac:manage"},
	)
	srv.ListSystemMemberCandidates(profileSearchCtx, systemID, generated.ListSystemMemberCandidatesParams{
		Page:    1,
		PerPage: 20,
		Search:  "finance",
	})
	if profileSearchW.Code != http.StatusOK {
		t.Fatalf("profile search member candidates status = %d, want %d, body=%s", profileSearchW.Code, http.StatusOK, profileSearchW.Body.String())
	}
	mustDecodeJSON(t, profileSearchW.Body.Bytes(), &candidates)
	if len(candidates.Items) != 1 || candidates.Items[0].Username != "bob" {
		t.Fatalf("profile search candidates = %+v, want only bob", candidates.Items)
	}
}

func TestAddSystemMember_RejectsDisabledUser(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	systemID := "system-add-disabled-member"
	mustCreateSystem(t, client, systemID, "disabled-member", "owner-1")
	mustCreateSystemBinding(t, client, "owner-1", systemID, "owner")
	if _, err := client.User.Create().
		SetID("user-disabled-member-add").
		SetUsername("disabled.member.add").
		SetDisplayName("Disabled Member Add").
		SetEmail("disabled.member.add@example.com").
		SetEnabled(false).
		Save(t.Context()); err != nil {
		t.Fatalf("create disabled user: %v", err)
	}

	addCtx, addW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems/"+systemID+"/members",
		`{"user_id":"user-disabled-member-add","role":"viewer"}`,
		"owner-1",
		[]string{"rbac:manage"},
	)
	srv.AddSystemMember(addCtx, systemID)
	if addW.Code != http.StatusConflict {
		t.Fatalf("add disabled member status = %d, want %d, body=%s", addW.Code, http.StatusConflict, addW.Body.String())
	}
	assertErrorCode(t, addW.Body.Bytes(), "USER_DISABLED")

	count, err := client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ("user-disabled-member-add"),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count disabled user member bindings: %v", err)
	}
	if count != 0 {
		t.Fatalf("disabled user member binding count = %d, want 0", count)
	}
}

func TestListUsers_SupportsSearch(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	for _, user := range []struct {
		id          string
		username    string
		displayName string
		email       string
	}{
		{id: "user-alice-search", username: "alice", displayName: "Alice Zhang", email: "alice@example.com"},
		{id: "user-bob-search", username: "bob", displayName: "Bob Platform", email: "bob@example.com"},
		{id: "user-carol-search", username: "carol", displayName: "Carol Ops", email: "ops@example.com"},
	} {
		_, err := client.User.Create().
			SetID(user.id).
			SetUsername(user.username).
			SetDisplayName(user.displayName).
			SetEmail(user.email).
			SetEnabled(true).
			Save(t.Context())
		if err != nil {
			t.Fatalf("create user %s: %v", user.id, err)
		}
	}

	searchCtx, searchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users?page=1&per_page=20&search=ops",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(searchCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  "ops",
	})
	if searchW.Code != http.StatusOK {
		t.Fatalf("list users search status = %d, want %d, body=%s", searchW.Code, http.StatusOK, searchW.Body.String())
	}

	var users generated.UserList
	mustDecodeJSON(t, searchW.Body.Bytes(), &users)
	if len(users.Items) != 1 || users.Items[0].Username != "carol" {
		t.Fatalf("search users = %+v, want only carol", users.Items)
	}

	quotedCtx, quotedW := newAuthedGinContext(
		t,
		http.MethodGet,
		`/admin/users?page=1&per_page=20&search=display_name:"Alice%20Zhang"`,
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(quotedCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  `display_name:"Alice Zhang"`,
	})
	if quotedW.Code != http.StatusOK {
		t.Fatalf("quoted list users search status = %d, want %d, body=%s", quotedW.Code, http.StatusOK, quotedW.Body.String())
	}

	mustDecodeJSON(t, quotedW.Body.Bytes(), &users)
	if len(users.Items) != 1 || users.Items[0].Username != "alice" {
		t.Fatalf("quoted search users = %+v, want only alice", users.Items)
	}
}

func TestListUsers_UsesObservedProfileFieldsForColumnsAndSearch(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-users-profile-fields").
		SetName("corp-directory").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetSortOrder(0).
		SetCreatedBy("test").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	userEnt, err := client.User.Create().
		SetID("user-profile-search").
		SetUsername("alice.directory@example.com").
		SetDisplayName("Alice Directory").
		SetEmail("alice.directory@example.com").
		SetEnabled(true).
		SetAuthProviderID(providerEnt.ID).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if _, createErr := client.UserDirectoryProfile.Create().
		SetID("profile-user-profile-search").
		SetUserID(userEnt.ID).
		SetAttributes(map[string]interface{}{
			"department": "Engineering",
			"section":    "Platform",
			"location":   "Site A",
		}).
		SetLastSyncedAt(time.Now().UTC()).
		SetUser(userEnt).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create user directory profile: %v", createErr)
	}

	roleEnt, err := client.Role.Create().
		SetID("role-user-profile-search").
		SetName("TeamLead").
		SetDisplayName("Team Lead").
		SetPermissions([]string{"user:manage"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	if _, err := client.RoleBinding.Create().
		SetID("binding-user-profile-search").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("test").
		Save(t.Context()); err != nil {
		t.Fatalf("create role binding: %v", err)
	}

	searchCtx, searchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users?page=1&per_page=20&search=department:Engineering",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(searchCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  "department:Engineering",
	})
	if searchW.Code != http.StatusOK {
		t.Fatalf("list users search status = %d, want %d, body=%s", searchW.Code, http.StatusOK, searchW.Body.String())
	}

	var users generated.UserList
	mustDecodeJSON(t, searchW.Body.Bytes(), &users)
	if len(users.Items) != 1 || users.Items[0].Username != "alice.directory@example.com" {
		t.Fatalf("search users = %+v, want only alice.directory@example.com", users.Items)
	}
	if got, ok := users.Items[0].ProfileAttributes["department"].(string); !ok || got != "Engineering" {
		t.Fatalf("profile_attributes[department] = %#v, want %q", users.Items[0].ProfileAttributes["department"], "Engineering")
	}
	if got, ok := users.Items[0].ProfileAttributes["section"].(string); !ok || got != "Platform" {
		t.Fatalf("profile_attributes[section] = %#v, want %q", users.Items[0].ProfileAttributes["section"], "Platform")
	}
	if got, ok := users.Items[0].ProfileAttributes["location"].(string); !ok || got != "Site A" {
		t.Fatalf("profile_attributes[location] = %#v, want %q", users.Items[0].ProfileAttributes["location"], "Site A")
	}
	if len(users.ProfileFields) != 3 {
		t.Fatalf("profile_fields len = %d, want 3", len(users.ProfileFields))
	}
	if got := []string{users.ProfileFields[0].Key, users.ProfileFields[1].Key, users.ProfileFields[2].Key}; !slices.Equal(got, []string{"department", "location", "section"}) {
		t.Fatalf("profile field keys = %v, want [department location section]", got)
	}

	observedSearchCtx, observedSearchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users?page=1&per_page=20&search=location:Site%20A",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(observedSearchCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  "location:\"Site A\"",
	})
	if observedSearchW.Code != http.StatusOK {
		t.Fatalf("observed field search status = %d, want %d, body=%s", observedSearchW.Code, http.StatusOK, observedSearchW.Body.String())
	}
	var observedUsers generated.UserList
	mustDecodeJSON(t, observedSearchW.Body.Bytes(), &observedUsers)
	if len(observedUsers.Items) != 1 || observedUsers.Items[0].Username != "alice.directory@example.com" {
		t.Fatalf("observed field search users = %+v, want only alice.directory@example.com", observedUsers.Items)
	}

	statusSearchCtx, statusSearchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users?page=1&per_page=20&search=status:enabled",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(statusSearchCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  "status:enabled",
	})
	if statusSearchW.Code != http.StatusOK {
		t.Fatalf("status field search status = %d, want %d, body=%s", statusSearchW.Code, http.StatusOK, statusSearchW.Body.String())
	}
	var statusUsers generated.UserList
	mustDecodeJSON(t, statusSearchW.Body.Bytes(), &statusUsers)
	if len(statusUsers.Items) != 1 || statusUsers.Items[0].Username != "alice.directory@example.com" {
		t.Fatalf("status field search users = %+v, want only alice.directory@example.com", statusUsers.Items)
	}

	roleSearchCtx, roleSearchW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/users?page=1&per_page=20&search=role:Team%20Lead",
		"",
		"admin-1",
		[]string{"rbac:read"},
	)
	srv.ListUsers(roleSearchCtx, generated.ListUsersParams{
		Page:    1,
		PerPage: 20,
		Search:  "role:\"Team Lead\"",
	})
	if roleSearchW.Code != http.StatusOK {
		t.Fatalf("role field search status = %d, want %d, body=%s", roleSearchW.Code, http.StatusOK, roleSearchW.Body.String())
	}
	var roleUsers generated.UserList
	mustDecodeJSON(t, roleSearchW.Body.Bytes(), &roleUsers)
	if len(roleUsers.Items) != 1 || roleUsers.Items[0].Username != "alice.directory@example.com" {
		t.Fatalf("role field search users = %+v, want only alice.directory@example.com", roleUsers.Items)
	}
}

func TestAuthProviderConfigSecretsStayEncryptedAndRevealOnlyForEditableResponses(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{
			"name":"Corp SSO Secrets",
			"auth_type":"oidc",
			"enabled":true,
			"config":{
				"issuer_url":"https://issuer.example.com",
				"client_id":"shepherd",
				"client_secret":"top-secret"
			}
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createCtx)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, want %d, body=%s", createW.Code, http.StatusCreated, createW.Body.String())
	}

	var created generated.AuthProvider
	mustDecodeJSON(t, createW.Body.Bytes(), &created)
	if got := created.Config["client_secret"]; got != "top-secret" {
		t.Fatalf("create response client_secret = %#v, want original secret", got)
	}

	stored, err := client.AuthProvider.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("get stored provider: %v", err)
	}
	storedSecret, _ := stored.Config["client_secret"].(string)
	if storedSecret == "" || storedSecret == "top-secret" {
		t.Fatalf("stored client_secret = %q, want encrypted value", storedSecret)
	}

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviders(listCtx)
	if listW.Code != http.StatusOK {
		t.Fatalf("list providers status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}

	var listResp generated.AuthProviderList
	mustDecodeJSON(t, listW.Body.Bytes(), &listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("listed auth providers = %d, want 1", len(listResp.Items))
	}
	if got := listResp.Items[0].Config["client_secret"]; got != "top-secret" {
		t.Fatalf("editable list response client_secret = %#v, want original secret", got)
	}

	readOnlyListCtx, readOnlyListW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers",
		"",
		"reader-1",
		[]string{"auth_provider:read"},
	)
	srv.ListAuthProviders(readOnlyListCtx)
	if readOnlyListW.Code != http.StatusOK {
		t.Fatalf("read-only list providers status = %d, want %d, body=%s", readOnlyListW.Code, http.StatusOK, readOnlyListW.Body.String())
	}
	var readOnlyListResp generated.AuthProviderList
	mustDecodeJSON(t, readOnlyListW.Body.Bytes(), &readOnlyListResp)
	if got := readOnlyListResp.Items[0].Config["client_secret"]; got != provider.AuthProviderProtectedFieldMask {
		t.Fatalf("read-only list response client_secret = %#v, want placeholder", got)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+created.Id,
		`{
			"config":{
				"issuer_url":"https://issuer.example.com",
				"client_id":"shepherd-updated",
				"client_secret":"`+provider.AuthProviderProtectedFieldMask+`"
			}
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAuthProvider(updateCtx, created.Id)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update provider status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	updatedStored, err := client.AuthProvider.Get(t.Context(), created.Id)
	if err != nil {
		t.Fatalf("get updated provider: %v", err)
	}
	if got := updatedStored.Config["client_secret"]; got != stored.Config["client_secret"] {
		t.Fatalf("stored encrypted secret changed unexpectedly: got=%#v want=%#v", got, stored.Config["client_secret"])
	}
	if got := updatedStored.Config["client_id"]; got != "shepherd-updated" {
		t.Fatalf("stored client_id = %#v, want %q", got, "shepherd-updated")
	}
}

func TestUpdateAuthProvider_DisablingRevokesLinkedUserSessions(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	pool := testutil.OpenPGXPool(t, "admin_identity_disable_provider_revoke")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	srv := NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		AuthSessions:  authSessions,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})
	ctx := t.Context()

	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-disable-revoke").
		SetName("Disable Revoke Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{
			"issuer":        "https://sso.example.com",
			"client_id":     "shepherd",
			"client_secret": "secret",
		}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	linkedUser, err := client.User.Create().
		SetID("user-disable-provider-revoke").
		SetUsername("disable.provider.revoke").
		SetEnabled(true).
		SetAuthProviderID(providerEnt.ID).
		SetExternalID("external-disable-provider-revoke").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed linked user: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, linkedUser.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerEnt.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)
	srv.UpdateAuthProvider(updateCtx, providerEnt.ID)
	if updateW.Code != http.StatusOK {
		t.Fatalf("disable provider status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}
	afterVersion, err := authSessions.CurrentSessionVersion(ctx, linkedUser.ID)
	if err != nil {
		t.Fatalf("read session version after disable: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after provider disable = %d, want %d", afterVersion, beforeVersion+1)
	}
	reloadedProvider, err := client.AuthProvider.Get(ctx, providerEnt.ID)
	if err != nil {
		t.Fatalf("reload provider: %v", err)
	}
	if reloadedProvider.Enabled {
		t.Fatal("provider should be disabled")
	}
}

func TestUpdateAuthProvider_RollsBackWhenLinkedSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"admin_identity_disable_provider_revoke_fail",
	)
	ctx := t.Context()

	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-disable-revoke-fail").
		SetName("Disable Revoke Fail Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{
			"issuer":        "https://sso.example.com",
			"client_id":     "shepherd",
			"client_secret": "secret",
		}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	linkedUser, createErr := client.User.Create().
		SetID("user-disable-provider-revoke-fail").
		SetUsername("disable.provider.revoke.fail").
		SetEnabled(true).
		SetAuthProviderID(providerEnt.ID).
		SetExternalID("external-disable-provider-revoke-fail").
		Save(ctx)
	if createErr != nil {
		t.Fatalf("seed linked user: %v", createErr)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, linkedUser.ID)

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerEnt.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)
	srv.UpdateAuthProvider(updateCtx, providerEnt.ID)
	if updateW.Code != http.StatusInternalServerError {
		t.Fatalf("disable provider status = %d, want %d, body=%s", updateW.Code, http.StatusInternalServerError, updateW.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	reloadedProvider, err := client.AuthProvider.Get(ctx, providerEnt.ID)
	if err != nil {
		t.Fatalf("reload provider: %v", err)
	}
	if !reloadedProvider.Enabled {
		t.Fatal("provider should remain enabled after session revocation failure")
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestAuthProviderStage2CFlow(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)

	discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		baseURL := "http://" + r.Host
		_, _ = w.Write([]byte(`{
			"issuer": "` + baseURL + `",
			"authorization_endpoint": "` + baseURL + `/authorize",
			"token_endpoint": "` + baseURL + `/token",
			"jwks_uri": "` + baseURL + `/jwks",
			"userinfo_endpoint": "` + baseURL + `/userinfo",
			"id_token_signing_alg_values_supported": ["RS256"]
		}`))
	}))
	defer discovery.Close()

	createProviderCtx, createProviderW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{
			"name":"Corp SSO Stage2C",
			"auth_type":"oidc",
			"enabled":true,
			"config":{
				"issuer":"`+discovery.URL+`",
				"client_id":"shepherd",
				"client_secret":"secret",
				"sample_users":[
					{"groups":["DevOps-Team","QA-Team"],"department":"Engineering"},
					{"groups":["Platform-Admin"],"department":"IT"}
				]
			}
		}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createProviderCtx)
	if createProviderW.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, want %d, body=%s", createProviderW.Code, http.StatusCreated, createProviderW.Body.String())
	}
	var authProvider generated.AuthProvider
	mustDecodeJSON(t, createProviderW.Body.Bytes(), &authProvider)

	testConnCtx, testConnW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+authProvider.Id+"/test-connection",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.TestAuthProviderConnection(testConnCtx, authProvider.Id)
	if testConnW.Code != http.StatusOK {
		t.Fatalf("test connection status = %d, want %d, body=%s", testConnW.Code, http.StatusOK, testConnW.Body.String())
	}
	var connResp generated.AuthProviderConnectionTestResult
	mustDecodeJSON(t, testConnW.Body.Bytes(), &connResp)
	if !connResp.Success {
		t.Fatalf("expected connection success, body=%s", testConnW.Body.String())
	}

	sampleCtx, sampleW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+authProvider.Id+"/sample",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderSample(sampleCtx, authProvider.Id)
	if sampleW.Code != http.StatusOK {
		t.Fatalf("sample status = %d, want %d, body=%s", sampleW.Code, http.StatusOK, sampleW.Body.String())
	}
	var sample generated.AuthProviderSampleResponse
	mustDecodeJSON(t, sampleW.Body.Bytes(), &sample)
	if len(sample.Fields) == 0 {
		t.Fatalf("expected sample fields, got empty: %s", sampleW.Body.String())
	}

	syncCtx, syncW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+authProvider.Id+"/cohorts/sync",
		`{"cohort_kind":"group","source_field":"groups","cohorts":["DevOps-Team","QA-Team","Platform-Admin"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.SyncAuthProviderCohorts(syncCtx, authProvider.Id)
	if syncW.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want %d, body=%s", syncW.Code, http.StatusOK, syncW.Body.String())
	}
	var syncResp generated.ExternalCohortSyncResponse
	mustDecodeJSON(t, syncW.Body.Bytes(), &syncResp)
	if len(syncResp.Items) != 3 {
		t.Fatalf("expected 3 synced cohorts, got %d", len(syncResp.Items))
	}

	listCohortsCtx, listCohortsW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+authProvider.Id+"/cohorts",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviderCohorts(listCohortsCtx, authProvider.Id)
	if listCohortsW.Code != http.StatusOK {
		t.Fatalf("list cohorts status = %d, want %d, body=%s", listCohortsW.Code, http.StatusOK, listCohortsW.Body.String())
	}
	var cohortsResp generated.ExternalCohortList
	mustDecodeJSON(t, listCohortsW.Body.Bytes(), &cohortsResp)
	if len(cohortsResp.Items) != 3 {
		t.Fatalf("expected 3 listed cohorts, got %d", len(cohortsResp.Items))
	}
	if cohortsResp.Items[0].CohortKind != "group" {
		t.Fatalf("expected cohort kind group, got %q", cohortsResp.Items[0].CohortKind)
	}

	createRoleCtx, createRoleW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/roles",
		`{"name":"Stage2CRole","permissions":["vm:read"],"enabled":true}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateRole(createRoleCtx)
	if createRoleW.Code != http.StatusCreated {
		t.Fatalf("create role status = %d, want %d, body=%s", createRoleW.Code, http.StatusCreated, createRoleW.Body.String())
	}
	var createdRole generated.Role
	mustDecodeJSON(t, createRoleW.Body.Bytes(), &createdRole)

	createMappingCtx, createMappingW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+authProvider.Id+"/cohort-mappings",
		`{"cohort_kind":"group","cohort_key":"DevOps-Team","role_id":"`+createdRole.Id+`","scope_type":"global","allowed_environments":["test","prod"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProviderCohortMapping(createMappingCtx, authProvider.Id)
	if createMappingW.Code != http.StatusCreated {
		t.Fatalf("create mapping status = %d, want %d, body=%s", createMappingW.Code, http.StatusCreated, createMappingW.Body.String())
	}
	var mapping generated.ExternalCohortMapping
	mustDecodeJSON(t, createMappingW.Body.Bytes(), &mapping)
	if mapping.Id == "" {
		t.Fatal("mapping id is empty")
	}

	listMappingsCtx, listMappingsW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+authProvider.Id+"/cohort-mappings",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviderCohortMappings(listMappingsCtx, authProvider.Id)
	if listMappingsW.Code != http.StatusOK {
		t.Fatalf("list mappings status = %d, want %d, body=%s", listMappingsW.Code, http.StatusOK, listMappingsW.Body.String())
	}
	var listResp generated.ExternalCohortMappingList
	mustDecodeJSON(t, listMappingsW.Body.Bytes(), &listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(listResp.Items))
	}

	updateMappingCtx, updateMappingW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+authProvider.Id+"/cohort-mappings/"+mapping.Id,
		`{"allowed_environments":["test"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAuthProviderCohortMapping(updateMappingCtx, authProvider.Id, mapping.Id)
	if updateMappingW.Code != http.StatusOK {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateMappingW.Code, http.StatusOK, updateMappingW.Body.String())
	}

	deleteMappingCtx, deleteMappingW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+authProvider.Id+"/cohort-mappings/"+mapping.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteMappingCtx, authProvider.Id, mapping.Id)
	if got := deleteMappingCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete mapping status = %d, want %d, body=%s", got, http.StatusNoContent, deleteMappingW.Body.String())
	}
}

func TestAuthProviderCohortMapping_RejectsDisabledRole(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()
	providerEnt, err := client.AuthProvider.Create().
		SetID("provider-disabled-role-mapping").
		SetName("Disabled Role Mapping Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer": "https://sso.example.com"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	enabledRole, err := client.Role.Create().
		SetID("role-disabled-mapping-enabled").
		SetName("disabled_mapping_enabled").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create enabled role: %v", err)
	}
	disabledRole, err := client.Role.Create().
		SetID("role-disabled-mapping-disabled").
		SetName("disabled_mapping_disabled").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(false).
		Save(ctx)
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}

	createMappingCtx, createMappingW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerEnt.ID+"/cohort-mappings",
		`{"cohort_kind":"group","cohort_key":"Disabled-Team","role_id":"`+disabledRole.ID+`","scope_type":"global"}`,
		"admin-1",
		[]string{"auth_provider:mapping_create"},
	)
	srv.CreateAuthProviderCohortMapping(createMappingCtx, providerEnt.ID)
	if createMappingW.Code != http.StatusConflict {
		t.Fatalf("create disabled-role mapping status = %d, want %d, body=%s", createMappingW.Code, http.StatusConflict, createMappingW.Body.String())
	}
	assertErrorCode(t, createMappingW.Body.Bytes(), "ROLE_DISABLED")
	disabledMappingCount, err := client.ExternalCohortMapping.Query().
		Where(
			externalcohortmapping.ProviderIDEQ(providerEnt.ID),
			externalcohortmapping.RoleIDEQ(disabledRole.ID),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count disabled-role mappings: %v", err)
	}
	if disabledMappingCount != 0 {
		t.Fatalf("disabled-role mapping count = %d, want 0", disabledMappingCount)
	}

	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-disabled-role-update").
		SetProviderID(providerEnt.ID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(enabledRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create existing mapping: %v", err)
	}
	updateMappingCtx, updateMappingW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerEnt.ID+"/cohort-mappings/"+mappingEnt.ID,
		`{"role_id":"`+disabledRole.ID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateMappingCtx, providerEnt.ID, mappingEnt.ID)
	if updateMappingW.Code != http.StatusConflict {
		t.Fatalf("update disabled-role mapping status = %d, want %d, body=%s", updateMappingW.Code, http.StatusConflict, updateMappingW.Body.String())
	}
	assertErrorCode(t, updateMappingW.Body.Bytes(), "ROLE_DISABLED")
	reloaded, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("reload mapping after disabled-role update: %v", err)
	}
	if reloaded.RoleID != enabledRole.ID {
		t.Fatalf("mapping role_id = %q, want unchanged %q", reloaded.RoleID, enabledRole.ID)
	}
}

func TestListAuthProviderTypesAndRejectUnknownType(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)

	listCtx, listW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-provider-types",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviderTypes(listCtx)
	if listW.Code != http.StatusOK {
		t.Fatalf("list provider types status = %d, want %d, body=%s", listW.Code, http.StatusOK, listW.Body.String())
	}
	var listResp generated.AuthProviderTypeList
	mustDecodeJSON(t, listW.Body.Bytes(), &listResp)
	if len(listResp.Items) == 0 {
		t.Fatalf("expected provider type items, got empty: %s", listW.Body.String())
	}

	typeKeys := make([]string, 0, len(listResp.Items))
	for _, item := range listResp.Items {
		typeKeys = append(typeKeys, item.Type)
	}
	for _, expected := range []string{"oidc", "ldap", "wecom"} {
		if !slices.Contains(typeKeys, expected) {
			t.Fatalf("provider type list missing %q: %#v", expected, typeKeys)
		}
	}
	for _, removed := range []string{"generic", "sso"} {
		if slices.Contains(typeKeys, removed) {
			t.Fatalf("provider type list includes removed placeholder %q: %#v", removed, typeKeys)
		}
	}

	createCtx, createW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{"name":"Unknown plugin","auth_type":"unknown-custom-plugin","enabled":true,"config":{"test_endpoint":"https://example.com/health"}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createCtx)
	if createW.Code != http.StatusBadRequest {
		t.Fatalf("create unknown provider status = %d, want %d, body=%s", createW.Code, http.StatusBadRequest, createW.Body.String())
	}
}

func TestUpdateUser_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "admin_identity_update_user_revoke_fail")

	userRow, err := client.User.Create().
		SetID("user-update-revoke-fail").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userRow.ID)

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/users/"+userRow.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"user:manage"},
	)
	srv.UpdateUser(updateCtx, userRow.ID)
	if updateW.Code != http.StatusInternalServerError {
		t.Fatalf("update status = %d, want %d, body=%s", updateW.Code, http.StatusInternalServerError, updateW.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)

	reloaded, err := client.User.Get(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if !reloaded.Enabled {
		t.Fatal("expected user to remain enabled after failed session revocation")
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestDeleteUser_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	pool := testutil.OpenPGXPool(t, "admin_identity_delete_user_revoke_fail")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	srv := NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		AuthSessions: authSessions,
	})
	ctx := t.Context()

	userRow, err := client.User.Create().
		SetID("user-delete-revoke-fail").
		SetUsername("delete.revoke.fail").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-delete-revoke-fail").
		SetName("DeleteRevokeFailRole").
		SetPermissions([]string{"vm:read"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	bindingRow, err := client.RoleBinding.Create().
		SetID("binding-delete-revoke-fail").
		SetUser(userRow).
		SetRole(roleRow).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	grantRow, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-revoke-fail").
		SetUserID(userRow.ID).
		SetProviderID("provider-delete-revoke-fail").
		SetBindingKey("group:ops").
		SetRoleBindingID(bindingRow.ID).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}
	preferenceRow, err := client.UserPreference.Create().
		SetID("preference-delete-revoke-fail").
		SetUserID(userRow.ID).
		SetKey("theme").
		SetValue(map[string]interface{}{"mode": "dark"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed preference: %v", err)
	}
	exemptionRow, err := client.RateLimitExemption.Create().
		SetID(userRow.ID).
		SetExemptedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed rate-limit exemption: %v", err)
	}
	overrideRow, err := client.RateLimitUserOverride.Create().
		SetID(userRow.ID).
		SetCooldownSeconds(15).
		SetUpdatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed rate-limit override: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userRow.ID)

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userRow.ID,
		"",
		"admin-1",
		[]string{"user:manage"},
	)
	srv.DeleteUser(deleteCtx, userRow.ID)
	if deleteW.Code != http.StatusInternalServerError {
		t.Fatalf("delete user status = %d, want %d, body=%s", deleteW.Code, http.StatusInternalServerError, deleteW.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	if _, getErr := client.User.Get(ctx, userRow.ID); getErr != nil {
		t.Fatalf("user should remain after failed session revocation: %v", getErr)
	}
	if _, getErr := client.ExternalCohortGrant.Get(ctx, grantRow.ID); getErr != nil {
		t.Fatalf("external cohort grant should remain after failed session revocation: %v", getErr)
	}
	if _, getErr := client.RoleBinding.Get(ctx, bindingRow.ID); getErr != nil {
		t.Fatalf("role binding should remain after failed session revocation: %v", getErr)
	}
	if _, getErr := client.UserPreference.Get(ctx, preferenceRow.ID); getErr != nil {
		t.Fatalf("user preference should remain after failed session revocation: %v", getErr)
	}
	if _, getErr := client.RateLimitExemption.Get(ctx, exemptionRow.ID); getErr != nil {
		t.Fatalf("rate-limit exemption should remain after failed session revocation: %v", getErr)
	}
	if _, getErr := client.RateLimitUserOverride.Get(ctx, overrideRow.ID); getErr != nil {
		t.Fatalf("rate-limit override should remain after failed session revocation: %v", getErr)
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestDeleteUser_RevokesSessionsOnSuccess(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	pool := testutil.OpenPGXPool(t, "admin_identity_delete_user_revoke_success")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	srv := NewServer(ServerDeps{
		EntClient:    client,
		Pool:         pool,
		AuthSessions: authSessions,
	})
	ctx := t.Context()

	userRow, err := client.User.Create().
		SetID("user-delete-revoke-success").
		SetUsername("delete.revoke.success").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userRow.ID)
	if err != nil {
		t.Fatalf("seed auth session subject: %v", err)
	}
	if _, createErr := client.RateLimitExemption.Create().
		SetID(userRow.ID).
		SetExemptedBy("admin-1").
		Save(ctx); createErr != nil {
		t.Fatalf("seed rate-limit exemption: %v", createErr)
	}
	if _, createErr := client.RateLimitUserOverride.Create().
		SetID(userRow.ID).
		SetMaxPendingChildren(3).
		SetUpdatedBy("admin-1").
		Save(ctx); createErr != nil {
		t.Fatalf("seed rate-limit override: %v", createErr)
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userRow.ID,
		"",
		"admin-1",
		[]string{"user:manage"},
	)
	srv.DeleteUser(deleteCtx, userRow.ID)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete user status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}
	if _, getErr := client.User.Get(ctx, userRow.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("user should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RateLimitExemption.Get(ctx, userRow.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("rate-limit exemption should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RateLimitUserOverride.Get(ctx, userRow.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("rate-limit override should be deleted, err=%v", getErr)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(ctx, userRow.ID)
	if err != nil {
		t.Fatalf("read auth session version after delete: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after delete = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func TestCreateUserRoleBinding_RevokesSessionsWithSinglePooledConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bootstrapPool := testutil.OpenPGXPool(t, "role_binding_single_connection")
	limitedConfig := bootstrapPool.Config().Copy()
	limitedConfig.MinConns = 0
	limitedConfig.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), limitedConfig)
	if err != nil {
		t.Fatalf("create single-connection pool: %v", err)
	}
	t.Cleanup(pool.Close)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	srv := NewServer(ServerDeps{EntClient: client, Pool: pool, AuthSessions: authSessions})

	userRow, err := client.User.Create().
		SetID("user-role-binding-single-connection").
		SetUsername("role.binding.single.connection").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role-binding target user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-binding-single-connection").
		SetName("role_binding_single_connection").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role for single-connection binding: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("seed auth session version: %v", err)
	}

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)
	requestCtx, cancel := context.WithTimeout(requestContext.Request.Context(), 5*time.Second)
	defer cancel()
	requestContext.Request = requestContext.Request.WithContext(requestCtx)

	srv.CreateUserRoleBinding(requestContext, userRow.ID)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("read auth session version after role binding create: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version = %d, want %d", afterVersion, beforeVersion+1)
	}
	bindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(entuser.IDEQ(userRow.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count created role bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("role binding count = %d, want 1", bindingCount)
	}
}

func TestCreateUserRoleBinding_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"admin_identity_create_role_binding_revoke_fail",
	)
	ctx := t.Context()

	userRow, err := client.User.Create().
		SetID("user-role-binding-create-revoke-fail").
		SetUsername("role.binding.create.revoke.fail").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role-binding target user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-binding-create-revoke-fail").
		SetName("role_binding_create_revoke_fail").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role for role binding: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userRow.ID)

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.CreateUserRoleBinding(requestContext, userRow.ID)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf(
			"create role binding status = %d, want %d body=%s",
			response.Code,
			http.StatusInternalServerError,
			response.Body.String(),
		)
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)

	bindingCount, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(userRow.ID)),
			rolebinding.HasRoleWith(entrole.IDEQ(roleRow.ID)),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count role bindings after rollback: %v", err)
	}
	if bindingCount != 0 {
		t.Fatalf("role binding count after rollback = %d, want 0", bindingCount)
	}
}

func TestDeleteUserRoleBinding_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"admin_identity_delete_role_binding_revoke_fail",
	)
	ctx := t.Context()

	userRow, err := client.User.Create().
		SetID("user-role-binding-delete-revoke-fail").
		SetUsername("role.binding.delete.revoke.fail").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role-binding target user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-binding-delete-revoke-fail").
		SetName("role_binding_delete_revoke_fail").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role for role binding: %v", err)
	}
	bindingRow, err := client.RoleBinding.Create().
		SetID("binding-delete-revoke-fail").
		SetUserID(userRow.ID).
		SetRoleID(roleRow.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	grantRow, err := client.ExternalCohortGrant.Create().
		SetID("grant-role-binding-delete-revoke-fail").
		SetUserID(userRow.ID).
		SetProviderID("provider-role-binding-delete-revoke-fail").
		SetBindingKey("role-binding-delete-revoke-fail").
		SetRoleBindingID(bindingRow.ID).
		SetSourceMappingIds([]string{"mapping-role-binding-delete-revoke-fail"}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create external cohort grant: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userRow.ID)

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userRow.ID+"/role-bindings/"+bindingRow.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.DeleteUserRoleBinding(requestContext, userRow.ID, bindingRow.ID)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf(
			"delete role binding status = %d, want %d body=%s",
			response.Code,
			http.StatusInternalServerError,
			response.Body.String(),
		)
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
	if _, err := client.RoleBinding.Get(ctx, bindingRow.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantRow.ID); err != nil {
		t.Fatalf("external cohort grant should remain after rollback: %v", err)
	}
}

func TestUpdateAuthProviderCohortMapping_ClearsScopeWhenRequested(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-clear-scope").
		SetName("cohort_mapping_clear_scope").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-clear-scope").
		SetProviderID("provider-clear-scope").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create external cohort mapping: %v", err)
	}

	updateEnvsCtx, updateEnvsW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-clear-scope/cohort-mappings/"+mappingEnt.ID,
		`{"allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateEnvsCtx, "provider-clear-scope", mappingEnt.ID)
	if updateEnvsW.Code != http.StatusOK {
		t.Fatalf("update envs status = %d, want %d, body=%s", updateEnvsW.Code, http.StatusOK, updateEnvsW.Body.String())
	}
	afterEnvsUpdate, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("query mapping after env update: %v", err)
	}
	if afterEnvsUpdate.ScopeType != "system" {
		t.Fatalf("scope_type after env update = %q, want system", afterEnvsUpdate.ScopeType)
	}
	if afterEnvsUpdate.ScopeID != "system-old" {
		t.Fatalf("scope_id after env update = %q, want system-old", afterEnvsUpdate.ScopeID)
	}

	clearScopeCtx, clearScopeW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-clear-scope/cohort-mappings/"+mappingEnt.ID,
		`{"scope_type":"global"}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(clearScopeCtx, "provider-clear-scope", mappingEnt.ID)
	if clearScopeW.Code != http.StatusOK {
		t.Fatalf("clear scope status = %d, want %d, body=%s", clearScopeW.Code, http.StatusOK, clearScopeW.Body.String())
	}
	var updated generated.ExternalCohortMapping
	mustDecodeJSON(t, clearScopeW.Body.Bytes(), &updated)
	if updated.ScopeType != "global" {
		t.Fatalf("response scope_type = %q, want global", updated.ScopeType)
	}
	if updated.ScopeId != "" {
		t.Fatalf("response scope_id = %q, want empty", updated.ScopeId)
	}
	reloaded, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("query mapping after clear scope: %v", err)
	}
	if reloaded.ScopeType != "global" {
		t.Fatalf("persisted scope_type = %q, want global", reloaded.ScopeType)
	}
	if reloaded.ScopeID != "" {
		t.Fatalf("persisted scope_id = %q, want empty", reloaded.ScopeID)
	}
	if got := slices.Clone(reloaded.AllowedEnvironments); !slices.Equal(got, []string{"test"}) {
		t.Fatalf("allowed_environments = %#v, want [test]", got)
	}
}

func TestUpdateAuthProviderCohortMapping_MigratesExistingGrantToUpdatedBinding(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	oldRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-migrate-old").
		SetName("cohort_mapping_update_migrate_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	newRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-migrate-new").
		SetName("cohort_mapping_update_migrate_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-update-migrate").
		SetUsername("cohort.mapping.update.migrate").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-migrate").
		SetProviderID("provider-update-migrate").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	oldBindingKey := externalCohortMappingBindingKey(mappingEnt)
	oldBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-migrate-old").
		SetUserID(userEnt.ID).
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role binding: %v", err)
	}
	oldGrantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-migrate-old").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-migrate").
		SetBindingKey(oldBindingKey).
		SetRoleBindingID(oldBindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old grant: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-update-migrate/cohort-mappings/"+mappingEnt.ID,
		`{"role_id":"`+newRoleEnt.ID+`","scope_type":"service","scope_id":"service-new","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateCtx, "provider-update-migrate", mappingEnt.ID)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	reloadedMapping, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("reload mapping: %v", err)
	}
	newBindingKey := externalCohortMappingBindingKey(reloadedMapping)
	if newBindingKey == oldBindingKey {
		t.Fatal("expected binding key to change after mapping update")
	}
	if _, getErr := client.ExternalCohortGrant.Get(ctx, oldGrantEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("old grant should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RoleBinding.Get(ctx, oldBindingEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("old managed role binding should be deleted, err=%v", getErr)
	}
	targetGrant, err := client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userEnt.ID),
			externalcohortgrant.ProviderIDEQ("provider-update-migrate"),
			externalcohortgrant.BindingKeyEQ(newBindingKey),
		).
		Only(ctx)
	if err != nil {
		t.Fatalf("query migrated grant: %v", err)
	}
	if !slices.Equal(targetGrant.SourceMappingIds, []string{mappingEnt.ID}) {
		t.Fatalf("migrated grant source_mapping_ids = %#v, want [%q]", targetGrant.SourceMappingIds, mappingEnt.ID)
	}
	if targetGrant.RoleBindingID == oldBindingEnt.ID {
		t.Fatal("migrated grant reused old role binding")
	}
	targetBinding, err := client.RoleBinding.Get(ctx, targetGrant.RoleBindingID)
	if err != nil {
		t.Fatalf("query migrated role binding: %v", err)
	}
	if targetBinding.CreatedBy != externalCohortRoleBindingActor {
		t.Fatalf("migrated role binding created_by = %q, want mapper actor", targetBinding.CreatedBy)
	}
	if targetBinding.ScopeType != "service" {
		t.Fatalf("migrated role binding scope_type = %q, want service", targetBinding.ScopeType)
	}
	if targetBinding.ScopeID != "service-new" {
		t.Fatalf("migrated role binding scope_id = %q, want service-new", targetBinding.ScopeID)
	}
	if !slices.Equal(targetBinding.AllowedEnvironments, []string{"test"}) {
		t.Fatalf("migrated role binding allowed_environments = %#v, want [test]", targetBinding.AllowedEnvironments)
	}
	targetRole, err := targetBinding.QueryRole().Only(ctx)
	if err != nil {
		t.Fatalf("query migrated role binding role: %v", err)
	}
	if targetRole.ID != newRoleEnt.ID {
		t.Fatalf("migrated role binding role = %q, want %q", targetRole.ID, newRoleEnt.ID)
	}
}

func TestUpdateAuthProviderCohortMapping_RevokesAffectedUserSessions(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "admin_identity_update_mapping_revoke")
	ctx := t.Context()

	oldRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-revoke-old").
		SetName("cohort_mapping_update_revoke_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	newRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-revoke-new").
		SetName("cohort_mapping_update_revoke_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-update-revoke").
		SetUsername("cohort.mapping.update.revoke").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-revoke").
		SetProviderID("provider-update-revoke").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-revoke-old").
		SetUserID(userEnt.ID).
		SetRoleID(oldRoleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-revoke-old").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-revoke").
		SetBindingKey(externalCohortMappingBindingKey(mappingEnt)).
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old grant: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-update-revoke/cohort-mappings/"+mappingEnt.ID,
		`{"role_id":"`+newRoleEnt.ID+`","scope_type":"service","scope_id":"service-new","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateCtx, "provider-update-revoke", mappingEnt.ID)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}
	if _, getErr := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("old grant should be deleted, err=%v", getErr)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("read session version after mapping update: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after mapping update = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func TestUpdateAuthProviderCohortMapping_MergesIntoExistingTargetGrant(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	oldRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-merge-old").
		SetName("cohort_mapping_update_merge_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	newRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-merge-new").
		SetName("cohort_mapping_update_merge_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-update-merge").
		SetUsername("cohort.mapping.update.merge").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingMove, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-merge-move").
		SetProviderID("provider-update-merge").
		SetCohortKind("group").
		SetCohortKey("ops-move").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create move mapping: %v", err)
	}
	mappingOldKeep, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-merge-old-keep").
		SetProviderID("provider-update-merge").
		SetCohortKind("group").
		SetCohortKey("ops-old-keep").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create old keep mapping: %v", err)
	}
	mappingTargetKeep, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-merge-target-keep").
		SetProviderID("provider-update-merge").
		SetCohortKind("group").
		SetCohortKey("ops-target-keep").
		SetRoleID(newRoleEnt.ID).
		SetScopeType("service").
		SetScopeID("service-new").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create target keep mapping: %v", err)
	}
	oldBindingKey := externalCohortMappingBindingKey(mappingMove)
	targetBindingKey := externalCohortMappingBindingKey(mappingTargetKeep)
	oldBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-merge-old").
		SetUserID(userEnt.ID).
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role binding: %v", err)
	}
	oldGrantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-merge-old").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-merge").
		SetBindingKey(oldBindingKey).
		SetRoleBindingID(oldBindingEnt.ID).
		SetSourceMappingIds([]string{mappingMove.ID, mappingOldKeep.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old grant: %v", err)
	}
	targetBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-merge-target").
		SetUserID(userEnt.ID).
		SetRoleID(newRoleEnt.ID).
		SetScopeType("service").
		SetScopeID("service-new").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create target role binding: %v", err)
	}
	targetGrantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-merge-target").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-merge").
		SetBindingKey(targetBindingKey).
		SetRoleBindingID(targetBindingEnt.ID).
		SetSourceMappingIds([]string{mappingTargetKeep.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create target grant: %v", err)
	}

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-update-merge/cohort-mappings/"+mappingMove.ID,
		`{"role_id":"`+newRoleEnt.ID+`","scope_type":"service","scope_id":"service-new","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateCtx, "provider-update-merge", mappingMove.ID)
	if updateW.Code != http.StatusOK {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateW.Code, http.StatusOK, updateW.Body.String())
	}

	reloadedOldGrant, err := client.ExternalCohortGrant.Get(ctx, oldGrantEnt.ID)
	if err != nil {
		t.Fatalf("old grant should remain: %v", err)
	}
	if !slices.Equal(reloadedOldGrant.SourceMappingIds, []string{mappingOldKeep.ID}) {
		t.Fatalf("old grant source_mapping_ids = %#v, want [%q]", reloadedOldGrant.SourceMappingIds, mappingOldKeep.ID)
	}
	if _, getErr := client.RoleBinding.Get(ctx, oldBindingEnt.ID); getErr != nil {
		t.Fatalf("old shared role binding should remain: %v", getErr)
	}
	reloadedTargetGrant, err := client.ExternalCohortGrant.Get(ctx, targetGrantEnt.ID)
	if err != nil {
		t.Fatalf("target grant should remain: %v", err)
	}
	wantTargetSourceIDs := []string{mappingMove.ID, mappingTargetKeep.ID}
	if !slices.Equal(reloadedTargetGrant.SourceMappingIds, wantTargetSourceIDs) {
		t.Fatalf("target grant source_mapping_ids = %#v, want %#v", reloadedTargetGrant.SourceMappingIds, wantTargetSourceIDs)
	}
	if reloadedTargetGrant.RoleBindingID != targetBindingEnt.ID {
		t.Fatalf("target grant role_binding_id = %q, want existing %q", reloadedTargetGrant.RoleBindingID, targetBindingEnt.ID)
	}
	grants, err := client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userEnt.ID),
			externalcohortgrant.ProviderIDEQ("provider-update-merge"),
		).
		All(ctx)
	if err != nil {
		t.Fatalf("query user provider grants: %v", err)
	}
	if len(grants) != 2 {
		t.Fatalf("grant count = %d, want 2", len(grants))
	}
}

func TestUpdateAuthProviderCohortMapping_RollsBackWhenGrantMigrationFails(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	oldRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-rollback-old").
		SetName("cohort_mapping_update_rollback_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	newRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-rollback-new").
		SetName("cohort_mapping_update_rollback_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-update-rollback").
		SetUsername("cohort.mapping.update.rollback").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-rollback").
		SetProviderID("provider-update-rollback").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	oldBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-rollback-old").
		SetUserID(userEnt.ID).
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role binding: %v", err)
	}
	oldGrantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-rollback-old").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-rollback").
		SetBindingKey(externalCohortMappingBindingKey(mappingEnt)).
		SetRoleBindingID(oldBindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old grant: %v", err)
	}
	client.RoleBinding.Use(enthook.On(
		enthook.FixedError(errors.New("role binding create unavailable")),
		ent.OpCreate,
	))

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-update-rollback/cohort-mappings/"+mappingEnt.ID,
		`{"role_id":"`+newRoleEnt.ID+`","scope_type":"service","scope_id":"service-new","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateCtx, "provider-update-rollback", mappingEnt.ID)
	if updateW.Code != http.StatusInternalServerError {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateW.Code, http.StatusInternalServerError, updateW.Body.String())
	}
	reloadedMapping, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("mapping should remain after rollback: %v", err)
	}
	if reloadedMapping.RoleID != oldRoleEnt.ID {
		t.Fatalf("mapping role_id = %q, want old role %q", reloadedMapping.RoleID, oldRoleEnt.ID)
	}
	if reloadedMapping.ScopeType != "system" {
		t.Fatalf("mapping scope_type = %q, want system", reloadedMapping.ScopeType)
	}
	if reloadedMapping.ScopeID != "system-old" {
		t.Fatalf("mapping scope_id = %q, want system-old", reloadedMapping.ScopeID)
	}
	if !slices.Equal(reloadedMapping.AllowedEnvironments, []string{"prod"}) {
		t.Fatalf("mapping allowed_environments = %#v, want [prod]", reloadedMapping.AllowedEnvironments)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, oldGrantEnt.ID); err != nil {
		t.Fatalf("old grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, oldBindingEnt.ID); err != nil {
		t.Fatalf("old role binding should remain after rollback: %v", err)
	}
}

func TestUpdateAuthProviderCohortMapping_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"admin_identity_update_mapping_revoke_fail",
	)
	ctx := t.Context()

	oldRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-revoke-fail-old").
		SetName("cohort_mapping_update_revoke_fail_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role: %v", err)
	}
	newRoleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-update-revoke-fail-new").
		SetName("cohort_mapping_update_revoke_fail_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-update-revoke-fail").
		SetUsername("cohort.mapping.update.revoke.fail").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-update-revoke-fail").
		SetProviderID("provider-update-revoke-fail").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	oldBindingEnt, err := client.RoleBinding.Create().
		SetID("rb-update-revoke-fail-old").
		SetUserID(userEnt.ID).
		SetRoleID(oldRoleEnt.ID).
		SetScopeType("system").
		SetScopeID("system-old").
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old role binding: %v", err)
	}
	oldGrantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-update-revoke-fail-old").
		SetUserID(userEnt.ID).
		SetProviderID("provider-update-revoke-fail").
		SetBindingKey(externalCohortMappingBindingKey(mappingEnt)).
		SetRoleBindingID(oldBindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old grant: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userEnt.ID)

	updateCtx, updateW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/provider-update-revoke-fail/cohort-mappings/"+mappingEnt.ID,
		`{"role_id":"`+newRoleEnt.ID+`","scope_type":"service","scope_id":"service-new","allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateCtx, "provider-update-revoke-fail", mappingEnt.ID)
	if updateW.Code != http.StatusInternalServerError {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateW.Code, http.StatusInternalServerError, updateW.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	reloadedMapping, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID)
	if err != nil {
		t.Fatalf("mapping should remain after rollback: %v", err)
	}
	if reloadedMapping.RoleID != oldRoleEnt.ID {
		t.Fatalf("mapping role_id = %q, want old role %q", reloadedMapping.RoleID, oldRoleEnt.ID)
	}
	if reloadedMapping.ScopeType != "system" {
		t.Fatalf("mapping scope_type = %q, want system", reloadedMapping.ScopeType)
	}
	if reloadedMapping.ScopeID != "system-old" {
		t.Fatalf("mapping scope_id = %q, want system-old", reloadedMapping.ScopeID)
	}
	if !slices.Equal(reloadedMapping.AllowedEnvironments, []string{"prod"}) {
		t.Fatalf("mapping allowed_environments = %#v, want [prod]", reloadedMapping.AllowedEnvironments)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, oldGrantEnt.ID); err != nil {
		t.Fatalf("old grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, oldBindingEnt.ID); err != nil {
		t.Fatalf("old role binding should remain after rollback: %v", err)
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestDeleteAuthProviderCohortMapping_ReconcilesExistingGrants(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-delete-grants").
		SetName("cohort_mapping_delete_grants").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-delete-grants").
		SetUsername("cohort.mapping.delete.grants").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingSingle, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-single-source").
		SetProviderID("provider-delete-grants").
		SetCohortKind("group").
		SetCohortKey("ops-single").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create single-source mapping: %v", err)
	}
	mappingSharedDelete, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-shared-source").
		SetProviderID("provider-delete-grants").
		SetCohortKind("group").
		SetCohortKey("ops-shared-a").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create shared delete mapping: %v", err)
	}
	mappingSharedKeep, err := client.ExternalCohortMapping.Create().
		SetID("mapping-keep-shared-source").
		SetProviderID("provider-delete-grants").
		SetCohortKind("group").
		SetCohortKey("ops-shared-b").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create shared keep mapping: %v", err)
	}
	bindingSingle, err := client.RoleBinding.Create().
		SetID("rb-delete-single-source").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create single-source role binding: %v", err)
	}
	grantSingle, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-single-source").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-grants").
		SetBindingKey("single-binding").
		SetRoleBindingID(bindingSingle.ID).
		SetSourceMappingIds([]string{mappingSingle.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create single-source grant: %v", err)
	}
	bindingShared, err := client.RoleBinding.Create().
		SetID("rb-delete-shared-source").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create shared role binding: %v", err)
	}
	grantShared, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-shared-source").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-grants").
		SetBindingKey("shared-binding").
		SetRoleBindingID(bindingShared.ID).
		SetSourceMappingIds([]string{mappingSharedDelete.ID, mappingSharedKeep.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create shared grant: %v", err)
	}

	deleteSingleCtx, deleteSingleW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/provider-delete-grants/cohort-mappings/"+mappingSingle.ID,
		"",
		"admin-1",
		[]string{"auth_provider:mapping_delete"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteSingleCtx, "provider-delete-grants", mappingSingle.ID)
	if got := deleteSingleCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete single-source mapping status = %d, want %d, body=%s", got, http.StatusNoContent, deleteSingleW.Body.String())
	}
	if _, getErr := client.ExternalCohortGrant.Get(ctx, grantSingle.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("single-source grant should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RoleBinding.Get(ctx, bindingSingle.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("single-source managed role binding should be deleted, err=%v", getErr)
	}

	deleteSharedCtx, deleteSharedW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/provider-delete-grants/cohort-mappings/"+mappingSharedDelete.ID,
		"",
		"admin-1",
		[]string{"auth_provider:mapping_delete"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteSharedCtx, "provider-delete-grants", mappingSharedDelete.ID)
	if got := deleteSharedCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete shared-source mapping status = %d, want %d, body=%s", got, http.StatusNoContent, deleteSharedW.Body.String())
	}
	reloadedGrantShared, err := client.ExternalCohortGrant.Get(ctx, grantShared.ID)
	if err != nil {
		t.Fatalf("shared grant should remain: %v", err)
	}
	if !slices.Equal(reloadedGrantShared.SourceMappingIds, []string{mappingSharedKeep.ID}) {
		t.Fatalf("shared grant source_mapping_ids = %#v, want [%q]", reloadedGrantShared.SourceMappingIds, mappingSharedKeep.ID)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingShared.ID); err != nil {
		t.Fatalf("shared managed role binding should remain: %v", err)
	}
}

func TestDeleteAuthProviderCohortMapping_RevokesAffectedUserSessions(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "admin_identity_delete_mapping_revoke")
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-delete-revoke").
		SetName("cohort_mapping_delete_revoke").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-delete-revoke").
		SetUsername("cohort.mapping.delete.revoke").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-revoke").
		SetProviderID("provider-delete-revoke").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-revoke").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-revoke").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-revoke").
		SetBindingKey("delete-revoke-binding").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/provider-delete-revoke/cohort-mappings/"+mappingEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:mapping_delete"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteCtx, "provider-delete-revoke", mappingEnt.ID)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete mapping status = %d, want %d, body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}
	if _, getErr := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("grant should be deleted, err=%v", getErr)
	}
	if _, getErr := client.RoleBinding.Get(ctx, bindingEnt.ID); !ent.IsNotFound(getErr) {
		t.Fatalf("managed role binding should be deleted, err=%v", getErr)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(ctx, userEnt.ID)
	if err != nil {
		t.Fatalf("read session version after mapping delete: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after mapping delete = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func TestDeleteAuthProviderCohortMapping_RollsBackWhenGrantCleanupFails(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-delete-rollback").
		SetName("cohort_mapping_delete_rollback").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-delete-rollback").
		SetUsername("cohort.mapping.delete.rollback").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-rollback").
		SetProviderID("provider-delete-rollback").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-rollback").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-rollback").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-rollback").
		SetBindingKey("rollback-binding").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	client.RoleBinding.Use(enthook.On(
		enthook.FixedError(errors.New("role binding delete unavailable")),
		ent.OpDeleteOne,
	))

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/provider-delete-rollback/cohort-mappings/"+mappingEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:mapping_delete"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteCtx, "provider-delete-rollback", mappingEnt.ID)
	if deleteW.Code != http.StatusInternalServerError {
		t.Fatalf("delete mapping status = %d, want %d, body=%s", deleteW.Code, http.StatusInternalServerError, deleteW.Body.String())
	}
	if _, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID); err != nil {
		t.Fatalf("mapping should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); err != nil {
		t.Fatalf("grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingEnt.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
}

func TestDeleteAuthProviderCohortMapping_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()

	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"admin_identity_delete_mapping_revoke_fail",
	)
	ctx := t.Context()

	roleEnt, err := client.Role.Create().
		SetID("role-cohort-mapping-delete-revoke-fail").
		SetName("cohort_mapping_delete_revoke_fail").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	userEnt, err := client.User.Create().
		SetID("user-cohort-mapping-delete-revoke-fail").
		SetUsername("cohort.mapping.delete.revoke.fail").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	mappingEnt, err := client.ExternalCohortMapping.Create().
		SetID("mapping-delete-revoke-fail").
		SetProviderID("provider-delete-revoke-fail").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	bindingEnt, err := client.RoleBinding.Create().
		SetID("rb-delete-revoke-fail").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("create role binding: %v", err)
	}
	grantEnt, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-revoke-fail").
		SetUserID(userEnt.ID).
		SetProviderID("provider-delete-revoke-fail").
		SetBindingKey("delete-revoke-fail-binding").
		SetRoleBindingID(bindingEnt.ID).
		SetSourceMappingIds([]string{mappingEnt.ID}).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create grant: %v", err)
	}
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, userEnt.ID)

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/provider-delete-revoke-fail/cohort-mappings/"+mappingEnt.ID,
		"",
		"admin-1",
		[]string{"auth_provider:mapping_delete"},
	)
	srv.DeleteAuthProviderCohortMapping(deleteCtx, "provider-delete-revoke-fail", mappingEnt.ID)
	if deleteW.Code != http.StatusInternalServerError {
		t.Fatalf("delete mapping status = %d, want %d, body=%s", deleteW.Code, http.StatusInternalServerError, deleteW.Body.String())
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)
	if _, err := client.ExternalCohortMapping.Get(ctx, mappingEnt.ID); err != nil {
		t.Fatalf("mapping should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantEnt.ID); err != nil {
		t.Fatalf("grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingEnt.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestDeleteUser_RollsBackAssociatedRowsWhenDeleteStepFails(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	userRow, err := client.User.Create().
		SetID("user-delete-rollback").
		SetUsername("delete.rollback").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-delete-rollback").
		SetName("DeleteRollbackRole").
		SetPermissions([]string{"vm:read"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	bindingRow, err := client.RoleBinding.Create().
		SetID("binding-delete-rollback").
		SetUser(userRow).
		SetRole(roleRow).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	grantRow, err := client.ExternalCohortGrant.Create().
		SetID("grant-delete-rollback").
		SetUserID(userRow.ID).
		SetProviderID("provider-delete-rollback").
		SetBindingKey("group:ops").
		SetRoleBindingID(bindingRow.ID).
		SetLastAppliedAt(time.Now()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}
	resourceBindingRow, err := client.ResourceRoleBinding.Create().
		SetID("resource-binding-delete-rollback").
		SetUserID(userRow.ID).
		SetResourceType("system").
		SetResourceID("system-delete-rollback").
		SetRole("viewer").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed resource role binding: %v", err)
	}
	notificationRow, err := client.Notification.Create().
		SetID("notification-delete-rollback").
		SetType(notification.TypeVM_STATUS_CHANGE).
		SetTitle("VM changed").
		SetMessage("The VM state changed").
		SetUser(userRow).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed notification: %v", err)
	}
	profileRow, err := client.UserDirectoryProfile.Create().
		SetID("directory-profile-delete-rollback").
		SetUser(userRow).
		SetAttributes(map[string]interface{}{"department": "platform"}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user directory profile: %v", err)
	}
	preferenceRow, err := client.UserPreference.Create().
		SetID("preference-delete-rollback").
		SetUserID(userRow.ID).
		SetKey("theme").
		SetValue(map[string]interface{}{"mode": "dark"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed preference: %v", err)
	}
	exemptionRow, err := client.RateLimitExemption.Create().
		SetID(userRow.ID).
		SetExemptedBy("admin-1").
		SetReason("temporary support").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed rate-limit exemption: %v", err)
	}
	overrideRow, err := client.RateLimitUserOverride.Create().
		SetID(userRow.ID).
		SetMaxPendingParents(2).
		SetUpdatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed rate-limit override: %v", err)
	}

	client.UserPreference.Use(enthook.On(
		enthook.FixedError(errors.New("user preference delete unavailable")),
		ent.OpDelete,
	))

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userRow.ID,
		"",
		"admin-1",
		[]string{"user:manage"},
	)
	srv.DeleteUser(deleteCtx, userRow.ID)
	if deleteW.Code != http.StatusInternalServerError {
		t.Fatalf("delete user status = %d, want %d, body=%s", deleteW.Code, http.StatusInternalServerError, deleteW.Body.String())
	}

	if _, err := client.User.Get(ctx, userRow.ID); err != nil {
		t.Fatalf("user should remain after rollback: %v", err)
	}
	if _, err := client.ExternalCohortGrant.Get(ctx, grantRow.ID); err != nil {
		t.Fatalf("external cohort grant should remain after rollback: %v", err)
	}
	if _, err := client.RoleBinding.Get(ctx, bindingRow.ID); err != nil {
		t.Fatalf("role binding should remain after rollback: %v", err)
	}
	if _, err := client.ResourceRoleBinding.Get(ctx, resourceBindingRow.ID); err != nil {
		t.Fatalf("resource role binding should remain after rollback: %v", err)
	}
	if _, err := client.Notification.Get(ctx, notificationRow.ID); err != nil {
		t.Fatalf("notification should remain after rollback: %v", err)
	}
	if _, err := client.UserDirectoryProfile.Get(ctx, profileRow.ID); err != nil {
		t.Fatalf("user directory profile should remain after rollback: %v", err)
	}
	if _, err := client.UserPreference.Get(ctx, preferenceRow.ID); err != nil {
		t.Fatalf("user preference should remain after rollback: %v", err)
	}
	if _, err := client.RateLimitExemption.Get(ctx, exemptionRow.ID); err != nil {
		t.Fatalf("rate-limit exemption should remain after rollback: %v", err)
	}
	if _, err := client.RateLimitUserOverride.Get(ctx, overrideRow.ID); err != nil {
		t.Fatalf("rate-limit override should remain after rollback: %v", err)
	}
}

func TestDeleteUser_DeletesNotificationAndDirectoryProfileForeignKeyRows(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()
	userRow, err := client.User.Create().
		SetID("user-delete-fk-rows").
		SetUsername("delete.fk.rows").
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user with associated FK rows: %v", err)
	}
	notificationRow, err := client.Notification.Create().
		SetID("notification-delete-fk-rows").
		SetType(notification.TypeVM_STATUS_CHANGE).
		SetTitle("VM changed").
		SetMessage("The VM state changed").
		SetUser(userRow).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user notification: %v", err)
	}
	profileRow, err := client.UserDirectoryProfile.Create().
		SetID("directory-profile-delete-fk-rows").
		SetUser(userRow).
		SetAttributes(map[string]interface{}{"department": "platform"}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user directory profile: %v", err)
	}

	deleteCtx, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userRow.ID,
		"",
		"admin-1",
		[]string{"user:manage"},
	)
	srv.DeleteUser(deleteCtx, userRow.ID)

	if deleteCtx.Writer.Status() != http.StatusNoContent {
		t.Fatalf(
			"delete user status = %d, want %d body=%s",
			deleteCtx.Writer.Status(),
			http.StatusNoContent,
			deleteResponse.Body.String(),
		)
	}
	if _, err := client.User.Get(ctx, userRow.ID); !ent.IsNotFound(err) {
		t.Fatalf("deleted user lookup error = %v, want not found", err)
	}
	if _, err := client.Notification.Get(ctx, notificationRow.ID); !ent.IsNotFound(err) {
		t.Fatalf("deleted notification lookup error = %v, want not found", err)
	}
	if _, err := client.UserDirectoryProfile.Get(ctx, profileRow.ID); !ent.IsNotFound(err) {
		t.Fatalf("deleted directory profile lookup error = %v, want not found", err)
	}
}

func newAdminIdentityTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client, pool := testutil.OpenEntPostgresWithPool(t, "admin_identity")
	vmInfra := newGenericStorageProfileProvider()
	return NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		VMService:     service.NewVMService(vmInfra),
		ClusterPolicy: service.NewClusterPolicyService(client),
		ApprovalReqs:  service.NewApprovalRequirementService(client),
		DirectorySync: service.NewDirectorySyncService(client),
	}), client
}

func newAdminIdentityTestServerWithAuthSessions(t *testing.T, prefix string) (*Server, *ent.Client, *service.AuthSessionManager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pool := testutil.OpenPGXPool(t, prefix)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	vmInfra := newGenericStorageProfileProvider()
	return NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		AuthSessions:  authSessions,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		VMService:     service.NewVMService(vmInfra),
		ClusterPolicy: service.NewClusterPolicyService(client),
		ApprovalReqs:  service.NewApprovalRequirementService(client),
		DirectorySync: service.NewDirectorySyncService(client),
	}), client, authSessions
}
