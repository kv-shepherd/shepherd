package specembed

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalSpecEmbedded(t *testing.T) {
	if len(CanonicalSpec) == 0 {
		t.Fatal("embedded canonical OpenAPI spec is empty")
	}
}

func TestCanonicalSpecMatchesSourceFile(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "..", "api", "openapi.yaml")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read canonical source spec: %v", err)
	}
	if !bytes.Equal(CanonicalSpec, source) {
		t.Fatalf("embedded spec mismatch: %s is not in sync with embedded bytes", sourcePath)
	}
}
