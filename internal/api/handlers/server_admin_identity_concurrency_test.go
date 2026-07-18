package handlers

import (
	"context"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/provider"
)

func TestUpdateAuthProviderConcurrentDisjointConfigPatchesPreserveBothChanges(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers",
		`{
			"name":"Concurrent Provider Config",
			"auth_type":"oidc",
			"enabled":true,
			"config":{
				"issuer_url":"https://issuer.initial.example.com",
				"client_id":"client-initial",
				"client_secret":"secret-initial"
			}
		}`,
		"admin-1",
		[]string{"auth_provider:configure"},
	)
	srv.CreateAuthProvider(createContext)
	if createResponse.Code != http.StatusCreated {
		t.Fatalf(
			"create auth provider status = %d, want %d, body=%s",
			createResponse.Code,
			http.StatusCreated,
			createResponse.Body.String(),
		)
	}
	var created generated.AuthProvider
	mustDecodeJSON(t, createResponse.Body.Bytes(), &created)

	issuerContext, issuerResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+created.Id,
		`{"config":{"issuer_url":"https://issuer.updated.example.com"}}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)
	clientContext, clientResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+created.Id,
		`{"config":{"client_id":"client-updated"}}`,
		"admin-1",
		[]string{"auth_provider:update"},
	)

	release, blockerPID := holdAuthProviderMutationLock(t, srv.pool, created.Id)
	issuerDone := runHandlerAsync(func() { srv.UpdateAuthProvider(issuerContext, created.Id) })
	clientDone := runHandlerAsync(func() { srv.UpdateAuthProvider(clientContext, created.Id) })
	waitForAuthProviderMutationWaiters(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, issuerDone, "issuer config patch")
	waitForHandlerCompletion(t, clientDone, "client ID config patch")

	assertConcurrentPatchStatuses(t, issuerResponse.Code, issuerResponse.Body.String(), clientResponse.Code, clientResponse.Body.String())

	reloaded, err := client.AuthProvider.Get(ctx, created.Id)
	if err != nil {
		t.Fatalf("reload auth provider after concurrent patches: %v", err)
	}
	if got := reloaded.Config["issuer_url"]; got != "https://issuer.updated.example.com" {
		t.Fatalf("stored issuer_url = %#v, want concurrent issuer change", got)
	}
	if got := reloaded.Config["client_id"]; got != "client-updated" {
		t.Fatalf("stored client_id = %#v, want concurrent client ID change", got)
	}
	storedSecret, _ := reloaded.Config["client_secret"].(string)
	if storedSecret == "" || storedSecret == "secret-initial" || storedSecret == provider.AuthProviderProtectedFieldMask {
		t.Fatalf("stored client_secret = %q, want the original encrypted secret", storedSecret)
	}
}

func TestUpdateAuthProviderCohortMappingConcurrentDisjointPatchesReconcileFinalMapping(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)
	ctx := t.Context()

	const (
		providerID = "provider-concurrent-mapping-patch"
		mappingID  = "mapping-concurrent-patch"
		userID     = "user-concurrent-mapping-patch"
		oldRoleID  = "role-concurrent-mapping-old"
		newRoleID  = "role-concurrent-mapping-new"
	)
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Concurrent Mapping Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx); err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	if _, err := client.Role.Create().
		SetID(oldRoleID).
		SetName("concurrent_mapping_old").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed old mapping role: %v", err)
	}
	if _, err := client.Role.Create().
		SetID(newRoleID).
		SetName("concurrent_mapping_new").
		SetPermissions([]string{"vm:read", "vm:operate"}).
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed new mapping role: %v", err)
	}
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("concurrent.mapping.patch").
		SetEnabled(true).
		Save(ctx); err != nil {
		t.Fatalf("seed mapped user: %v", err)
	}
	mapping, err := client.ExternalCohortMapping.Create().
		SetID(mappingID).
		SetProviderID(providerID).
		SetCohortKind("group").
		SetCohortKey("platform").
		SetRoleID(oldRoleID).
		SetScopeType(scopeTypeGlobal).
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort mapping: %v", err)
	}
	oldBinding, err := client.RoleBinding.Create().
		SetID("binding-concurrent-mapping-old").
		SetUserID(userID).
		SetRoleID(oldRoleID).
		SetScopeType(scopeTypeGlobal).
		SetAllowedEnvironments([]string{"prod"}).
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed managed role binding: %v", err)
	}
	oldGrant, err := client.ExternalCohortGrant.Create().
		SetID("grant-concurrent-mapping-old").
		SetUserID(userID).
		SetProviderID(providerID).
		SetBindingKey(externalCohortMappingBindingKey(mapping)).
		SetRoleBindingID(oldBinding.ID).
		SetSourceMappingIds([]string{mappingID}).
		SetLastAppliedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed external cohort grant: %v", err)
	}

	roleContext, roleResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerID+"/cohort-mappings/"+mappingID,
		`{"role_id":"`+newRoleID+`"}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)
	environmentsContext, environmentsResponse := newAuthedGinContext(
		t,
		http.MethodPatch,
		"/admin/auth-providers/"+providerID+"/cohort-mappings/"+mappingID,
		`{"allowed_environments":["test"]}`,
		"admin-1",
		[]string{"auth_provider:mapping_update"},
	)

	release, blockerPID := holdAuthProviderMutationLock(t, srv.pool, providerID)
	roleDone := runHandlerAsync(func() {
		srv.UpdateAuthProviderCohortMapping(roleContext, providerID, mappingID)
	})
	environmentsDone := runHandlerAsync(func() {
		srv.UpdateAuthProviderCohortMapping(environmentsContext, providerID, mappingID)
	})
	waitForAuthProviderMutationWaiters(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, roleDone, "mapping role patch")
	waitForHandlerCompletion(t, environmentsDone, "mapping environments patch")

	assertConcurrentPatchStatuses(
		t,
		roleResponse.Code,
		roleResponse.Body.String(),
		environmentsResponse.Code,
		environmentsResponse.Body.String(),
	)

	reloaded, err := client.ExternalCohortMapping.Get(ctx, mappingID)
	if err != nil {
		t.Fatalf("reload mapping after concurrent patches: %v", err)
	}
	if reloaded.RoleID != newRoleID {
		t.Fatalf("mapping role_id = %q, want concurrent role change %q", reloaded.RoleID, newRoleID)
	}
	if !slices.Equal(reloaded.AllowedEnvironments, []string{"test"}) {
		t.Fatalf("mapping allowed_environments = %#v, want concurrent environment change [test]", reloaded.AllowedEnvironments)
	}

	grants, err := client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(userID),
			externalcohortgrant.ProviderIDEQ(providerID),
		).
		All(ctx)
	if err != nil {
		t.Fatalf("query reconciled external cohort grants: %v", err)
	}
	if len(grants) != 1 {
		t.Fatalf("reconciled grant count = %d, want exactly 1", len(grants))
	}
	finalGrant := grants[0]
	if finalGrant.ID == oldGrant.ID {
		t.Fatal("concurrent mapping reconciliation retained the stale grant")
	}
	if finalGrant.BindingKey != externalCohortMappingBindingKey(reloaded) {
		t.Fatalf(
			"reconciled grant binding_key = %q, want final mapping key %q",
			finalGrant.BindingKey,
			externalCohortMappingBindingKey(reloaded),
		)
	}
	if !slices.Equal(finalGrant.SourceMappingIds, []string{mappingID}) {
		t.Fatalf("reconciled grant source_mapping_ids = %#v, want [%q]", finalGrant.SourceMappingIds, mappingID)
	}
	finalBinding, err := client.RoleBinding.Get(ctx, finalGrant.RoleBindingID)
	if err != nil {
		t.Fatalf("query reconciled managed role binding: %v", err)
	}
	finalRole, err := finalBinding.QueryRole().Only(ctx)
	if err != nil {
		t.Fatalf("query role for reconciled managed role binding: %v", err)
	}
	if finalRole.ID != newRoleID || finalBinding.ScopeType != scopeTypeGlobal {
		t.Fatalf(
			"reconciled binding role/scope = %q/%q, want %q/%q",
			finalRole.ID,
			finalBinding.ScopeType,
			newRoleID,
			scopeTypeGlobal,
		)
	}
	if !slices.Equal(finalBinding.AllowedEnvironments, []string{"test"}) {
		t.Fatalf("reconciled binding allowed_environments = %#v, want [test]", finalBinding.AllowedEnvironments)
	}
	managedBindingCount, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(entuser.IDEQ(userID)),
			rolebinding.CreatedByEQ(externalCohortRoleBindingActor),
		).
		Count(ctx)
	if err != nil {
		t.Fatalf("count reconciled managed role bindings: %v", err)
	}
	if managedBindingCount != 1 {
		t.Fatalf("managed role binding count = %d, want exactly 1 final binding", managedBindingCount)
	}
	if _, err := client.RoleBinding.Get(ctx, oldBinding.ID); !ent.IsNotFound(err) {
		t.Fatalf("stale managed role binding lookup error = %v, want not found", err)
	}
}

