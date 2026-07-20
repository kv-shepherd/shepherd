package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ariga.io/atlas/atlasexec"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	atlasExecPathEnv         = "ATLAS_EXEC_PATH"
	atlasMigrationDirEnv     = "ATLAS_MIGRATION_DIR"
	atlasRevisionTable       = "atlas_schema_revisions"
	bundledAtlasExecPath     = "/usr/local/bin/atlas"
	bundledAtlasMigrations   = "/usr/local/share/shepherd/migrations/atlas"
	coreSchemaTable          = "users"
	startupMigrationLockKey  = "shepherd:startup-migrations"
	atlasStartupEnvironment  = "shepherd-startup"
	atlasStartupDatabaseURL  = "SHEPHERD_ATLAS_DATABASE_URL"
	atlasStartupMigrationDir = "SHEPHERD_ATLAS_MIGRATION_DIR_URL"
)

const atlasStartupConfig = `
variable "database_url" {
  type    = string
  default = getenv("SHEPHERD_ATLAS_DATABASE_URL")
}

variable "migration_dir" {
  type    = string
  default = getenv("SHEPHERD_ATLAS_MIGRATION_DIR_URL")
}

env {
  name = atlas.env
  url  = var.database_url
  migration {
    dir = var.migration_dir
  }
}
`

type databaseMigrationState struct {
	HasCoreSchema     bool
	HasAtlasRevisions bool
}

// EnsureStartupMigrations keeps the application schema and River tables current
// before the main application bootstrap opens long-lived DB clients.
func EnsureStartupMigrations(ctx context.Context, cfg config.DatabaseConfig) error {
	if !cfg.AutoApplyVersionedMigrations {
		return nil
	}

	dsn := cfg.DSN()
	lockConfig, err := startupMigrationLockConfig(cfg)
	if err != nil {
		return fmt.Errorf("resolve startup migration lock endpoint: %w", err)
	}
	// Atlas also uses session-level PostgreSQL advisory locks internally. Give it
	// the exact session-capable endpoint used by the outer startup lock; sending
	// Atlas through a PgBouncer transaction pool would let its lock and unlock
	// queries reach different PostgreSQL backends.
	atlasDSN := lockConfig.ConnString()
	releaseLock, err := acquireStartupMigrationLock(ctx, lockConfig)
	if err != nil {
		return fmt.Errorf("acquire startup migration lock: %w", err)
	}
	defer releaseLock()

	state, err := inspectDatabaseMigrationState(ctx, dsn)
	if err != nil {
		return fmt.Errorf("inspect database migration state: %w", err)
	}
	if !state.HasCoreSchema && state.HasAtlasRevisions {
		return fmt.Errorf("database has %s but missing core schema table %q", atlasRevisionTable, coreSchemaTable)
	}

	migrationDir, err := resolveAtlasMigrationDir()
	if err != nil {
		return err
	}
	atlasExecPath, err := resolveAtlasExecPath()
	if err != nil {
		return err
	}

	logger.Info("Preparing database schema before server bootstrap",
		zap.Bool("has_core_schema", state.HasCoreSchema),
		zap.Bool("has_atlas_revisions", state.HasAtlasRevisions),
		zap.String("atlas_migration_dir", migrationDir),
	)

	if !state.HasAtlasRevisions {
		if shouldBootstrapCurrentSchemaBeforeAtlas(state) {
			logger.Info("Bootstrapping empty database schema with Ent before setting Atlas baseline")
			if err := bootstrapCurrentSchema(ctx, cfg); err != nil {
				return err
			}

			latestVersion, err := latestMigrationVersion(migrationDir)
			if err != nil {
				return err
			}
			if err := setAtlasVersion(ctx, atlasExecPath, migrationDir, atlasDSN, latestVersion); err != nil {
				return err
			}
		} else {
			logger.Info("Adopting existing database schema with Atlas revisions")
			if err := applyAtlasMigrations(ctx, atlasExecPath, migrationDir, atlasDSN, atlasexec.MigrateApplyParams{
				AllowDirty: true,
			}); err != nil {
				return err
			}
		}
	} else {
		if err := applyAtlasMigrations(ctx, atlasExecPath, migrationDir, atlasDSN, atlasexec.MigrateApplyParams{}); err != nil {
			return err
		}
	}

	if err := applyRiverStartupMigrations(ctx, cfg); err != nil {
		return err
	}
	return nil
}

