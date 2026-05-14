package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestExtractJWTToken_FallsBackToCookie(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "shepherd_session", Value: "cookie-token"})

	token, source, err := extractJWTToken(req, "shepherd_session")
	if err != nil {
		t.Fatalf("extractJWTToken() error = %v", err)
	}
	if token != "cookie-token" {
		t.Fatalf("token = %q, want cookie-token", token)
	}
	if source != tokenSourceCookie {
		t.Fatalf("source = %q, want cookie", source)
	}
}

func TestJWTAuthWithConfig_BackfillsAuthorizationHeaderFromCookie(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	cfg := JWTConfig{
		SigningKey: []byte("cookie-header-backfill-signing-key-1234567890"),
		Issuer:     "shepherd",
		ExpiresIn:  time.Hour,
		CookieName: "shepherd_session",
	}
	token, _, err := GenerateToken(cfg, "u-1", "alice", []string{"operator"}, []string{"vm:read"})
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	router := gin.New()
	router.Use(JWTAuthWithConfig(cfg))
	router.GET("/auth/me", func(c *gin.Context) {
		c.String(http.StatusOK, c.GetHeader("Authorization"))
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "shepherd_session", Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Body.String(), "Bearer "+token; got != want {
		t.Fatalf("authorization header = %q, want %q", got, want)
	}
}

func TestJWTAuthWithConfig_ClearsCookieOnInvalidCookieToken(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	issueCfg := JWTConfig{
		SigningKey: []byte("expired-cookie-token-signing-key-1234567890"),
		Issuer:     "shepherd",
		ExpiresIn:  -time.Hour,
		CookieName: "shepherd_session",
	}
	token, _, err := GenerateToken(issueCfg, "u-1", "alice", nil, nil)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	router := gin.New()
	router.Use(JWTAuthWithConfig(JWTConfig{
		SigningKey: issueCfg.SigningKey,
		Issuer:     issueCfg.Issuer,
		CookieName: issueCfg.CookieName,
	}))
	router.GET("/auth/me", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "shepherd_session", Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	cookieHeader := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("Set-Cookie missing session name: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "Max-Age=0") {
		t.Fatalf("Set-Cookie should expire the session cookie: %s", cookieHeader)
	}
}

func TestJWTAuthWithConfig_ClearsEmptyCookie(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(JWTAuthWithConfig(JWTConfig{
		SigningKey: []byte("empty-cookie-token-signing-key-1234567890"),
		Issuer:     "shepherd",
		CookieName: "shepherd_session",
	}))
	router.GET("/auth/me", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req.AddCookie(&http.Cookie{Name: "shepherd_session", Value: ""})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	cookieHeader := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("Set-Cookie missing session name: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "Max-Age=0") {
		t.Fatalf("Set-Cookie should expire the session cookie: %s", cookieHeader)
	}
}
