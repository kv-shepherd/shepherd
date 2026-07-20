package infrastructure

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	fakeAtlasHelperProcessEnv = "STARTUP_MIGRATION_FAKE_ATLAS_HELPER"
	fakeAtlasTestBinaryEnv    = "STARTUP_MIGRATION_FAKE_ATLAS_TEST_BINARY"
	fakeAtlasLogEnv           = "STARTUP_MIGRATION_FAKE_ATLAS_LOG"
	fakeAtlasFailCommandEnv   = "STARTUP_MIGRATION_FAKE_ATLAS_FAIL_COMMAND"
	fakeAtlasExpectedURLEnv   = "STARTUP_MIGRATION_FAKE_ATLAS_EXPECTED_DATABASE_URL"
	fakeAtlasValidateOnlyEnv  = "STARTUP_MIGRATION_FAKE_ATLAS_VALIDATE_ENV_ONLY"
	fakeAtlasRedactedURL      = "[redacted]"
)

type startupMigrationTestDatabase struct {
	config config.DatabaseConfig
	pool   *pgxpool.Pool
}

var (
	startupMigrationLoggerOnce sync.Once
	errStartupMigrationLogger  error
)

func newStartupMigrationTestDatabase(t *testing.T, prefix string) *startupMigrationTestDatabase {
	t.Helper()
	ensureStartupMigrationTestLogger(t)

	baseDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if baseDSN == "" {
		baseDSN = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if baseDSN == "" {
		t.Fatal("PostgreSQL test DSN is required")
	}
	adminDSN := strings.TrimSpace(os.Getenv("TEST_DATABASE_ADMIN_URL"))
	if adminDSN == "" {
		adminDSN = baseDSN
	}

	adminConfig, parseErr := pgxpool.ParseConfig(adminDSN)
	if parseErr != nil {
		t.Fatalf("parse PostgreSQL admin DSN: %v", parseErr)
	}
	adminPool, adminPoolErr := pgxpool.NewWithConfig(t.Context(), adminConfig)
	if adminPoolErr != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", adminPoolErr)
	}
	if err := waitForStartupMigrationPostgres(t.Context(), adminPool); err != nil {
		adminPool.Close()
		t.Fatalf("wait for PostgreSQL admin pool: %v", err)
	}

	databaseName := "sm_" + sanitizeStartupMigrationDatabasePrefix(prefix) + "_" + strings.ReplaceAll(uuid.NewString()[:13], "-", "")
	quotedDatabaseName := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminPool.Exec(t.Context(), "CREATE DATABASE "+quotedDatabaseName); err != nil {
		adminPool.Close()
		t.Fatalf(
			"create PostgreSQL test database %s (set TEST_DATABASE_ADMIN_URL to a DSN with CREATEDB): %v",
			databaseName,
			err,
		)
	}

	databaseDSN, dsnErr := startupMigrationDatabaseDSN(baseDSN, databaseName)
	if dsnErr != nil {
		_, _ = adminPool.Exec(context.Background(), "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)")
		adminPool.Close()
		t.Fatalf("build PostgreSQL test database DSN: %v", dsnErr)
	}

	pool, poolErr := pgxpool.New(t.Context(), databaseDSN)
	if poolErr != nil {
		_, _ = adminPool.Exec(context.Background(), "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)")
		adminPool.Close()
		t.Fatalf("open PostgreSQL test database: %v", poolErr)
	}
	if err := waitForStartupMigrationPostgres(t.Context(), pool); err != nil {
		pool.Close()
		_, _ = adminPool.Exec(context.Background(), "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)")
		adminPool.Close()
		t.Fatalf("ping PostgreSQL test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := adminPool.Exec(ctx, "DROP DATABASE "+quotedDatabaseName+" WITH (FORCE)"); err != nil {
			t.Errorf("drop PostgreSQL test database %s: %v", databaseName, err)
		}
		adminPool.Close()
	})

	return &startupMigrationTestDatabase{
		config: config.DatabaseConfig{
			URL:                          databaseDSN,
			MaxConns:                     8,
			MinConns:                     0,
			AutoApplyVersionedMigrations: true,
		},
		pool: pool,
	}
}

