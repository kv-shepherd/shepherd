package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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
