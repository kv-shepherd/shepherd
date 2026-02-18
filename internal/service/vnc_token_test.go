package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestVNCTokenManager_IssueAndValidateSingleUse(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		"shepherd-test",
		2*time.Hour,
		nil,
	)

	token, claims, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if token == "" {
		t.Fatal("Issue() token is empty")
	}
	if claims.JTI == "" {
		t.Fatal("Issue() claims.jti is empty")
	}

	validated, err := manager.ValidateAndConsume(context.Background(), token, "vm-1")
	if err != nil {
		t.Fatalf("ValidateAndConsume(first) error = %v", err)
	}
	if validated.VMID != "vm-1" {
		t.Fatalf("ValidateAndConsume().VMID = %q, want %q", validated.VMID, "vm-1")
	}

	_, err = manager.ValidateAndConsume(context.Background(), token, "vm-1")
	if !errors.Is(err, ErrVNCTokenReplayed) {
		t.Fatalf("ValidateAndConsume(replay) err = %v, want %v", err, ErrVNCTokenReplayed)
	}
}

func TestVNCTokenManager_ValidateRejectsVMMismatch(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		"shepherd-test",
		2*time.Hour,
		nil,
	)

	token, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	_, err = manager.ValidateAndConsume(context.Background(), token, "vm-2")
	if !errors.Is(err, ErrVNCTokenVMMismatch) {
		t.Fatalf("ValidateAndConsume() err = %v, want %v", err, ErrVNCTokenVMMismatch)
	}
}

func TestVNCTokenManager_IssueFailsWithoutSigningKey(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(nil, "shepherd-test", time.Hour, nil)
	if _, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test"); !errors.Is(err, ErrVNCTokenSigningKeyMissing) {
		t.Fatalf("Issue() err = %v, want %v", err, ErrVNCTokenSigningKeyMissing)
	}
}

func TestPostgresVNCReplayStore_ConsumeSingleUseAcrossInstances(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "vnc_replay_store")

	storeA := NewPostgresVNCReplayStore(pool)
	storeB := NewPostgresVNCReplayStore(pool)

	tokenID := "jti-" + strings.ReplaceAll(t.Name(), "/", "-")
	allowed, err := storeA.Consume(t.Context(), tokenID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("storeA.Consume() error = %v", err)
	}
	if !allowed {
		t.Fatal("storeA.Consume() = false, want true on first consume")
	}

	allowed, err = storeB.Consume(t.Context(), tokenID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("storeB.Consume(replay) error = %v", err)
	}
	if allowed {
		t.Fatal("storeB.Consume(replay) = true, want false")
	}

	var usedAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT used_at FROM vnc_replay_markers WHERE token_id = $1`, tokenID).Scan(&usedAt); err != nil {
		t.Fatalf("query replay marker used_at: %v", err)
	}
	if usedAt.IsZero() {
		t.Fatal("replay marker used_at is zero")
	}
}

func TestVNCTokenManager_ValidateAndConsume_UsesPostgresReplayStore(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "vnc_replay_manager")
	signingKey := []byte("vnc-signing-key-123456789012345678901234567890")

	managerA := NewVNCTokenManager(signingKey, "shepherd-test", time.Hour, NewPostgresVNCReplayStore(pool))
	managerB := NewVNCTokenManager(signingKey, "shepherd-test", time.Hour, NewPostgresVNCReplayStore(pool))

	token, _, err := managerA.Issue("user-1", "vm-1", "cluster-a", "team-test")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	if _, err := managerA.ValidateAndConsume(t.Context(), token, "vm-1"); err != nil {
		t.Fatalf("managerA.ValidateAndConsume(first) error = %v", err)
	}

	if _, err := managerB.ValidateAndConsume(t.Context(), token, "vm-1"); !errors.Is(err, ErrVNCTokenReplayed) {
		t.Fatalf("managerB.ValidateAndConsume(replay) err = %v, want %v", err, ErrVNCTokenReplayed)
	}
}
