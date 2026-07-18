package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/pkg/worker"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type authSessionPGXTxExecutor struct {
	tx pgx.Tx
}

func (e authSessionPGXTxExecutor) ExecContext(ctx context.Context, query string, args ...any) error {
	_, err := e.tx.Exec(ctx, query, args...)
	return err
}

func newAuthSessionTestDependencies(t *testing.T, prefix string) (*ent.Client, *AuthSessionManager) {
	t.Helper()

	pool := testutil.OpenPGXPool(t, prefix)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })

	manager := NewAuthSessionManager(pool, client, 0)
	if manager == nil {
		t.Fatal("NewAuthSessionManager() returned nil")
	}
	return client, manager
}

func TestAuthSessionManagerValidateClaimsRejectsStaleSessionVersion(t *testing.T) {
	t.Parallel()

	client, manager := newAuthSessionTestDependencies(t, "auth_session_validate")
	userRow, err := client.User.Create().
		SetID("user-session-version").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	claims := &middleware.JWTClaims{
		UserID:         userRow.ID,
		Username:       userRow.Username,
		SessionVersion: 1,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userRow.ID,
		},
	}
	if validateErr := manager.ValidateClaims(t.Context(), claims); validateErr != nil {
		t.Fatalf("ValidateClaims() initial error = %v", validateErr)
	}

	if revokeErr := manager.RevokeUserSessions(t.Context(), userRow.ID, "test"); revokeErr != nil {
		t.Fatalf("RevokeUserSessions() error = %v", revokeErr)
	}
	if validateErr := manager.ValidateClaims(t.Context(), claims); !errors.Is(validateErr, middleware.ErrJWTSessionStale) {
		t.Fatalf("ValidateClaims() error = %v, want %v", validateErr, middleware.ErrJWTSessionStale)
	}

	version, err := manager.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("CurrentSessionVersion() error = %v", err)
	}
	claims.SessionVersion = version
	if validateErr := manager.ValidateClaims(t.Context(), claims); validateErr != nil {
		t.Fatalf("ValidateClaims() refreshed error = %v", validateErr)
	}
}

func TestAuthSessionManagerValidateClaimsRejectsDisabledAuthProvider(t *testing.T) {
	t.Parallel()

	client, manager := newAuthSessionTestDependencies(t, "auth_session_disabled_provider")
	providerRow, err := client.AuthProvider.Create().
		SetID("provider-session-disabled").
		SetName("Disabled Session Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer": "https://sso.example.com"}).
		SetEnabled(false).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed auth provider: %v", err)
	}
	userRow, err := client.User.Create().
		SetID("user-session-disabled-provider").
		SetUsername("disabled.provider.user").
		SetEnabled(true).
		SetAuthProviderID(providerRow.ID).
		SetExternalID("external-disabled-provider-user").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	version, err := manager.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("CurrentSessionVersion() error = %v", err)
	}
	claims := &middleware.JWTClaims{
		UserID:         userRow.ID,
		Username:       userRow.Username,
		SessionVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userRow.ID,
		},
	}

	if validateErr := manager.ValidateClaims(t.Context(), claims); !errors.Is(validateErr, middleware.ErrJWTSubjectDisabled) {
		t.Fatalf("ValidateClaims() error = %v, want %v", validateErr, middleware.ErrJWTSubjectDisabled)
	}
	if err := client.AuthProvider.UpdateOneID(providerRow.ID).SetEnabled(true).Exec(t.Context()); err != nil {
		t.Fatalf("enable auth provider: %v", err)
	}
	if validateErr := manager.ValidateClaims(t.Context(), claims); validateErr != nil {
		t.Fatalf("ValidateClaims() after provider enable error = %v", validateErr)
	}
}

func TestAuthSessionManagerRevokeTokenMarksTokenRevoked(t *testing.T) {
	t.Parallel()

	client, manager := newAuthSessionTestDependencies(t, "auth_session_revoke")
	if _, err := client.User.Create().
		SetID("user-revoke-token").
		SetUsername("bob").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := manager.RevokeToken(context.Background(), "token-123", "user-revoke-token", expiresAt, "logout"); err != nil {
		t.Fatalf("RevokeToken() error = %v", err)
	}

	revoked, err := manager.IsRevoked(t.Context(), "token-123")
	if err != nil {
		t.Fatalf("IsRevoked() error = %v", err)
	}
	if !revoked {
		t.Fatal("IsRevoked() = false, want true")
	}
}

