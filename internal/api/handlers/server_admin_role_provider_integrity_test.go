package handlers

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortmapping"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
	"kv-shepherd.io/shepherd/internal/service"
)

func TestDeleteRole_ConcurrentCohortMappingCreateKeepsReferencedRole(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "delete-role-mapping-race")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "delete-role-mapping-race", "delete_role_mapping_race")

	mappingContext, mappingResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/cohort-mappings",
		`{"cohort_kind":"group","cohort_key":"operators","role_id":"`+roleRow.ID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_create"},
	)
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+roleRow.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, roleRow.ID)
	mappingDone := runHandlerAsync(func() {
		srv.CreateAuthProviderCohortMapping(mappingContext, providerRow.ID)
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() { srv.DeleteRole(deleteContext, roleRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, mappingDone, "cohort mapping create before role delete")
	waitForHandlerCompletion(t, deleteDone, "role delete after cohort mapping create")

	if mappingResponse.Code != http.StatusCreated {
		t.Fatalf("mapping create status = %d, want %d body=%s", mappingResponse.Code, http.StatusCreated, mappingResponse.Body.String())
	}
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("role delete status = %d, want %d body=%s", deleteResponse.Code, http.StatusConflict, deleteResponse.Body.String())
	}
	assertErrorCode(t, deleteResponse.Body.Bytes(), "ROLE_IN_USE")
	if _, err := client.Role.Get(t.Context(), roleRow.ID); err != nil {
		t.Fatalf("referenced role should remain after concurrent mapping create: %v", err)
	}
	mappingCount, err := client.ExternalCohortMapping.Query().
		Where(
			externalcohortmapping.ProviderIDEQ(providerRow.ID),
			externalcohortmapping.RoleIDEQ(roleRow.ID),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count committed cohort mappings: %v", err)
	}
	if mappingCount != 1 {
		t.Fatalf("cohort mapping count = %d, want 1", mappingCount)
	}
}

func TestDeleteRole_ConcurrentRoleBindingCreateKeepsReferencedRole(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "delete_role_binding_race")
	userRow := seedEnabledRoleAssignmentUser(t, client, "delete-role-binding-race", "delete.role.binding.race")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "delete-role-binding-race", "delete_role_binding_race")

	bindingContext, bindingResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+roleRow.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, roleRow.ID)
	bindingDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(bindingContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() { srv.DeleteRole(deleteContext, roleRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, bindingDone, "role binding create before role delete")
	waitForHandlerCompletion(t, deleteDone, "role delete after role binding create")

	if bindingResponse.Code != http.StatusCreated {
		t.Fatalf("role binding create status = %d, want %d body=%s", bindingResponse.Code, http.StatusCreated, bindingResponse.Body.String())
	}
	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("role delete status = %d, want %d body=%s", deleteResponse.Code, http.StatusConflict, deleteResponse.Body.String())
	}
	assertErrorCode(t, deleteResponse.Body.Bytes(), "ROLE_IN_USE")
	if _, err := client.Role.Get(t.Context(), roleRow.ID); err != nil {
		t.Fatalf("referenced role should remain after concurrent binding create: %v", err)
	}
	assertRoleAssignmentBindingCount(t, client, userRow.ID, roleRow.ID, 1)
}

