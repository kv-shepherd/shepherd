package modules

import (
	"reflect"
	"testing"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/config"
)

func TestGovernanceModule_RegisterWorkers_RegistersRetentionWorkers(t *testing.T) {
	t.Parallel()

	workers := river.NewWorkers()
	module := NewGovernanceModule(&Infrastructure{EntClient: &ent.Client{}})

	module.RegisterWorkers(workers)

	workersValue := reflect.ValueOf(workers).Elem().FieldByName("workersMap")
	if !workersValue.IsValid() {
		t.Fatal("workersMap field not found")
	}
	if got := workersValue.Len(); got != 2 {
		t.Fatalf("registered workers = %d, want 2", got)
	}
	for _, kind := range []string{"notification_cleanup", "event_archive"} {
		if !workersValue.MapIndex(reflect.ValueOf(kind)).IsValid() {
			t.Fatalf("worker kind %q not registered", kind)
		}
	}
}

func TestNewServerDeps_BuildsSecurityDeps(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Session: config.SessionConfig{Lifetime: 2},
		Security: config.SecurityConfig{
			SessionSecret:       "session-secret-1234567890123456789012",
			EncryptionKey:       "3031323334353637383961626364656630313233343536373839616263646566",
			JWTVerificationKeys: []string{" verify-a ", "", "verify-b"},
		},
	}
	infra := &Infrastructure{
		EntClient: &ent.Client{},
	}

	deps := NewServerDeps(cfg, infra, nil)

	if got, want := string(deps.JWTCfg.SigningKey), cfg.Security.SessionSecret; got != want {
		t.Fatalf("SigningKey = %q, want %q", got, want)
	}
	if got := len(deps.JWTCfg.VerificationKeys); got != 2 {
		t.Fatalf("VerificationKeys len = %d, want 2", got)
	}
	if got, want := string(deps.JWTCfg.VerificationKeys[0]), "verify-a"; got != want {
		t.Fatalf("VerificationKeys[0] = %q, want %q", got, want)
	}
	if got, want := string(deps.JWTCfg.VerificationKeys[1]), "verify-b"; got != want {
		t.Fatalf("VerificationKeys[1] = %q, want %q", got, want)
	}
	if got := len(deps.EncryptionKey); got != 32 {
		t.Fatalf("EncryptionKey len = %d, want 32", got)
	}
}