func ensureStartupMigrationTestLogger(t *testing.T) {
	t.Helper()
	startupMigrationLoggerOnce.Do(func() {
		errStartupMigrationLogger = logger.Init("error", "json")
	})
	if errStartupMigrationLogger != nil {
		t.Fatalf("initialize startup migration test logger: %v", errStartupMigrationLogger)
	}
}

func startupMigrationDatabaseDSN(baseDSN, databaseName string) (string, error) {
	if strings.Contains(baseDSN, "://") {
		parsed, err := url.Parse(baseDSN)
		if err != nil {
			return "", err
		}
		parsed.Path = "/" + databaseName
		parsed.RawPath = ""
		query := parsed.Query()
		query.Set("search_path", "public")
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}
	if strings.TrimSpace(baseDSN) == "" {
		return "", fmt.Errorf("base DSN is empty")
	}
	return strings.TrimSpace(baseDSN) + " database=" + databaseName + " search_path=public", nil
}

func waitForStartupMigrationPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := pool.Ping(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return fmt.Errorf("PostgreSQL was not ready: %w", lastErr)
}

func sanitizeStartupMigrationDatabasePrefix(prefix string) string {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	var b strings.Builder
	for _, r := range prefix {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "test"
	}
	return b.String()
}

func (db *startupMigrationTestDatabase) tableExists(t *testing.T, tableName string) bool {
	t.Helper()

	var exists bool
	if err := db.pool.QueryRow(t.Context(), `
SELECT EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = $1
)
`, tableName).Scan(&exists); err != nil {
		t.Fatalf("check table %s: %v", tableName, err)
	}
	return exists
}

func (db *startupMigrationTestDatabase) createCoreSchemaMarker(t *testing.T) {
	t.Helper()

	if _, err := db.pool.Exec(t.Context(), `
CREATE TABLE users (
    id text PRIMARY KEY,
    marker text NOT NULL
);
INSERT INTO users (id, marker) VALUES ('legacy-user', 'preserve-me');
`); err != nil {
		t.Fatalf("create legacy core schema marker: %v", err)
	}
}

func (db *startupMigrationTestDatabase) createAtlasRevisionMarker(t *testing.T, version string) {
	t.Helper()

	if _, err := db.pool.Exec(t.Context(), `
CREATE TABLE atlas_schema_revisions (
    version text PRIMARY KEY
)
`); err != nil {
		t.Fatalf("create Atlas revision table: %v", err)
	}
	if _, err := db.pool.Exec(t.Context(), `
INSERT INTO atlas_schema_revisions (version) VALUES ($1)
`, version); err != nil {
		t.Fatalf("insert Atlas revision marker: %v", err)
	}
}

type fakeAtlasFixture struct {
	execPath     string
	migrationDir string
	logPath      string
	version      string
}