func TestDeleteRole_ConcurrentManagedRoleBindingCreateUsesRoleLock(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "delete-role-managed-binding-race")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "delete-role-managed-binding-race", "delete_role_managed_binding_race")
	if _, err := client.ExternalCohortMapping.Create().
		SetID("delete-role-managed-binding-race").
		SetProviderID(providerRow.ID).
		SetCohortKind("group").
		SetCohortKey("operators").
		SetRoleID(roleRow.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed managed binding mapping: %v", err)
	}
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/roles/"+roleRow.ID,
		"",
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdRoleAssignmentRowLock(t, srv.pool, roleRow.ID)
	managedUserID := ""
	var managedErr error
	managedDone := runHandlerAsync(func() {
		managedErr = WithTx(t.Context(), client, func(tx *ent.Tx) error {
			if err := service.LockAuthProviderMutation(t.Context(), tx, providerRow.ID); err != nil {
				return err
			}
			result, err := service.NewExternalAuthService(client).
				WithTransaction(tx).
				UpsertExternalUser(t.Context(), providerRow.ID, runtimecontract.AuthResult{
					ExternalID:  "delete-role-managed-binding-race",
					Username:    "delete.role.managed.binding.race",
					DisplayName: "Managed Role Lock",
					Enabled:     true,
					Cohorts: []runtimecontract.ExternalCohort{
						{Kind: "group", Key: "operators", DisplayName: "Operators"},
					},
				})
			if err != nil {
				return err
			}
			if result == nil || result.User == nil {
				return fmt.Errorf("managed role binding reconciliation returned no user")
			}
			managedUserID = result.User.ID
			return nil
		})
	})
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() { srv.DeleteRole(deleteContext, roleRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, managedDone, "managed role binding reconciliation")
	if managedErr != nil {
		t.Fatalf("managed role binding reconciliation: %v", managedErr)
	}
	waitForHandlerCompletion(t, deleteDone, "role delete after managed role binding create")

	if deleteResponse.Code != http.StatusConflict {
		t.Fatalf("role delete status = %d, want %d body=%s", deleteResponse.Code, http.StatusConflict, deleteResponse.Body.String())
	}
	assertErrorCode(t, deleteResponse.Body.Bytes(), "ROLE_IN_USE")
	if _, err := client.Role.Get(t.Context(), roleRow.ID); err != nil {
		t.Fatalf("managed-binding role should remain: %v", err)
	}
	if managedUserID == "" {
		t.Fatal("managed user ID is empty after successful reconciliation")
	}
	managedBindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.CreatedByEQ(externalCohortRoleBindingActor)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count managed role bindings: %v", err)
	}
	if managedBindingCount != 1 {
		t.Fatalf("managed role binding count = %d, want 1", managedBindingCount)
	}
}

func TestUpdateAuthProviderCohortMapping_LocksUsersBeforeRole(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "mapping_user_role_lock_order")
	providerRow := seedRoleAssignmentAuthProvider(t, client, "mapping-user-role-lock-order")
	oldRole := seedEnabledRoleAssignmentRole(t, client, "mapping-lock-order-old", "mapping_lock_order_old")
	targetRole := seedEnabledRoleAssignmentRole(t, client, "mapping-lock-order-target", "mapping_lock_order_target")
	userRow := seedEnabledRoleAssignmentUser(t, client, "mapping-lock-order-user", "mapping.lock.order.user")
	mappingRow, err := client.ExternalCohortMapping.Create().
		SetID("mapping-user-role-lock-order").
		SetProviderID(providerRow.ID).
		SetCohortKind("group").
		SetCohortKey("operators").
		SetRoleID(oldRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed mapping lock-order mapping: %v", err)
	}
	oldBinding, err := client.RoleBinding.Create().
		SetID("mapping-user-role-lock-order-old-binding").
		SetUserID(userRow.ID).
		SetRoleID(oldRole.ID).
		SetScopeType(scopeTypeGlobal).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed mapping lock-order managed binding: %v", err)
	}
	if _, grantErr := client.ExternalCohortGrant.Create().
		SetID("mapping-user-role-lock-order-grant").
		SetUserID(userRow.ID).
		SetProviderID(providerRow.ID).
		SetBindingKey(externalCohortMappingBindingKey(mappingRow)).
		SetRoleBindingID(oldBinding.ID).
		SetSourceMappingIds([]string{mappingRow.ID}).
		SetLastAppliedAt(time.Now().UTC()).
		Save(t.Context()); grantErr != nil {
		t.Fatalf("seed mapping lock-order grant: %v", grantErr)
	}

	manualContext, manualResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: targetRole.ID}),
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

	releaseUser, userBlockerPID, userBlockerTx := holdUserAssignmentRowLock(t, srv.pool, userRow.ID)
	releaseRole, _ := holdRoleAssignmentRowLock(t, srv.pool, targetRole.ID)
	manualDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(manualContext, userRow.ID) })
	// Reuse the blocker transaction for observation so the probe does not
	// require a fifth connection when the test pool has four connections.
	waitForBlockedAdvisoryCalls(t, userBlockerTx, userBlockerPID, 1)
	mappingDone := runHandlerAsync(func() {
		srv.UpdateAuthProviderCohortMapping(mappingContext, providerRow.ID, mappingRow.ID)
	})
	// Both writers must queue on the user row before either can request the
	// target role row. The opposite order creates a user <-> role deadlock.
	waitForBlockedAdvisoryCalls(t, userBlockerTx, userBlockerPID, 2)
	releaseUser()
	releaseRole()
	waitForHandlerCompletion(t, manualDone, "manual role binding in lock-order race")
	waitForHandlerCompletion(t, mappingDone, "mapping update in lock-order race")

	if manualResponse.Code != http.StatusCreated {
		t.Fatalf("manual role binding status = %d, want %d body=%s", manualResponse.Code, http.StatusCreated, manualResponse.Body.String())
	}
	if mappingResponse.Code != http.StatusOK {
		t.Fatalf("mapping update status = %d, want %d body=%s", mappingResponse.Code, http.StatusOK, mappingResponse.Body.String())
	}
	reloadedMapping, err := client.ExternalCohortMapping.Get(t.Context(), mappingRow.ID)
	if err != nil {
		t.Fatalf("reload mapping after lock-order race: %v", err)
	}
	if reloadedMapping.RoleID != targetRole.ID {
		t.Fatalf("mapping role = %q, want %q", reloadedMapping.RoleID, targetRole.ID)
	}
}