func assertConcurrentPatchStatuses(t *testing.T, firstStatus int, firstBody string, secondStatus int, secondBody string) {
	t.Helper()
	if firstStatus != http.StatusOK || secondStatus != http.StatusOK {
		t.Fatalf(
			"concurrent patch statuses = %d/%d, want %d/%d; bodies=%s / %s",
			firstStatus,
			secondStatus,
			http.StatusOK,
			http.StatusOK,
			firstBody,
			secondBody,
		)
	}
}

func holdAuthProviderMutationLock(t *testing.T, pool *pgxpool.Pool, providerID string) (release func(), blockerPID int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire auth provider blocker connection: %v", err)
	}
	if _, err := conn.Exec(ctx, `
SELECT pg_advisory_lock(
  hashtextextended(current_schema() || ':auth_provider:' || $1, 0)
)
`, providerID); err != nil {
		conn.Release()
		t.Fatalf("hold auth provider mutation lock: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		conn.Release()
		t.Fatalf("query auth provider blocker PID: %v", err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			defer conn.Release()
			var unlocked bool
			err := conn.QueryRow(cleanupCtx, `
SELECT pg_advisory_unlock(
  hashtextextended(current_schema() || ':auth_provider:' || $1, 0)
)
`, providerID).Scan(&unlocked)
			if err != nil || !unlocked {
				_ = conn.Conn().Close(cleanupCtx)
				t.Fatalf("release auth provider mutation lock: unlocked=%t err=%v", unlocked, err)
			}
		})
	}
	t.Cleanup(release)
	return release, blockerPID
}

