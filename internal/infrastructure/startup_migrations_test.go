package infrastructure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ariga.io/atlas/atlasexec"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/repository/batchreplay"
)

func TestIsAtlasMigrationDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if isAtlasMigrationDir(dir) {
		t.Fatal("isAtlasMigrationDir() = true, want false without atlas.sum and sql files")
	}
	if err := os.WriteFile(filepath.Join(dir, "atlas.sum"), []byte("h1:test\n"), 0o644); err != nil {
		t.Fatalf("write atlas.sum: %v", err)
	}
	if isAtlasMigrationDir(dir) {
		t.Fatal("isAtlasMigrationDir() = true, want false without sql files")
	}
	if err := os.WriteFile(filepath.Join(dir, "20260427000100_notification_i18n_contract.sql"), []byte("-- test\n"), 0o644); err != nil {
		t.Fatalf("write sql file: %v", err)
	}
	if !isAtlasMigrationDir(dir) {
		t.Fatal("isAtlasMigrationDir() = false, want true with atlas.sum and sql files")
	}
}

func TestShouldBootstrapCurrentSchemaBeforeAtlas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state databaseMigrationState
		want  bool
	}{
		{
			name: "empty database without atlas revisions",
			state: databaseMigrationState{
				HasCoreSchema:     false,
				HasAtlasRevisions: false,
			},
			want: true,
		},
		{
			name: "legacy schema without atlas revisions",
			state: databaseMigrationState{
				HasCoreSchema:     true,
				HasAtlasRevisions: false,
			},
			want: false,
		},
		{
			name: "atlas-managed schema",
			state: databaseMigrationState{
				HasCoreSchema:     true,
				HasAtlasRevisions: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldBootstrapCurrentSchemaBeforeAtlas(tt.state); got != tt.want {
				t.Fatalf("shouldBootstrapCurrentSchemaBeforeAtlas() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigureStartupAtlasClientKeepsDatabaseURLOutOfConfig(t *testing.T) {
	t.Setenv("ATLAS_NO_UPDATE_NOTIFIER", "1")
	t.Setenv("ATLAS_NO_UPGRADE_SUGGESTIONS", "1")

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	client, err := atlasexec.NewClient(t.TempDir(), executable)
	if err != nil {
		t.Fatalf("create atlas client: %v", err)
	}
	const databaseURL = "postgres://migration-user:credential-value@database/shepherd"
	configURL, cleanup, err := configureStartupAtlasClient(client, databaseURL, t.TempDir())
	if err != nil {
		t.Fatalf("configureStartupAtlasClient() error = %v", err)
	}
	defer cleanup()

	configPath := filepath.FromSlash(strings.TrimPrefix(configURL, "file://"))
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read temporary atlas config: %v", err)
	}
	if strings.Contains(string(content), databaseURL) || strings.Contains(string(content), "credential-value") {
		t.Fatal("temporary atlas config contains database credentials")
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat temporary atlas config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("temporary atlas config permissions = %o, want 600", info.Mode().Perm())
	}

	message := "migration failed for " + databaseURL
	if got := redactAtlasMigrationError(message, databaseURL); strings.Contains(got, databaseURL) || strings.Contains(got, "credential-value") {
		t.Fatalf("redacted atlas error still contains database credentials: %q", got)
	}

	const encodedDatabaseURL = "postgres://migration-user:credential%2Fvalue@database/shepherd"
	normalizedMessage := "migration failed with password credential/value (credential%2Fvalue or credential%2fvalue)"
	if got := redactAtlasMigrationError(normalizedMessage, encodedDatabaseURL); strings.Contains(got, "credential/value") || strings.Contains(got, "credential%2Fvalue") || strings.Contains(got, "credential%2fvalue") {
		t.Fatalf("redacted normalized atlas error still contains database credentials: %q", got)
	}
}

func TestStartupMigrationLockConfigUsesSessionEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		cfg               config.DatabaseConfig
		wantHost          string
		wantPort          uint16
		wantUser          string
		wantPassword      string
		wantDatabase      string
		wantRuntimeParams map[string]string
	}{
		{
			name: "default database endpoint",
			cfg: config.DatabaseConfig{
				URL: "postgres://migration-user@database:5432/shepherd?sslmode=require",
			},
			wantHost:     "database",
			wantPort:     5432,
			wantUser:     "migration-user",
			wantDatabase: "shepherd",
		},
		{
			name: "PgBouncer session endpoint preserves credentials database and query",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user:credential%2Fvalue@transaction-pool:6432/shepherd?application_name=startup&options=-c%20statement_timeout%3D5s&sslmode=require",
				WorkerHost: "session-pool",
				WorkerPort: 6433,
			},
			wantHost:     "session-pool",
			wantPort:     6433,
			wantUser:     "migration-user",
			wantPassword: "credential/value",
			wantDatabase: "shepherd",
			wantRuntimeParams: map[string]string{
				"application_name": "startup",
				"options":          "-c statement_timeout=5s",
			},
		},
		{
			name: "session endpoint inherits the base port",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user@transaction-pool:6432/shepherd?sslmode=require",
				WorkerHost: "session-pool",
			},
			wantHost:     "session-pool",
			wantPort:     6432,
			wantUser:     "migration-user",
			wantDatabase: "shepherd",
		},
		{
			name: "session endpoint also replaces TLS fallback endpoints",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user@transaction-pool:6432/shepherd?sslmode=prefer",
				WorkerHost: "session-pool",
				WorkerPort: 6433,
			},
			wantHost:     "session-pool",
			wantPort:     6433,
			wantUser:     "migration-user",
			wantDatabase: "shepherd",
		},
		{
			name: "multi-host URL is replaced by one bracketed IPv6 session endpoint",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user:credential%2Fvalue@transaction-a:6432,transaction-b:6433/shepherd?application_name=startup&host=ignored-host&port=7444&sslmode=verify-full",
				WorkerHost: "[2001:db8::42]",
				WorkerPort: 6434,
			},
			wantHost:     "2001:db8::42",
			wantPort:     6434,
			wantUser:     "migration-user",
			wantPassword: "credential/value",
			wantDatabase: "shepherd",
			wantRuntimeParams: map[string]string{
				"application_name": "startup",
			},
		},
		{
			name: "libpq keyword DSN removes hostaddr and uses session endpoint",
			cfg: config.DatabaseConfig{
				URL:        `host=transaction-pool hostaddr=192.0.2.10 port=6432 user=migration-user password='credential/value' dbname=shepherd sslmode=require application_name='https://startup.example' options='-c statement_timeout=5s'`,
				WorkerHost: "session-pool",
				WorkerPort: 6433,
			},
			wantHost:     "session-pool",
			wantPort:     6433,
			wantUser:     "migration-user",
			wantPassword: "credential/value",
			wantDatabase: "shepherd",
			wantRuntimeParams: map[string]string{
				"application_name": "https://startup.example",
				"options":          "-c statement_timeout=5s",
			},
		},
		{
			name: "libpq keyword session host cannot inject settings",
			cfg: config.DatabaseConfig{
				URL:        "host=transaction-pool port=6432 user=migration-user dbname=shepherd sslmode=require application_name=startup",
				WorkerHost: `session'pool\ application_name=attacker`,
				WorkerPort: 6433,
			},
			wantHost:     `session'pool\ application_name=attacker`,
			wantPort:     6433,
			wantUser:     "migration-user",
			wantDatabase: "shepherd",
			wantRuntimeParams: map[string]string{
				"application_name": "startup",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := startupMigrationLockConfig(test.cfg)
			if err != nil {
				t.Fatalf("startupMigrationLockConfig() error = %v", err)
			}
			if got.Host != test.wantHost || got.Port != test.wantPort || got.Database != test.wantDatabase {
				t.Fatalf(
					"startupMigrationLockConfig() endpoint = (%q, %d, %q), want (%q, %d, %q)",
					got.Host,
					got.Port,
					got.Database,
					test.wantHost,
					test.wantPort,
					test.wantDatabase,
				)
			}
			if got.User != test.wantUser {
				t.Fatalf("startupMigrationLockConfig() user = %q, want %q", got.User, test.wantUser)
			}
			if got.Password != test.wantPassword {
				t.Fatal("startupMigrationLockConfig() did not preserve the database password")
			}
			if (strings.Contains(test.cfg.URL, "sslmode=require") || strings.Contains(test.cfg.URL, "sslmode=verify-full")) && got.TLSConfig == nil {
				t.Fatal("startupMigrationLockConfig() did not preserve the TLS mode")
			}
			if test.cfg.WorkerHost != "" {
				if got.TLSConfig != nil && got.TLSConfig.ServerName != "" && got.TLSConfig.ServerName != test.wantHost {
					t.Fatalf(
						"startupMigrationLockConfig() TLS server name = %q, want %q",
						got.TLSConfig.ServerName,
						test.wantHost,
					)
				}
				if test.name == "session endpoint also replaces TLS fallback endpoints" && len(got.Fallbacks) == 0 {
					t.Fatal("startupMigrationLockConfig() returned no TLS fallback endpoint")
				}
				for fallbackIndex, fallback := range got.Fallbacks {
					if fallback.Host != test.wantHost || fallback.Port != test.wantPort {
						t.Fatalf(
							"startupMigrationLockConfig() fallback %d endpoint = (%q, %d), want (%q, %d)",
							fallbackIndex,
							fallback.Host,
							fallback.Port,
							test.wantHost,
							test.wantPort,
						)
					}
					if fallback.TLSConfig != nil && fallback.TLSConfig.ServerName != "" && fallback.TLSConfig.ServerName != test.wantHost {
						t.Fatalf(
							"startupMigrationLockConfig() fallback %d TLS server name = %q, want %q",
							fallbackIndex,
							fallback.TLSConfig.ServerName,
							test.wantHost,
						)
					}
				}
			}
			for key, want := range test.wantRuntimeParams {
				if got.RuntimeParams[key] != want {
					t.Fatalf("startupMigrationLockConfig() runtime param %s = %q, want %q", key, got.RuntimeParams[key], want)
				}
			}
			if test.cfg.WorkerHost != "" {
				if _, exists := got.RuntimeParams["hostaddr"]; exists {
					t.Fatal("startupMigrationLockConfig() preserved a keyword hostaddr endpoint override")
				}
			}
		})
	}
}

