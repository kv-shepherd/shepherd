package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestHasPlatformAdmin(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	t.Run("returns true when permission exists", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("permissions", []string{"system:read", "platform:admin"})
		if !hasPlatformAdmin(c) {
			t.Fatal("hasPlatformAdmin() = false, want true")
		}
	})

	t.Run("returns false when context missing permissions", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if hasPlatformAdmin(c) {
			t.Fatal("hasPlatformAdmin() = true, want false")
		}
	})
}

func TestIsValidMemberRole(t *testing.T) {
	t.Parallel()

	valid := []string{
		resourcerolebinding.RoleOwner.String(),
		resourcerolebinding.RoleAdmin.String(),
		resourcerolebinding.RoleMember.String(),
		resourcerolebinding.RoleViewer.String(),
	}
	for _, role := range valid {
		if !isValidMemberRole(role) {
			t.Fatalf("isValidMemberRole(%q) = false, want true", role)
		}
	}

	invalid := []string{"", "platform-admin", "operator", "read-only"}
	for _, role := range invalid {
		if isValidMemberRole(role) {
			t.Fatalf("isValidMemberRole(%q) = true, want false", role)
		}
	}
}

func TestToSystemMember(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC().Round(time.Second)
	binding := &ent.ResourceRoleBinding{
		UserID:    "user-1",
		Role:      resourcerolebinding.RoleMember,
		CreatedAt: createdAt,
	}

	t.Run("falls back to user id when user entity absent", func(t *testing.T) {
		t.Parallel()
		member := toSystemMember(binding, nil, nil)
		if member.UserId != "user-1" {
			t.Fatalf("UserId = %q, want %q", member.UserId, "user-1")
		}
		if member.Username != "user-1" {
			t.Fatalf("Username = %q, want fallback user id", member.Username)
		}
		if member.Role != "member" {
			t.Fatalf("Role = %q, want %q", member.Role, "member")
		}
	})

	t.Run("fills user profile fields when user present", func(t *testing.T) {
		t.Parallel()
		user := &ent.User{
			ID:          "user-1",
			Username:    "alice",
			Email:       "alice@example.com",
			DisplayName: "Alice",
		}
		member := toSystemMember(binding, user, nil)
		if member.Username != "alice" {
			t.Fatalf("Username = %q, want %q", member.Username, "alice")
		}
		if member.Email != "alice@example.com" {
			t.Fatalf("Email = %q, want %q", member.Email, "alice@example.com")
		}
		if member.DisplayName != "Alice" {
			t.Fatalf("DisplayName = %q, want %q", member.DisplayName, "Alice")
		}
	})

	t.Run("projects requested profile attributes when available", func(t *testing.T) {
		t.Parallel()
		user := &ent.User{
			ID:          "user-1",
			Username:    "alice",
			Email:       "alice@example.com",
			DisplayName: "Alice",
			Edges: ent.UserEdges{
				DirectoryProfile: &ent.UserDirectoryProfile{
					Attributes: map[string]interface{}{
						"department": "Engineering",
						"section":    "Platform",
					},
				},
			},
		}
		member := toSystemMember(binding, user, []string{"department", "section"})
		if member.ProfileAttributes["department"] != "Engineering" {
			t.Fatalf("ProfileAttributes[department] = %#v, want %q", member.ProfileAttributes["department"], "Engineering")
		}
		if member.ProfileAttributes["section"] != "Platform" {
			t.Fatalf("ProfileAttributes[section] = %#v, want %q", member.ProfileAttributes["section"], "Platform")
		}
	})
}

