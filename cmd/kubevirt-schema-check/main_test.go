package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareSemver(t *testing.T) {
	t.Parallel()

	if got := compareSemver("1.7.0", "1.8.0"); got >= 0 {
		t.Fatalf("compareSemver(1.7.0, 1.8.0) = %d, want < 0", got)
	}
	if got := compareSemver("1.8.0", "1.8.0"); got != 0 {
		t.Fatalf("compareSemver(1.8.0, 1.8.0) = %d, want 0", got)
	}
	if got := compareSemver("1.8.1", "1.8.0"); got <= 0 {
		t.Fatalf("compareSemver(1.8.1, 1.8.0) = %d, want > 0", got)
	}
}

func TestReadLocalVersionFromManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	data := []byte(`{"entities":{"instancesize":{"current_version":"kubevirt-v1.7.0"}}}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	got, err := readLocalVersionFromManifest(path)
	if err != nil {
		t.Fatalf("readLocalVersionFromManifest returned error: %v", err)
	}
	if got != "1.7.0" {
		t.Fatalf("readLocalVersionFromManifest = %q, want %q", got, "1.7.0")
	}
}
