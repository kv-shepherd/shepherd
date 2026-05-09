package handlers

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresLoginAttemptStore struct {
	pool    *pgxpool.Pool
	init    sync.Once
	initErr error
}

const (
	createLoginRateLimitBucketsTableSQL = `
CREATE TABLE IF NOT EXISTS login_rate_limit_buckets (
	bucket_key TEXT PRIMARY KEY,
	blocked_until TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	createLoginRateLimitFailuresTableSQL = `
CREATE TABLE IF NOT EXISTS login_rate_limit_failures (
	bucket_key TEXT NOT NULL REFERENCES login_rate_limit_buckets(bucket_key) ON DELETE CASCADE,
	failed_at TIMESTAMPTZ NOT NULL
);`
	createLoginRateLimitFailuresBucketIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_login_rate_limit_failures_bucket_time
ON login_rate_limit_failures (bucket_key, failed_at);`
	lockLoginRateLimitBucketSQL    = `SELECT pg_advisory_xact_lock(hashtext($1)::bigint)`
	pruneLoginRateLimitFailuresSQL = `
DELETE FROM login_rate_limit_failures
WHERE bucket_key = ANY($1)
  AND failed_at < $2;`
	clearExpiredLoginRateLimitBlocksSQL = `
UPDATE login_rate_limit_buckets
SET blocked_until = NULL,
	updated_at = NOW()
WHERE bucket_key = ANY($1)
  AND blocked_until IS NOT NULL
  AND blocked_until <= $2;`
	readLoginRateLimitMaxBlockSQL = `
SELECT COALESCE(MAX(blocked_until), 'epoch'::timestamptz)
FROM login_rate_limit_buckets
WHERE bucket_key = ANY($1)
  AND blocked_until > $2;`
	ensureLoginRateLimitBucketsSQL = `
INSERT INTO login_rate_limit_buckets (bucket_key, updated_at)
SELECT key, NOW()
FROM unnest($1::text[]) AS item(key)
ON CONFLICT (bucket_key) DO NOTHING;`
	insertLoginRateLimitFailuresSQL = `
INSERT INTO login_rate_limit_failures (bucket_key, failed_at)
SELECT key, $2
FROM unnest($1::text[]) AS item(key);`
	updateLoginRateLimitBlocksAfterFailureSQL = `
UPDATE login_rate_limit_buckets AS b
SET blocked_until = CASE
		WHEN counts.failure_count >= $4 THEN $3
		WHEN b.blocked_until IS NOT NULL AND b.blocked_until > $2 THEN b.blocked_until
		ELSE NULL
	END,
	updated_at = NOW()
FROM (
	SELECT bucket_key, COUNT(*) AS failure_count
	FROM login_rate_limit_failures
	WHERE bucket_key = ANY($1)
	GROUP BY bucket_key
) AS counts
WHERE b.bucket_key = counts.bucket_key;`
	deleteLoginRateLimitFailuresSQL = `
DELETE FROM login_rate_limit_failures
WHERE bucket_key = ANY($1);`
	deleteLoginRateLimitBucketsSQL = `
DELETE FROM login_rate_limit_buckets
WHERE bucket_key = ANY($1);`
)

func newPostgresLoginAttemptStore(pool *pgxpool.Pool) loginAttemptStore {
	if pool == nil {
		return nil
	}
	return &postgresLoginAttemptStore{pool: pool}
}