func TestStartupMigrationLockConfigErrorsDoNotExposeCredentials(t *testing.T) {
	t.Parallel()

	const sentinelPassword = "startup-lock-sentinel-password"
	for _, test := range []struct {
		name string
		cfg  config.DatabaseConfig
	}{
		{
			name: "malformed URL",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user:" + sentinelPassword + "%zz@transaction-pool/shepherd",
				WorkerHost: "session-pool",
			},
		},
		{
			name: "malformed keyword DSN",
			cfg: config.DatabaseConfig{
				URL:        "host=transaction-pool user=migration-user password='" + sentinelPassword,
				WorkerHost: "session-pool",
			},
		},
		{
			name: "invalid rewritten endpoint",
			cfg: config.DatabaseConfig{
				URL:        "postgres://migration-user:" + sentinelPassword + "@transaction-pool/shepherd?sslmode=require",
				WorkerHost: "[not-an-ipv6-address]",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := startupMigrationLockConfig(test.cfg)
			if err == nil {
				t.Fatal("startupMigrationLockConfig() error = nil, want malformed DSN failure")
			}
			if strings.Contains(err.Error(), sentinelPassword) || strings.Contains(err.Error(), test.cfg.URL) {
				t.Fatalf("startupMigrationLockConfig() error exposes database credentials: %q", err)
			}
		})
	}
}

