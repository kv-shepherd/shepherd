package usecase

import "testing"

func TestBatchIdempotencyLockKeyUsesPersistedReplayNormalization(t *testing.T) {
	plain := BatchIdempotencyLockKey("actor-1", "POWER_START", "opaque-key")
	padded := BatchIdempotencyLockKey(
		"actor-1",
		"START",
		"\u3000\topaque-key\u0085\n",
	)
	if padded != plain {
		t.Fatalf("normalized lock key = %q, want %q", padded, plain)
	}
}