func (s *postgresLoginAttemptStore) Allow(ctx context.Context, keys []string, now time.Time, window time.Duration) (time.Duration, error) {
	if s == nil || s.pool == nil {
		return 0, nil
	}
	keys = normalizeLoginAttemptStoreKeys(keys)
	if len(keys) == 0 {
		return 0, nil
	}
	if err := s.ensureSchema(); err != nil {
		return 0, err
	}
	var blockedUntil time.Time
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockLoginAttemptStoreKeys(ctx, tx, keys); err != nil {
			return err
		}
		cutoff := now.Add(-window)
		if _, err := tx.Exec(ctx, pruneLoginRateLimitFailuresSQL, keys, cutoff); err != nil {
			return fmt.Errorf("prune login rate limit failures: %w", err)
		}
		if _, err := tx.Exec(ctx, clearExpiredLoginRateLimitBlocksSQL, keys, now); err != nil {
			return fmt.Errorf("clear login rate limit blocks: %w", err)
		}
		if err := tx.QueryRow(ctx, readLoginRateLimitMaxBlockSQL, keys, now).Scan(&blockedUntil); err != nil {
			return fmt.Errorf("read login rate limit block: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if blockedUntil.After(now) {
		return blockedUntil.Sub(now), nil
	}
	return 0, nil
}

func (s *postgresLoginAttemptStore) RecordFailure(ctx context.Context, keys []string, now time.Time, window, blockDuration time.Duration, maxFailures int) error {
	if s == nil || s.pool == nil {
		return nil
	}
	keys = normalizeLoginAttemptStoreKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	if err := s.ensureSchema(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockLoginAttemptStoreKeys(ctx, tx, keys); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, ensureLoginRateLimitBucketsSQL, keys); err != nil {
			return fmt.Errorf("ensure login rate limit buckets: %w", err)
		}
		cutoff := now.Add(-window)
		if _, err := tx.Exec(ctx, pruneLoginRateLimitFailuresSQL, keys, cutoff); err != nil {
			return fmt.Errorf("prune login rate limit failures: %w", err)
		}
		if _, err := tx.Exec(ctx, insertLoginRateLimitFailuresSQL, keys, now); err != nil {
			return fmt.Errorf("insert login rate limit failures: %w", err)
		}
		blockedUntil := now.Add(blockDuration)
		if _, err := tx.Exec(ctx, updateLoginRateLimitBlocksAfterFailureSQL, keys, now, blockedUntil, maxFailures); err != nil {
			return fmt.Errorf("update login rate limit block: %w", err)
		}
		return nil
	})
}

func (s *postgresLoginAttemptStore) RecordSuccess(ctx context.Context, keys []string) error {
	if s == nil || s.pool == nil {
		return nil
	}
	keys = normalizeLoginAttemptStoreKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	if err := s.ensureSchema(); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockLoginAttemptStoreKeys(ctx, tx, keys); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, deleteLoginRateLimitFailuresSQL, keys); err != nil {
			return fmt.Errorf("delete login rate limit failures: %w", err)
		}
		if _, err := tx.Exec(ctx, deleteLoginRateLimitBucketsSQL, keys); err != nil {
			return fmt.Errorf("delete login rate limit buckets: %w", err)
		}
		return nil
	})
}

func (s *postgresLoginAttemptStore) ensureSchema() error {
	if s == nil || s.pool == nil {
		return nil
	}
	s.init.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		s.initErr = ensureLoginAttemptStoreSchema(ctx, s.pool)
	})
	return s.initErr
}

func ensureLoginAttemptStoreSchema(ctx context.Context, exec pgxExecutor) error {
	for _, stmt := range []string{
		createLoginRateLimitBucketsTableSQL,
		createLoginRateLimitFailuresTableSQL,
		createLoginRateLimitFailuresBucketIndexSQL,
	} {
		if _, err := exec.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("ensure login rate limit schema: %w", err)
		}
	}
	return nil
}

type pgxExecutor interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
}

func lockLoginAttemptStoreKeys(ctx context.Context, tx pgx.Tx, keys []string) error {
	for _, key := range keys {
		if _, err := tx.Exec(ctx, lockLoginRateLimitBucketSQL, key); err != nil {
			return fmt.Errorf("lock login rate limit bucket: %w", err)
		}
	}
	return nil
}

func normalizeLoginAttemptStoreKeys(keys []string) []string {
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		key = normalizeLoginAttemptIdentity(key)
		if key != "" {
			normalized = append(normalized, key)
		}
	}
	sort.Strings(normalized)
	return normalized
}
