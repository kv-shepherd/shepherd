package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
)

var nonIdentChars = regexp.MustCompile(`[^a-z0-9_]+`)

// OpenEntPostgres opens an Ent test client backed by PostgreSQL with isolated schema per test.
// It fails fast when TEST_DATABASE_URL/DATABASE_URL is missing to enforce ADR PostgreSQL-only tests.
func OpenEntPostgres(t *testing.T, prefix string) *ent.Client {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if dsn == "" {
		t.Fatalf("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	schema := newSchemaName(prefix)

	adminDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres admin connection: %v", err)
	}
	t.Cleanup(func() { _ = adminDB.Close() })

	if pingErr := waitForPostgresReady(context.Background(), adminDB.PingContext); pingErr != nil {
		t.Fatalf("ping postgres: %v", pingErr)
	}

	if _, execErr := adminDB.ExecContext(context.Background(), fmt.Sprintf(`CREATE SCHEMA %q`, schema)); execErr != nil {
		t.Fatalf("create test schema %q: %v", schema, execErr)
	}

	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema))
	})

	schemaDSN, err := dsnWithSearchPath(dsn, schema)
	if err != nil {
		t.Fatalf("build postgres DSN with search_path: %v", err)
	}

	testDB, err := sql.Open("pgx", schemaDSN)
	if err != nil {
		t.Fatalf("open postgres test connection: %v", err)
	}
	t.Cleanup(func() { _ = testDB.Close() })
	if pingErr := waitForPostgresReady(context.Background(), testDB.PingContext); pingErr != nil {
		t.Fatalf("ping postgres test connection: %v", pingErr)
	}

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, testDB))))
	t.Cleanup(func() { _ = client.Close() })
	for _, statement := range []string{batchreplay.EnsureHashFunctionSQL, batchreplay.EnsureLookupIndexSQL} {
		if _, execErr := testDB.ExecContext(t.Context(), statement); execErr != nil {
			t.Fatalf("ensure batch replay lookup support: %v", execErr)
		}
	}
	return client
}

// OpenEntPostgresWithPool opens an Ent client and exposes the same underlying
// pgxpool. Use it when the code under test coordinates Ent and raw pgx work in
// one database session namespace.
func OpenEntPostgresWithPool(t *testing.T, prefix string) (*ent.Client, *pgxpool.Pool) {
	t.Helper()

	pool := OpenPGXPool(t, prefix)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	for _, statement := range []string{batchreplay.EnsureHashFunctionSQL, batchreplay.EnsureLookupIndexSQL} {
		if _, execErr := pool.Exec(t.Context(), statement); execErr != nil {
			t.Fatalf("ensure batch replay lookup support: %v", execErr)
		}
	}
	return client, pool
}

func dsnWithSearchPath(dsn, schema string) (string, error) {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return "", fmt.Errorf("parse DSN: %w", err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		return u.String(), nil
	}

	if strings.Contains(dsn, "search_path=") {
		re := regexp.MustCompile(`search_path=\S+`)
		return re.ReplaceAllString(dsn, "search_path="+schema), nil
	}
	return dsn + " search_path=" + schema, nil
}

func newSchemaName(prefix string) string {
	base := strings.ToLower(prefix)
	base = strings.ReplaceAll(base, "-", "_")
	base = nonIdentChars.ReplaceAllString(base, "_")
	base = strings.Trim(base, "_")
	if base == "" {
		base = "test"
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	// River prefixes its longest notification topic ("river_leadership") with
	// "<schema>.". Keep test schemas within the resulting PostgreSQL channel
	// limit so transactional River tests work even with long test names.
	const maxRiverSchemaLen = 63 - len(".") - len("river_leadership")
	maxBaseLen := maxRiverSchemaLen - len("t__") - len(suffix)
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	return fmt.Sprintf("t_%s_%s", base, suffix)
}