func TestAuthSessionManagerRevokeUsersSessionsTxUsesCallerTransaction(t *testing.T) {
	t.Parallel()

	client, manager := newAuthSessionTestDependencies(t, "auth_session_revoke_users_tx")
	ctx := t.Context()
	if err := manager.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	const (
		firstUserID  = "auth-session-tx-first"
		secondUserID = "auth-session-tx-second"
	)
	for _, userID := range []string{firstUserID, secondUserID} {
		version, err := manager.CurrentSessionVersion(ctx, userID)
		if err != nil {
			t.Fatalf("CurrentSessionVersion(%q) error = %v", userID, err)
		}
		if version != defaultJWTSessionVersion {
			t.Fatalf("CurrentSessionVersion(%q) = %d, want %d", userID, version, defaultJWTSessionVersion)
		}
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin commit transaction: %v", err)
	}
	if revokeErr := manager.RevokeUsersSessionsTx(
		ctx,
		tx,
		[]string{" " + secondUserID + " ", firstUserID, secondUserID, ""},
		"test_batch",
	); revokeErr != nil {
		_ = tx.Rollback()
		t.Fatalf("RevokeUsersSessionsTx() error = %v", revokeErr)
	}
	if commitErr := tx.Commit(); commitErr != nil {
		t.Fatalf("commit session revocation transaction: %v", commitErr)
	}

	for _, userID := range []string{firstUserID, secondUserID} {
		version, versionErr := manager.CurrentSessionVersion(ctx, userID)
		if versionErr != nil {
			t.Fatalf("CurrentSessionVersion(%q) after commit error = %v", userID, versionErr)
		}
		if version != defaultJWTSessionVersion+1 {
			t.Fatalf("CurrentSessionVersion(%q) after commit = %d, want %d", userID, version, defaultJWTSessionVersion+1)
		}
	}

	rollbackTx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin rollback transaction: %v", err)
	}
	if revokeErr := manager.RevokeUserSessionsTx(ctx, rollbackTx, firstUserID, "test_rollback"); revokeErr != nil {
		_ = rollbackTx.Rollback()
		t.Fatalf("RevokeUserSessionsTx() error = %v", revokeErr)
	}
	if rollbackErr := rollbackTx.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback session revocation transaction: %v", rollbackErr)
	}

	version, err := manager.CurrentSessionVersion(ctx, firstUserID)
	if err != nil {
		t.Fatalf("CurrentSessionVersion() after rollback error = %v", err)
	}
	if version != defaultJWTSessionVersion+1 {
		t.Fatalf("CurrentSessionVersion() after rollback = %d, want %d", version, defaultJWTSessionVersion+1)
	}
}

