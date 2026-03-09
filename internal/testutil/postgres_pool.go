package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// OpenPGXPool opens a pgxpool backed by PostgreSQL with isolated schema per test.
// It fails fast when TEST_DATABASE_URL/DATABASE_URL is missing to enforce ADR PostgreSQL-only tests.
func OpenPGXPool(t *testing.T, prefix string) *pgxpool.Pool {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatalf("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	schema := newSchemaName(prefix)
	ctx := context.Background()

	adminPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open postgres admin pool: %v", err)
	}
	t.Cleanup(adminPool.Close)

	if pingErr := waitForPostgresReady(ctx, adminPool.Ping); pingErr != nil {
		t.Fatalf("ping postgres: %v", pingErr)
	}

	if _, execErr := adminPool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %q`, schema)); execErr != nil {
		t.Fatalf("create test schema %q: %v", schema, execErr)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
	})

	schemaDSN, err := dsnWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatalf("build postgres DSN with search_path: %v", err)
	}

	testPool, err := pgxpool.New(ctx, schemaDSN)
	if err != nil {
		t.Fatalf("open postgres test pool: %v", err)
	}
	t.Cleanup(testPool.Close)

	if pingErr := waitForPostgresReady(ctx, testPool.Ping); pingErr != nil {
		t.Fatalf("ping postgres test pool: %v", pingErr)
	}

	return testPool
}
