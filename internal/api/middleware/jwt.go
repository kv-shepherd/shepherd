package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// JWTClaims defines custom JWT claims for Shepherd.
type JWTClaims struct {
	UserID         string   `json:"user_id"`
	Username       string   `json:"username"`
	Roles          []string `json:"roles"`
	Permissions    []string `json:"permissions"`
	SessionVersion int64    `json:"session_version,omitempty"`
	jwt.RegisteredClaims
}

const defaultJWTLeeway = 30 * time.Second

const (
	tokenSourceHeader = "header"
	tokenSourceCookie = "cookie"
)

var (
	ErrJWTSigningKeyMissing = errors.New("jwt signing key is not configured")
	ErrTokenRevoked         = errors.New("token revoked")
	ErrTokenIDRequired      = errors.New("token id is required for revocation checks")
	ErrJWTSubjectDisabled   = errors.New("jwt subject is disabled")
	ErrJWTSubjectNotFound   = errors.New("jwt subject was not found")
	ErrJWTSessionStale      = errors.New("jwt session is stale")
)

// TokenRevocationChecker checks whether a token JTI is revoked.
type TokenRevocationChecker interface {
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// JWTClaimsValidator validates resolved claims against live application state.
type JWTClaimsValidator interface {
	ValidateClaims(ctx context.Context, claims *JWTClaims) error
}

// JWTConfig holds JWT signing configuration.
type JWTConfig struct {
	SigningKey        []byte
	VerificationKeys  [][]byte
	Issuer            string
	ExpiresIn         time.Duration
	Leeway            time.Duration
	CookieName        string
	RevocationChecker TokenRevocationChecker
	ClaimsValidator   JWTClaimsValidator
}

// GenerateToken creates a signed JWT for the given user.
func GenerateToken(cfg JWTConfig, userID, username string, roles, permissions []string) (string, time.Time, error) {
	return GenerateTokenWithSessionVersion(cfg, userID, username, roles, permissions, 1)
}

// GenerateTokenWithSessionVersion creates a signed JWT for the given user and session version.
func GenerateTokenWithSessionVersion(
	cfg JWTConfig,
	userID, username string,
	roles, permissions []string,
	sessionVersion int64,
) (string, time.Time, error) {
	if len(cfg.SigningKey) == 0 {
		return "", time.Time{}, ErrJWTSigningKeyMissing
	}
	if sessionVersion < 1 {
		sessionVersion = 1
	}

	now := time.Now()
	expiresAt := now.Add(cfg.ExpiresIn)
	tokenID, err := uuid.NewV7()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token id: %w", err)
	}

	claims := JWTClaims{
		UserID:         userID,
		Username:       username,
		Roles:          roles,
		Permissions:    permissions,
		SessionVersion: sessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(cfg.SigningKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return tokenString, expiresAt, nil
}

func (cfg JWTConfig) parserOptions() []jwt.ParserOption {
	leeway := cfg.Leeway
	if leeway <= 0 {
		leeway = defaultJWTLeeway
	}

	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithLeeway(leeway),
		jwt.WithExpirationRequired(),
		// Keep nbf optional for compatibility with legacy V1 tokens minted
		// before NotBefore was introduced; when present it is still validated.
		jwt.WithIssuedAt(),
	}
	if cfg.Issuer != "" {
		opts = append(opts, jwt.WithIssuer(cfg.Issuer))
	}
	return opts
}

func (cfg JWTConfig) verificationKeySet() jwt.VerificationKeySet {
	keys := make([]jwt.VerificationKey, 0, 1+len(cfg.VerificationKeys))
	seen := make(map[string]struct{}, 1+len(cfg.VerificationKeys))

	if len(cfg.SigningKey) > 0 {
		keys = append(keys, cfg.SigningKey)
		seen[string(cfg.SigningKey)] = struct{}{}
	}

	for _, key := range cfg.VerificationKeys {
		if len(key) == 0 {
			continue
		}
		if _, ok := seen[string(key)]; ok {
			continue
		}
		keys = append(keys, key)
		seen[string(key)] = struct{}{}
	}

	return jwt.VerificationKeySet{Keys: keys}
}

func (cfg JWTConfig) keyfunc() jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		keySet := cfg.verificationKeySet()
		switch len(keySet.Keys) {
		case 0:
			return nil, ErrJWTSigningKeyMissing
		case 1:
			return keySet.Keys[0], nil
		default:
			return keySet, nil
		}
	}
}

