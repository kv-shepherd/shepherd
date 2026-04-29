package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeRevocationChecker struct {
	revoked map[string]bool
	err     error
}

func (f fakeRevocationChecker) IsRevoked(_ context.Context, tokenID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[tokenID], nil
}

type fakeClaimsValidator struct {
	called bool
	err    error
}

func (f *fakeClaimsValidator) ValidateClaims(_ context.Context, claims *JWTClaims) error {
	f.called = true
	if claims == nil {
		return errors.New("claims are required")
	}
	return f.err
}

func TestJWTConfigValidateToken_Success(t *testing.T) {
	cfg := JWTConfig{
		SigningKey: []byte("test-signing-key-1234567890123456"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}

	token, _, err := GenerateToken(cfg, "u-1", "alice", []string{"operator"}, []string{"vm:read"})
	require.NoError(t, err)

	claims, err := cfg.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "u-1", claims.UserID)
	assert.Equal(t, "alice", claims.Username)
	assert.NotEmpty(t, claims.ID)
	require.NotNil(t, claims.NotBefore)
}

func TestJWTConfigValidateToken_RejectsInvalidIssuer(t *testing.T) {
	issuerCfg := JWTConfig{
		SigningKey: []byte("issuer-key-123456789012345678901234"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}
	token, _, err := GenerateToken(issuerCfg, "u-1", "alice", nil, nil)
	require.NoError(t, err)

	validatorCfg := JWTConfig{
		SigningKey: issuerCfg.SigningKey,
		Issuer:     "other-issuer",
	}
	_, err = validatorCfg.ValidateToken(context.Background(), token)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwt.ErrTokenInvalidIssuer)
}

func TestJWTConfigValidateToken_SupportsVerificationKeyRotation(t *testing.T) {
	oldKey := []byte("old-key-123456789012345678901234567890")
	newKey := []byte("new-key-123456789012345678901234567890")

	token, _, err := GenerateToken(JWTConfig{
		SigningKey: oldKey,
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}, "u-1", "alice", nil, nil)
	require.NoError(t, err)

	claims, err := JWTConfig{
		SigningKey:       newKey,
		VerificationKeys: [][]byte{oldKey},
		Issuer:           "shepherd",
	}.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "u-1", claims.UserID)
}

func TestJWTConfigValidateToken_RejectsNoneSigningMethod(t *testing.T) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodNone, JWTClaims{
		UserID: "u-1",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "shepherd",
			Subject:   "u-1",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	})
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = JWTConfig{
		SigningKey: []byte("signing-key-123456789012345678901234"),
		Issuer:     "shepherd",
	}.ValidateToken(context.Background(), tokenString)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwt.ErrTokenSignatureInvalid)
}

func TestJWTConfigValidateToken_AllowsLegacyTokenWithoutNotBefore(t *testing.T) {
	now := time.Now()
	legacyClaims := JWTClaims{
		UserID:   "u-legacy",
		Username: "legacy-user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "shepherd",
			Subject:   "u-legacy",
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        "legacy-jti",
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, legacyClaims).
		SignedString([]byte("legacy-signing-key-1234567890123456789"))
	require.NoError(t, err)

	claims, err := JWTConfig{
		SigningKey: []byte("legacy-signing-key-1234567890123456789"),
		Issuer:     "shepherd",
	}.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, "u-legacy", claims.UserID)
	assert.Nil(t, claims.NotBefore)
}

func TestJWTConfigValidateToken_RevocationCheck(t *testing.T) {
	cfg := JWTConfig{
		SigningKey: []byte("revocation-key-1234567890123456789012"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}
	token, _, err := GenerateToken(cfg, "u-1", "alice", nil, nil)
	require.NoError(t, err)

	claims, err := cfg.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	require.NotEmpty(t, claims.ID)

	_, err = JWTConfig{
		SigningKey: cfg.SigningKey,
		Issuer:     "shepherd",
		RevocationChecker: fakeRevocationChecker{
			revoked: map[string]bool{claims.ID: true},
		},
	}.ValidateToken(context.Background(), token)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTokenRevoked)
}

func TestJWTConfigValidateToken_RequiresSigningKey(t *testing.T) {
	token, _, err := GenerateToken(JWTConfig{
		SigningKey: []byte("key-to-sign-valid-token-1234567890123456"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}, "u-1", "alice", nil, nil)
	require.NoError(t, err)

	_, err = JWTConfig{Issuer: "shepherd"}.ValidateToken(context.Background(), token)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwt.ErrTokenUnverifiable)
	assert.ErrorIs(t, err, ErrJWTSigningKeyMissing)
}

func TestJWTConfigValidateToken_RevocationCheckerError(t *testing.T) {
	cfg := JWTConfig{
		SigningKey: []byte("revocation-error-key-1234567890123456"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}
	token, _, err := GenerateToken(cfg, "u-1", "alice", nil, nil)
	require.NoError(t, err)

	_, err = JWTConfig{
		SigningKey: cfg.SigningKey,
		Issuer:     cfg.Issuer,
		RevocationChecker: fakeRevocationChecker{
			err: errors.New("db down"),
		},
	}.ValidateToken(context.Background(), token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check token revocation")
}

func TestGenerateTokenWithSessionVersion_EmbedsSessionVersion(t *testing.T) {
	cfg := JWTConfig{
		SigningKey: []byte("session-version-key-12345678901234567890"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}

	token, _, err := GenerateTokenWithSessionVersion(cfg, "u-1", "alice", nil, nil, 7)
	require.NoError(t, err)

	claims, err := cfg.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	assert.Equal(t, int64(7), claims.SessionVersion)
}

func TestJWTConfigValidateToken_InvokesClaimsValidator(t *testing.T) {
	cfg := JWTConfig{
		SigningKey: []byte("claims-validator-key-1234567890123456789"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
	}
	token, _, err := GenerateTokenWithSessionVersion(cfg, "u-1", "alice", nil, nil, 3)
	require.NoError(t, err)

	validator := &fakeClaimsValidator{}
	claims, err := JWTConfig{
		SigningKey:      cfg.SigningKey,
		Issuer:          cfg.Issuer,
		ClaimsValidator: validator,
	}.ValidateToken(context.Background(), token)
	require.NoError(t, err)
	require.True(t, validator.called)
	assert.Equal(t, int64(3), claims.SessionVersion)
}