// startupMigrationLockConfig uses the session-capable endpoint when a PgBouncer
// dual-pool deployment provides one. Session advisory locks are unsafe through
// a transaction-pool endpoint because lock and unlock may reach different
// PostgreSQL backends. WorkerHost is a trusted deployment contract: it must be a
// single session-capable endpoint for the same PostgreSQL cluster, database, and
// role as the primary DSN.
func startupMigrationLockConfig(cfg config.DatabaseConfig) (*pgx.ConnConfig, error) {
	dsn := cfg.DSN()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		// pgx parse failures may include the input DSN. Do not let a malformed
		// operator-provided URL copy credentials into startup logs.
		return nil, fmt.Errorf("parse database connection configuration: invalid PostgreSQL DSN")
	}

	workerHost := strings.TrimSpace(cfg.WorkerHost)
	if workerHost == "" {
		return parsed, nil
	}
	if cfg.WorkerPort < 0 || cfg.WorkerPort > 65535 {
		return nil, fmt.Errorf("worker database port %d is outside the valid range", cfg.WorkerPort)
	}
	workerPort := parsed.Port
	if cfg.WorkerPort > 0 {
		workerPort = uint16(cfg.WorkerPort)
	}
	sessionDSN, err := startupMigrationSessionDSN(dsn, workerHost, workerPort)
	if err != nil {
		return nil, err
	}
	parsed, err = pgx.ParseConfig(sessionDSN)
	if err != nil {
		return nil, fmt.Errorf("parse session-capable database connection configuration: invalid PostgreSQL DSN")
	}
	return parsed, nil
}

func startupMigrationSessionDSN(dsn, workerHost string, workerPort uint16) (string, error) {
	workerHost = strings.TrimSpace(workerHost)
	if workerHost == "" {
		return "", fmt.Errorf("worker database host is required")
	}
	if strings.Contains(workerHost, ",") {
		return "", fmt.Errorf("worker database host must be one session-capable endpoint")
	}
	if strings.HasPrefix(workerHost, "[") || strings.HasSuffix(workerHost, "]") {
		if !strings.HasPrefix(workerHost, "[") || !strings.HasSuffix(workerHost, "]") {
			return "", fmt.Errorf("worker database host has invalid IPv6 brackets")
		}
		unbracketed := strings.TrimSuffix(strings.TrimPrefix(workerHost, "["), "]")
		if net.ParseIP(unbracketed) == nil {
			return "", fmt.Errorf("worker database host has invalid IPv6 brackets")
		}
		workerHost = unbracketed
	}
	if workerPort == 0 {
		return "", fmt.Errorf("worker database port is required")
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil || parsed.Scheme == "" {
			return "", fmt.Errorf("rewrite session-capable database URL: invalid PostgreSQL URL")
		}
		parsed.Host = net.JoinHostPort(workerHost, fmt.Sprintf("%d", workerPort))
		// URI query endpoint overrides would otherwise win over the authority
		// rewritten above. All non-endpoint options remain untouched.
		query := parsed.Query()
		query.Del("host")
		query.Del("hostaddr")
		query.Del("port")
		parsed.RawQuery = query.Encode()
		return parsed.String(), nil
	}

	// Strip every endpoint selector, including libpq's hostaddr. Merely
	// appending host/port would leave hostaddr able to route Atlas or the outer
	// lock to the transaction endpoint. Non-endpoint settings retain their
	// original quoted representation, and the replacement host is quoted so it
	// cannot inject another conninfo setting.
	preserved, err := stripPostgresKeywordEndpoint(dsn)
	if err != nil {
		return "", fmt.Errorf("rewrite session-capable keyword DSN: invalid PostgreSQL conninfo")
	}
	parts := make([]string, 0, 3)
	if preserved != "" {
		parts = append(parts, preserved)
	}
	parts = append(parts,
		"host="+quotePostgresKeywordValue(workerHost),
		fmt.Sprintf("port=%d", workerPort),
	)
	return strings.Join(parts, " "), nil
}

func quotePostgresKeywordValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return `'` + value + `'`
}

func stripPostgresKeywordEndpoint(dsn string) (string, error) {
	var preserved []string
	for offset := 0; ; {
		offset = skipPostgresKeywordSpace(dsn, offset)
		if offset == len(dsn) {
			break
		}
		settingStart := offset
		relativeEquals := strings.IndexByte(dsn[offset:], '=')
		if relativeEquals < 0 {
			return "", fmt.Errorf("keyword setting is missing equals sign")
		}
		equals := offset + relativeEquals
		key := strings.TrimSpace(dsn[offset:equals])
		if key == "" {
			return "", fmt.Errorf("keyword setting name is empty")
		}
		offset = skipPostgresKeywordSpace(dsn, equals+1)
		if offset < len(dsn) && dsn[offset] == '\'' {
			offset++
			closed := false
			for offset < len(dsn) {
				switch dsn[offset] {
				case '\\':
					offset += 2
				case '\'':
					offset++
					closed = true
				default:
					offset++
				}
				if closed {
					break
				}
			}
			if !closed {
				return "", fmt.Errorf("keyword setting has unterminated quoted value")
			}
		} else {
			for offset < len(dsn) && !isPostgresKeywordSpace(dsn[offset]) {
				if dsn[offset] == '\\' {
					offset += 2
				} else {
					offset++
				}
				if offset > len(dsn) {
					return "", fmt.Errorf("keyword setting has invalid trailing escape")
				}
			}
		}
		settingEnd := offset
		switch strings.ToLower(key) {
		case "host", "hostaddr", "port":
		default:
			preserved = append(preserved, strings.TrimSpace(dsn[settingStart:settingEnd]))
		}
	}
	return strings.Join(preserved, " "), nil
}

func skipPostgresKeywordSpace(value string, offset int) int {
	for offset < len(value) && isPostgresKeywordSpace(value[offset]) {
		offset++
	}
	return offset
}

func isPostgresKeywordSpace(value byte) bool {
	switch value {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func acquireStartupMigrationLock(ctx context.Context, cfg *pgx.ConnConfig) (func(), error) {
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open migration lock connection: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, startupMigrationLockKey); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = conn.Close(closeCtx)
		cancel()
		return nil, fmt.Errorf("lock startup migrations: %w", err)
	}

	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if _, unlockErr := conn.Exec(unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, startupMigrationLockKey); unlockErr != nil {
			logger.Error("failed to release startup migration lock", zap.Error(unlockErr))
		}
		cancel()

		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if closeErr := conn.Close(closeCtx); closeErr != nil {
			logger.Error("failed to close startup migration lock connection", zap.Error(closeErr))
		}
		closeCancel()
	}, nil
}

func shouldBootstrapCurrentSchemaBeforeAtlas(state databaseMigrationState) bool {
	return !state.HasCoreSchema && !state.HasAtlasRevisions
}

