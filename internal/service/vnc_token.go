package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrVNCTokenSigningKeyMissing    = errors.New("vnc token signing key is not configured")
	ErrVNCTokenEncryptionKeyMissing = errors.New("vnc token encryption key is not configured")
	ErrVNCTokenEncryptionKeyInvalid = errors.New("vnc token encryption key must be 32 bytes")
	ErrVNCTokenIDMissing            = errors.New("vnc token id is required")
	ErrVNCTokenVMMismatch           = errors.New("vnc token vm mismatch")
	ErrVNCTokenReplayed             = errors.New("vnc token already used")
	ErrVNCReplayStoreUnavailable    = errors.New("vnc replay store is unavailable")
	ErrVNCTokenCiphertextInvalid    = errors.New("vnc token ciphertext is invalid")
	ErrVNCTokenDecryptFailed        = errors.New("vnc token decryption failed")
)

const encryptedVNCTokenPrefix = "encv1."

// VNCReplayStore tracks consumed single-use VNC token IDs.
type VNCReplayStore interface {
	Consume(ctx context.Context, tokenID string, expiresAt time.Time) (bool, error)
}

// InMemoryVNCReplayStore is a process-local replay marker store for V1.
type InMemoryVNCReplayStore struct {
	mu   sync.Mutex
	used map[string]time.Time
}

// NewInMemoryVNCReplayStore creates an in-memory replay marker store.
func NewInMemoryVNCReplayStore() *InMemoryVNCReplayStore {
	return &InMemoryVNCReplayStore{used: make(map[string]time.Time)}
}

// Consume marks tokenID as used when first seen, and rejects replay.
func (s *InMemoryVNCReplayStore) Consume(ctx context.Context, tokenID string, expiresAt time.Time) (bool, error) {
	now := time.Now().UTC()
	_ = ctx

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, exp := range s.used {
		if exp.Before(now) {
			delete(s.used, id)
		}
	}

	if _, exists := s.used[tokenID]; exists {
		return false, nil
	}

	s.used[tokenID] = expiresAt.UTC()
	return true, nil
}

// PostgresVNCReplayStore persists replay markers to PostgreSQL so single-use
// semantics hold across replicas.
type PostgresVNCReplayStore struct {
	pool     *pgxpool.Pool
	initOnce sync.Once
	initErr  error
}

const (
	createVNCReplayMarkerTableSQL = `
CREATE TABLE IF NOT EXISTS vnc_replay_markers (
	token_id TEXT PRIMARY KEY,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	createVNCReplayMarkerExpiryIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_vnc_replay_markers_expires_at
ON vnc_replay_markers (expires_at);`
	insertVNCReplayMarkerSQL = `
INSERT INTO vnc_replay_markers (token_id, expires_at, used_at)
VALUES ($1, $2, NOW())
ON CONFLICT (token_id) DO NOTHING;`
)

// NewPostgresVNCReplayStore creates a replay store backed by PostgreSQL.
func NewPostgresVNCReplayStore(pool *pgxpool.Pool) *PostgresVNCReplayStore {
	return &PostgresVNCReplayStore{pool: pool}
}

