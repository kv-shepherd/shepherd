package batchreplay

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
)

func TestNormalizePreservesOpaqueContentsAndTrimsBoundaryWhitespace(t *testing.T) {
	key := strings.Repeat("opaque-😀-", 512)
	if got := Normalize("\u3000\t" + key + "\u0085\n"); got != key {
		t.Fatalf("Normalize() length/content mismatch: got %d bytes, want %d", len(got), len(key))
	}
}

func TestDigestNormalizesOnlyBoundaryWhitespace(t *testing.T) {
	want := Digest("opaque\u3000inside")
	got := Digest("\u3000\topaque\u3000inside\u0085\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("Digest() differs for boundary-whitespace variants")
	}
	if len(got) != sha256.Size {
		t.Fatalf("Digest() length = %d, want %d", len(got), sha256.Size)
	}
}
