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
	_ "github.com/jackc/pgx/v5/stdlib"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
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

	if err := adminDB.PingContext(context.Background()); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}

	if _, err := adminDB.ExecContext(context.Background(), fmt.Sprintf(`CREATE SCHEMA "%s"`, schema)); err != nil {
		t.Fatalf("create test schema %q: %v", schema, err)
	}

	t.Cleanup(func() {
		_, _ = adminDB.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
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

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, testDB))))
	t.Cleanup(func() { _ = client.Close() })
	return client
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
	const maxPostgresIdentLen = 63
	maxBaseLen := maxPostgresIdentLen - len("t__") - len(suffix)
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	return fmt.Sprintf("t_%s_%s", base, suffix)
}