func TestAuthSessionManagerRevokeUsersSessionsTxUsesStableOverlappingLockOrder(t *testing.T) {
	_, manager := newAuthSessionTestDependencies(t, "auth_session_revoke_users_lock_order")
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if err := manager.EnsureSchema(ctx); err != nil {
		t.Fatalf("EnsureSchema() error = %v", err)
	}

	const (
		firstUserID  = "auth-session-lock-first"
		secondUserID = "auth-session-lock-second"
	)
	for _, userID := range []string{firstUserID, secondUserID} {
		if err := manager.ActivateUserSession(ctx, userID, defaultJWTSessionVersion); err != nil {
			t.Fatalf("seed ActivateUserSession(%q): %v", userID, err)
		}
	}

	blockerTx, err := manager.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin blocker transaction: %v", err)
	}
	blockerOpen := true
	t.Cleanup(func() {
		if blockerOpen {
			_ = blockerTx.Rollback(context.Background())
		}
	})
	var blockerPID int32
	if queryPIDErr := blockerTx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); queryPIDErr != nil {
		t.Fatalf("query blocker backend pid: %v", queryPIDErr)
	}
	if _, lockSubjectErr := blockerTx.Exec(
		ctx,
		`SELECT user_id FROM auth_session_subjects WHERE user_id = $1 FOR UPDATE`,
		firstUserID,
	); lockSubjectErr != nil {
		t.Fatalf("lock first session subject: %v", lockSubjectErr)
	}

	firstTx, err := manager.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin first revocation transaction: %v", err)
	}
	firstOpen := true
	t.Cleanup(func() {
		if firstOpen {
			_ = firstTx.Rollback(context.Background())
		}
	})
	secondTx, err := manager.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second revocation transaction: %v", err)
	}
	secondOpen := true
	t.Cleanup(func() {
		if secondOpen {
			_ = secondTx.Rollback(context.Background())
		}
	})

	backendPID := func(tx pgx.Tx) int32 {
		t.Helper()
		var pid int32
		if queryPIDErr := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); queryPIDErr != nil {
			t.Fatalf("query revocation backend pid: %v", queryPIDErr)
		}
		return pid
	}
	firstPID := backendPID(firstTx)
	secondPID := backendPID(secondTx)

	pools, err := worker.NewPools(ctx, worker.PoolConfig{GeneralPoolSize: 2, K8sPoolSize: 1})
	if err != nil {
		t.Fatalf("create session revocation worker pool: %v", err)
	}
	t.Cleanup(pools.Shutdown)
	start := make(chan struct{})
	results := make(chan error, 2)
	revoke := func(taskCtx context.Context, tx pgx.Tx, userIDs []string) {
		select {
		case <-start:
		case <-taskCtx.Done():
			results <- taskCtx.Err()
			return
		}
		if revokeErr := manager.RevokeUsersSessionsTx(
			taskCtx,
			authSessionPGXTxExecutor{tx: tx},
			userIDs,
			"overlapping_lock_order",
		); revokeErr != nil {
			results <- revokeErr
			return
		}
		results <- tx.Commit(taskCtx)
	}
	if submitFirstErr := pools.General.Submit(ctx, func(taskCtx context.Context) {
		revoke(taskCtx, firstTx, []string{secondUserID, firstUserID})
	}); submitFirstErr != nil {
		t.Fatalf("submit first session revocation: %v", submitFirstErr)
	}
	if submitSecondErr := pools.General.Submit(ctx, func(taskCtx context.Context) {
		revoke(taskCtx, secondTx, []string{firstUserID, secondUserID})
	}); submitSecondErr != nil {
		t.Fatalf("submit second session revocation: %v", submitSecondErr)
	}
	close(start)

	waitForBlocker := func(pid int32) bool {
		t.Helper()
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			var blocked, directlyBlocked bool
			if queryBlockersErr := manager.pool.QueryRow(
				ctx,
				`SELECT cardinality(pg_blocking_pids($1)) > 0,
				        $2 = ANY(pg_blocking_pids($1))`,
				pid,
				blockerPID,
			).Scan(&blocked, &directlyBlocked); queryBlockersErr != nil {
				t.Fatalf("query blockers for backend %d: %v", pid, queryBlockersErr)
			}
			if blocked {
				return directlyBlocked
			}
			select {
			case result := <-results:
				t.Fatalf("revocation completed before blocking on the sorted first row: %v", result)
			case <-deadline.C:
				t.Fatalf("backend %d did not block while first session subject was locked", pid)
			case <-ticker.C:
			}
		}
	}
	firstDirect := waitForBlocker(firstPID)
	secondDirect := waitForBlocker(secondPID)
	if !firstDirect && !secondDirect {
		t.Fatalf("neither revocation backend was directly blocked by first-row blocker %d", blockerPID)
	}

	probeTx, err := manager.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin second-row probe transaction: %v", err)
	}
	if _, err := probeTx.Exec(
		ctx,
		`SELECT user_id FROM auth_session_subjects WHERE user_id = $1 FOR UPDATE NOWAIT`,
		secondUserID,
	); err != nil {
		_ = probeTx.Rollback(context.Background())
		t.Fatalf("second session subject was locked before the sorted first subject: %v", err)
	}
	if err := probeTx.Rollback(ctx); err != nil {
		t.Fatalf("rollback second-row probe transaction: %v", err)
	}

	if err := blockerTx.Commit(ctx); err != nil {
		t.Fatalf("release first-row blocker: %v", err)
	}
	blockerOpen = false
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("overlapping session revocation: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("wait for overlapping session revocations: %v", ctx.Err())
		}
	}
	firstOpen = false
	secondOpen = false

	for _, userID := range []string{firstUserID, secondUserID} {
		version, err := manager.CurrentSessionVersion(ctx, userID)
		if err != nil {
			t.Fatalf("CurrentSessionVersion(%q): %v", userID, err)
		}
		if want := defaultJWTSessionVersion + 2; version != want {
			t.Fatalf("CurrentSessionVersion(%q) = %d, want %d", userID, version, want)
		}
	}
}

func TestAuthSessionManagerRetriesSchemaAfterCanceledInitialization(t *testing.T) {
	t.Parallel()

	_, manager := newAuthSessionTestDependencies(t, "auth_session_schema_retry")

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	err := manager.ensureSchema(canceledCtx)
	if err == nil {
		t.Fatal("ensureSchema(canceled) error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ensureSchema(canceled) error = %v, want %v", err, context.Canceled)
	}
	if manager.initialized {
		t.Fatal("manager initialized after canceled schema init, want retryable failure")
	}

	if _, err := manager.CurrentSessionVersion(t.Context(), "user-auth-session-schema-retry"); err != nil {
		t.Fatalf("CurrentSessionVersion() after schema retry error = %v", err)
	}
	if !manager.initialized {
		t.Fatal("manager initialized = false after successful retry")
	}
}