func TestToGeneratedUser_IgnoresDisabledRoles(t *testing.T) {
	t.Parallel()

	user := &ent.User{
		ID:       "user-role-projection",
		Username: "role.projection",
		Edges: ent.UserEdges{
			RoleBindings: []*ent.RoleBinding{
				{
					Edges: ent.RoleBindingEdges{
						Role: &ent.Role{Name: "disabled-admin", Enabled: false},
					},
				},
				{
					Edges: ent.RoleBindingEdges{
						Role: &ent.Role{Name: " operator ", Enabled: true},
					},
				},
				{
					Edges: ent.RoleBindingEdges{
						Role: &ent.Role{Name: "viewer", Enabled: true},
					},
				},
			},
		},
	}

	got := toGeneratedUser(user, nil)
	wantRoles := []string{"operator", "viewer"}
	if !slices.Equal(got.Roles, wantRoles) {
		t.Fatalf("Roles = %#v, want %#v", got.Roles, wantRoles)
	}
}

func TestListUsers_SearchFieldClassesReturnObservableRows(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	const userID = "user-search-field-classes"
	userEnt, err := client.User.Create().
		SetID(userID).
		SetUsername("search.field.user").
		SetDisplayName("Search Field User").
		SetEmail("search.fields@example.com").
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create searchable user: %v", err)
	}
	if _, createProfileErr := client.UserDirectoryProfile.Create().
		SetID("profile-search-field-classes").
		SetUserID(userID).
		SetAttributes(map[string]interface{}{"department": "Reliability"}).
		SetLastSyncedAt(time.Now().UTC()).
		SetUser(userEnt).
		Save(t.Context()); createProfileErr != nil {
		t.Fatalf("create searchable directory profile: %v", createProfileErr)
	}
	roleEnt, err := client.Role.Create().
		SetID("role-search-field-classes").
		SetName("ReliabilityOperator").
		SetDisplayName("Reliability Operator").
		SetPermissions([]string{"system:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create searchable role: %v", err)
	}
	if _, err := client.RoleBinding.Create().
		SetID("binding-search-field-classes").
		SetUserID(userID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("test").
		Save(t.Context()); err != nil {
		t.Fatalf("create searchable role binding: %v", err)
	}

	tests := []struct {
		name       string
		search     string
		wantResult bool
	}{
		{name: "username", search: "username:search.field.user", wantResult: true},
		{name: "display name alias", search: `display-name:"Search Field User"`, wantResult: true},
		{name: "mail alias", search: "mail:search.fields@example.com", wantResult: true},
		{name: "disabled status", search: "status:off", wantResult: true},
		{name: "role", search: "roles:ReliabilityOperator", wantResult: true},
		{name: "observed profile", search: "department:Reliability", wantResult: true},
		{name: "free text", search: "search.fields", wantResult: true},
		{name: "unknown field", search: "missing:value", wantResult: false},
		{name: "invalid status", search: "status:sometimes", wantResult: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, w := newAuthedGinContext(
				t,
				http.MethodGet,
				"/admin/users",
				"",
				"admin-1",
				[]string{"rbac:read"},
			)
			srv.ListUsers(c, generated.ListUsersParams{Page: 1, PerPage: 20, Search: tt.search})
			if w.Code != http.StatusOK {
				t.Fatalf("ListUsers(%q) status = %d body=%s", tt.search, w.Code, w.Body.String())
			}
			var response generated.UserList
			mustDecodeJSON(t, w.Body.Bytes(), &response)
			found := false
			for _, item := range response.Items {
				if item.Id == userID {
					found = true
					break
				}
			}
			if found != tt.wantResult {
				t.Fatalf("ListUsers(%q) found target = %v, want %v; items=%+v", tt.search, found, tt.wantResult, response.Items)
			}
		})
	}
}

func TestMemberHandler_ListSystemMembers_RequestContextCanceled(t *testing.T) {
	t.Parallel()

	srv, _ := newSystemBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-cancelled/members", "", "user-a", []string{"system:read"})
	reqCtx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(reqCtx)

	srv.ListSystemMembers(c, "sys-cancelled")

	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for canceled request, got %q", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for canceled request", w.Code, http.StatusOK)
	}
}

