package modules

import (
	"context"
	"reflect"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestNewInfrastructure_ResolvesBootstrapSecuritySecrets(t *testing.T) {
	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	cfg := newTestInfrastructureConfig(t, "modules_bootstrap", 0)
	infra, err := NewInfrastructure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewInfrastructure() error = %v", err)
	}
	defer infra.Close()

	if got := len(cfg.Security.SessionSecret); got != 64 {
		t.Fatalf("resolved session secret length = %d, want 64", got)
	}
	if got := len(cfg.Security.EncryptionKey); got != 64 {
		t.Fatalf("resolved encryption key length = %d, want 64", got)
	}
	if got := len(infra.EncryptionKey); got != 32 {
		t.Fatalf("decoded infrastructure encryption key length = %d, want 32", got)
	}
}

func TestNewInfrastructure_WiresK8sOperationTimeoutIntoHealthChecker(t *testing.T) {
	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	const operationTimeout = 17 * time.Second
	cfg := newTestInfrastructureConfig(t, "modules_health_timeout", operationTimeout)
	infra, err := NewInfrastructure(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewInfrastructure() error = %v", err)
	}
	defer infra.Close()

	if infra.HealthCheck == nil {
		t.Fatal("HealthCheck is nil")
	}
	got := time.Duration(reflect.ValueOf(infra.HealthCheck).Elem().FieldByName("operationTimeout").Int())
	if got != operationTimeout {
		t.Fatalf("health checker operation timeout = %s, want %s", got, operationTimeout)
	}
}

func newTestInfrastructureConfig(t *testing.T, databaseName string, operationTimeout time.Duration) *config.Config {
	t.Helper()

	pool := testutil.OpenPGXPool(t, databaseName)
	return &config.Config{
		Server: config.ServerConfig{
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		},
		Database: config.DatabaseConfig{
			URL:         pool.Config().ConnString(),
			MaxConns:    4,
			MinConns:    1,
			AutoMigrate: true,
		},
		K8s: config.K8sConfig{
			OperationTimeout: operationTimeout,
		},
		Worker: config.WorkerConfig{
			GeneralPoolSize: 1,
			K8sPoolSize:     1,
		},
	}
}
