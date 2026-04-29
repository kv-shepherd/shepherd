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
	entuser "kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

var (
	ErrAuthSessionStoreUnavailable = errors.New("auth session store is unavailable")
	ErrAuthSessionTokenIDMissing   = errors.New("auth session token id is required")
	ErrAuthSessionUserIDMissing    = errors.New("auth session user id is required")
)

const defaultJWTSessionVersion int64 = 1

const (
	createAuthSessionSubjectsTableSQL = `
CREATE TABLE IF NOT EXISTS auth_session_subjects (
	user_id TEXT PRIMARY KEY,
	session_version BIGINT NOT NULL DEFAULT 1 CHECK (session_version >= 1),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
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
	ensureAuthSessionSubjectSQL = `
INSERT INTO auth_session_subjects (user_id, session_version, updated_at)
VALUES ($1, 1, NOW())
ON CONFLICT (user_id) DO UPDATE
SET user_id = EXCLUDED.user_id
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
INSERT INTO auth_session_subjects (user_id, session_version, updated_at)
SELECT DISTINCT item.user_id, 2, NOW()
FROM unnest($1::text[]) AS item(user_id)
WHERE item.user_id <> ''
ON CONFLICT (user_id) DO UPDATE
SET session_version = auth_session_subjects.session_version + 1,
	updated_at = NOW();`
	readAuthSessionVersionSQL = `
SELECT session_version
FROM auth_session_subjects
WHERE user_id = $1;`
)

// AuthSessionManager provides JWT revocation and live subject validation.
type AuthSessionManager struct {
	pool     *pgxpool.Pool
	client   *ent.Client
	now      func() time.Time
	initOnce sync.Once
	initErr  error
}

// NewAuthSessionManager creates a PostgreSQL-backed auth session manager.
func NewAuthSessionManager(pool *pgxpool.Pool, client *ent.Client) *AuthSessionManager {
	if pool == nil || client == nil {
		return nil
	}
	return &AuthSessionManager{
		pool:   pool,
		client: client,
		now:    func() time.Time { return time.Now().UTC() },
	}
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
	if err := m.ensureSchema(); err != nil {
		return 0, err
	}

	var version int64
	if err := m.pool.QueryRow(ctx, ensureAuthSessionSubjectSQL, userID).Scan(&version); err != nil {
		return 0, fmt.Errorf("ensure auth session subject: %w", err)
	}
	if version < defaultJWTSessionVersion {
		return defaultJWTSessionVersion, nil
	}
	return version, nil
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
	if err := m.ensureSchema(); err != nil {
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

// RevokeUsersSessions invalidates all current and future JWTs issued before the new version for the given users.
func (m *AuthSessionManager) RevokeUsersSessions(ctx context.Context, userIDs []string, reason string) error {
	if m == nil || m.pool == nil {
		return ErrAuthSessionStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.ensureSchema(); err != nil {
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
	if err := m.ensureSchema(); err != nil {
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

	version, err := m.readSessionVersion(ctx, userID)
	if err != nil {
		return err
	}
	if normalizeJWTSessionVersion(claims.SessionVersion) != version {
		return middleware.ErrJWTSessionStale
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
	if err := m.ensureSchema(); err != nil {
		return 0, err
	}

	var version int64
	err := m.pool.QueryRow(ctx, readAuthSessionVersionSQL, userID).Scan(&version)
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

func (m *AuthSessionManager) ensureSchema() error {
	m.initOnce.Do(func() {
		if m == nil || m.pool == nil {
			m.initErr = ErrAuthSessionStoreUnavailable
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		for _, statement := range []string{
			createAuthSessionSubjectsTableSQL,
			createAuthRevokedTokensTableSQL,
			createAuthRevokedTokensExpiryIndexSQL,
			createAuthRevokedTokensUserIndexSQL,
		} {
			if _, err := m.pool.Exec(ctx, statement); err != nil {
				m.initErr = fmt.Errorf("initialize auth session schema: %w", err)
				return
			}
		}
	})
	return m.initErr
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
