package service

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"kv-shepherd.io/shepherd/internal/testutil"
)

var testVNCEncryptionKey = []byte("0123456789abcdef0123456789abcdef")

func TestVNCTokenManager_IssueAndValidateSingleUse(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		testVNCEncryptionKey,
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
	if strings.Count(token, ".") == 2 {
		t.Fatalf("Issue() token = %q, want encrypted envelope instead of raw JWT", token)
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
		testVNCEncryptionKey,
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

func TestVNCTokenManager_Validate_DoesNotConsumeSingleUseToken(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		testVNCEncryptionKey,
		"shepherd-test",
		2*time.Hour,
		nil,
	)

	token, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	claims, err := manager.Validate(token, "vm-1")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.VMID != "vm-1" {
		t.Fatalf("Validate().VMID = %q, want %q", claims.VMID, "vm-1")
	}

	if _, err := manager.ValidateAndConsume(context.Background(), token, "vm-1"); err != nil {
		t.Fatalf("ValidateAndConsume(after validate) error = %v", err)
	}
}

func TestVNCTokenManager_IssueFailsWithoutSigningKey(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(nil, testVNCEncryptionKey, "shepherd-test", time.Hour, nil)
	if _, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test"); !errors.Is(err, ErrVNCTokenSigningKeyMissing) {
		t.Fatalf("Issue() err = %v, want %v", err, ErrVNCTokenSigningKeyMissing)
	}
}

func TestVNCTokenManager_IssueFailsWithoutEncryptionKey(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		nil,
		"shepherd-test",
		time.Hour,
		nil,
	)
	if _, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test"); !errors.Is(err, ErrVNCTokenEncryptionKeyMissing) {
		t.Fatalf("Issue() err = %v, want %v", err, ErrVNCTokenEncryptionKeyMissing)
	}
}

func TestVNCTokenManager_ValidateAndConsume_AcceptsLegacySignedJWT(t *testing.T) {
	t.Parallel()

	signingKey := []byte("vnc-signing-key-123456789012345678901234567890")
	manager := NewVNCTokenManager(signingKey, testVNCEncryptionKey, "shepherd-test", time.Hour, nil)

	now := time.Now().UTC()
	tokenID := "legacy-" + strings.ReplaceAll(t.Name(), "/", "-")
	legacyToken := jwt.NewWithClaims(jwt.SigningMethodHS256, VNCJWTClaims{
		VMID:      "vm-1",
		ClusterID: "cluster-a",
		Namespace: "team-test",
		SingleUse: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "shepherd-test",
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	})
	signed, err := legacyToken.SignedString(signingKey)
	if err != nil {
		t.Fatalf("legacyToken.SignedString() error = %v", err)
	}

	claims, err := manager.ValidateAndConsume(context.Background(), signed, "vm-1")
	if err != nil {
		t.Fatalf("ValidateAndConsume(legacy) error = %v", err)
	}
	if claims.ID != tokenID {
		t.Fatalf("claims.ID = %q, want %q", claims.ID, tokenID)
	}
}

func TestVNCTokenManager_ValidateAndConsume_RejectsTamperedCiphertext(t *testing.T) {
	t.Parallel()

	manager := NewVNCTokenManager(
		[]byte("vnc-signing-key-123456789012345678901234567890"),
		testVNCEncryptionKey,
		"shepherd-test",
		time.Hour,
		nil,
	)
	token, _, err := manager.Issue("user-1", "vm-1", "cluster-a", "team-test")
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	encoded := strings.TrimPrefix(token, encryptedVNCTokenPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	if len(ciphertext) == 0 {
		t.Fatal("ciphertext is empty")
	}
	ciphertext[len(ciphertext)-1] ^= 0x01

	tampered := encryptedVNCTokenPrefix + base64.RawURLEncoding.EncodeToString(ciphertext)
	if _, err := manager.ValidateAndConsume(context.Background(), tampered, "vm-1"); !errors.Is(err, ErrVNCTokenDecryptFailed) {
		t.Fatalf("ValidateAndConsume(tampered) err = %v, want %v", err, ErrVNCTokenDecryptFailed)
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

func TestPostgresVNCReplayStore_RetriesSchemaAfterCanceledInitialization(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "vnc_replay_schema_retry")
	store := NewPostgresVNCReplayStore(pool)

	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	err := store.ensureSchema(canceledCtx)
	if err == nil {
		t.Fatal("ensureSchema(canceled) error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ensureSchema(canceled) error = %v, want %v", err, context.Canceled)
	}
	if store.initialized {
		t.Fatal("store initialized after canceled schema init, want retryable failure")
	}

	allowed, err := store.Consume(t.Context(), "jti-schema-retry", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("Consume() after schema retry error = %v", err)
	}
	if !allowed {
		t.Fatal("Consume() after schema retry = false, want first consume")
	}
	if !store.initialized {
		t.Fatal("store initialized = false after successful retry")
	}
}

func TestVNCTokenManager_ValidateAndConsume_UsesPostgresReplayStore(t *testing.T) {
	t.Parallel()

	pool := testutil.OpenPGXPool(t, "vnc_replay_manager")
	signingKey := []byte("vnc-signing-key-123456789012345678901234567890")

	managerA := NewVNCTokenManager(signingKey, testVNCEncryptionKey, "shepherd-test", time.Hour, NewPostgresVNCReplayStore(pool))
	managerB := NewVNCTokenManager(signingKey, testVNCEncryptionKey, "shepherd-test", time.Hour, NewPostgresVNCReplayStore(pool))

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
