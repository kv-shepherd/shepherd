package infrastructure

import (
	"os"
	"path/filepath"
	"testing"
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
