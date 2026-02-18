package config

import (
	"testing"
)

func TestEnsureSecrets_GeneratesMissingValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	if err := cfg.ensureSecrets(); err != nil {
		t.Fatalf("ensureSecrets() error = %v", err)
	}

	if cfg.Security.SessionSecret == "" {
		t.Fatal("session secret should be auto-generated")
	}
	if cfg.Security.EncryptionKey == "" {
		t.Fatal("encryption key should be auto-generated")
	}
	// 32 random bytes hex-encoded -> 64 chars.
	if len(cfg.Security.SessionSecret) != 64 {
		t.Fatalf("session secret length = %d, want 64", len(cfg.Security.SessionSecret))
	}
	if len(cfg.Security.EncryptionKey) != 64 {
		t.Fatalf("encryption key length = %d, want 64", len(cfg.Security.EncryptionKey))
	}
}

func TestEnsureSecrets_PreservesProvidedValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Security: SecurityConfig{
			SessionSecret: "abcdefghijklmnopqrstuvwxyzABCDEF123456", // 38 chars
			EncryptionKey: "keep-existing-encryption-key",
		},
	}

	if err := cfg.ensureSecrets(); err != nil {
		t.Fatalf("ensureSecrets() error = %v", err)
	}

	if got := cfg.Security.SessionSecret; got != "abcdefghijklmnopqrstuvwxyzABCDEF123456" {
		t.Fatalf("session secret changed unexpectedly: %q", got)
	}
	if got := cfg.Security.EncryptionKey; got != "keep-existing-encryption-key" {
		t.Fatalf("encryption key changed unexpectedly: %q", got)
	}
}

func TestConfigValidate_RejectsShortSessionSecret(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Security: SecurityConfig{
			SessionSecret: "short-secret",
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for short session secret, got nil")
	}
}
