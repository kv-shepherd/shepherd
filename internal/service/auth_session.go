package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/authprovider"
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

var (
	ErrAuthSessionStoreUnavailable = errors.New("auth session store is unavailable")
	ErrAuthSessionTokenIDMissing   = errors.New("auth session token id is required")
	ErrAuthSessionUserIDMissing    = errors.New("auth session user id is required")
	ErrAuthSessionVersionChanged   = errors.New("auth session version changed")
)

const defaultJWTSessionVersion int64 = 1

const (
	createAuthSessionSubjectsTableSQL = `
CREATE TABLE IF NOT EXISTS auth_session_subjects (
	user_id TEXT PRIMARY KEY,
	session_version BIGINT NOT NULL DEFAULT 1 CHECK (session_version >= 1),
	last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	ensureAuthSessionSubjectsLastActivityColumnSQL = `
ALTER TABLE auth_session_subjects
ADD COLUMN IF NOT EXISTS last_activity_at TIMESTAMPTZ;`
	backfillAuthSessionSubjectsLastActivitySQL = `
UPDATE auth_session_subjects
SET last_activity_at = COALESCE(last_activity_at, updated_at, NOW())
WHERE last_activity_at IS NULL;`
	setAuthSessionSubjectsLastActivityDefaultSQL = `
ALTER TABLE auth_session_subjects
ALTER COLUMN last_activity_at SET DEFAULT NOW();`
	setAuthSessionSubjectsLastActivityNotNullSQL = `
ALTER TABLE auth_session_subjects
ALTER COLUMN last_activity_at SET NOT NULL;`
	createAuthRevokedTokensTableSQL = `
CREATE TABLE IF NOT EXISTS auth_revoked_tokens (
	token_id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	revoked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	reason TEXT NOT NULL DEFAULT ''
);`
	//nolint:gosec // Static SQL DDL string; no credentials or secrets embedded.
	createAuthRevokedTokensExpiryIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_auth_revoked_tokens_expires_at
ON auth_revoked_tokens (expires_at);`
	//nolint:gosec // Static SQL DDL string; no credentials or secrets embedded.
	createAuthRevokedTokensUserIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_auth_revoked_tokens_user_id
ON auth_revoked_tokens (user_id);`
	activateAuthSessionSubjectSQL = `
INSERT INTO auth_session_subjects (user_id, session_version, last_activity_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (user_id) DO UPDATE
SET last_activity_at = NOW(),
	updated_at = NOW()
WHERE auth_session_subjects.session_version = EXCLUDED.session_version
RETURNING session_version;`
	//nolint:gosec // Static SQL DML string; no credentials or secrets embedded.
	revokeAuthTokenSQL = `
INSERT INTO auth_revoked_tokens (token_id, user_id, expires_at, revoked_at, reason)
VALUES ($1, $2, $3, NOW(), $4)
ON CONFLICT (token_id) DO UPDATE
SET user_id = EXCLUDED.user_id,
	expires_at = EXCLUDED.expires_at,
	reason = EXCLUDED.reason;`
	//nolint:gosec // Static SQL query string; no credentials or secrets embedded.
	isAuthTokenRevokedSQL = `
SELECT EXISTS (
	SELECT 1
	FROM auth_revoked_tokens
	WHERE token_id = $1
	  AND revoked_at IS NOT NULL
	  AND expires_at > NOW()
);`
	incrementAuthSessionVersionsSQL = `
INSERT INTO auth_session_subjects (user_id, session_version, last_activity_at, updated_at)
SELECT item.user_id, 2, NOW(), NOW()
FROM unnest($1::text[]) AS item(user_id)
WHERE item.user_id <> ''
ORDER BY item.user_id
ON CONFLICT (user_id) DO UPDATE
SET session_version = auth_session_subjects.session_version + 1,
	last_activity_at = NOW(),
	updated_at = NOW();`
	readAuthSessionStateSQL = `
SELECT session_version, last_activity_at
FROM auth_session_subjects
WHERE user_id = $1;`
	touchAuthSessionActivitySQL = `
UPDATE auth_session_subjects
SET last_activity_at = $2,
	updated_at = NOW()
WHERE user_id = $1
  AND (last_activity_at IS NULL OR last_activity_at < $3);`
	authSessionSchemaInitTimeout = 5 * time.Second
)

// AuthSessionManager provides JWT revocation and live subject validation.
type AuthSessionManager struct {
	pool        *pgxpool.Pool
	client      *ent.Client
	idleTimeout time.Duration
	now         func() time.Time
	initMu      sync.Mutex
	initialized bool
}

// AuthSessionTxExecutor is the minimal transaction surface needed to update
// auth-session state alongside an application transaction.
type AuthSessionTxExecutor interface {
	ExecContext(context.Context, string, ...any) error
}

// NewAuthSessionManager creates a PostgreSQL-backed auth session manager.
func NewAuthSessionManager(pool *pgxpool.Pool, client *ent.Client, idleTimeout time.Duration) *AuthSessionManager {
	if pool == nil || client == nil {
		return nil
	}
	return &AuthSessionManager{
		pool:        pool,
		client:      client,
		idleTimeout: idleTimeout,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// EnsureSchema prepares the auxiliary auth-session tables before callers open
// a transaction that will update them through RevokeUsersSessionsTx.
func (m *AuthSessionManager) EnsureSchema(ctx context.Context) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return m.ensureSchema(ctx)
}

// CurrentSessionVersion returns the current JWT session version for a user.
func (m *AuthSessionManager) CurrentSessionVersion(ctx context.Context, userID string) (int64, error) {
	if m == nil || m.pool == nil {
		return defaultJWTSessionVersion, ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrAuthSessionUserIDMissing
	}
	return m.readSessionVersion(ctx, userID)
}

// ActivateUserSession records authenticated activity only when the session
// version still matches the authorization snapshot used to sign the token.
// Unlike CurrentSessionVersion, this operation is intentionally stateful and
// must only be called after credentials and token signing have succeeded.
func (m *AuthSessionManager) ActivateUserSession(
	ctx context.Context,
	userID string,
	expectedVersion int64,
) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAuthSessionUserIDMissing
	}
	if expectedVersion < defaultJWTSessionVersion {
		return fmt.Errorf("expected auth session version must be at least %d", defaultJWTSessionVersion)
	}
	if err := m.ensureSchema(ctx); err != nil {
		return err
	}

	var activatedVersion int64
	err := m.pool.QueryRow(ctx, activateAuthSessionSubjectSQL, userID, expectedVersion).Scan(&activatedVersion)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrAuthSessionVersionChanged
	case err != nil:
		return fmt.Errorf("activate auth session subject: %w", err)
	case activatedVersion != expectedVersion:
		return ErrAuthSessionVersionChanged
	default:
		return nil
	}
}

// RevokeToken adds a token identifier to the revocation list.
func (m *AuthSessionManager) RevokeToken(
	ctx context.Context,
	tokenID string,
	userID string,
	expiresAt time.Time,
	reason string,
) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return ErrAuthSessionTokenIDMissing
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAuthSessionUserIDMissing
	}
	if expiresAt.IsZero() {
		expiresAt = m.now().Add(time.Hour)
	}
	if err := m.ensureSchema(ctx); err != nil {
		return err
	}
	if _, err := m.pool.Exec(ctx, revokeAuthTokenSQL, tokenID, userID, expiresAt.UTC(), strings.TrimSpace(reason)); err != nil {
		return fmt.Errorf("revoke auth token: %w", err)
	}
	return nil
}

// RevokeUserSessions invalidates all current and future JWTs issued before the new version for a user.
func (m *AuthSessionManager) RevokeUserSessions(ctx context.Context, userID, reason string) error {
	return m.RevokeUsersSessions(ctx, []string{userID}, reason)
}

// RevokeUserSessionsTx invalidates a user's sessions through the caller's
// transaction. This keeps the version bump atomic with user deletion and safe
// when a serializable transaction is retried.
func (m *AuthSessionManager) RevokeUserSessionsTx(
	ctx context.Context,
	exec AuthSessionTxExecutor,
	userID string,
	reason string,
) error {
	return m.RevokeUsersSessionsTx(ctx, exec, []string{userID}, reason)
}

// RevokeUsersSessionsTx invalidates sessions for the given users through the
// caller's transaction. Callers must initialize the auxiliary schema before
// opening that transaction so schema setup never needs a second connection.
func (m *AuthSessionManager) RevokeUsersSessionsTx(
	ctx context.Context,
	exec AuthSessionTxExecutor,
	userIDs []string,
	reason string,
) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if exec == nil {
		return fmt.Errorf("auth session transaction executor is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.initMu.Lock()
	initialized := m.initialized
	m.initMu.Unlock()
	if !initialized {
		return fmt.Errorf("auth session schema is not initialized")
	}

	normalized := normalizeAuthSessionUserIDs(userIDs)
	if len(normalized) == 0 {
		return nil
	}
	_ = reason
	if err := exec.ExecContext(ctx, incrementAuthSessionVersionsSQL, normalized); err != nil {
		return fmt.Errorf("increment auth session versions in transaction: %w", err)
	}
	return nil
}

// RevokeUsersSessions invalidates all current and future JWTs issued before the new version for the given users.
func (m *AuthSessionManager) RevokeUsersSessions(ctx context.Context, userIDs []string, reason string) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.ensureSchema(ctx); err != nil {
		return err
	}

	normalized := normalizeAuthSessionUserIDs(userIDs)
	if len(normalized) == 0 {
		return nil
	}
	_ = reason
	if _, err := m.pool.Exec(ctx, incrementAuthSessionVersionsSQL, normalized); err != nil {
		return fmt.Errorf("increment auth session versions: %w", err)
	}
	return nil
}

// IsRevoked checks whether a token identifier has been revoked.
func (m *AuthSessionManager) IsRevoked(ctx context.Context, tokenID string) (bool, error) {
	if m == nil || m.pool == nil {
		return false, ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return false, ErrAuthSessionTokenIDMissing
	}
	if err := m.ensureSchema(ctx); err != nil {
		return false, err
	}

	var revoked bool
	if err := m.pool.QueryRow(ctx, isAuthTokenRevokedSQL, tokenID).Scan(&revoked); err != nil {
		return false, fmt.Errorf("query auth token revocation: %w", err)
	}
	return revoked, nil
}

// ValidateClaims rejects tokens for deleted/disabled users or stale session versions.
func (m *AuthSessionManager) ValidateClaims(ctx context.Context, claims *middleware.JWTClaims) error {
	if m == nil || m.client == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if claims == nil {
		return fmt.Errorf("jwt claims are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	userID := strings.TrimSpace(claims.UserID)
	if userID == "" {
		userID = strings.TrimSpace(claims.Subject)
	}
	if userID == "" {
		return middleware.ErrJWTSubjectNotFound
	}

	userRow, err := m.client.User.Query().
		Where(entuser.IDEQ(userID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return middleware.ErrJWTSubjectNotFound
		}
		return fmt.Errorf("query jwt subject: %w", err)
	}
	if !userRow.Enabled {
		return middleware.ErrJWTSubjectDisabled
	}
	if authProviderID := strings.TrimSpace(userRow.AuthProviderID); authProviderID != "" {
		providerRow, providerErr := m.client.AuthProvider.Query().
			Where(authprovider.IDEQ(authProviderID)).
			Only(ctx)
		if providerErr != nil {
			if ent.IsNotFound(providerErr) {
				return middleware.ErrJWTSubjectDisabled
			}
			return fmt.Errorf("query jwt subject auth provider: %w", providerErr)
		}
		if !providerRow.Enabled {
			return middleware.ErrJWTSubjectDisabled
		}
	}

	version, err := m.readSessionVersion(ctx, userID)
	if err != nil {
		return err
	}
	if normalizeJWTSessionVersion(claims.SessionVersion) != version {
		return middleware.ErrJWTSessionStale
	}
	if m.idleTimeout > 0 {
		lastActivityAt, readErr := m.readLastActivityAt(ctx, userID)
		if readErr != nil {
			return readErr
		}
		now := m.now()
		if !lastActivityAt.IsZero() && now.Sub(lastActivityAt) > m.idleTimeout {
			return middleware.ErrJWTSessionStale
		}
		if touchErr := m.touchLastActivityAt(ctx, userID, lastActivityAt, now); touchErr != nil {
			return touchErr
		}
	}
	return nil
}

func (m *AuthSessionManager) readSessionVersion(ctx context.Context, userID string) (int64, error) {
	if m == nil || m.pool == nil {
		return defaultJWTSessionVersion, ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrAuthSessionUserIDMissing
	}
	if err := m.ensureSchema(ctx); err != nil {
		return 0, err
	}

	var (
		version        int64
		lastActivityAt time.Time
	)
	err := m.pool.QueryRow(ctx, readAuthSessionStateSQL, userID).Scan(&version, &lastActivityAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return defaultJWTSessionVersion, nil
	case err != nil:
		return 0, fmt.Errorf("read auth session version: %w", err)
	case version < defaultJWTSessionVersion:
		return defaultJWTSessionVersion, nil
	default:
		return version, nil
	}
}

func (m *AuthSessionManager) readLastActivityAt(ctx context.Context, userID string) (time.Time, error) {
	if m == nil || m.pool == nil {
		return time.Time{}, ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return time.Time{}, ErrAuthSessionUserIDMissing
	}
	if err := m.ensureSchema(ctx); err != nil {
		return time.Time{}, err
	}

	var (
		version        int64
		lastActivityAt time.Time
	)
	err := m.pool.QueryRow(ctx, readAuthSessionStateSQL, userID).Scan(&version, &lastActivityAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return time.Time{}, nil
	case err != nil:
		return time.Time{}, fmt.Errorf("read auth session last activity: %w", err)
	default:
		return lastActivityAt.UTC(), nil
	}
}

func (m *AuthSessionManager) touchLastActivityAt(ctx context.Context, userID string, previous, now time.Time) error {
	if m == nil || m.pool == nil || m.idleTimeout <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ErrAuthSessionUserIDMissing
	}

	writeInterval := m.idleTimeout / 4
	if writeInterval <= 0 || writeInterval > time.Minute {
		writeInterval = time.Minute
	}
	if !previous.IsZero() && now.Sub(previous) < writeInterval {
		return nil
	}
	threshold := now.Add(-writeInterval).UTC()
	if _, err := m.pool.Exec(ctx, touchAuthSessionActivitySQL, userID, now.UTC(), threshold); err != nil {
		return fmt.Errorf("touch auth session activity: %w", err)
	}
	return nil
}

func (m *AuthSessionManager) ensureSchema(ctx context.Context) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.initMu.Lock()
	defer m.initMu.Unlock()
	if m.initialized {
		return nil
	}

	schemaCtx, cancel := context.WithTimeout(ctx, authSessionSchemaInitTimeout)
	defer cancel()

	for _, statement := range []string{
		createAuthSessionSubjectsTableSQL,
		ensureAuthSessionSubjectsLastActivityColumnSQL,
		backfillAuthSessionSubjectsLastActivitySQL,
		setAuthSessionSubjectsLastActivityDefaultSQL,
		setAuthSessionSubjectsLastActivityNotNullSQL,
		createAuthRevokedTokensTableSQL,
		createAuthRevokedTokensExpiryIndexSQL,
		createAuthRevokedTokensUserIndexSQL,
	} {
		if _, err := m.pool.Exec(schemaCtx, statement); err != nil {
			return fmt.Errorf("initialize auth session schema: %w", err)
		}
	}
	m.initialized = true
	return nil
}

func normalizeAuthSessionUserIDs(userIDs []string) []string {
	normalized := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			continue
		}
		normalized = append(normalized, userID)
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func normalizeJWTSessionVersion(version int64) int64 {
	if version < defaultJWTSessionVersion {
		return defaultJWTSessionVersion
	}
	return version
}