func TestStartupAtlasCommandsUseSessionEndpointEnvironment(t *testing.T) {
	ensureStartupMigrationTestLogger(t)

	cfg := config.DatabaseConfig{
		URL:        "postgres://migration-user:credential%2Fvalue@transaction-a:6432,transaction-b:6433/shepherd?application_name=startup&options=-c%20statement_timeout%3D5s&sslmode=require",
		WorkerHost: "session-pool.internal",
		WorkerPort: 6434,
	}
	lockConfig, err := startupMigrationLockConfig(cfg)
	if err != nil {
		t.Fatalf("startupMigrationLockConfig() error = %v", err)
	}
	if lockConfig.Host != cfg.WorkerHost || lockConfig.Port != uint16(cfg.WorkerPort) {
		t.Fatalf("session endpoint = %s:%d, want %s:%d", lockConfig.Host, lockConfig.Port, cfg.WorkerHost, cfg.WorkerPort)
	}
	if lockConfig.User != "migration-user" || lockConfig.Password != "credential/value" || lockConfig.Database != "shepherd" {
		t.Fatal("session DSN did not preserve the user, password, and database")
	}
	if lockConfig.RuntimeParams["application_name"] != "startup" || lockConfig.RuntimeParams["options"] != "-c statement_timeout=5s" {
		t.Fatalf("session runtime params = %#v, want preserved application_name/options", lockConfig.RuntimeParams)
	}
	sessionDSN := lockConfig.ConnString()
	if strings.Contains(sessionDSN, "transaction-a") || strings.Contains(sessionDSN, "transaction-b") {
		t.Fatal("session DSN still contains a transaction-pool endpoint")
	}

	for _, command := range []string{"apply", "set"} {
		t.Run(command, func(t *testing.T) {
			fakeAtlas := configureStartupMigrationFakeAtlas(t)
			t.Setenv(fakeAtlasExpectedURLEnv, sessionDSN)
			t.Setenv(fakeAtlasValidateOnlyEnv, "1")

			var commandErr error
			switch command {
			case "apply":
				commandErr = applyAtlasMigrations(
					t.Context(),
					fakeAtlas.execPath,
					fakeAtlas.migrationDir,
					sessionDSN,
					atlasexec.MigrateApplyParams{},
				)
			case "set":
				commandErr = setAtlasVersion(
					t.Context(),
					fakeAtlas.execPath,
					fakeAtlas.migrationDir,
					sessionDSN,
					fakeAtlas.version,
				)
			}
			if commandErr != nil {
				t.Fatalf("Atlas migrate %s error = %v", command, commandErr)
			}
			calls := fakeAtlas.calls(t)
			if len(calls) != 1 {
				t.Fatalf("fake Atlas calls = %#v, want one migrate %s call", calls, command)
			}
			assertStartupMigrationAtlasCall(t, calls[0], command, false)
		})
	}
}

func TestLatestMigrationVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := []string{
		"20260320000100_adr0047_vm_status_enum.sql",
		"20260427000100_notification_i18n_contract.sql",
		"README.md",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("-- test\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := latestMigrationVersion(dir)
	if err != nil {
		t.Fatalf("latestMigrationVersion() error = %v", err)
	}
	if got != "20260427000100" {
		t.Fatalf("latestMigrationVersion() = %q, want %q", got, "20260427000100")
	}
}

func TestAtlasExecPathCandidatesIncludeReleaseLayout(t *testing.T) {
	t.Parallel()

	exePath := filepath.Join(string(filepath.Separator), "opt", "shepherd", "shepherd")
	got := atlasExecPathCandidates("", exePath)
	want := filepath.Join(string(filepath.Separator), "opt", "shepherd", "atlas")

	if !containsPath(got, want) {
		t.Fatalf("atlasExecPathCandidates() = %#v, want %q", got, want)
	}
}

func TestAtlasMigrationDirCandidatesIncludeReleaseLayout(t *testing.T) {
	t.Parallel()

	exePath := filepath.Join(string(filepath.Separator), "opt", "shepherd", "shepherd")
	got := atlasMigrationDirCandidates("", "", exePath)
	want := filepath.Join(string(filepath.Separator), "opt", "shepherd", "share", "shepherd", "migrations", "atlas")

	if !containsPath(got, want) {
		t.Fatalf("atlasMigrationDirCandidates() = %#v, want %q", got, want)
	}
}

func TestEnsureStartupMigrations_AutoApplyDisabledHasNoSideEffects(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "disabled")
	fakeAtlas := configureStartupMigrationFakeAtlas(t)
	cfg := database.config
	cfg.AutoApplyVersionedMigrations = false

	if err := EnsureStartupMigrations(t.Context(), cfg); err != nil {
		t.Fatalf("EnsureStartupMigrations() with auto-apply disabled returned error: %v", err)
	}

	for _, tableName := range []string{coreSchemaTable, atlasRevisionTable, "river_migration", "river_job"} {
		if database.tableExists(t, tableName) {
			t.Fatalf("table %q exists after disabled startup migration, want no database side effects", tableName)
		}
	}
	if calls := fakeAtlas.calls(t); len(calls) != 0 {
		t.Fatalf("fake Atlas calls = %#v, want none when auto-apply is disabled", calls)
	}
}

func TestEnsureStartupMigrations_EmptyDatabaseBootstrapsBaselineAndRiver(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "empty")
	fakeAtlas := configureStartupMigrationFakeAtlas(t)

	if err := EnsureStartupMigrations(t.Context(), database.config); err != nil {
		t.Fatalf("EnsureStartupMigrations() unexpected error: %v", err)
	}

	for _, tableName := range []string{coreSchemaTable, atlasRevisionTable, "river_migration", "river_job", "river_queue"} {
		if !database.tableExists(t, tableName) {
			t.Fatalf("table %q is missing after empty-database startup migration", tableName)
		}
	}
	var requestIDMaxLength *int
	if err := database.pool.QueryRow(t.Context(), `
SELECT character_maximum_length
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'batch_tickets'
  AND column_name = 'request_id'
`).Scan(&requestIDMaxLength); err != nil {
		t.Fatalf("query bootstrapped request_id column: %v", err)
	}
	if requestIDMaxLength != nil {
		t.Fatalf("bootstrapped request_id maximum length = %d, want unbounded text", *requestIDMaxLength)
	}
	var requestIDIndexCount int
	if err := database.pool.QueryRow(t.Context(), `
SELECT count(*)
FROM pg_indexes
WHERE schemaname = current_schema()
  AND tablename = 'batch_tickets'
  AND indexdef ILIKE '%request_id%'
`).Scan(&requestIDIndexCount); err != nil {
		t.Fatalf("query bootstrapped request_id indexes: %v", err)
	}
	if requestIDIndexCount != 1 {
		t.Fatalf("bootstrapped request_id indexes = %d, want one governed replay lookup index", requestIDIndexCount)
	}
	var requestIDIndexDefinition string
	if err := database.pool.QueryRow(t.Context(), `
SELECT indexdef
FROM pg_indexes
WHERE schemaname = current_schema()
  AND tablename = 'batch_tickets'
  AND indexname = $1
`, batchreplay.LookupIndexName).Scan(&requestIDIndexDefinition); err != nil {
		t.Fatalf("query bootstrapped replay lookup index: %v", err)
	}
	for _, fragment := range []string{"created_by", "batch_type", "sha256", "btrim", "request_id IS NOT NULL"} {
		if !strings.Contains(requestIDIndexDefinition, fragment) {
			t.Fatalf("replay lookup index = %q, want fragment %q", requestIDIndexDefinition, fragment)
		}
	}
	var baselineCount int
	if err := database.pool.QueryRow(t.Context(), `
SELECT count(*) FROM atlas_schema_revisions WHERE version = $1
`, fakeAtlas.version).Scan(&baselineCount); err != nil {
		t.Fatalf("query Atlas baseline: %v", err)
	}
	if baselineCount != 1 {
		t.Fatalf("Atlas baseline count for %q = %d, want 1", fakeAtlas.version, baselineCount)
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 1 {
		t.Fatalf("fake Atlas calls = %#v, want one migrate set call", calls)
	}
	assertStartupMigrationAtlasCall(t, calls[0], "set", false)
	if got := calls[0][len(calls[0])-1]; got != fakeAtlas.version {
		t.Fatalf("migrate set version = %q, want %q", got, fakeAtlas.version)
	}
}