func inspectDatabaseMigrationState(ctx context.Context, dsn string) (databaseMigrationState, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return databaseMigrationState{}, fmt.Errorf("open inspection pool: %w", err)
	}
	defer pool.Close()

	state := databaseMigrationState{}
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, coreSchemaTable).Scan(&state.HasCoreSchema); err != nil {
		return databaseMigrationState{}, fmt.Errorf("check core schema table: %w", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1
		)
	`, atlasRevisionTable).Scan(&state.HasAtlasRevisions); err != nil {
		return databaseMigrationState{}, fmt.Errorf("check atlas revision table: %w", err)
	}
	return state, nil
}

func bootstrapCurrentSchema(ctx context.Context, cfg config.DatabaseConfig) error {
	db, err := NewDatabaseClients(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open database for clean bootstrap: %w", err)
	}
	defer db.Close()

	if err := db.ApplyEntSchema(ctx); err != nil {
		return err
	}
	return nil
}

func applyRiverStartupMigrations(ctx context.Context, cfg config.DatabaseConfig) error {
	db, err := NewDatabaseClients(ctx, cfg)
	if err != nil {
		return fmt.Errorf("open database for river migrations: %w", err)
	}
	defer db.Close()

	return db.ApplyRiverMigrations(ctx)
}

func applyAtlasMigrations(ctx context.Context, atlasExecPath, migrationDir, dsn string, params atlasexec.MigrateApplyParams) error {
	stderr := &bytes.Buffer{}
	client, err := atlasexec.NewClient(filepath.Dir(migrationDir), atlasExecPath)
	if err != nil {
		return fmt.Errorf("init atlas client: %w", err)
	}
	client.SetStderr(stderr)
	configURL, cleanup, err := configureStartupAtlasClient(client, dsn, migrationDir)
	if err != nil {
		return err
	}
	defer cleanup()

	params.URL = ""
	params.DirURL = ""
	params.Env = atlasStartupEnvironment
	params.ConfigURL = configURL
	result, err := client.MigrateApply(ctx, &params)
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("atlas migrate apply: %s", redactAtlasMigrationError(msg, dsn))
	}

	logger.Info("Atlas migration check completed",
		zap.String("current_version", result.Current),
		zap.String("target_version", result.Target),
		zap.Int("applied_files", len(result.Applied)),
	)
	return nil
}

func setAtlasVersion(ctx context.Context, atlasExecPath, migrationDir, dsn, version string) error {
	stderr := &bytes.Buffer{}
	client, err := atlasexec.NewClient(filepath.Dir(migrationDir), atlasExecPath)
	if err != nil {
		return fmt.Errorf("init atlas client: %w", err)
	}
	client.SetStderr(stderr)
	configURL, cleanup, err := configureStartupAtlasClient(client, dsn, migrationDir)
	if err != nil {
		return err
	}
	defer cleanup()

	if err := client.MigrateSet(ctx, &atlasexec.MigrateSetParams{
		ConfigURL: configURL,
		Env:       atlasStartupEnvironment,
		Version:   version,
	}); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("atlas migrate set: %s", redactAtlasMigrationError(msg, dsn))
	}

	logger.Info("Atlas baseline recorded for bootstrapped database",
		zap.String("version", version),
	)
	return nil
}

func configureStartupAtlasClient(
	client *atlasexec.Client,
	dsn, migrationDir string,
) (configURL string, cleanup func(), err error) {
	if client == nil {
		return "", nil, fmt.Errorf("atlas client is required")
	}
	configFile, err := os.CreateTemp("", "shepherd-atlas-startup-*.hcl")
	if err != nil {
		return "", nil, fmt.Errorf("create temporary atlas config: %w", err)
	}
	configPath := configFile.Name()
	cleanup = func() { _ = os.Remove(configPath) }
	if _, err := configFile.WriteString(atlasStartupConfig); err != nil {
		_ = configFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("write temporary atlas config: %w", err)
	}
	if err := configFile.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close temporary atlas config: %w", err)
	}

	environment := atlasexec.NewOSEnviron()
	// atlasexec always injects these two values and rejects caller overrides.
	// Remove inherited copies before SetEnv so operator environments that set
	// them explicitly do not make startup configuration fail.
	delete(environment, "ATLAS_NO_UPDATE_NOTIFIER")
	delete(environment, "ATLAS_NO_UPGRADE_SUGGESTIONS")
	environment[atlasStartupDatabaseURL] = dsn
	environment[atlasStartupMigrationDir] = fileURL(migrationDir)
	if err := client.SetEnv(environment); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("configure atlas environment: %w", err)
	}
	return fileURL(configPath), cleanup, nil
}

func redactAtlasMigrationError(message, dsn string) string {
	message = strings.TrimSpace(message)
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return message
	}
	message = strings.ReplaceAll(message, dsn, "[redacted]")

	// Atlas and database drivers may render a connection URL differently from
	// the operator-provided DSN. Redact decoded and URL-encoded password forms
	// as well as the exact DSN so normalized error messages cannot leak only the
	// credential portion.
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil || parsed.Password == "" {
		return message
	}
	queryEscapedPassword := url.QueryEscape(parsed.Password)
	userInfoEscapedPassword := strings.TrimPrefix(url.UserPassword("", parsed.Password).String(), ":")
	passwordForms := []string{
		parsed.Password,
		queryEscapedPassword,
		lowercaseURLPercentEscapes(queryEscapedPassword),
		userInfoEscapedPassword,
		lowercaseURLPercentEscapes(userInfoEscapedPassword),
	}
	for _, password := range passwordForms {
		if password != "" {
			message = strings.ReplaceAll(message, password, "[redacted]")
		}
	}
	return message
}

func lowercaseURLPercentEscapes(value string) string {
	var normalized strings.Builder
	normalized.Grow(len(value))
	for index := 0; index < len(value); {
		if value[index] == '%' && index+2 < len(value) {
			normalized.WriteByte('%')
			normalized.WriteString(strings.ToLower(value[index+1 : index+3]))
			index += 3
			continue
		}
		normalized.WriteByte(value[index])
		index++
	}
	return normalized.String()
}

func resolveAtlasExecPath() (string, error) {
	exePath, _ := os.Executable()
	candidates := atlasExecPathCandidates(os.Getenv(atlasExecPathEnv), exePath)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		// #nosec G703 -- Atlas executable path comes from operator config or bundled install locations.
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("atlas executable not found; set %s or bundle atlas with the runtime artifact", atlasExecPathEnv)
}

func atlasExecPathCandidates(envPath, exePath string) []string {
	candidates := []string{
		strings.TrimSpace(envPath),
		bundledAtlasExecPath,
	}
	if strings.TrimSpace(exePath) != "" {
		candidates = append(candidates, filepath.Join(filepath.Dir(exePath), "atlas"))
	}
	return candidates
}

func resolveAtlasMigrationDir() (string, error) {
	cwd, _ := os.Getwd()
	exePath, _ := os.Executable()
	candidates := atlasMigrationDirCandidates(os.Getenv(atlasMigrationDirEnv), cwd, exePath)
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if isAtlasMigrationDir(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("atlas migration directory not found; set %s or bundle migrations with the runtime artifact", atlasMigrationDirEnv)
}

func atlasMigrationDirCandidates(envPath, cwd, exePath string) []string {
	candidates := []string{
		strings.TrimSpace(envPath),
		bundledAtlasMigrations,
	}
	if strings.TrimSpace(cwd) != "" {
		candidates = append(candidates, filepath.Join(cwd, "migrations", "atlas"))
	}
	if strings.TrimSpace(exePath) != "" {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "share", "shepherd", "migrations", "atlas"),
			filepath.Join(exeDir, "..", "share", "shepherd", "migrations", "atlas"),
			filepath.Join(exeDir, "..", "..", "migrations", "atlas"),
		)
	}
	return candidates
}

func isAtlasMigrationDir(path string) bool {
	if path == "" {
		return false
	}
	// #nosec G703 -- Migration directory candidates are validated before use and must contain atlas.sum.
	if _, err := os.Stat(filepath.Join(path, "atlas.sum")); err != nil {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".sql") {
			return true
		}
	}
	return false
}

func latestMigrationVersion(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read atlas migration dir: %w", err)
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		files = append(files, entry.Name())
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no atlas migration files found in %s", path)
	}

	sort.Strings(files)
	latest := strings.TrimSuffix(files[len(files)-1], ".sql")
	if idx := strings.IndexByte(latest, '_'); idx > 0 {
		latest = latest[:idx]
	}
	if latest == "" {
		return "", fmt.Errorf("cannot derive atlas version from %s", files[len(files)-1])
	}
	return latest, nil
}

func fileURL(path string) string {
	return "file://" + filepath.ToSlash(path)
}