func TestSyncAuthProviderCohorts_ConcurrentDeleteRejectsStaleProvider(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "cohort-sync-delete-race")

	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerRow.ID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	syncContext, syncResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/cohorts/sync",
		`{"cohort_kind":"group","source_field":"groups","cohorts":["operators"]}`,
		"admin-1",
		[]string{"auth_provider:sync"},
	)

	release, blockerPID := holdAuthProviderMutationLock(t, srv.pool, providerRow.ID)
	deleteDone := runHandlerAsync(func() { srv.DeleteAuthProvider(deleteContext, providerRow.ID) })
	waitForAuthProviderMutationWaiters(t, srv.pool, blockerPID, 1)
	syncDone := runHandlerAsync(func() { srv.SyncAuthProviderCohorts(syncContext, providerRow.ID) })
	waitForAuthProviderMutationWaiters(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, deleteDone, "auth provider delete before cohort sync")
	waitForHandlerCompletion(t, syncDone, "cohort sync after auth provider delete")

	if deleteContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf("provider delete status = %d, want %d body=%s", deleteContext.Writer.Status(), http.StatusNoContent, deleteResponse.Body.String())
	}
	if syncResponse.Code != http.StatusNotFound {
		t.Fatalf("cohort sync status = %d, want %d body=%s", syncResponse.Code, http.StatusNotFound, syncResponse.Body.String())
	}
	assertErrorCode(t, syncResponse.Body.Bytes(), "AUTH_PROVIDER_NOT_FOUND")
	cohortCount, err := client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ(providerRow.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count cohorts after provider delete race: %v", err)
	}
	if cohortCount != 0 {
		t.Fatalf("orphan cohort count = %d, want 0", cohortCount)
	}
}

