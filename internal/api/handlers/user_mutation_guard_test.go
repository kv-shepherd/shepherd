package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/ratelimitexemption"
	"kv-shepherd.io/shepherd/ent/ratelimituseroverride"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/ent/system"
	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestUserMutationGuard_DeleteWinsBeforeAddSystemMemberWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_member")
	userID := seedUserMutationGuardTarget(t, client, "member")
	systemID := "system-" + uuid.NewString()
	if _, err := client.System.Create().
		SetID(systemID).
		SetName("gm-" + uuid.NewString()[:8]).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed system for guarded member add: %v", err)
	}

	deleteContext, _ := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	addContext, addResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems/"+systemID+"/members",
		mustJSON(t, generated.SystemMemberCreateRequest{
			UserId: userID,
			Role:   generated.SystemMemberCreateRequestRole("member"),
		}),
		"admin-1",
		[]string{"platform:admin"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	addDone := runHandlerAsync(func() { srv.AddSystemMember(addContext, systemID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, deleteDone, "delete user")
	waitForHandlerCompletion(t, addDone, "add system member")

	if deleteContext.Writer.Status() != http.StatusNoContent || addResponse.Code != http.StatusNotFound {
		t.Fatalf(
			"guarded delete/add statuses = %d/%d, want %d/%d; add body=%s",
			deleteContext.Writer.Status(),
			addResponse.Code,
			http.StatusNoContent,
			http.StatusNotFound,
			addResponse.Body.String(),
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_DeleteWinsBeforeRateLimitExemptionWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_exemption")
	userID := seedUserMutationGuardTarget(t, client, "exemption")
	deleteContext, _ := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	exemptionContext, exemptionResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"`+userID+`","reason":"trusted automation"}`,
		"admin-1",
		[]string{"rate_limit:manage"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	exemptionDone := runHandlerAsync(func() { srv.CreateRateLimitExemption(exemptionContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, deleteDone, "delete user")
	waitForHandlerCompletion(t, exemptionDone, "create rate-limit exemption")

	if deleteContext.Writer.Status() != http.StatusNoContent || exemptionResponse.Code != http.StatusNotFound {
		t.Fatalf(
			"guarded delete/exemption statuses = %d/%d, want %d/%d; exemption body=%s",
			deleteContext.Writer.Status(),
			exemptionResponse.Code,
			http.StatusNoContent,
			http.StatusNotFound,
			exemptionResponse.Body.String(),
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_DeleteWinsBeforeRateLimitOverrideWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_override")
	userID := seedUserMutationGuardTarget(t, client, "override")
	deleteContext, _ := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	overrideContext, overrideResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/"+userID,
		`{"max_pending_parents":9}`,
		"admin-1",
		[]string{"rate_limit:manage"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	overrideDone := runHandlerAsync(func() { srv.UpdateRateLimitUserOverrides(overrideContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, deleteDone, "delete user")
	waitForHandlerCompletion(t, overrideDone, "create rate-limit override")

	if deleteContext.Writer.Status() != http.StatusNoContent || overrideResponse.Code != http.StatusNotFound {
		t.Fatalf(
			"guarded delete/override statuses = %d/%d, want %d/%d; override body=%s",
			deleteContext.Writer.Status(),
			overrideResponse.Code,
			http.StatusNoContent,
			http.StatusNotFound,
			overrideResponse.Body.String(),
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_DeleteWinsBeforeSystemCreationWithoutOrphanOwner(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_system_create")
	userID := seedUserMutationGuardTarget(t, client, "system-create")
	deleteContext, _ := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems",
		mustJSON(t, generated.SystemCreateRequest{Name: "gs-" + uuid.NewString()[:8]}),
		userID,
		[]string{"platform:admin"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	createDone := runHandlerAsync(func() { srv.CreateSystem(createContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, deleteDone, "delete user")
	waitForHandlerCompletion(t, createDone, "create system")

	if deleteContext.Writer.Status() != http.StatusNoContent || createResponse.Code != http.StatusUnauthorized {
		t.Fatalf(
			"guarded delete/system-create statuses = %d/%d, want %d/%d; create body=%s",
			deleteContext.Writer.Status(),
			createResponse.Code,
			http.StatusNoContent,
			http.StatusUnauthorized,
			createResponse.Body.String(),
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
	systemCount, err := client.System.Query().Where(system.CreatedByEQ(userID)).Count(t.Context())
	if err != nil {
		t.Fatalf("count systems created by deleted user: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("systems created by deleted user = %d, want 0", systemCount)
	}
}

func TestUserMutationGuard_DisableWinsBeforeSystemCreation(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_system_disable")
	userID := seedUserMutationGuardTarget(t, client, "system-disable")
	systemName := "gd-" + uuid.NewString()[:8]
	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems",
		mustJSON(t, generated.SystemCreateRequest{Name: systemName}),
		userID,
		[]string{"platform:admin"},
	)

	disableTx, beginErr := srv.pool.Begin(t.Context())
	if beginErr != nil {
		t.Fatalf("begin user disable transaction: %v", beginErr)
	}
	disableOpen := true
	t.Cleanup(func() {
		if disableOpen {
			_ = disableTx.Rollback(context.Background())
		}
	})
	if _, err := disableTx.Exec(
		t.Context(),
		`SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(userID),
	); err != nil {
		t.Fatalf("lock user before disable: %v", err)
	}
	if _, err := disableTx.Exec(t.Context(), `UPDATE users SET enabled = FALSE WHERE id = $1`, userID); err != nil {
		t.Fatalf("disable user before system create: %v", err)
	}
	var blockerPID int32
	if err := disableTx.QueryRow(t.Context(), `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query disable transaction PID: %v", err)
	}

	createDone := runHandlerAsync(func() { srv.CreateSystem(createContext) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	if err := disableTx.Commit(t.Context()); err != nil {
		t.Fatalf("commit user disable before system create: %v", err)
	}
	disableOpen = false
	waitForHandlerCompletion(t, createDone, "create system after user disable")

	if createResponse.Code != http.StatusUnauthorized {
		t.Fatalf(
			"system create after disable status = %d, want %d body=%s",
			createResponse.Code,
			http.StatusUnauthorized,
			createResponse.Body.String(),
		)
	}
	systemCount, err := client.System.Query().Where(system.NameEQ(systemName)).Count(t.Context())
	if err != nil {
		t.Fatalf("count systems after creator disable: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("systems created after creator disable = %d, want 0", systemCount)
	}
}

func TestUserMutationGuard_AddSystemMemberWinsBeforeDeleteWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_member_writer_first")
	userID := seedUserMutationGuardTarget(t, client, "member-writer-first")
	systemID := "system-" + uuid.NewString()
	if _, err := client.System.Create().
		SetID(systemID).
		SetName("gw-" + uuid.NewString()[:8]).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("seed writer-first member system: %v", err)
	}
	addContext, addResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems/"+systemID+"/members",
		mustJSON(t, generated.SystemMemberCreateRequest{
			UserId: userID,
			Role:   generated.SystemMemberCreateRequestRole("member"),
		}),
		"admin-1",
		[]string{"platform:admin"},
	)

	deleteStatus, _, writerStatus, writerBody := runUserMutationWriterBeforeDelete(
		t,
		srv,
		userID,
		func() { srv.AddSystemMember(addContext, systemID) },
		addResponse,
	)
	if writerStatus != http.StatusCreated || deleteStatus != http.StatusNoContent {
		t.Fatalf(
			"guarded add/delete statuses = %d/%d, want %d/%d; add body=%s",
			writerStatus,
			deleteStatus,
			http.StatusCreated,
			http.StatusNoContent,
			writerBody,
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_RateLimitExemptionWinsBeforeDeleteWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_exemption_writer_first")
	userID := seedUserMutationGuardTarget(t, client, "exemption-writer-first")
	exemptionContext, exemptionResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"`+userID+`","reason":"trusted automation"}`,
		"admin-1",
		[]string{"rate_limit:manage"},
	)

	deleteStatus, _, writerStatus, writerBody := runUserMutationWriterBeforeDelete(
		t,
		srv,
		userID,
		func() { srv.CreateRateLimitExemption(exemptionContext) },
		exemptionResponse,
	)
	if writerStatus != http.StatusOK || deleteStatus != http.StatusNoContent {
		t.Fatalf(
			"guarded exemption/delete statuses = %d/%d, want %d/%d; exemption body=%s",
			writerStatus,
			deleteStatus,
			http.StatusOK,
			http.StatusNoContent,
			writerBody,
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_RateLimitOverrideWinsBeforeDeleteWithoutOrphan(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_override_writer_first")
	userID := seedUserMutationGuardTarget(t, client, "override-writer-first")
	overrideContext, overrideResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/"+userID,
		`{"max_pending_parents":9}`,
		"admin-1",
		[]string{"rate_limit:manage"},
	)

	deleteStatus, _, writerStatus, writerBody := runUserMutationWriterBeforeDelete(
		t,
		srv,
		userID,
		func() { srv.UpdateRateLimitUserOverrides(overrideContext, userID) },
		overrideResponse,
	)
	if writerStatus != http.StatusOK || deleteStatus != http.StatusNoContent {
		t.Fatalf(
			"guarded override/delete statuses = %d/%d, want %d/%d; override body=%s",
			writerStatus,
			deleteStatus,
			http.StatusOK,
			http.StatusNoContent,
			writerBody,
		)
	}
	assertUserAndOrphanRowsAbsent(t, client, userID)
}

func TestUserMutationGuard_SystemCreationWinsBeforeDeletePreservesLastOwner(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_system_writer_first")
	userID := seedUserMutationGuardTarget(t, client, "system-writer-first")
	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/systems",
		mustJSON(t, generated.SystemCreateRequest{Name: "gw-" + uuid.NewString()[:8]}),
		userID,
		[]string{"platform:admin"},
	)

	deleteStatus, deleteBody, writerStatus, writerBody := runUserMutationWriterBeforeDelete(
		t,
		srv,
		userID,
		func() { srv.CreateSystem(createContext) },
		createResponse,
	)
	if writerStatus != http.StatusCreated || deleteStatus != http.StatusConflict {
		t.Fatalf(
			"guarded system-create/delete statuses = %d/%d, want %d/%d; create body=%s delete body=%s",
			writerStatus,
			deleteStatus,
			http.StatusCreated,
			http.StatusConflict,
			writerBody,
			deleteBody,
		)
	}
	assertErrorCode(t, []byte(deleteBody), "LAST_OWNER_CANNOT_BE_REMOVED")
	if _, err := client.User.Get(t.Context(), userID); err != nil {
		t.Fatalf("system creator should remain after last-owner conflict: %v", err)
	}
	ownerCount, err := client.ResourceRoleBinding.Query().
		Where(
			resourcerolebinding.UserIDEQ(userID),
			resourcerolebinding.ResourceTypeEQ("system"),
			resourcerolebinding.RoleEQ(resourcerolebinding.RoleOwner),
		).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count writer-first system owner bindings: %v", err)
	}
	if ownerCount != 1 {
		t.Fatalf("writer-first owner binding count = %d, want 1", ownerCount)
	}
}

func TestDeleteUser_WaitsForCommittedForeignKeyWriterThenCleansIt(t *testing.T) {
	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "user_guard_fk_writer_first")
	userID := seedUserMutationGuardTarget(t, client, "fk-writer-first")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	writerTx, err := srv.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin foreign-key writer transaction: %v", err)
	}
	writerOpen := true
	t.Cleanup(func() {
		if writerOpen {
			_ = writerTx.Rollback(context.Background())
		}
	})
	preferenceID := "preference-" + uuid.NewString()
	if _, execErr := writerTx.Exec(ctx, `
INSERT INTO user_preferences (id, created_at, updated_at, key, value, user_id)
VALUES ($1, NOW(), NOW(), 'theme', '{"mode":"dark"}'::jsonb, $2)
`, preferenceID, userID); execErr != nil {
		t.Fatalf("insert foreign-key row before user delete: %v", execErr)
	}
	var blockerPID int32
	if scanErr := writerTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); scanErr != nil {
		t.Fatalf("query foreign-key writer PID: %v", scanErr)
	}

	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	if commitErr := writerTx.Commit(ctx); commitErr != nil {
		t.Fatalf("commit foreign-key writer before user delete: %v", commitErr)
	}
	writerOpen = false
	waitForHandlerCompletion(t, deleteDone, "delete user after foreign-key writer")

	if deleteContext.Writer.Status() != http.StatusNoContent {
		t.Fatalf(
			"delete status = %d, want %d body=%s",
			deleteContext.Writer.Status(),
			http.StatusNoContent,
			deleteResponse.Body.String(),
		)
	}
	if _, err := client.UserPreference.Get(t.Context(), preferenceID); !ent.IsNotFound(err) {
		t.Fatalf("foreign-key row committed before delete lookup error = %v, want not found", err)
	}
	if _, err := client.User.Get(t.Context(), userID); !ent.IsNotFound(err) {
		t.Fatalf("deleted user lookup error = %v, want not found", err)
	}
}