func configureStartupMigrationFakeAtlas(t *testing.T) fakeAtlasFixture {
	t.Helper()

	dir := t.TempDir()
	migrationDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(migrationDir, 0o755); err != nil {
		t.Fatalf("create fake Atlas migration directory: %v", err)
	}
	version := "20260717000100"
	if err := os.WriteFile(filepath.Join(migrationDir, "atlas.sum"), []byte("h1:fake\n"), 0o644); err != nil {
		t.Fatalf("write fake atlas.sum: %v", err)
	}
	fixtureMigration := `
CREATE TABLE IF NOT EXISTS startup_fake_atlas_applied (
    version text PRIMARY KEY
);
INSERT INTO startup_fake_atlas_applied (version)
VALUES ('` + version + `')
ON CONFLICT (version) DO NOTHING;
`
	if err := os.WriteFile(filepath.Join(migrationDir, version+"_startup_test.sql"), []byte(fixtureMigration), 0o644); err != nil {
		t.Fatalf("write fake Atlas migration: %v", err)
	}

	testBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve infrastructure test binary: %v", err)
	}
	execPath := filepath.Join(dir, "atlas")
	wrapper := `#!/bin/sh
exec "$STARTUP_MIGRATION_FAKE_ATLAS_TEST_BINARY" -test.run '^TestStartupMigrationFakeAtlasHelperProcess$' -- "$@"
`
	if err := os.WriteFile(execPath, []byte(wrapper), 0o755); err != nil {
		t.Fatalf("write fake Atlas executable: %v", err)
	}

	logPath := filepath.Join(dir, "atlas-calls.jsonl")
	t.Setenv(atlasExecPathEnv, execPath)
	t.Setenv(atlasMigrationDirEnv, migrationDir)
	t.Setenv(fakeAtlasHelperProcessEnv, "1")
	t.Setenv(fakeAtlasTestBinaryEnv, testBinary)
	t.Setenv(fakeAtlasLogEnv, logPath)
	t.Setenv(fakeAtlasFailCommandEnv, "")
	t.Setenv(fakeAtlasExpectedURLEnv, "")
	t.Setenv(fakeAtlasValidateOnlyEnv, "")

	return fakeAtlasFixture{
		execPath:     execPath,
		migrationDir: migrationDir,
		logPath:      logPath,
		version:      version,
	}
}

func (f fakeAtlasFixture) calls(t *testing.T) [][]string {
	t.Helper()

	file, err := os.Open(f.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("open fake Atlas call log: %v", err)
	}
	defer file.Close()

	var calls [][]string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var call []string
		if err := json.Unmarshal(scanner.Bytes(), &call); err != nil {
			t.Fatalf("decode fake Atlas call %q: %v", scanner.Text(), err)
		}
		calls = append(calls, call)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan fake Atlas calls: %v", err)
	}
	return calls
}

func startupMigrationHasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func startupMigrationFlagValue(args []string, flag string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}

// TestStartupMigrationFakeAtlasHelperProcess is executed only through the
// temporary Atlas wrapper created by configureStartupMigrationFakeAtlas.
func TestStartupMigrationFakeAtlasHelperProcess(t *testing.T) {
	if os.Getenv(fakeAtlasHelperProcessEnv) != "1" {
		return
	}
	exitCode := runStartupMigrationFakeAtlas(os.Args)
	if exitCode != 0 {
		t.Errorf("fake Atlas helper exit code = %d, want 0", exitCode)
	}
	os.Exit(exitCode)
}