func TestAuthSessionManagerValidateClaimsRejectsIdleSession(t *testing.T) {
	t.Parallel()

	client, manager := newAuthSessionTestDependencies(t, "auth_session_idle")
	manager.idleTimeout = 10 * time.Minute
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }

	userRow, err := client.User.Create().
		SetID("user-idle-timeout").
		SetUsername("carol").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	version, err := manager.CurrentSessionVersion(t.Context(), userRow.ID)
	if err != nil {
		t.Fatalf("CurrentSessionVersion() error = %v", err)
	}
	if err := manager.ActivateUserSession(t.Context(), userRow.ID, version); err != nil {
		t.Fatalf("ActivateUserSession() error = %v", err)
	}

	claims := &middleware.JWTClaims{
		UserID:         userRow.ID,
		Username:       userRow.Username,
		SessionVersion: version,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userRow.ID,
		},
	}
	if validateErr := manager.ValidateClaims(t.Context(), claims); validateErr != nil {
		t.Fatalf("ValidateClaims() initial error = %v", validateErr)
	}

	manager.now = func() time.Time { return now.Add(11 * time.Minute) }
	if validateErr := manager.ValidateClaims(t.Context(), claims); !errors.Is(validateErr, middleware.ErrJWTSessionStale) {
		t.Fatalf("ValidateClaims() error = %v, want %v", validateErr, middleware.ErrJWTSessionStale)
	}
}

func TestAuthSessionManagerCurrentVersionDoesNotRefreshActivity(t *testing.T) {
	t.Parallel()

	_, manager := newAuthSessionTestDependencies(t, "auth_session_read_only_version")
	ctx := t.Context()
	const userID = "user-read-only-session-version"
	if err := manager.ActivateUserSession(ctx, userID, defaultJWTSessionVersion); err != nil {
		t.Fatalf("ActivateUserSession() error = %v", err)
	}

	wantActivity := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	if _, err := manager.pool.Exec(ctx, `
UPDATE auth_session_subjects
SET last_activity_at = $2, updated_at = $2
WHERE user_id = $1
`, userID, wantActivity); err != nil {
		t.Fatalf("seed auth session activity: %v", err)
	}
	if version, err := manager.CurrentSessionVersion(ctx, userID); err != nil {
		t.Fatalf("CurrentSessionVersion() error = %v", err)
	} else if version != defaultJWTSessionVersion {
		t.Fatalf("CurrentSessionVersion() = %d, want %d", version, defaultJWTSessionVersion)
	}

	var gotActivity time.Time
	if err := manager.pool.QueryRow(ctx, `
SELECT last_activity_at
FROM auth_session_subjects
WHERE user_id = $1
`, userID).Scan(&gotActivity); err != nil {
		t.Fatalf("read auth session activity: %v", err)
	}
	if !gotActivity.Equal(wantActivity) {
		t.Fatalf("last_activity_at = %s, want unchanged %s", gotActivity, wantActivity)
	}
}

func TestAuthSessionManagerActivationRejectsStaleVersionWithoutRefreshingActivity(t *testing.T) {
	t.Parallel()

	_, manager := newAuthSessionTestDependencies(t, "auth_session_stale_activation")
	ctx := t.Context()
	const userID = "user-stale-session-activation"
	if err := manager.RevokeUserSessions(ctx, userID, "seed newer version"); err != nil {
		t.Fatalf("RevokeUserSessions() error = %v", err)
	}

	wantActivity := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	if _, err := manager.pool.Exec(ctx, `
UPDATE auth_session_subjects
SET last_activity_at = $2, updated_at = $2
WHERE user_id = $1
`, userID, wantActivity); err != nil {
		t.Fatalf("seed auth session activity: %v", err)
	}
	if err := manager.ActivateUserSession(ctx, userID, defaultJWTSessionVersion); !errors.Is(err, ErrAuthSessionVersionChanged) {
		t.Fatalf("ActivateUserSession() error = %v, want %v", err, ErrAuthSessionVersionChanged)
	}

	var gotActivity time.Time
	if err := manager.pool.QueryRow(ctx, `
SELECT last_activity_at
FROM auth_session_subjects
WHERE user_id = $1
`, userID).Scan(&gotActivity); err != nil {
		t.Fatalf("read auth session activity: %v", err)
	}
	if !gotActivity.Equal(wantActivity) {
		t.Fatalf("last_activity_at = %s, want unchanged %s", gotActivity, wantActivity)
	}
}
