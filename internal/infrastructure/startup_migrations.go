package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ariga.io/atlas/atlasexec"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	atlasExecPathEnv       = "ATLAS_EXEC_PATH"
	atlasMigrationDirEnv   = "ATLAS_MIGRATION_DIR"
	atlasRevisionTable     = "atlas_schema_revisions"
	bundledAtlasExecPath   = "/usr/local/bin/atlas"
	bundledAtlasMigrations = "/usr/local/share/shepherd/migrations/atlas"
	coreSchemaTable        = "users"
)

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
			if err := setAtlasVersion(ctx, atlasExecPath, migrationDir, dsn, latestVersion); err != nil {
				return err
			}
		} else {
			logger.Info("Adopting existing database schema with Atlas revisions")
			if err := applyAtlasMigrations(ctx, atlasExecPath, migrationDir, dsn, atlasexec.MigrateApplyParams{
				URL:        dsn,
				DirURL:     fileURL(migrationDir),
				AllowDirty: true,
			}); err != nil {
				return err
			}
		}
	} else {
		if err := applyAtlasMigrations(ctx, atlasExecPath, migrationDir, dsn, atlasexec.MigrateApplyParams{
			URL:    dsn,
			DirURL: fileURL(migrationDir),
		}); err != nil {
			return err
		}
	}

	if err := applyRiverStartupMigrations(ctx, cfg); err != nil {
		return err
	}
	return nil
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

	params.URL = dsn
	params.DirURL = fileURL(migrationDir)
	result, err := client.MigrateApply(ctx, &params)
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("atlas migrate apply: %s", msg)
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

	if err := client.MigrateSet(ctx, &atlasexec.MigrateSetParams{
		URL:     dsn,
		DirURL:  fileURL(migrationDir),
		Version: version,
	}); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("atlas migrate set: %s", msg)
	}

	logger.Info("Atlas baseline recorded for bootstrapped database",
		zap.String("version", version),
	)
	return nil
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
