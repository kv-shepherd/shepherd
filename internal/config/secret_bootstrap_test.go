package config

import (
	"testing"
	"time"
)

func TestConfigValidate_AllowsMissingRuntimeSecretsBeforeBootstrap(t *testing.T) {
	t.Parallel()

	cfg := &Config{Server: validServerConfigForSecretBootstrapTest()}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil before bootstrap secret resolution", err)
	}
}

func TestConfigValidateResolvedSecuritySecrets_RejectsMissingValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	if err := cfg.ValidateResolvedSecuritySecrets(); err == nil {
		t.Fatal("ValidateResolvedSecuritySecrets() expected error for missing secrets, got nil")
	}
}

func TestConfigValidateResolvedSecuritySecrets_RejectsShortSessionSecret(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Security: SecurityConfig{
			SessionSecret: "short-secret",
			EncryptionKey: "3031323334353637383961626364656630313233343536373839616263646566",
		},
	}
	if err := cfg.ValidateResolvedSecuritySecrets(); err == nil {
		t.Fatal("ValidateResolvedSecuritySecrets() expected error for short session secret, got nil")
	}
}

func TestConfigValidateResolvedSecuritySecrets_RejectsInvalidEncryptionKey(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Security: SecurityConfig{
			SessionSecret: "session-secret-1234567890123456789012",
			EncryptionKey: "not-hex",
		},
	}
	if err := cfg.ValidateResolvedSecuritySecrets(); err == nil {
		t.Fatal("ValidateResolvedSecuritySecrets() expected error for invalid encryption key, got nil")
	}
}

func TestConfigValidateResolvedSecuritySecrets_AcceptsResolvedValues(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Server: validServerConfigForSecretBootstrapTest(),
		Security: SecurityConfig{
			SessionSecret: "session-secret-1234567890123456789012",
			EncryptionKey: "3031323334353637383961626364656630313233343536373839616263646566",
		},
	}
	if err := cfg.ValidateResolvedSecuritySecrets(); err != nil {
		t.Fatalf("ValidateResolvedSecuritySecrets() error = %v", err)
	}
}

func validServerConfigForSecretBootstrapTest() ServerConfig {
	return ServerConfig{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
}