func waitForAuthProviderMutationWaiters(t *testing.T, pool *pgxpool.Pool, blockerPID int32, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	lastCount := -1
	for {
		err := pool.QueryRow(ctx, `
SELECT count(DISTINCT waiter.pid)
FROM pg_locks AS blocker
JOIN pg_locks AS waiter
  ON waiter.locktype = blocker.locktype
 AND waiter.database IS NOT DISTINCT FROM blocker.database
 AND waiter.classid IS NOT DISTINCT FROM blocker.classid
 AND waiter.objid IS NOT DISTINCT FROM blocker.objid
 AND waiter.objsubid IS NOT DISTINCT FROM blocker.objsubid
JOIN pg_stat_activity AS activity
  ON activity.pid = waiter.pid
WHERE blocker.pid = $1
  AND blocker.locktype = 'advisory'
  AND blocker.granted
  AND NOT waiter.granted
  AND activity.datname = current_database()
  AND activity.state = 'active'
  AND $1 = ANY(pg_blocking_pids(waiter.pid))
  AND activity.query LIKE '%pg_advisory_xact_lock%'
`, blockerPID).Scan(&lastCount)
		if err != nil {
			t.Fatalf("query auth provider waiters for blocker PID %d: %v", blockerPID, err)
		}
		if lastCount == want {
			return
		}
		if lastCount > want {
			t.Fatalf("auth provider mutation waiters = %d, want exactly %d", lastCount, want)
		}
		select {
		case <-ctx.Done():
			t.Fatalf(
				"auth provider mutation waiters for blocker PID %d = %d, want %d before timeout: %v",
				blockerPID,
				lastCount,
				want,
				ctx.Err(),
			)
		case <-ticker.C:
		}
	}
}