// ValidateToken validates token signature + standard claims and checks optional revocation.
func (cfg JWTConfig) ValidateToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, cfg.keyfunc(), cfg.parserOptions()...)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	if cfg.RevocationChecker != nil {
		if claims.ID == "" {
			return nil, ErrTokenIDRequired
		}
		revoked, err := cfg.RevocationChecker.IsRevoked(ctx, claims.ID)
		if err != nil {
			return nil, fmt.Errorf("check token revocation: %w", err)
		}
		if revoked {
			return nil, ErrTokenRevoked
		}
	}

	if cfg.ClaimsValidator != nil {
		if err := cfg.ClaimsValidator.ValidateClaims(ctx, claims); err != nil {
			return nil, err
		}
	}

	return claims, nil
}

// JWTAuth returns a Gin middleware that validates Bearer tokens and populates context.
func JWTAuthWithConfig(cfg JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString, source, err := extractJWTToken(c.Request, cfg.CookieName)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": err.Error(),
			})
			return
		}
		claims, err := cfg.ValidateToken(c.Request.Context(), tokenString)

		if err != nil {
			code := "UNAUTHORIZED"
			msg := "invalid token"
			switch {
			case errors.Is(err, jwt.ErrTokenExpired):
				msg = "token expired"
			case errors.Is(err, jwt.ErrTokenNotValidYet), errors.Is(err, jwt.ErrTokenUsedBeforeIssued):
				msg = "token not active"
			case errors.Is(err, ErrTokenRevoked):
				msg = "token revoked"
			case errors.Is(err, ErrJWTSubjectDisabled), errors.Is(err, ErrJWTSubjectNotFound), errors.Is(err, ErrJWTSessionStale):
				msg = "session no longer valid"
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    code,
				"message": msg,
				"source":  source,
			})
			return
		}

		// Populate context for downstream handlers.
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
		c.Set("token_id", claims.ID)
		if claims.ExpiresAt != nil {
			c.Set("token_expires_at", claims.ExpiresAt.Time)
		}
		if source == tokenSourceCookie && strings.TrimSpace(c.Request.Header.Get("Authorization")) == "" {
			c.Request.Header.Set("Authorization", "Bearer "+tokenString)
		}
		c.Request = c.Request.WithContext(
			SetUserContext(c.Request.Context(), claims.UserID, claims.Username, claims.Roles),
		)

		c.Next()
	}
}

// JWTAuth is a compatibility wrapper for legacy call sites.
func JWTAuth(signingKey []byte) gin.HandlerFunc {
	return JWTAuthWithConfig(JWTConfig{SigningKey: signingKey})
}

func extractJWTToken(r *http.Request, cookieName string) (token, source string, err error) {
	if r == nil {
		return "", "", fmt.Errorf("request is required")
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			return "", "", fmt.Errorf("invalid authorization header format")
		}
		token := strings.TrimSpace(parts[1])
		if token == "" {
			return "", "", fmt.Errorf("missing bearer token")
		}
		return token, tokenSourceHeader, nil
	}

	cookieName = strings.TrimSpace(cookieName)
	if cookieName != "" {
		cookie, err := r.Cookie(cookieName)
		if err == nil {
			token := strings.TrimSpace(cookie.Value)
			if token == "" {
				return "", "", fmt.Errorf("missing auth session cookie")
			}
			return token, tokenSourceCookie, nil
		}
	}

	return "", "", fmt.Errorf("missing authorization header or auth session cookie")
}