func TestEnsureStartupMigrations_LegacyCoreSchemaUsesDirtyAdoption(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "legacy")
	database.createCoreSchemaMarker(t)
	fakeAtlas := configureStartupMigrationFakeAtlas(t)

	if err := EnsureStartupMigrations(t.Context(), database.config); err != nil {
		t.Fatalf("EnsureStartupMigrations() unexpected error: %v", err)
	}

	var marker string
	if err := database.pool.QueryRow(t.Context(), `SELECT marker FROM users WHERE id = 'legacy-user'`).Scan(&marker); err != nil {
		t.Fatalf("query preserved legacy row: %v", err)
	}
	if marker != "preserve-me" {
		t.Fatalf("legacy marker = %q, want preserve-me", marker)
	}
	var fixtureMigrationCount int
	if err := database.pool.QueryRow(t.Context(), `
SELECT count(*) FROM startup_fake_atlas_applied WHERE version = $1
`, fakeAtlas.version).Scan(&fixtureMigrationCount); err != nil {
		t.Fatalf("query fake Atlas migration side effect: %v", err)
	}
	if fixtureMigrationCount != 1 {
		t.Fatalf("fake Atlas migration side effects = %d, want 1", fixtureMigrationCount)
	}
	for _, tableName := range []string{atlasRevisionTable, "river_migration", "river_job"} {
		if !database.tableExists(t, tableName) {
			t.Fatalf("table %q is missing after legacy dirty adoption", tableName)
		}
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 1 {
		t.Fatalf("fake Atlas calls = %#v, want one dirty migrate apply call", calls)
	}
	assertStartupMigrationAtlasCall(t, calls[0], "apply", true)
}

func TestEnsureStartupMigrations_RejectsRevisionsWithoutCoreSchemaBeforeAtlas(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "invalidstate")
	database.createAtlasRevisionMarker(t, "existing")
	t.Setenv(atlasExecPathEnv, filepath.Join(t.TempDir(), "missing-atlas"))
	t.Setenv(atlasMigrationDirEnv, filepath.Join(t.TempDir(), "missing-migrations"))

	err := EnsureStartupMigrations(t.Context(), database.config)
	if err == nil {
		t.Fatal("EnsureStartupMigrations() error = nil, want revisions/core invariant failure")
	}
	if !strings.Contains(err.Error(), "missing core schema table \"users\"") {
		t.Fatalf("EnsureStartupMigrations() error = %v, want missing core schema context", err)
	}
	if database.tableExists(t, coreSchemaTable) {
		t.Fatal("core schema was created for invalid revisions-without-core state")
	}
	if database.tableExists(t, "river_migration") || database.tableExists(t, "river_job") {
		t.Fatal("River schema was created despite invalid database migration state")
	}
}

func TestEnsureStartupMigrations_AtlasSetFailureStopsBeforeRiver(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "setfail")
	fakeAtlas := configureStartupMigrationFakeAtlas(t)
	t.Setenv(fakeAtlasFailCommandEnv, "set")

	err := EnsureStartupMigrations(t.Context(), database.config)
	if err == nil {
		t.Fatal("EnsureStartupMigrations() error = nil, want Atlas set failure")
	}
	if !strings.Contains(err.Error(), "atlas migrate set: fake atlas set failure") {
		t.Fatalf("EnsureStartupMigrations() error = %v, want Atlas set failure context", err)
	}
	if !database.tableExists(t, coreSchemaTable) {
		t.Fatal("Ent bootstrap did not complete before the injected Atlas set failure")
	}
	if database.tableExists(t, atlasRevisionTable) {
		t.Fatal("Atlas revision table exists after injected migrate set failure")
	}
	if database.tableExists(t, "river_migration") || database.tableExists(t, "river_job") {
		t.Fatal("River migrations ran after Atlas set failure")
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 1 {
		t.Fatalf("fake Atlas calls = %#v, want one failing migrate set call", calls)
	}
	assertStartupMigrationAtlasCall(t, calls[0], "set", false)
}

