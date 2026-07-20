package handlers

import (
	"net/http"
	"slices"
	"testing"

	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestIdentityMutationsFailClosedWhenAuthSessionSchemaIsUnavailable(t *testing.T) {
	t.Parallel()

	client, pool := testutil.OpenEntPostgresWithPool(t, "identity_schema_failure")
	closedPool := testutil.OpenPGXPool(t, "closed_auth_session_schema")
	authSessions := service.NewAuthSessionManager(closedPool, client, 0)
	closedPool.Close()
	srv := NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		AuthSessions:  authSessions,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})
	ctx := t.Context()
	const oldPassword = "OldPassw0rd!2026"
	originalPasswordHash, err := HashPassword(oldPassword)
	if err != nil {
		t.Fatalf("hash seeded password: %v", err)
	}
	userRow, err := client.User.Create().
		SetID("user-session-schema-failure").
		SetUsername("session.schema.failure").
		SetPasswordHash(originalPasswordHash).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	roleRow, err := client.Role.Create().
		SetID("role-session-schema-failure").
		SetName("SessionSchemaFailureRole").
		SetPermissions([]string{"system:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	createBindingRole, err := client.Role.Create().
		SetID("role-create-session-schema-failure").
		SetName("CreateSessionSchemaFailureRole").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed create-binding role: %v", err)
	}
	bindingRow, err := client.RoleBinding.Create().
		SetID("binding-session-schema-failure").
		SetUserID(userRow.ID).
		SetRoleID(roleRow.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed role binding: %v", err)
	}
	providerRow, err := client.AuthProvider.Create().
		SetID("provider-session-schema-failure").
		SetName("Session Schema Failure Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	externalAdapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID: "schema-failure-external-user",
			Username:   "schema.failure.external",
			Enabled:    true,
		},
	})
	const externalProviderID = "provider-external-schema-failure"
	if _, seedProviderErr := client.AuthProvider.Create().
		SetID(externalProviderID).
		SetName("External Schema Failure Provider").
		SetAuthType(externalAdapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx); seedProviderErr != nil {
		t.Fatalf("seed external auth provider: %v", seedProviderErr)
	}

	changePasswordCtx, changePasswordW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/change-password",
		`{"old_password":"`+oldPassword+`","new_password":"T7!vQ9#zR2@kL8$m"}`,
		userRow.ID,
		nil,
	)
	srv.ChangePassword(changePasswordCtx)
	assertInternalErrorResponse(t, "change password", changePasswordW.Code, changePasswordW.Body.String())

	externalLoginCtx, externalLoginW := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+externalProviderID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"schema.failure","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(externalLoginCtx, externalProviderID)
	assertInternalErrorResponse(t, "external credential login", externalLoginW.Code, externalLoginW.Body.String())
	if externalLoginW.Header().Get("Set-Cookie") != "" {
		t.Fatalf("external credential login unexpectedly set a session cookie: %s", externalLoginW.Header().Get("Set-Cookie"))
	}

	updateUserCtx, updateUserW := newAuthedGinContext(t, http.MethodPatch, "/admin/users/"+userRow.ID, `{"enabled":false}`, "admin-1", []string{"user:manage"})
	srv.UpdateUser(updateUserCtx, userRow.ID)
	assertInternalErrorResponse(t, "update user", updateUserW.Code, updateUserW.Body.String())

	deleteUserCtx, deleteUserW := newAuthedGinContext(t, http.MethodDelete, "/admin/users/"+userRow.ID, "", "admin-1", []string{"user:manage"})
	srv.DeleteUser(deleteUserCtx, userRow.ID)
	assertInternalErrorResponse(t, "delete user", deleteUserW.Code, deleteUserW.Body.String())

	createBindingCtx, createBindingW := newAuthedGinContext(t, http.MethodPost, "/admin/users/"+userRow.ID+"/role-bindings", `{"role_id":"`+createBindingRole.ID+`"}`, "admin-1", []string{"rbac:manage"})
	srv.CreateUserRoleBinding(createBindingCtx, userRow.ID)
	assertInternalErrorResponse(t, "create role binding", createBindingW.Code, createBindingW.Body.String())

	deleteBindingCtx, deleteBindingW := newAuthedGinContext(t, http.MethodDelete, "/admin/users/"+userRow.ID+"/role-bindings/"+bindingRow.ID, "", "admin-1", []string{"rbac:manage"})
	srv.DeleteUserRoleBinding(deleteBindingCtx, userRow.ID, bindingRow.ID)
	assertInternalErrorResponse(t, "delete role binding", deleteBindingW.Code, deleteBindingW.Body.String())

	updateRoleCtx, updateRoleW := newAuthedGinContext(t, http.MethodPatch, "/admin/roles/"+roleRow.ID, `{"permissions":["system:read","vm:read"]}`, "admin-1", []string{"rbac:manage"})
	srv.UpdateRole(updateRoleCtx, roleRow.ID)
	assertInternalErrorResponse(t, "update role", updateRoleW.Code, updateRoleW.Body.String())

	updateProviderCtx, updateProviderW := newAuthedGinContext(t, http.MethodPatch, "/admin/auth-providers/"+providerRow.ID, `{"enabled":false}`, "admin-1", []string{"auth_provider:update"})
	srv.UpdateAuthProvider(updateProviderCtx, providerRow.ID)
	assertInternalErrorResponse(t, "update auth provider", updateProviderW.Code, updateProviderW.Body.String())

	deleteProviderCtx, deleteProviderW := newAuthedGinContext(t, http.MethodDelete, "/admin/auth-providers/"+providerRow.ID, "", "admin-1", []string{"auth_provider:delete"})
	srv.DeleteAuthProvider(deleteProviderCtx, providerRow.ID)
	assertInternalErrorResponse(t, "delete auth provider", deleteProviderW.Code, deleteProviderW.Body.String())

	updateMappingCtx, updateMappingW := newAuthedGinContext(t, http.MethodPatch, "/admin/auth-providers/"+providerRow.ID+"/cohort-mappings/mapping-missing", `{}`, "admin-1", []string{"auth_provider:mapping_update"})
	srv.UpdateAuthProviderCohortMapping(updateMappingCtx, providerRow.ID, "mapping-missing")
	assertInternalErrorResponse(t, "update cohort mapping", updateMappingW.Code, updateMappingW.Body.String())

	deleteMappingCtx, deleteMappingW := newAuthedGinContext(t, http.MethodDelete, "/admin/auth-providers/"+providerRow.ID+"/cohort-mappings/mapping-missing", "", "admin-1", []string{"auth_provider:mapping_delete"})
	srv.DeleteAuthProviderCohortMapping(deleteMappingCtx, providerRow.ID, "mapping-missing")
	assertInternalErrorResponse(t, "delete cohort mapping", deleteMappingW.Code, deleteMappingW.Body.String())

	refreshedUser, err := client.User.Get(ctx, userRow.ID)
	if err != nil || !refreshedUser.Enabled {
		t.Fatalf("user changed after fail-closed mutations: user=%+v err=%v", refreshedUser, err)
	}
	if refreshedUser.PasswordHash != originalPasswordHash {
		t.Fatal("password hash changed after auth-session schema failure")
	}
	if _, loadBindingErr := client.RoleBinding.Get(ctx, bindingRow.ID); loadBindingErr != nil {
		t.Fatalf("existing role binding was removed after schema failure: %v", loadBindingErr)
	}
	if count, countBindingsErr := client.RoleBinding.Query().Count(ctx); countBindingsErr != nil || count != 1 {
		t.Fatalf("role binding count after schema failure = %d/%v, want 1", count, countBindingsErr)
	}
	refreshedRole, err := client.Role.Get(ctx, roleRow.ID)
	if err != nil || !slices.Equal(refreshedRole.Permissions, []string{"system:read"}) {
		t.Fatalf("role changed after schema failure: role=%+v err=%v", refreshedRole, err)
	}
	refreshedProvider, err := client.AuthProvider.Get(ctx, providerRow.ID)
	if err != nil || !refreshedProvider.Enabled {
		t.Fatalf("auth provider changed after schema failure: provider=%+v err=%v", refreshedProvider, err)
	}
	if count, err := client.User.Query().Where(user.AuthProviderIDEQ(externalProviderID)).Count(ctx); err != nil || count != 0 {
		t.Fatalf("external provider user count after schema failure = %d/%v, want 0", count, err)
	}
	if count, err := client.ExternalCohortMapping.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("external cohort mapping count after schema failure = %d/%v, want 0", count, err)
	}
	if count, err := client.ExternalCohort.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("external cohort count after schema failure = %d/%v, want 0", count, err)
	}
}

func assertInternalErrorResponse(t *testing.T, operation string, status int, body string) {
	t.Helper()
	if status != http.StatusInternalServerError {
		t.Fatalf("%s status = %d, want %d body=%s", operation, status, http.StatusInternalServerError, body)
	}
	assertErrorCode(t, []byte(body), "INTERNAL_ERROR")
}