func runStartupMigrationFakeAtlas(processArgs []string) int {
	args := processArgs
	for i, arg := range processArgs {
		if arg == "--" {
			args = processArgs[i+1:]
			break
		}
	}
	safeArgs := redactStartupMigrationArgs(args)
	if len(args) < 2 || args[0] != "migrate" || args[1] != "set" && args[1] != "apply" {
		fmt.Fprintf(os.Stderr, "fake atlas: unsupported argv %q\n", safeArgs)
		return 2
	}

	encoded, marshalErr := json.Marshal(safeArgs)
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: encode argv: %v\n", marshalErr)
		return 2
	}
	logFile, openErr := os.OpenFile(os.Getenv(fakeAtlasLogEnv), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if openErr != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: open call log: %v\n", openErr)
		return 2
	}
	if _, writeErr := logFile.Write(append(encoded, '\n')); writeErr != nil {
		_ = logFile.Close()
		fmt.Fprintf(os.Stderr, "fake atlas: write call log: %v\n", writeErr)
		return 2
	}
	if closeErr := logFile.Close(); closeErr != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: close call log: %v\n", closeErr)
		return 2
	}

	command := args[1]
	if os.Getenv(fakeAtlasFailCommandEnv) == command {
		fmt.Fprintf(os.Stderr, "fake atlas %s failure\n", command)
		return 42
	}
	dsn := strings.TrimSpace(os.Getenv(atlasStartupDatabaseURL))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "fake atlas: database URL environment is required")
		return 2
	}
	if expectedDSN := os.Getenv(fakeAtlasExpectedURLEnv); expectedDSN != "" && dsn != expectedDSN {
		fmt.Fprintln(os.Stderr, "fake atlas: database URL environment does not match expected endpoint")
		return 2
	}
	dirURL := strings.TrimSpace(os.Getenv(atlasStartupMigrationDir))
	if !strings.HasPrefix(dirURL, "file://") {
		fmt.Fprintln(os.Stderr, "fake atlas: migration directory environment is required")
		return 2
	}
	if os.Getenv(fakeAtlasValidateOnlyEnv) == "1" {
		if command == "apply" {
			fmt.Fprintln(os.Stdout, `{"Current":"validated","Target":"validated","Applied":[]}`)
		}
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, connectErr := pgx.Connect(ctx, dsn)
	if connectErr != nil {
		fmt.Fprintln(os.Stderr, "fake atlas: connect database failed")
		return 2
	}
	defer conn.Close(ctx)

	// Real Atlas serializes migration operations with an advisory lock. The
	// fake preserves that property so concurrency tests exercise the startup
	// orchestration rather than races inside the fake implementation.
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(71520260717)`); err != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: acquire advisory lock: %v\n", err)
		return 2
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(71520260717)`) }()

	if _, err := conn.Exec(ctx, `
CREATE TABLE IF NOT EXISTS atlas_schema_revisions (
    version text PRIMARY KEY
)
`); err != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: create revisions table: %v\n", err)
		return 2
	}

	version := "fake-applied"
	if command == "set" {
		version = args[len(args)-1]
		if version == "" || strings.HasPrefix(version, "-") {
			fmt.Fprintln(os.Stderr, "fake atlas: migrate set version is required")
			return 2
		}
	}
	if command == "apply" {
		migrationDir, parseErr := fakeAtlasMigrationDir(dirURL)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, "fake atlas: invalid migration directory")
			return 2
		}
		entries, readErr := os.ReadDir(migrationDir)
		if readErr != nil {
			fmt.Fprintln(os.Stderr, "fake atlas: read migration directory failed")
			return 2
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
				continue
			}
			migrationSQL, readErr := os.ReadFile(filepath.Join(migrationDir, entry.Name()))
			if readErr != nil {
				fmt.Fprintln(os.Stderr, "fake atlas: read migration failed")
				return 2
			}
			if _, execErr := conn.Exec(ctx, string(migrationSQL)); execErr != nil {
				fmt.Fprintln(os.Stderr, "fake atlas: execute migration failed")
				return 2
			}
			version = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
	}
	if _, err := conn.Exec(ctx, `
INSERT INTO atlas_schema_revisions (version)
VALUES ($1)
ON CONFLICT (version) DO NOTHING
`, version); err != nil {
		fmt.Fprintf(os.Stderr, "fake atlas: record revision: %v\n", err)
		return 2
	}

	if command == "apply" {
		result, resultErr := json.Marshal(map[string]interface{}{
			"Current": version,
			"Target":  version,
			"Applied": []interface{}{},
		})
		if resultErr != nil {
			fmt.Fprintf(os.Stderr, "fake atlas: encode apply result: %v\n", resultErr)
			return 2
		}
		fmt.Fprintln(os.Stdout, string(result))
	}
	return 0
}

func redactStartupMigrationArgs(args []string) []string {
	safe := append([]string(nil), args...)
	for i := 0; i+1 < len(safe); i++ {
		if safe[i] == "--url" {
			safe[i+1] = fakeAtlasRedactedURL
		}
	}
	return safe
}

func fakeAtlasMigrationDir(dirURL string) (string, error) {
	parsed, err := url.Parse(dirURL)
	if err != nil || parsed.Scheme != "file" || parsed.Path == "" {
		return "", fmt.Errorf("invalid file URL")
	}
	return filepath.FromSlash(parsed.Path), nil
}