func TestEnsureStartupMigrations_AtlasApplyFailureStopsBeforeRiver(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "applyfail")
	database.createCoreSchemaMarker(t)
	fakeAtlas := configureStartupMigrationFakeAtlas(t)
	t.Setenv(fakeAtlasFailCommandEnv, "apply")

	err := EnsureStartupMigrations(t.Context(), database.config)
	if err == nil {
		t.Fatal("EnsureStartupMigrations() error = nil, want Atlas apply failure")
	}
	if !strings.Contains(err.Error(), "atlas migrate apply: fake atlas apply failure") {
		t.Fatalf("EnsureStartupMigrations() error = %v, want Atlas apply failure context", err)
	}
	if database.tableExists(t, atlasRevisionTable) {
		t.Fatal("Atlas revision table exists after injected migrate apply failure")
	}
	if database.tableExists(t, "river_migration") || database.tableExists(t, "river_job") {
		t.Fatal("River migrations ran after Atlas apply failure")
	}
	var marker string
	if err := database.pool.QueryRow(t.Context(), `SELECT marker FROM users WHERE id = 'legacy-user'`).Scan(&marker); err != nil {
		t.Fatalf("query legacy row after Atlas failure: %v", err)
	}
	if marker != "preserve-me" {
		t.Fatalf("legacy marker = %q after Atlas failure, want preserve-me", marker)
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 1 {
		t.Fatalf("fake Atlas calls = %#v, want one failing migrate apply call", calls)
	}
	assertStartupMigrationAtlasCall(t, calls[0], "apply", true)
}

func TestEnsureStartupMigrations_RiverFailureIsReturned(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "riverfail")
	database.createCoreSchemaMarker(t)
	database.createAtlasRevisionMarker(t, "existing")
	if _, err := database.pool.Exec(t.Context(), `CREATE TABLE river_migration (broken text)`); err != nil {
		t.Fatalf("create incompatible River migration table: %v", err)
	}
	fakeAtlas := configureStartupMigrationFakeAtlas(t)

	err := EnsureStartupMigrations(t.Context(), database.config)
	if err == nil {
		t.Fatal("EnsureStartupMigrations() error = nil, want River migration failure")
	}
	if !strings.Contains(err.Error(), "river migrate up") {
		t.Fatalf("EnsureStartupMigrations() error = %v, want River migration context", err)
	}
	if database.tableExists(t, "river_job") {
		t.Fatal("river_job exists after incompatible River migration state")
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 1 {
		t.Fatalf("fake Atlas calls = %#v, want one normal migrate apply call", calls)
	}
	assertStartupMigrationAtlasCall(t, calls[0], "apply", false)
}

func TestEnsureStartupMigrations_ConcurrentEmptyDatabaseCallsConverge(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "concurrent")
	fakeAtlas := configureStartupMigrationFakeAtlas(t)

	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	lockConn, acquireErr := database.pool.Acquire(ctx)
	if acquireErr != nil {
		t.Fatalf("acquire startup migration blocker connection: %v", acquireErr)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(cleanupCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, startupMigrationLockKey); unlockErr != nil {
				_ = lockConn.Conn().Close(cleanupCtx)
			}
		}
		lockConn.Release()
	})
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, startupMigrationLockKey); err != nil {
		t.Fatalf("hold startup migration advisory lock: %v", err)
	}
	lockHeld = true
	var blockerPID int32
	if err := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatalf("query startup migration blocker pid: %v", err)
	}

	start := make(chan struct{})
	var group errgroup.Group
	for range 2 {
		group.Go(func() error {
			<-start
			return EnsureStartupMigrations(ctx, database.config)
		})
	}
	close(start)
	releaseBlockerAndWait := func() error {
		if lockHeld {
			releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if _, releaseErr := lockConn.Exec(releaseCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, startupMigrationLockKey); releaseErr != nil {
				_ = lockConn.Conn().Close(releaseCtx)
			}
			releaseCancel()
			lockHeld = false
		}
		return group.Wait()
	}

	blockedCalls := 0
	var activityErr error
	require.Eventually(t, func() bool {
		activityErr = database.pool.QueryRow(ctx, `
SELECT count(*)
FROM pg_stat_activity AS activity
WHERE activity.datname = current_database()
  AND activity.state = 'active'
  AND $1 = ANY(pg_blocking_pids(activity.pid))
  AND activity.query LIKE '%pg_advisory_lock%'
`, blockerPID).Scan(&blockedCalls)
		return activityErr != nil || blockedCalls == 2
	}, 10*time.Second, 10*time.Millisecond, "concurrent startup migrations did not block on the outer lock")
	if activityErr != nil {
		if migrationErr := releaseBlockerAndWait(); migrationErr != nil {
			t.Fatalf("query blocked startup migration calls: %v; concurrent migration returned: %v", activityErr, migrationErr)
		}
		t.Fatalf("query blocked startup migration calls: %v", activityErr)
	}
	if blockedCalls != 2 {
		if migrationErr := releaseBlockerAndWait(); migrationErr != nil {
			t.Fatalf("blocked startup migration calls = %d, want 2 before releasing the outer lock; concurrent migration returned: %v", blockedCalls, migrationErr)
		}
		t.Fatalf("blocked startup migration calls = %d, want 2 before releasing the outer lock", blockedCalls)
	}
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, startupMigrationLockKey); err != nil {
		if migrationErr := releaseBlockerAndWait(); migrationErr != nil {
			t.Fatalf("release startup migration advisory lock: %v; concurrent migration returned: %v", err, migrationErr)
		}
		t.Fatalf("release startup migration advisory lock: %v", err)
	}
	lockHeld = false

	if migrationErr := group.Wait(); migrationErr != nil {
		t.Fatalf("concurrent EnsureStartupMigrations() returned error: %v", migrationErr)
	}
	for _, tableName := range []string{coreSchemaTable, atlasRevisionTable, "river_migration", "river_job"} {
		if !database.tableExists(t, tableName) {
			t.Fatalf("table %q is missing after concurrent startup migrations", tableName)
		}
	}
	var revisionCount int
	if err := database.pool.QueryRow(t.Context(), `SELECT count(*) FROM atlas_schema_revisions`).Scan(&revisionCount); err != nil {
		t.Fatalf("count Atlas revisions after concurrent startup: %v", err)
	}
	if revisionCount < 1 {
		t.Fatal("no Atlas revision was recorded after concurrent startup")
	}

	calls := fakeAtlas.calls(t)
	if len(calls) != 2 {
		t.Fatalf("fake Atlas calls = %#v, want two completed startup paths", calls)
	}
	var setCalls, applyCalls int
	for _, call := range calls {
		if len(call) < 2 || call[0] != "migrate" || call[1] != "set" && call[1] != "apply" {
			t.Fatalf("unexpected concurrent Atlas call: %#v", call)
		}
		switch call[1] {
		case "set":
			setCalls++
			assertStartupMigrationAtlasCall(t, call, "set", false)
		case "apply":
			applyCalls++
			assertStartupMigrationAtlasCall(t, call, "apply", false)
		}
	}
	if setCalls != 1 || applyCalls != 1 {
		t.Fatalf("concurrent Atlas calls = %d set/%d apply, want one baseline followed by one lock-internal recheck apply; calls=%#v", setCalls, applyCalls, calls)
	}
}

