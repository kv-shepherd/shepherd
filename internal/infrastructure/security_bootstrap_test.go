package infrastructure

import (
	"context"
	"encoding/hex"
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

func TestResolveBootstrapSecuritySecrets_RejectsInvalidExplicitValuesWithoutDB(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		security config.SecurityConfig
		wantErr  string
	}{
		{
			name: "short session secret",
			security: config.SecurityConfig{
				SessionSecret: "too-short",
			},
			wantErr: "security.session_secret must be at least 32 characters",
		},
		{
			name: "invalid encryption key hex",
			security: config.SecurityConfig{
				EncryptionKey: "not-hex",
			},
			wantErr: "encryption_key",
		},
		{
			name: "wrong encryption key length",
			security: config.SecurityConfig{
				EncryptionKey: "30313233",
			},
			wantErr: "32 bytes",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveBootstrapSecuritySecrets(context.Background(), nil, tc.security)
			if err == nil {
				t.Fatal("ResolveBootstrapSecuritySecrets() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ResolveBootstrapSecuritySecrets() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveBootstrapSecuritySecrets_RequiresDatabaseForMissingSecret(t *testing.T) {
	t.Parallel()

	_, err := ResolveBootstrapSecuritySecrets(context.Background(), nil, config.SecurityConfig{
		SessionSecret: "session-secret-1234567890123456789012",
	})
	if err == nil {
		t.Fatal("ResolveBootstrapSecuritySecrets() error = nil, want database pool error")
	}
	if !strings.Contains(err.Error(), "requires a database pool") {
		t.Fatalf("ResolveBootstrapSecuritySecrets() error = %v, want database pool message", err)
	}
}

func TestStableBootstrapSecuritySecretID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		keyName string
		want    string
	}{
		{
			name:    "session secret",
			keyName: "SESSION_SECRET",
			want:    "system-secret-session-secret",
		},
		{
			name:    "encryption key trims whitespace",
			keyName: " ENCRYPTION_KEY ",
			want:    "system-secret-encryption-key",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stableBootstrapSecuritySecretID(tc.keyName); got != tc.want {
				t.Fatalf("stableBootstrapSecuritySecretID(%q) = %q, want %q", tc.keyName, got, tc.want)
			}
		})
	}
}

func TestGenerateSecureRandomHex(t *testing.T) {
	t.Parallel()

	got, err := generateSecureRandomHex(32)
	if err != nil {
		t.Fatalf("generateSecureRandomHex() error = %v", err)
	}
	if len(got) != 64 {
		t.Fatalf("generateSecureRandomHex(32) length = %d, want 64", len(got))
	}
	decoded, err := hex.DecodeString(got)
	if err != nil {
		t.Fatalf("generateSecureRandomHex() returned invalid hex %q: %v", got, err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded random secret length = %d, want 32", len(decoded))
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
