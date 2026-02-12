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
	UserID      string   `json:"user_id"`
	Username    string   `json:"username"`
	Roles       []string `json:"roles"`
	Permissions []string `json:"permissions"`
	jwt.RegisteredClaims
}

const defaultJWTLeeway = 30 * time.Second

var (
	ErrJWTSigningKeyMissing = errors.New("jwt signing key is not configured")
	ErrTokenRevoked         = errors.New("token revoked")
	ErrTokenIDRequired      = errors.New("token id is required for revocation checks")
)

// TokenRevocationChecker checks whether a token JTI is revoked.
type TokenRevocationChecker interface {
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

// JWTConfig holds JWT signing configuration.
type JWTConfig struct {
	SigningKey        []byte
	VerificationKeys  [][]byte
	Issuer            string
	ExpiresIn         time.Duration
	Leeway            time.Duration
	RevocationChecker TokenRevocationChecker
}

// GenerateToken creates a signed JWT for the given user.
func GenerateToken(cfg JWTConfig, userID, username string, roles, permissions []string) (string, time.Time, error) {
	if len(cfg.SigningKey) == 0 {
		return "", time.Time{}, ErrJWTSigningKeyMissing
	}

	now := time.Now()
	expiresAt := now.Add(cfg.ExpiresIn)
	tokenID, err := uuid.NewV7()
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token id: %w", err)
	}

	claims := JWTClaims{
		UserID:      userID,
		Username:    username,
		Roles:       roles,
		Permissions: permissions,
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

	return claims, nil
}

// JWTAuth returns a Gin middleware that validates Bearer tokens and populates context.
func JWTAuthWithConfig(cfg JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "UNAUTHORIZED",
				"message": "invalid authorization header format",
			})
			return
		}

		tokenString := parts[1]
		claims, err := cfg.ValidateToken(c.Request.Context(), tokenString)

		if err != nil {
			code := "UNAUTHORIZED"
			msg := "invalid token"
			if errors.Is(err, jwt.ErrTokenExpired) {
				msg = "token expired"
			} else if errors.Is(err, jwt.ErrTokenNotValidYet) || errors.Is(err, jwt.ErrTokenUsedBeforeIssued) {
				msg = "token not active"
			} else if errors.Is(err, ErrTokenRevoked) {
				msg = "token revoked"
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    code,
				"message": msg,
			})
			return
		}

		// Populate context for downstream handlers.
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("roles", claims.Roles)
		c.Set("permissions", claims.Permissions)
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