func TestMemberHandler_ListSystemMembers_IncludesDirectoryProfileFields(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
	systemID := "sys-members"
	mustCreateSystem(t, client, systemID, "shop-members", "seed")
	if _, err := client.User.Create().
		SetID("user-member-1").
		SetUsername("member.one").
		SetDisplayName("Member One").
		SetEmail("member.one@example.com").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := client.UserDirectoryProfile.Create().
		SetID("profile-member-1").
		SetUserID("user-member-1").
		SetAttributes(map[string]interface{}{
			"department": "Engineering",
			"section":    "Platform",
		}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(t.Context()); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := client.ResourceRoleBinding.Create().
		SetID("binding-member-1").
		SetUserID("user-member-1").
		SetResourceType("system").
		SetResourceID(systemID).
		SetRole(resourcerolebinding.RoleMember).
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create member binding: %v", err)
	}
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/"+systemID+"/members", "", "user-member-1", []string{"system:read"})
	srv.ListSystemMembers(c, systemID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemMemberList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.ProfileFields) == 0 {
		t.Fatal("expected profile_fields in response")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].ProfileAttributes["department"] != "Engineering" {
		t.Fatalf("department = %#v, want Engineering", resp.Items[0].ProfileAttributes["department"])
	}
}

func TestMemberHandler_UpdateSystemMemberRole_ProtectsLastOwner(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
	systemID, owners := seedSystemOwners(t, client, 1)

	c, w := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+systemID+"/members/"+owners[0],
		`{"role":"admin"}`,
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	srv.UpdateSystemMemberRole(c, systemID, owners[0])

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "LAST_OWNER_CANNOT_BE_REMOVED")
	assertSystemMemberRole(t, client, systemID, owners[0], resourcerolebinding.RoleOwner)
}

func TestMemberHandler_DeleteSystemMember_ProtectsLastOwner(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
	systemID, owners := seedSystemOwners(t, client, 1)

	c, w := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemID+"/members/"+owners[0],
		"",
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	srv.DeleteSystemMember(c, systemID, owners[0])

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "LAST_OWNER_CANNOT_BE_REMOVED")
	assertSystemMemberRole(t, client, systemID, owners[0], resourcerolebinding.RoleOwner)
}

func TestMemberHandler_OwnerMutations_SucceedWhenAnotherOwnerRemains(t *testing.T) {
	t.Parallel()

	t.Run("demote", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		systemID, owners := seedSystemOwners(t, client, 2)

		c, w := newAuthedGinContext(
			t,
			http.MethodPatch,
			"/systems/"+systemID+"/members/"+owners[0],
			`{"role":"admin"}`,
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.UpdateSystemMemberRole(c, systemID, owners[0])

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		assertSystemMemberRole(t, client, systemID, owners[0], resourcerolebinding.RoleAdmin)
		assertSingleSystemOwner(t, client, systemID)
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()
		srv, client := newSystemBehaviorTestServer(t)
		systemID, owners := seedSystemOwners(t, client, 2)

		c, w := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/systems/"+systemID+"/members/"+owners[0],
			"",
			"platform-admin",
			[]string{"rbac:manage", "platform:admin"},
		)
		srv.DeleteSystemMember(c, systemID, owners[0])

		if c.Writer.Status() != http.StatusNoContent {
			t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
		}
		exists, err := client.ResourceRoleBinding.Query().
			Where(
				resourcerolebinding.ResourceTypeEQ("system"),
				resourcerolebinding.ResourceIDEQ(systemID),
				resourcerolebinding.UserIDEQ(owners[0]),
			).
			Exist(t.Context())
		if err != nil {
			t.Fatalf("check deleted member: %v", err)
		}
		if exists {
			t.Fatalf("member %q still exists after delete", owners[0])
		}
		assertSingleSystemOwner(t, client, systemID)
	})
}

