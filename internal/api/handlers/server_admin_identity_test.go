package handlers

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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

func TestAuthProviderStage2CFlow(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)

	discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"issuer":"https://` + r.Host + `"}`))
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
	var provider generated.AuthProvider
	mustDecodeJSON(t, createProviderW.Body.Bytes(), &provider)

	testConnCtx, testConnW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+provider.Id+"/test-connection",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.TestAuthProviderConnection(testConnCtx, provider.Id)
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
		"/admin/auth-providers/"+provider.Id+"/sample",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderSample(sampleCtx, provider.Id)
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
		"/admin/auth-providers/"+provider.Id+"/sync",
		`{"source_field":"groups","groups":["DevOps-Team","QA-Team","Platform-Admin"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.SyncAuthProviderGroups(syncCtx, provider.Id)
	if syncW.Code != http.StatusOK {
		t.Fatalf("sync status = %d, want %d, body=%s", syncW.Code, http.StatusOK, syncW.Body.String())
	}
	var syncResp generated.AuthProviderGroupSyncResponse
	mustDecodeJSON(t, syncW.Body.Bytes(), &syncResp)
	if len(syncResp.Items) != 3 {
		t.Fatalf("expected 3 synced groups, got %d", len(syncResp.Items))
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
		"/admin/auth-providers/"+provider.Id+"/group-mappings",
		`{"external_group_id":"DevOps-Team","role_id":"`+createdRole.Id+`","scope_type":"global","allowed_environments":["test","prod"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.CreateAuthProviderGroupMapping(createMappingCtx, provider.Id)
	if createMappingW.Code != http.StatusCreated {
		t.Fatalf("create mapping status = %d, want %d, body=%s", createMappingW.Code, http.StatusCreated, createMappingW.Body.String())
	}
	var mapping generated.IdPGroupMapping
	mustDecodeJSON(t, createMappingW.Body.Bytes(), &mapping)
	if mapping.Id == "" {
		t.Fatal("mapping id is empty")
	}

	listMappingsCtx, listMappingsW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+provider.Id+"/group-mappings",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviderGroupMappings(listMappingsCtx, provider.Id)
	if listMappingsW.Code != http.StatusOK {
		t.Fatalf("list mappings status = %d, want %d, body=%s", listMappingsW.Code, http.StatusOK, listMappingsW.Body.String())
	}
	var listResp generated.IdPGroupMappingList
	mustDecodeJSON(t, listMappingsW.Body.Bytes(), &listResp)
	if len(listResp.Items) != 1 {
		t.Fatalf("expected 1 mapping, got %d", len(listResp.Items))
	}

	updateMappingCtx, updateMappingW := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+provider.Id+"/group-mappings/"+mapping.Id,
		`{"allowed_environments":["test"]}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.UpdateAuthProviderGroupMapping(updateMappingCtx, provider.Id, mapping.Id)
	if updateMappingW.Code != http.StatusOK {
		t.Fatalf("update mapping status = %d, want %d, body=%s", updateMappingW.Code, http.StatusOK, updateMappingW.Body.String())
	}

	deleteMappingCtx, deleteMappingW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+provider.Id+"/group-mappings/"+mapping.Id,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.DeleteAuthProviderGroupMapping(deleteMappingCtx, provider.Id, mapping.Id)
	if got := deleteMappingCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete mapping status = %d, want %d, body=%s", got, http.StatusNoContent, deleteMappingW.Body.String())
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
	for _, expected := range []string{"generic", "oidc", "ldap"} {
		if !slices.Contains(typeKeys, expected) {
			t.Fatalf("provider type list missing %q: %#v", expected, typeKeys)
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

func newAdminIdentityTestServer(t *testing.T) (*Server, *ent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	client := testutil.OpenEntPostgres(t, "admin_identity")
	return NewServer(ServerDeps{EntClient: client}), client
}
