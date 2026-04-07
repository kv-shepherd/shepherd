package modules

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func TestNewInfrastructure_ResolvesBootstrapSecuritySecrets(t *testing.T) {
	if err := logger.Init("error", "json"); err != nil {
		t.Fatalf("logger.Init() error = %v", err)
	}

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatal("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	adminPool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open postgres admin pool: %v", err)
	}
	defer adminPool.Close()

	schema := "t_modules_bootstrap_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, execErr := adminPool.Exec(context.Background(), fmt.Sprintf(`CREATE SCHEMA %q`, schema)); execErr != nil {
		t.Fatalf("create schema %q: %v", schema, execErr)
	}
	defer func() {
		_, _ = adminPool.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
	}()

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			URL:         dsnWithSearchPath(t, dsn, schema),
			MaxConns:    4,
			MinConns:    1,
			AutoMigrate: true,
		},
		K8s: config.K8sConfig{
			OperationTimeout: 0,
		},
		Worker: config.WorkerConfig{
			GeneralPoolSize: 1,
			K8sPoolSize:     1,
		},
	}

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

func dsnWithSearchPath(t *testing.T, dsn, schema string) string {
	t.Helper()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}