func runUserMutationWriterBeforeDelete(
	t *testing.T,
	srv *Server,
	userID string,
	runWriter func(),
	writerResponse *httptest.ResponseRecorder,
) (deleteStatus int, deleteBody string, writerStatus int, writerBody string) {
	t.Helper()
	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/users/"+userID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)

	release, blockerPID := holdUserMutationGuard(t, srv.pool, userID)
	writerDone := runHandlerAsync(runWriter)
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	deleteDone := runHandlerAsync(func() { srv.DeleteUser(deleteContext, userID) })
	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 2)
	release()
	waitForHandlerCompletion(t, writerDone, "user-associated writer")
	waitForHandlerCompletion(t, deleteDone, "delete user after writer")

	return deleteContext.Writer.Status(), deleteResponse.Body.String(), writerResponse.Code, writerResponse.Body.String()
}

func seedUserMutationGuardTarget(t *testing.T, client *ent.Client, suffix string) string {
	t.Helper()
	userID := "guard-user-" + uuid.NewString()
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername("guard." + suffix + "." + uuid.NewString()).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed user mutation target: %v", err)
	}
	return userID
}

func holdUserMutationGuard(t *testing.T, pool *pgxpool.Pool, userID string) (release func(), blockerPID int32) {
	t.Helper()
	return holdSchemaAdvisoryGuard(t, pool, userMutationAdvisoryLockKey(userID))
}

