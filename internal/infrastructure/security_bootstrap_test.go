package infrastructure

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/ent/systemsecret"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestResolveBootstrapSecuritySecrets_PrefersExplicitValuesWithoutDB(t *testing.T) {
	t.Parallel()

	explicit := config.SecurityConfig{
		SessionSecret: "session-secret-1234567890123456789012",
		EncryptionKey: "3031323334353637383961626364656630313233343536373839616263646566",
	}

	resolved, err := ResolveBootstrapSecuritySecrets(context.Background(), nil, explicit)
	if err != nil {
		t.Fatalf("ResolveBootstrapSecuritySecrets() error = %v", err)
	}
	if resolved.SessionSecret != explicit.SessionSecret {
		t.Fatalf("SessionSecret = %q, want explicit value", resolved.SessionSecret)
	}
	if resolved.EncryptionKey != explicit.EncryptionKey {
		t.Fatalf("EncryptionKey = %q, want explicit value", resolved.EncryptionKey)
	}
}

func TestResolveBootstrapSecuritySecrets_LoadsPersistedValues(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}
	pool := testutil.OpenPGXPool(t, "bootstrap_security_loads_persisted")
	createSystemSecretsTable(t, pool)

	const sessionSecret = "session-secret-1234567890123456789012"
	const encryptionKey = "3031323334353637383961626364656630313233343536373839616263646566"
	insertSystemSecretRow(t, pool, bootstrapSessionSecretKey, sessionSecret)
	insertSystemSecretRow(t, pool, bootstrapEncryptionKeyName, encryptionKey)

	resolved, err := ResolveBootstrapSecuritySecrets(context.Background(), pool, config.SecurityConfig{})
	if err != nil {
		t.Fatalf("ResolveBootstrapSecuritySecrets() error = %v", err)
	}
	if resolved.SessionSecret != sessionSecret {
		t.Fatalf("SessionSecret = %q, want persisted value", resolved.SessionSecret)
	}
	if resolved.EncryptionKey != encryptionKey {
		t.Fatalf("EncryptionKey = %q, want persisted value", resolved.EncryptionKey)
	}
}

func TestResolveBootstrapSecuritySecrets_GeneratesAndPersistsMissingValues(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_DATABASE_URL") == "" && os.Getenv("DATABASE_URL") == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}
	pool := testutil.OpenPGXPool(t, "bootstrap_security_generates")
	createSystemSecretsTable(t, pool)

	first, err := ResolveBootstrapSecuritySecrets(context.Background(), pool, config.SecurityConfig{})
	if err != nil {
		t.Fatalf("ResolveBootstrapSecuritySecrets() first call error = %v", err)
	}
	second, err := ResolveBootstrapSecuritySecrets(context.Background(), pool, config.SecurityConfig{})
	if err != nil {
		t.Fatalf("ResolveBootstrapSecuritySecrets() second call error = %v", err)
	}

	if len(first.SessionSecret) != 64 {
		t.Fatalf("generated session secret length = %d, want 64", len(first.SessionSecret))
	}
	if len(first.EncryptionKey) != 64 {
		t.Fatalf("generated encryption key length = %d, want 64", len(first.EncryptionKey))
	}
	if first.SessionSecret != second.SessionSecret {
		t.Fatalf("session secret changed across loads: %q != %q", first.SessionSecret, second.SessionSecret)
	}
	if first.EncryptionKey != second.EncryptionKey {
		t.Fatalf("encryption key changed across loads: %q != %q", first.EncryptionKey, second.EncryptionKey)
	}
}

func TestResolveBootstrapSecuritySecrets_ReleaseModeRejectsDatabaseFallback(t *testing.T) {
	t.Setenv("GIN_MODE", "release")

	_, err := ResolveBootstrapSecuritySecrets(context.Background(), nil, config.SecurityConfig{})
	if err == nil {
		t.Fatal("ResolveBootstrapSecuritySecrets() expected release-mode explicit secret error, got nil")
	}
	if !strings.Contains(err.Error(), "must be explicitly provided") {
		t.Fatalf("ResolveBootstrapSecuritySecrets() error = %v, want explicit-secret message", err)
	}
}

func createSystemSecretsTable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	const ddl = `
CREATE TABLE system_secrets (
	id text PRIMARY KEY,
	created_at timestamptz NOT NULL,
	updated_at timestamptz NOT NULL,
	key_name text NOT NULL UNIQUE,
	key_value text NOT NULL,
	source text NOT NULL
)`
	if _, err := pool.Exec(context.Background(), ddl); err != nil {
		t.Fatalf("create system_secrets table: %v", err)
	}
}

func insertSystemSecretRow(t *testing.T, pool *pgxpool.Pool, keyName, keyValue string) {
	t.Helper()

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO system_secrets (id, created_at, updated_at, key_name, key_value, source)
		VALUES ($1, now(), now(), $2, $3, $4)
	`, stableBootstrapSecuritySecretID(keyName), keyName, keyValue, systemsecret.SourceDbGenerated); err != nil {
		t.Fatalf("insert system secret %s: %v", keyName, err)
	}
}