func TestMemberHandler_ConcurrentOwnerMutations_NeverRemoveEveryOwner(t *testing.T) {
	srv, client := newSystemBehaviorTestServer(t)
	systemID, owners := seedSystemOwners(t, client, 2)

	updateContext, updateResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/systems/"+systemID+"/members/"+owners[0],
		`{"role":"admin"}`,
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemID+"/members/"+owners[1],
		"",
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	updateRequestContext, cancelUpdate := context.WithTimeout(updateContext.Request.Context(), 10*time.Second)
	deleteRequestContext, cancelDelete := context.WithTimeout(deleteContext.Request.Context(), 10*time.Second)
	t.Cleanup(cancelUpdate)
	t.Cleanup(cancelDelete)
	updateContext.Request = updateContext.Request.WithContext(updateRequestContext)
	deleteContext.Request = deleteContext.Request.WithContext(deleteRequestContext)

	releaseGuard, blockerPID := holdSystemMembershipGuard(t, srv.pool, systemID)
	updateDone := runHandlerAsync(func() {
		srv.UpdateSystemMemberRole(updateContext, systemID, owners[0])
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() {
		srv.DeleteSystemMember(deleteContext, systemID, owners[1])
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	releaseGuard()
	waitForHandlerCompletion(t, updateDone, "update first owner")
	waitForHandlerCompletion(t, deleteDone, "delete second owner")

	type mutationResult struct {
		status int
		body   string
	}
	got := []mutationResult{
		{status: updateContext.Writer.Status(), body: updateResponse.Body.String()},
		{status: deleteContext.Writer.Status(), body: deleteResponse.Body.String()},
	}

	successes := 0
	conflicts := 0
	for _, result := range got {
		switch result.status {
		case http.StatusOK, http.StatusNoContent:
			successes++
		case http.StatusConflict:
			conflicts++
			assertErrorCode(t, []byte(result.body), "LAST_OWNER_CANNOT_BE_REMOVED")
		default:
			t.Fatalf("unexpected concurrent mutation response: status=%d body=%s", result.status, result.body)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want exactly one of each; responses=%+v", successes, conflicts, got)
	}
	assertSingleSystemOwner(t, client, systemID)
}

func TestDeleteUser_ProtectsSystemOwnerInvariant(t *testing.T) {
	t.Run("sole owner is preserved", func(t *testing.T) {
		srv, client := newSystemBehaviorTestServer(t)
		systemID, owners := seedSystemOwners(t, client, 1)

		c, w := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/admin/users/"+owners[0],
			"",
			"platform-admin",
			[]string{"user:manage", "platform:admin"},
		)
		srv.DeleteUser(c, owners[0])

		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusConflict, w.Body.String())
		}
		assertErrorCode(t, w.Body.Bytes(), "LAST_OWNER_CANNOT_BE_REMOVED")
		if _, err := client.User.Get(t.Context(), owners[0]); err != nil {
			t.Fatalf("sole owner user should remain after rejected delete: %v", err)
		}
		assertSystemMemberRole(t, client, systemID, owners[0], resourcerolebinding.RoleOwner)
	})

	t.Run("owner can be deleted when another remains", func(t *testing.T) {
		srv, client := newSystemBehaviorTestServer(t)
		systemID, owners := seedSystemOwners(t, client, 2)

		c, w := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/admin/users/"+owners[0],
			"",
			"platform-admin",
			[]string{"user:manage", "platform:admin"},
		)
		srv.DeleteUser(c, owners[0])

		if c.Writer.Status() != http.StatusNoContent {
			t.Fatalf("status = %d, want %d body=%s", c.Writer.Status(), http.StatusNoContent, w.Body.String())
		}
		if _, err := client.User.Get(t.Context(), owners[0]); !ent.IsNotFound(err) {
			t.Fatalf("deleted owner user query error = %v, want not found", err)
		}
		assertSingleSystemOwner(t, client, systemID)
	})
}

