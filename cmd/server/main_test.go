package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun_ReturnsConfigValidationErrorForShortSessionSecret(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("security:\n  session_secret: short\n"), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	chdirErr := os.Chdir(tempDir)
	if chdirErr != nil {
		t.Fatalf("chdir temp dir: %v", chdirErr)
	}

	runErr := run()
	if runErr == nil {
		t.Fatal("run() error = nil, want config validation failure")
	}
	if !strings.Contains(runErr.Error(), "validate config") {
		t.Fatalf("run() error = %q, want validate config failure", runErr.Error())
	}
}