// Consume inserts a replay marker on first use and rejects token replay.
func (s *PostgresVNCReplayStore) Consume(ctx context.Context, tokenID string, expiresAt time.Time) (bool, error) {
	if s == nil || s.pool == nil {
		return false, ErrVNCReplayStoreUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tokenID = strings.TrimSpace(tokenID)
	if tokenID == "" {
		return false, ErrVNCTokenIDMissing
	}
	if err := s.ensureSchema(); err != nil {
		return false, err
	}

	tag, err := s.pool.Exec(ctx, insertVNCReplayMarkerSQL, tokenID, expiresAt.UTC())
	if err != nil {
		return false, fmt.Errorf("insert vnc replay marker: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (s *PostgresVNCReplayStore) ensureSchema() error {
	s.initOnce.Do(func() {
		if s == nil || s.pool == nil {
			s.initErr = ErrVNCReplayStoreUnavailable
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := s.pool.Exec(ctx, createVNCReplayMarkerTableSQL); err != nil {
			s.initErr = fmt.Errorf("create vnc replay marker table: %w", err)
			return
		}
		if _, err := s.pool.Exec(ctx, createVNCReplayMarkerExpiryIndexSQL); err != nil {
			s.initErr = fmt.Errorf("create vnc replay marker index: %w", err)
		}
	})
	return s.initErr
}

// VNCJWTClaims is the signed token payload for Stage 6.
type VNCJWTClaims struct {
	VMID      string `json:"vm_id"`
	ClusterID string `json:"cluster"`
	Namespace string `json:"namespace"`
	SingleUse bool   `json:"single_use"`
	jwt.RegisteredClaims
}

// VNCTokenManager issues and validates Stage 6 single-use VNC tokens.
type VNCTokenManager struct {
	signingKey    []byte
	encryptionKey []byte
	issuer        string
	ttl           time.Duration
	now           func() time.Time
	replay        VNCReplayStore
}

// NewVNCTokenManager creates a VNC token manager.
func NewVNCTokenManager(signingKey, encryptionKey []byte, issuer string, ttl time.Duration, replay VNCReplayStore) *VNCTokenManager {
	if ttl <= 0 {
		ttl = DefaultVNCTokenTTL
	}
	if replay == nil {
		replay = NewInMemoryVNCReplayStore()
	}
	return &VNCTokenManager{
		signingKey:    signingKey,
		encryptionKey: encryptionKey,
		issuer:        issuer,
		ttl:           ttl,
		now:           func() time.Time { return time.Now().UTC() },
		replay:        replay,
	}
}

// Issue creates a signed token and returns its canonical policy claims.
func (m *VNCTokenManager) Issue(subject, vmID, clusterID, namespace string) (token string, policyClaims VNCTokenClaims, err error) {
	if len(m.signingKey) == 0 {
		return "", VNCTokenClaims{}, ErrVNCTokenSigningKeyMissing
	}
	if validateErr := m.validateEncryptionKey(); validateErr != nil {
		return "", VNCTokenClaims{}, validateErr
	}

	now := m.now().UTC()
	tokenID, err := uuid.NewV7()
	if err != nil {
		return "", VNCTokenClaims{}, fmt.Errorf("generate vnc token id: %w", err)
	}

	policyClaims = BuildVNCTokenClaims(now, m.ttl, subject, vmID, clusterID, namespace, tokenID.String())

	claims := VNCJWTClaims{
		VMID:      policyClaims.VMID,
		ClusterID: policyClaims.ClusterID,
		Namespace: policyClaims.Namespace,
		SingleUse: policyClaims.SingleUse,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   policyClaims.Subject,
			ExpiresAt: jwt.NewNumericDate(policyClaims.ExpiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        policyClaims.JTI,
		},
	}

	signed := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err = signed.SignedString(m.signingKey)
	if err != nil {
		return "", VNCTokenClaims{}, fmt.Errorf("sign vnc token: %w", err)
	}
	token, err = m.encryptToken(token)
	if err != nil {
		return "", VNCTokenClaims{}, err
	}

	return token, policyClaims, nil
}

// ValidateAndConsume validates token signature+claims and consumes single-use token.
func (m *VNCTokenManager) ValidateAndConsume(ctx context.Context, token, expectedVMID string) (*VNCJWTClaims, error) {
	if len(m.signingKey) == 0 {
		return nil, ErrVNCTokenSigningKeyMissing
	}
	if validateErr := m.validateEncryptionKey(); validateErr != nil {
		return nil, validateErr
	}
	token, err := m.unwrapToken(token)
	if err != nil {
		return nil, err
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30 * time.Second),
	}
	if m.issuer != "" {
		opts = append(opts, jwt.WithIssuer(m.issuer))
	}

	parsed, err := jwt.ParseWithClaims(token, &VNCJWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		return m.signingKey, nil
	}, opts...)
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*VNCJWTClaims)
	if !ok || !parsed.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	if expectedVMID != "" && claims.VMID != expectedVMID {
		return nil, ErrVNCTokenVMMismatch
	}
	if claims.ID == "" {
		return nil, ErrVNCTokenIDMissing
	}

	if claims.SingleUse {
		allow, err := m.replay.Consume(ctx, claims.ID, claims.ExpiresAt.Time)
		if err != nil {
			return nil, fmt.Errorf("consume vnc token id: %w", err)
		}
		if !allow {
			return nil, ErrVNCTokenReplayed
		}
	}

	_ = ctx
	return claims, nil
}

func (m *VNCTokenManager) validateEncryptionKey() error {
	switch {
	case len(m.encryptionKey) == 0:
		return ErrVNCTokenEncryptionKeyMissing
	case len(m.encryptionKey) != 32:
		return ErrVNCTokenEncryptionKeyInvalid
	default:
		return nil
	}
}

func (m *VNCTokenManager) encryptToken(signedToken string) (string, error) {
	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create vnc token cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create vnc token aead: %w", err)
	}

	encrypted := aead.Seal(nil, nil, []byte(signedToken), nil)
	return encryptedVNCTokenPrefix + base64.RawURLEncoding.EncodeToString(encrypted), nil
}

func (m *VNCTokenManager) unwrapToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, encryptedVNCTokenPrefix) {
		return token, nil
	}

	encoded := strings.TrimPrefix(token, encryptedVNCTokenPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.Join(ErrVNCTokenCiphertextInvalid, err)
	}

	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create vnc token cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create vnc token aead: %w", err)
	}

	plaintext, err := aead.Open(nil, nil, ciphertext, nil)
	if err != nil {
		return "", errors.Join(ErrVNCTokenDecryptFailed, err)
	}
	return string(plaintext), nil
}
