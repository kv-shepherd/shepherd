package handlers

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	entrole "kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
)

func TestCreateUserRoleBinding_ConcurrentUserDisableRejectsStaleEnabledPrecheck(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"role_binding_concurrent_user_disable",
	)
	userRow := seedEnabledRoleAssignmentUser(t, client, "role-binding-user-disable", "role.binding.user.disable")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "role-binding-user-disable", "role_binding_user_disable")
	beforeVersion, versionErr := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if versionErr != nil {
		t.Fatalf("seed auth session version: %v", versionErr)
	}

	disableContext, disableResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/users/"+userRow.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"user:manage"},
	)
	bindingContext, bindingResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userRow.ID)
	disableDone := runHandlerAsync(func() { srv.UpdateUser(disableContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	bindingDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(bindingContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, disableDone, "concurrent user disable")
	waitForHandlerCompletion(t, bindingDone, "role binding after concurrent user disable")

	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable user status = %d, want %d body=%s", disableResponse.Code, http.StatusOK, disableResponse.Body.String())
	}
	if bindingResponse.Code != http.StatusConflict {
		t.Fatalf("role binding status = %d, want %d body=%s", bindingResponse.Code, http.StatusConflict, bindingResponse.Body.String())
	}
	assertErrorCode(t, bindingResponse.Body.Bytes(), "USER_DISABLED")
	assertRoleAssignmentBindingCount(t, client, userRow.ID, roleRow.ID, 0)

	reloadedUser, loadUserErr := client.User.Get(t.Context(), userRow.ID)
	if loadUserErr != nil {
		t.Fatalf("reload disabled user: %v", loadUserErr)
	}
	if reloadedUser.Enabled {
		t.Fatal("user enabled = true, want committed concurrent disable")
	}
	afterVersion, afterVersionErr := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if afterVersionErr != nil {
		t.Fatalf("read auth session version after race: %v", afterVersionErr)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("auth session version = %d, want only disable bump to %d", afterVersion, beforeVersion+1)
	}
}

func TestCreateUserRoleBinding_ConcurrentRoleDisableRejectsStaleEnabledPrecheck(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"role_binding_concurrent_role_disable",
	)
	userRow := seedEnabledRoleAssignmentUser(t, client, "role-binding-role-disable", "role.binding.role.disable")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "role-binding-role-disable", "role_binding_role_disable")
	beforeVersion, versionErr := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if versionErr != nil {
		t.Fatalf("seed auth session version: %v", versionErr)
	}

	disableContext, disableResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/roles/"+roleRow.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	bindingContext, bindingResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, roleRow.ID)
	disableDone := runHandlerAsync(func() { srv.UpdateRole(disableContext, roleRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	bindingDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(bindingContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, disableDone, "concurrent role disable")
	waitForHandlerCompletion(t, bindingDone, "role binding after concurrent role disable")

	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable role status = %d, want %d body=%s", disableResponse.Code, http.StatusOK, disableResponse.Body.String())
	}
	if bindingResponse.Code != http.StatusConflict {
		t.Fatalf("role binding status = %d, want %d body=%s", bindingResponse.Code, http.StatusConflict, bindingResponse.Body.String())
	}
	assertErrorCode(t, bindingResponse.Body.Bytes(), "ROLE_DISABLED")
	assertRoleAssignmentBindingCount(t, client, userRow.ID, roleRow.ID, 0)

	reloadedRole, loadRoleErr := client.Role.Get(t.Context(), roleRow.ID)
	if loadRoleErr != nil {
		t.Fatalf("reload disabled role: %v", loadRoleErr)
	}
	if reloadedRole.Enabled {
		t.Fatal("role enabled = true, want committed concurrent disable")
	}
	afterVersion, afterVersionErr := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if afterVersionErr != nil {
		t.Fatalf("read auth session version after race: %v", afterVersionErr)
	}
	if afterVersion != beforeVersion {
		t.Fatalf("auth session version = %d, want unchanged value %d", afterVersion, beforeVersion)
	}
}

func TestCreateAuthProviderCohortMapping_ConcurrentRoleDisableRejectsStaleEnabledPrecheck(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "mapping-create-role-disable")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "mapping-create-role-disable", "mapping_create_role_disable")

	disableContext, disableResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/roles/"+roleRow.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	mappingContext, mappingResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/cohort-mappings",
		`{"cohort_kind":"group","cohort_key":"operators","role_id":"`+roleRow.ID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_create"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, roleRow.ID)
	disableDone := runHandlerAsync(func() { srv.UpdateRole(disableContext, roleRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	mappingDone := runHandlerAsync(func() {
		srv.CreateAuthProviderCohortMapping(mappingContext, providerRow.ID)
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, disableDone, "concurrent mapping role disable")
	waitForHandlerCompletion(t, mappingDone, "cohort mapping create after concurrent role disable")

	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable role status = %d, want %d body=%s", disableResponse.Code, http.StatusOK, disableResponse.Body.String())
	}
	if mappingResponse.Code != http.StatusConflict {
		t.Fatalf("mapping create status = %d, want %d body=%s", mappingResponse.Code, http.StatusConflict, mappingResponse.Body.String())
	}
	assertErrorCode(t, mappingResponse.Body.Bytes(), "ROLE_DISABLED")
	mappingCount, countErr := client.ExternalCohortMapping.Query().
		Where(externalcohortmapping.ProviderIDEQ(providerRow.ID)).
		Count(t.Context())
	if countErr != nil {
		t.Fatalf("count cohort mappings after race: %v", countErr)
	}
	if mappingCount != 0 {
		t.Fatalf("cohort mapping count = %d, want 0", mappingCount)
	}
}

func TestUpdateAuthProviderCohortMapping_ConcurrentRoleDisableRejectsStaleEnabledPrecheck(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "mapping-update-role-disable")
	oldRole := seedEnabledRoleAssignmentRole(t, client, "mapping-update-old-role", "mapping_update_old_role")
	targetRole := seedEnabledRoleAssignmentRole(t, client, "mapping-update-target-role", "mapping_update_target_role")
	mappingRow, createMappingErr := client.ExternalCohortMapping.Create().
		SetID("mapping-update-role-disable").
		SetProviderID(providerRow.ID).
		SetCohortKind("group").
		SetCohortKey("operators").
		SetRoleID(oldRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if createMappingErr != nil {
		t.Fatalf("seed cohort mapping: %v", createMappingErr)
	}

	disableContext, disableResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/roles/"+targetRole.ID,
		`{"enabled":false}`,
		"admin-1",
		[]string{"rbac:manage"},
	)
	mappingContext, mappingResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerRow.ID+"/cohort-mappings/"+mappingRow.ID,
		`{"role_id":"`+targetRole.ID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, targetRole.ID)
	disableDone := runHandlerAsync(func() { srv.UpdateRole(disableContext, targetRole.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	mappingDone := runHandlerAsync(func() {
		srv.UpdateAuthProviderCohortMapping(mappingContext, providerRow.ID, mappingRow.ID)
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, disableDone, "concurrent target role disable")
	waitForHandlerCompletion(t, mappingDone, "cohort mapping update after concurrent role disable")

	if disableResponse.Code != http.StatusOK {
		t.Fatalf("disable role status = %d, want %d body=%s", disableResponse.Code, http.StatusOK, disableResponse.Body.String())
	}
	if mappingResponse.Code != http.StatusConflict {
		t.Fatalf("mapping update status = %d, want %d body=%s", mappingResponse.Code, http.StatusConflict, mappingResponse.Body.String())
	}
	assertErrorCode(t, mappingResponse.Body.Bytes(), "ROLE_DISABLED")
	reloadedMapping, loadMappingErr := client.ExternalCohortMapping.Get(t.Context(), mappingRow.ID)
	if loadMappingErr != nil {
		t.Fatalf("reload cohort mapping after race: %v", loadMappingErr)
	}
	if reloadedMapping.RoleID != oldRole.ID {
		t.Fatalf("mapping role ID = %q, want unchanged role %q", reloadedMapping.RoleID, oldRole.ID)
	}
}

func seedEnabledRoleAssignmentUser(t *testing.T, client *ent.Client, id, username string) *ent.User {
	t.Helper()
	userRow, createErr := client.User.Create().
		SetID(id).
		SetUsername(username).
		SetEnabled(true).
		Save(t.Context())
	if createErr != nil {
		t.Fatalf("seed enabled role-assignment user: %v", createErr)
	}
	return userRow
}

func seedEnabledRoleAssignmentRole(t *testing.T, client *ent.Client, id, name string) *ent.Role {
	t.Helper()
	roleRow, createErr := client.Role.Create().
		SetID(id).
		SetName(name).
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if createErr != nil {
		t.Fatalf("seed enabled role-assignment role: %v", createErr)
	}
	return roleRow
}

func seedRoleAssignmentAuthProvider(t *testing.T, client *ent.Client, id string) *ent.AuthProvider {
	t.Helper()
	providerRow, createErr := client.AuthProvider.Create().
		SetID(id).
		SetName("Role Assignment " + id).
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if createErr != nil {
		t.Fatalf("seed role-assignment auth provider: %v", createErr)
	}
	return providerRow
}

func assertRoleAssignmentBindingCount(t *testing.T, client *ent.Client, userID, roleID string, want int) {
	t.Helper()
	bindingCount, countErr := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(userID)),
			rolebinding.HasRoleWith(entrole.IDEQ(roleID)),
		).
		Count(t.Context())
	if countErr != nil {
		t.Fatalf("count role-assignment bindings: %v", countErr)
	}
	if bindingCount != want {
		t.Fatalf("role-assignment binding count = %d, want %d", bindingCount, want)
	}
}

func holdRoleAssignmentRowLock(t *testing.T, pool *pgxpool.Pool, roleID string) (release func(), blockerPID int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	conn, acquireErr := pool.Acquire(ctx)
	if acquireErr != nil {
		t.Fatalf("acquire role-row blocker connection: %v", acquireErr)
	}
	tx, beginErr := conn.Begin(ctx)
	if beginErr != nil {
		conn.Release()
		t.Fatalf("begin role-row blocker transaction: %v", beginErr)
	}
	var lockedRoleID string
	if lockErr := tx.QueryRow(ctx, `SELECT id FROM roles WHERE id = $1 FOR UPDATE`, roleID).Scan(&lockedRoleID); lockErr != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("lock role row: %v", lockErr)
	}
	if lockedRoleID != roleID {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("locked role ID = %q, want %q", lockedRoleID, roleID)
	}
	if pidErr := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); pidErr != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("query role-row blocker PID: %v", pidErr)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			defer conn.Release()
			if commitErr := tx.Commit(cleanupCtx); commitErr != nil {
				_ = conn.Conn().Close(cleanupCtx)
				t.Fatalf("release role-row lock: %v", commitErr)
			}
		})
	}
	t.Cleanup(release)
	return release, blockerPID
}
