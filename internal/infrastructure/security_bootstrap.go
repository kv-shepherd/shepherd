package infrastructure

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent/systemsecret"
	"kv-shepherd.io/shepherd/internal/config"
)

const (
	bootstrapSessionSecretKey  = "SESSION_SECRET"
	bootstrapEncryptionKeyName = "ENCRYPTION_KEY"
	undefinedTablePostgresErr  = "42P01"
)

// ResolveBootstrapSecuritySecrets applies ADR-0025 precedence:
// explicit config/env values > persisted DB values > generated + persisted values.
func ResolveBootstrapSecuritySecrets(
	ctx context.Context,
	pool *pgxpool.Pool,
	security config.SecurityConfig,
) (config.SecurityConfig, error) {
	resolved := security
	if err := validateExplicitSecurityOverrides(resolved); err != nil {
		return config.SecurityConfig{}, err
	}

	if hasResolvedSecuritySecrets(resolved) {
		return resolved, nil
	}
	if pool == nil {
		return config.SecurityConfig{}, fmt.Errorf("bootstrap security secret resolution requires a database pool")
	}

	stored, err := loadBootstrapSecuritySecrets(ctx, pool)
	if err != nil {
		return config.SecurityConfig{}, err
	}

	if strings.TrimSpace(resolved.SessionSecret) == "" {
		resolved.SessionSecret = stored[bootstrapSessionSecretKey]
	}
	if strings.TrimSpace(resolved.EncryptionKey) == "" {
		resolved.EncryptionKey = stored[bootstrapEncryptionKeyName]
	}

	if strings.TrimSpace(resolved.SessionSecret) == "" {
		value, ensureErr := ensureBootstrapSecuritySecret(ctx, pool, bootstrapSessionSecretKey)
		if ensureErr != nil {
			return config.SecurityConfig{}, ensureErr
		}
		resolved.SessionSecret = value
	}
	if strings.TrimSpace(resolved.EncryptionKey) == "" {
		value, ensureErr := ensureBootstrapSecuritySecret(ctx, pool, bootstrapEncryptionKeyName)
		if ensureErr != nil {
			return config.SecurityConfig{}, ensureErr
		}
		resolved.EncryptionKey = value
	}

	if err := validateExplicitSecurityOverrides(resolved); err != nil {
		return config.SecurityConfig{}, err
	}
	return resolved, nil
}

func hasResolvedSecuritySecrets(security config.SecurityConfig) bool {
	return strings.TrimSpace(security.SessionSecret) != "" && strings.TrimSpace(security.EncryptionKey) != ""
}

func validateExplicitSecurityOverrides(security config.SecurityConfig) error {
	if secret := strings.TrimSpace(security.SessionSecret); secret != "" && len(secret) < 32 {
		return fmt.Errorf("security.session_secret must be at least 32 characters")
	}
	if key := strings.TrimSpace(security.EncryptionKey); key != "" {
		if _, err := security.DecodeEncryptionKey(); err != nil {
			return err
		}
	}
	return nil
}

func loadBootstrapSecuritySecrets(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT key_name, key_value
		FROM system_secrets
		WHERE key_name = ANY($1)
	`, []string{bootstrapSessionSecretKey, bootstrapEncryptionKeyName})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == undefinedTablePostgresErr {
			return nil, fmt.Errorf("system_secrets table is missing; run migrations or provide SECURITY_SESSION_SECRET and SECURITY_ENCRYPTION_KEY")
		}
		return nil, fmt.Errorf("load bootstrap security secrets: %w", err)
	}
	defer rows.Close()

	values := make(map[string]string, 2)
	for rows.Next() {
		var keyName string
		var keyValue string
		if scanErr := rows.Scan(&keyName, &keyValue); scanErr != nil {
			return nil, fmt.Errorf("scan bootstrap security secret: %w", scanErr)
		}
		values[keyName] = keyValue
	}
	if rows.Err() != nil {
		return nil, fmt.Errorf("iterate bootstrap security secrets: %w", rows.Err())
	}
	return values, nil
}

func ensureBootstrapSecuritySecret(ctx context.Context, pool *pgxpool.Pool, keyName string) (string, error) {
	values, err := loadBootstrapSecuritySecrets(ctx, pool)
	if err != nil {
		return "", err
	}
	if existing := strings.TrimSpace(values[keyName]); existing != "" {
		return existing, nil
	}

	generated, err := generateSecureRandomHex(32)
	if err != nil {
		return "", fmt.Errorf("generate %s: %w", keyName, err)
	}

	now := time.Now().UTC()
	if _, execErr := pool.Exec(ctx, `
		INSERT INTO system_secrets (id, created_at, updated_at, key_name, key_value, source)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (key_name) DO NOTHING
	`, stableBootstrapSecuritySecretID(keyName), now, now, keyName, generated, systemsecret.SourceDbGenerated); execErr != nil {
		return "", fmt.Errorf("persist %s: %w", keyName, execErr)
	}

	values, err = loadBootstrapSecuritySecrets(ctx, pool)
	if err != nil {
		return "", err
	}
	if stored := strings.TrimSpace(values[keyName]); stored != "" {
		return stored, nil
	}
	return "", fmt.Errorf("bootstrap security secret %s was not persisted", keyName)
}

func stableBootstrapSecuritySecretID(keyName string) string {
	slug := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(keyName), "_", "-"))
	if slug == "" {
		slug = uuid.NewString()
	}
	return "system-secret-" + slug
}

func generateSecureRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return hex.EncodeToString(b), nil
}