func holdSystemMembershipGuard(t *testing.T, pool *pgxpool.Pool, systemID string) (release func(), blockerPID int32) {
	t.Helper()
	return holdSchemaAdvisoryGuard(t, pool, systemMembershipAdvisoryLockKey(systemID))
}

func holdSchemaAdvisoryGuard(t *testing.T, pool *pgxpool.Pool, key string) (release func(), blockerPID int32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire user mutation guard connection: %v", err)
	}
	if _, err := conn.Exec(
		ctx,
		`SELECT pg_advisory_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		key,
	); err != nil {
		conn.Release()
		t.Fatalf("hold schema advisory guard %q: %v", key, err)
	}
	if err := conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		conn.Release()
		t.Fatalf("query user mutation blocker PID: %v", err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cleanupCancel()
			var unlocked bool
			if err := conn.QueryRow(
				cleanupCtx,
				`SELECT pg_advisory_unlock(hashtextextended($1 || ':' || current_schema(), 0))`,
				key,
			).Scan(&unlocked); err != nil || !unlocked {
				_ = conn.Conn().Close(cleanupCtx)
				conn.Release()
				t.Fatalf("release schema advisory guard %q: unlocked=%t err=%v", key, unlocked, err)
			}
			conn.Release()
		})
	}
	t.Cleanup(release)
	return release, blockerPID
}

func runHandlerAsync(run func()) <-chan struct{} {
	done := make(chan struct{})
	var group errgroup.Group
	group.Go(func() error {
		defer close(done)
		run()
		return nil
	})
	return done
}

func waitForHandlerCompletion(t *testing.T, done <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s did not finish before timeout", operation)
	}
}

func assertUserAndOrphanRowsAbsent(t *testing.T, client *ent.Client, userID string) {
	t.Helper()
	if _, err := client.User.Get(t.Context(), userID); !ent.IsNotFound(err) {
		t.Fatalf("deleted user lookup error = %v, want ent not found", err)
	}
	resourceBindingCount, err := client.ResourceRoleBinding.Query().
		Where(resourcerolebinding.UserIDEQ(userID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count resource-role binding orphans: %v", err)
	}
	exemptionCount, err := client.RateLimitExemption.Query().
		Where(ratelimitexemption.IDEQ(userID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count rate-limit exemption orphans: %v", err)
	}
	overrideCount, err := client.RateLimitUserOverride.Query().
		Where(ratelimituseroverride.IDEQ(userID)).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count rate-limit override orphans: %v", err)
	}
	if resourceBindingCount != 0 || exemptionCount != 0 || overrideCount != 0 {
		t.Fatalf(
			"orphan rows = resource bindings:%d exemptions:%d overrides:%d, want 0/0/0",
			resourceBindingCount,
			exemptionCount,
			overrideCount,
		)
	}
}