func TestBatchRetryIdempotencyMigrationUpgradesLegacyRows(t *testing.T) {
	database := newStartupMigrationTestDatabase(t, "batchretry")
	if _, err := database.pool.Exec(t.Context(), `
CREATE TABLE tickets (
    id text PRIMARY KEY,
    status text NOT NULL,
    parent_ticket_id text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);
CREATE TABLE batch_tickets (
    id text PRIMARY KEY,
    batch_type text NOT NULL,
    request_id text,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL
);
INSERT INTO tickets (id, status, parent_ticket_id, created_at, updated_at) VALUES
    ('pending-child', 'PENDING', 'parent', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
    ('failed-child', 'FAILED', 'parent', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z');
INSERT INTO batch_tickets (id, batch_type, request_id, created_by, created_at) VALUES
    ('batch-oldest', 'BATCH_POWER', 'same-key', 'actor', '2026-01-01T00:00:00Z'),
    ('batch-newer', 'BATCH_POWER', 'same-key', 'actor', '2026-01-02T00:00:00Z'),
    ('batch-empty', 'BATCH_POWER', '', 'actor', '2026-01-03T00:00:00Z');
`); err != nil {
		t.Fatalf("create legacy batch schema: %v", err)
	}
	legacyLongRequestID := strings.Repeat("😀", 513)
	legacyVeryLongRequestID := strings.Repeat("😀", 4096)
	if _, err := database.pool.Exec(t.Context(), `
INSERT INTO batch_tickets (id, batch_type, request_id, created_by, created_at) VALUES
    ('batch-long', 'BATCH_DELETE', $1, 'actor', '2026-01-04T00:00:00Z'),
    ('batch-very-long', 'BATCH_DELETE', $2, 'actor', '2026-01-05T00:00:00Z')
`, legacyLongRequestID, legacyVeryLongRequestID); err != nil {
		t.Fatalf("insert legacy long batch keys: %v", err)
	}

	migrationPath := filepath.Join("..", "..", "migrations", "atlas", "20260718000100_batch_retry_idempotency.sql")
	migrationSQL, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read batch retry migration: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := database.pool.Exec(t.Context(), string(migrationSQL)); err != nil {
			t.Fatalf("apply batch retry migration attempt %d: %v", attempt, err)
		}
	}

	var pendingAttempts, failedAttempts int
	var failedAttemptAt time.Time
	if err := database.pool.QueryRow(t.Context(), `
SELECT
    (SELECT attempt_count FROM tickets WHERE id = 'pending-child'),
    (SELECT attempt_count FROM tickets WHERE id = 'failed-child'),
    (SELECT last_attempt_at FROM tickets WHERE id = 'failed-child')
`).Scan(&pendingAttempts, &failedAttempts, &failedAttemptAt); err != nil {
		t.Fatalf("query migrated ticket attempts: %v", err)
	}
	if pendingAttempts != 0 || failedAttempts != 1 {
		t.Fatalf("migrated attempts = pending:%d failed:%d, want 0/1", pendingAttempts, failedAttempts)
	}
	wantFailedAttemptAt := time.Date(2026, time.January, 2, 0, 0, 0, 0, time.UTC)
	if !failedAttemptAt.Equal(wantFailedAttemptAt) {
		t.Fatalf("failed last_attempt_at = %s, want %s", failedAttemptAt, wantFailedAttemptAt)
	}

	var oldestRequestID, newerRequestID, emptyRequestID, longStoredRequestID, veryLongStoredRequestID *string
	if err := database.pool.QueryRow(t.Context(), `
SELECT
    (SELECT request_id FROM batch_tickets WHERE id = 'batch-oldest'),
    (SELECT request_id FROM batch_tickets WHERE id = 'batch-newer'),
    (SELECT request_id FROM batch_tickets WHERE id = 'batch-empty'),
    (SELECT request_id FROM batch_tickets WHERE id = 'batch-long'),
    (SELECT request_id FROM batch_tickets WHERE id = 'batch-very-long')
`).Scan(&oldestRequestID, &newerRequestID, &emptyRequestID, &longStoredRequestID, &veryLongStoredRequestID); err != nil {
		t.Fatalf("query preserved batch keys: %v", err)
	}
	if oldestRequestID == nil || *oldestRequestID != "same-key" ||
		newerRequestID == nil || *newerRequestID != "same-key" ||
		emptyRequestID == nil || *emptyRequestID != "" {
		t.Fatalf("historical request IDs changed = oldest:%v newer:%v empty:%v", oldestRequestID, newerRequestID, emptyRequestID)
	}
	if longStoredRequestID == nil || *longStoredRequestID != legacyLongRequestID ||
		veryLongStoredRequestID == nil || *veryLongStoredRequestID != legacyVeryLongRequestID {
		t.Fatalf("long request IDs were not preserved: long:%v very-long:%v", longStoredRequestID != nil, veryLongStoredRequestID != nil)
	}
	var requestIDMaxLength *int
	if err := database.pool.QueryRow(t.Context(), `
SELECT character_maximum_length
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'batch_tickets'
  AND column_name = 'request_id'
`).Scan(&requestIDMaxLength); err != nil {
		t.Fatalf("query migrated request_id column length: %v", err)
	}
	if requestIDMaxLength != nil {
		t.Fatalf("migrated request_id maximum length = %d, want unbounded text", *requestIDMaxLength)
	}
	var replayedBatchID string
	if err := database.pool.QueryRow(t.Context(), `
SELECT id
FROM batch_tickets
WHERE created_by = 'actor'
  AND batch_type = 'BATCH_DELETE'
  AND request_id = $1
`, legacyVeryLongRequestID).Scan(&replayedBatchID); err != nil {
		t.Fatalf("replay exact historical long request ID: %v", err)
	}
	if replayedBatchID != "batch-very-long" {
		t.Fatalf("historical long-key replay ID = %q, want batch-very-long", replayedBatchID)
	}
}