func TestSyncAuthProviderCohorts_RollsBackWholeCohortSet(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	providerRow := seedRoleAssignmentAuthProvider(t, client, "cohort-sync-rollback")
	if _, err := srv.pool.Exec(t.Context(), `
CREATE SEQUENCE cohort_sync_failure_witness;
CREATE FUNCTION reject_selected_synced_cohort() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  PERFORM nextval('cohort_sync_failure_witness');
  RAISE EXCEPTION 'forced synced cohort insert failure';
END;
$$;
CREATE TRIGGER reject_selected_synced_cohort
BEFORE INSERT ON external_cohorts
FOR EACH ROW
WHEN (NEW.cohort_key = 'z-fail')
EXECUTE FUNCTION reject_selected_synced_cohort();
`); err != nil {
		t.Fatalf("install cohort sync failure trigger: %v", err)
	}

	syncContext, syncResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/cohorts/sync",
		`{"cohort_kind":"group","source_field":"groups","cohorts":["a-created-first","z-fail"]}`,
		"admin-1",
		[]string{"auth_provider:sync"},
	)
	srv.SyncAuthProviderCohorts(syncContext, providerRow.ID)

	if syncResponse.Code != http.StatusInternalServerError {
		t.Fatalf("cohort sync status = %d, want %d body=%s", syncResponse.Code, http.StatusInternalServerError, syncResponse.Body.String())
	}
	assertErrorCode(t, syncResponse.Body.Bytes(), "INTERNAL_ERROR")
	var failureReached bool
	if err := srv.pool.QueryRow(t.Context(), `SELECT is_called FROM cohort_sync_failure_witness`).Scan(&failureReached); err != nil {
		t.Fatalf("read cohort sync failure witness: %v", err)
	}
	if !failureReached {
		t.Fatal("cohort sync failure trigger was not reached")
	}
	cohortCount, err := client.ExternalCohort.Query().
		Where(externalcohort.ProviderIDEQ(providerRow.ID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count cohorts after failed sync: %v", err)
	}
	if cohortCount != 0 {
		t.Fatalf("cohort count after failed sync = %d, want atomic rollback to 0", cohortCount)
	}
}

func TestCreateUserRoleBinding_ConcurrentEquivalentRequestsCreateOnce(t *testing.T) {
	srv, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "role_binding_duplicate_race")
	userRow := seedEnabledRoleAssignmentUser(t, client, "role-binding-duplicate-race", "role.binding.duplicate.race")
	roleRow := seedEnabledRoleAssignmentRole(t, client, "role-binding-duplicate-race", "role_binding_duplicate_race")
	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("seed auth session version: %v", err)
	}

	firstContext, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)
	secondContext, secondResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/users/"+userRow.ID+"/role-bindings",
		mustJSON(t, userRoleBindingCreateRequest{RoleID: roleRow.ID}),
		"admin-1",
		[]string{"rbac:manage"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userRow.ID)
	firstDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(firstContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	secondDone := runHandlerAsync(func() { srv.CreateUserRoleBinding(secondContext, userRow.ID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, firstDone, "first equivalent role binding request")
	waitForHandlerCompletion(t, secondDone, "second equivalent role binding request")

	if firstResponse.Code != http.StatusCreated {
		t.Fatalf("first role binding status = %d, want %d body=%s", firstResponse.Code, http.StatusCreated, firstResponse.Body.String())
	}
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("second role binding status = %d, want %d body=%s", secondResponse.Code, http.StatusConflict, secondResponse.Body.String())
	}
	assertErrorCode(t, secondResponse.Body.Bytes(), "ROLE_BINDING_EXISTS")
	assertRoleAssignmentBindingCount(t, client, userRow.ID, roleRow.ID, 1)

	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("read auth session version after duplicate race: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("auth session version = %d, want one successful binding bump to %d", afterVersion, beforeVersion+1)
	}
}

func holdUserAssignmentRowLock(
	t *testing.T,
	pool *pgxpool.Pool,
	userID string,
) (release func(), blockerPID int32, blockerTx pgx.Tx) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire user-row blocker connection: %v", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		t.Fatalf("begin user-row blocker transaction: %v", err)
	}
	var lockedUserID string
	if err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("lock user row: %v", err)
	}
	if lockedUserID != userID {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("locked user ID = %q, want %q", lockedUserID, userID)
	}
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		_ = tx.Rollback(ctx)
		conn.Release()
		t.Fatalf("query user-row blocker PID: %v", err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			defer conn.Release()
			if err := tx.Commit(cleanupCtx); err != nil {
				_ = conn.Conn().Close(cleanupCtx)
				t.Fatalf("release user-row lock: %v", err)
			}
		})
	}
	t.Cleanup(release)
	return release, blockerPID, tx
}