func TestDeleteUser_ConcurrentWithMemberDelete_NeverRemovesEveryOwner(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "delete_user_owner_concurrency")
	systemID, owners := seedSystemOwners(t, client, 2)
	beforeSessionVersion, err := authSessions.CurrentSessionVersion(t.Context(), owners[0])
	if err != nil {
		t.Fatalf("seed owner auth session state: %v", err)
	}

	deleteUserContext, deleteUserResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+owners[0],
		"",
		"platform-admin",
		[]string{"user:manage", "platform:admin"},
	)
	deleteMemberContext, deleteMemberResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/systems/"+systemID+"/members/"+owners[1],
		"",
		"platform-admin",
		[]string{"rbac:manage", "platform:admin"},
	)
	deleteUserRequestContext, cancelDeleteUser := context.WithTimeout(deleteUserContext.Request.Context(), 10*time.Second)
	deleteMemberRequestContext, cancelDeleteMember := context.WithTimeout(deleteMemberContext.Request.Context(), 10*time.Second)
	t.Cleanup(cancelDeleteUser)
	t.Cleanup(cancelDeleteMember)
	deleteUserContext.Request = deleteUserContext.Request.WithContext(deleteUserRequestContext)
	deleteMemberContext.Request = deleteMemberContext.Request.WithContext(deleteMemberRequestContext)

	releaseGuard, blockerPID := holdSystemMembershipGuard(t, srv.pool, systemID)
	deleteUserDone := runHandlerAsync(func() {
		srv.DeleteUser(deleteUserContext, owners[0])
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteMemberDone := runHandlerAsync(func() {
		srv.DeleteSystemMember(deleteMemberContext, systemID, owners[1])
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	releaseGuard()
	waitForHandlerCompletion(t, deleteUserDone, "delete owner user")
	waitForHandlerCompletion(t, deleteMemberDone, "delete other owner membership")

	type mutationResult struct {
		operation string
		status    int
		body      string
	}
	got := []mutationResult{
		{operation: "delete user", status: deleteUserContext.Writer.Status(), body: deleteUserResponse.Body.String()},
		{operation: "delete member", status: deleteMemberContext.Writer.Status(), body: deleteMemberResponse.Body.String()},
	}

	successes := 0
	conflicts := 0
	deleteUserStatus := 0
	for _, result := range got {
		if result.operation == "delete user" {
			deleteUserStatus = result.status
		}
		switch result.status {
		case http.StatusNoContent:
			successes++
		case http.StatusConflict:
			conflicts++
			assertErrorCode(t, []byte(result.body), "LAST_OWNER_CANNOT_BE_REMOVED")
		default:
			t.Fatalf("unexpected %s response: status=%d body=%s", result.operation, result.status, result.body)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d, want exactly one of each; responses=%+v", successes, conflicts, got)
	}
	assertSingleSystemOwner(t, client, systemID)
	afterSessionVersion, err := authSessions.CurrentSessionVersion(t.Context(), owners[0])
	if err != nil {
		t.Fatalf("read owner auth session state after concurrency: %v", err)
	}
	wantSessionVersion := beforeSessionVersion
	if deleteUserStatus == http.StatusNoContent {
		wantSessionVersion++
	}
	if afterSessionVersion != wantSessionVersion {
		t.Fatalf(
			"deleted-user session version = %d, want %d for delete status %d",
			afterSessionVersion,
			wantSessionVersion,
			deleteUserStatus,
		)
	}
}

func seedSystemOwners(t *testing.T, client *ent.Client, count int) (systemID string, owners []string) {
	t.Helper()

	suffix := uuid.NewString()
	systemID = "sys-owner-" + suffix
	mustCreateSystem(t, client, systemID, "owners"+suffix[:8], "seed")
	owners = make([]string, 0, count)
	for i := range count {
		userID := fmt.Sprintf("owner-%d-%s", i, suffix)
		if _, err := client.User.Create().
			SetID(userID).
			SetUsername(userID).
			SetEnabled(true).
			Save(t.Context()); err != nil {
			t.Fatalf("create owner user: %v", err)
		}
		mustCreateSystemBinding(t, client, userID, systemID, resourcerolebinding.RoleOwner.String())
		owners = append(owners, userID)
	}
	return systemID, owners
}

func assertSystemMemberRole(
	t *testing.T,
	client *ent.Client,
	systemID string,
	userID string,
	want resourcerolebinding.Role,
) {
	t.Helper()

	binding, err := client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
			resourcerolebinding.UserIDEQ(userID),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query member role: %v", err)
	}
	if binding.Role != want {
		t.Fatalf("member role = %q, want %q", binding.Role, want)
	}
}

func assertSingleSystemOwner(t *testing.T, client *ent.Client, systemID string) {
	t.Helper()

	count, err := client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.ResourceIDEQ(systemID),
			resourcerolebinding.RoleEQ(resourcerolebinding.RoleOwner),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count system owners: %v", err)
	}
	if count != 1 {
		t.Fatalf("owner count = %d, want 1", count)
	}
}

func TestApplyUserSearch_SupportsMailAndRolesAliases(t *testing.T) {
	t.Parallel()

	_, client := newAdminIdentityTestServer(t)
	userEnt, err := client.User.Create().
		SetID("user-search-aliases").
		SetUsername("alice.alias").
		SetDisplayName("Alice Alias").
		SetEmail("alice.alias@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	roleEnt, err := client.Role.Create().
		SetID("role-search-aliases").
		SetName("TeamLead").
		SetDisplayName("Team Lead").
		SetPermissions([]string{"user:manage"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	if _, createErr := client.RoleBinding.Create().
		SetID("binding-search-aliases").
		SetUserID(userEnt.ID).
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("test").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create role binding: %v", createErr)
	}
	disabledRoleUser, err := client.User.Create().
		SetID("user-search-disabled-role").
		SetUsername("disabled.role.only").
		SetDisplayName("Disabled Role Only").
		SetEmail("disabled.role.only@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create disabled role user: %v", err)
	}
	disabledRole, err := client.Role.Create().
		SetID("role-search-dormant").
		SetName("DormantRole").
		SetDisplayName("Dormant Role").
		SetPermissions([]string{"user:manage"}).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create disabled role: %v", err)
	}
	if _, createErr := client.RoleBinding.Create().
		SetID("binding-search-disabled-role").
		SetUserID(disabledRoleUser.ID).
		SetRoleID(disabledRole.ID).
		SetScopeType("global").
		SetCreatedBy("test").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create disabled role binding: %v", createErr)
	}

	usersByMail, err := applyUserSearch(client.User.Query(), "mail:alice.alias@example.com", nil).All(t.Context())
	if err != nil {
		t.Fatalf("search by mail alias: %v", err)
	}
	if len(usersByMail) != 1 || usersByMail[0].ID != userEnt.ID {
		t.Fatalf("mail alias search returned %+v, want only %s", usersByMail, userEnt.ID)
	}

	usersByRoles, err := applyUserSearch(client.User.Query(), `roles:"Team Lead"`, nil).All(t.Context())
	if err != nil {
		t.Fatalf("search by roles alias: %v", err)
	}
	if len(usersByRoles) != 1 || usersByRoles[0].ID != userEnt.ID {
		t.Fatalf("roles alias search returned %+v, want only %s", usersByRoles, userEnt.ID)
	}

	usersByDisabledRole, err := applyUserSearch(client.User.Query(), `roles:"Dormant Role"`, nil).All(t.Context())
	if err != nil {
		t.Fatalf("search by disabled role alias: %v", err)
	}
	if len(usersByDisabledRole) != 0 {
		t.Fatalf("disabled role search returned %+v, want no users", usersByDisabledRole)
	}

	usersByDisabledRoleFreeText, err := applyUserSearch(client.User.Query(), "DormantRole", nil).All(t.Context())
	if err != nil {
		t.Fatalf("free-text search by disabled role: %v", err)
	}
	if len(usersByDisabledRoleFreeText) != 0 {
		t.Fatalf("disabled role free-text search returned %+v, want no users", usersByDisabledRoleFreeText)
	}
}