func assertStartupMigrationAtlasCall(
	t *testing.T,
	call []string,
	command string,
	wantAllowDirty bool,
) {
	t.Helper()

	if len(call) < 2 || call[0] != "migrate" || call[1] != command {
		t.Fatalf("Atlas call = %#v, want migrate %s", call, command)
	}
	if got := startupMigrationHasArg(call, "--url"); got {
		t.Fatalf("Atlas call contains --url; database credentials must not be passed through argv: %#v", call)
	}
	if got := startupMigrationHasArg(call, "--dir"); got {
		t.Fatalf("Atlas call contains --dir; startup configuration must supply it through the environment: %#v", call)
	}
	if got, ok := startupMigrationFlagValue(call, "--env"); !ok || got != atlasStartupEnvironment {
		t.Fatalf("Atlas --env = %q/%v, want %q", got, ok, atlasStartupEnvironment)
	}
	if got, ok := startupMigrationFlagValue(call, "--config"); !ok || !strings.HasPrefix(got, "file://") {
		t.Fatalf("Atlas --config = %q/%v, want temporary file URL", got, ok)
	}
	if got := startupMigrationHasArg(call, "--allow-dirty"); got != wantAllowDirty {
		t.Fatalf("Atlas --allow-dirty present = %v, want %v; argv=%#v", got, wantAllowDirty, call)
	}
	if command == "apply" {
		if got, ok := startupMigrationFlagValue(call, "--format"); !ok || got != "{{ json . }}" {
			t.Fatalf("Atlas --format = %q/%v, want JSON format; argv=%#v", got, ok, call)
		}
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}
