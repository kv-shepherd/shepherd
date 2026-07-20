package handlers

import (
	"net/http"
	"reflect"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestAdminAuthzMutationFailuresLeaveDatabaseUnchanged(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	builtInRole, seedRoleErr := client.Role.Create().
		SetID("role-builtin-delete-rejected").
		SetName("BuiltinDeleteRejected").
		SetPermissions([]string{"system:read"}).
		SetBuiltIn(true).
		SetEnabled(true).
		Save(ctx)
	if seedRoleErr != nil {
		t.Fatalf("seed built-in role: %v", seedRoleErr)
	}
	deleteBuiltInCtx, deleteBuiltInW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+builtInRole.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.DeleteRole(deleteBuiltInCtx, builtInRole.ID)
	if deleteBuiltInW.Code != http.StatusForbidden {
		t.Fatalf("delete built-in role status = %d, want %d body=%s", deleteBuiltInW.Code, http.StatusForbidden, deleteBuiltInW.Body.String())
	}
	assertErrorCode(t, deleteBuiltInW.Body.Bytes(), "BUILTIN_ROLE_IMMUTABLE")
	if _, err := client.Role.Get(ctx, builtInRole.ID); err != nil {
		t.Fatalf("built-in role changed after rejected delete: %v", err)
	}

	deleteMissingRoleCtx, deleteMissingRoleW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/role-missing",
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)
	srv.DeleteRole(deleteMissingRoleCtx, "role-missing")
	if deleteMissingRoleW.Code != http.StatusNotFound {
		t.Fatalf("delete missing role status = %d, want %d body=%s", deleteMissingRoleW.Code, http.StatusNotFound, deleteMissingRoleW.Body.String())
	}
	assertErrorCode(t, deleteMissingRoleW.Body.Bytes(), "ROLE_NOT_FOUND")

	createProviderCtx, createProviderW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{"name":"Failure Rollback OIDC","auth_type":"oidc","enabled":true,"config":{"issuer":"https://sso.example.com","client_id":"shepherd","client_secret":"secret"}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProvider(createProviderCtx)
	if createProviderW.Code != http.StatusCreated {
		t.Fatalf("create provider status = %d, want %d body=%s", createProviderW.Code, http.StatusCreated, createProviderW.Body.String())
	}
	var createdProvider generated.AuthProvider
	mustDecodeJSON(t, createProviderW.Body.Bytes(), &createdProvider)
	providerBefore, err := client.AuthProvider.Get(ctx, createdProvider.Id)
	if err != nil {
		t.Fatalf("load provider before invalid update: %v", err)
	}
	configBefore := make(map[string]interface{}, len(providerBefore.Config))
	for key, value := range providerBefore.Config {
		configBefore[key] = value
	}

	updateProviderCtx, updateProviderW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+createdProvider.Id,
		`{"config":{"issuer_url":"not-an-absolute-url"}}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)
	srv.UpdateAuthProvider(updateProviderCtx, createdProvider.Id)
	if updateProviderW.Code != http.StatusBadRequest {
		t.Fatalf("invalid provider update status = %d, want %d body=%s", updateProviderW.Code, http.StatusBadRequest, updateProviderW.Body.String())
	}
	assertErrorCode(t, updateProviderW.Body.Bytes(), "INVALID_REQUEST")
	providerAfter, err := client.AuthProvider.Get(ctx, createdProvider.Id)
	if err != nil {
		t.Fatalf("load provider after invalid update: %v", err)
	}
	if !reflect.DeepEqual(providerAfter.Config, configBefore) {
		t.Fatalf("provider config changed after invalid update: before=%#v after=%#v", configBefore, providerAfter.Config)
	}

	corruptProvider, err := client.AuthProvider.Create().
		SetID("provider-corrupt-config-update").
		SetName("Corrupt Config Update").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{
			"issuer":        "https://sso.example.com",
			"client_id":     "shepherd",
			"client_secret": "enc:v1:not-valid-base64",
		}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed provider with corrupt stored config: %v", err)
	}
	corruptConfigBefore := make(map[string]interface{}, len(corruptProvider.Config))
	for key, value := range corruptProvider.Config {
		corruptConfigBefore[key] = value
	}
	updateCorruptProviderCtx, updateCorruptProviderW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+corruptProvider.ID,
		`{"config":{"issuer_url":"https://replacement.example.com"}}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)
	srv.UpdateAuthProvider(updateCorruptProviderCtx, corruptProvider.ID)
	if updateCorruptProviderW.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt provider update status = %d, want %d body=%s", updateCorruptProviderW.Code, http.StatusInternalServerError, updateCorruptProviderW.Body.String())
	}
	assertErrorCode(t, updateCorruptProviderW.Body.Bytes(), "INTERNAL_ERROR")
	corruptProviderAfter, err := client.AuthProvider.Get(ctx, corruptProvider.ID)
	if err != nil {
		t.Fatalf("load corrupt provider after rejected update: %v", err)
	}
	if !reflect.DeepEqual(corruptProviderAfter.Config, corruptConfigBefore) {
		t.Fatalf("corrupt provider config changed after rejected update: before=%#v after=%#v", corruptConfigBefore, corruptProviderAfter.Config)
	}

	mappingRole, err := client.Role.Create().
		SetID("role-missing-provider-mapping").
		SetName("MissingProviderMappingRole").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed mapping role: %v", err)
	}
	createMappingCtx, createMappingW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/provider-missing/cohort-mappings",
		`{"cohort_kind":"group","cohort_key":"operations","role_id":"`+mappingRole.ID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_create"},
	)
	srv.CreateAuthProviderCohortMapping(createMappingCtx, "provider-missing")
	if createMappingW.Code != http.StatusNotFound {
		t.Fatalf("create mapping for missing provider status = %d, want %d body=%s", createMappingW.Code, http.StatusNotFound, createMappingW.Body.String())
	}
	assertErrorCode(t, createMappingW.Body.Bytes(), "AUTH_PROVIDER_NOT_FOUND")
	if count, err := client.ExternalCohortMapping.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("mapping count after rejected create = %d/%v, want 0", count, err)
	}
	if count, err := client.ExternalCohort.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("cohort count after rejected create = %d/%v, want 0", count, err)
	}

	updateMappingCtx, updateMappingW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+createdProvider.Id+"/cohort-mappings/mapping-missing",
		`{}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	srv.UpdateAuthProviderCohortMapping(updateMappingCtx, createdProvider.Id, "mapping-missing")
	if updateMappingW.Code != http.StatusNotFound {
		t.Fatalf("update missing mapping status = %d, want %d body=%s", updateMappingW.Code, http.StatusNotFound, updateMappingW.Body.String())
	}
	assertErrorCode(t, updateMappingW.Body.Bytes(), "EXTERNAL_COHORT_MAPPING_NOT_FOUND")
	if count, err := client.ExternalCohortMapping.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("mapping count after rejected update = %d/%v, want 0", count, err)
	}

	deleteUserCtx, deleteUserW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/user-missing",
		"",
		"admin-1",
		[]string{"user:manage"},
	)
	srv.DeleteUser(deleteUserCtx, "user-missing")
	if deleteUserW.Code != http.StatusNotFound {
		t.Fatalf("delete missing user status = %d, want %d body=%s", deleteUserW.Code, http.StatusNotFound, deleteUserW.Body.String())
	}
	assertErrorCode(t, deleteUserW.Body.Bytes(), "USER_NOT_FOUND")

	createSystemCtx, createSystemW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems",
		`{"name":"missing-actor","description":"must roll back"}`,
		"user-missing",
		[]string{"system:write"},
	)
	srv.CreateSystem(createSystemCtx)
	if createSystemW.Code != http.StatusUnauthorized {
		t.Fatalf("create system for missing actor status = %d, want %d body=%s", createSystemW.Code, http.StatusUnauthorized, createSystemW.Body.String())
	}
	assertErrorCode(t, createSystemW.Body.Bytes(), "UNAUTHORIZED")
	if count, err := client.System.Query().Count(ctx); err != nil || count != 0 {
		t.Fatalf("system count after rejected create = %d/%v, want 0", count, err)
	}
}
