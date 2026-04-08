package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
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
	}{
		{id: "user-alice", username: "alice", displayName: "Alice Zhang", email: "alice@example.com"},
		{id: "user-bob", username: "bob", displayName: "Bob Platform", email: "bob@example.com"},
		{id: "user-carol", username: "carol", displayName: "Carol Ops", email: "ops@example.com"},
		{id: "user-existing", username: "existing", displayName: "Existing Member", email: "existing@example.com"},
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
		SetAuthType("generic").
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
	for _, expected := range []string{"generic", "oidc", "ldap", "sso", "wecom"} {
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
	vmInfra := newGenericStorageProfileProvider()
	return NewServer(ServerDeps{
		EntClient:     client,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		VMService:     service.NewVMService(vmInfra),
		ClusterPolicy: service.NewClusterPolicyService(client),
		ApprovalReqs:  service.NewApprovalRequirementService(client),
		DirectorySync: service.NewDirectorySyncService(client),
	}), client
}
