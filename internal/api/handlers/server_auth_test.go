package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_UsesConfiguredCost(t *testing.T) {
	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}

	if cost != passwordHashCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, passwordHashCost)
	}
}

func TestGetCurrentUser_IncludesPermissions(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_me_permissions")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	role, err := client.Role.Create().
		SetID("role-1").
		SetName("Operator").
		SetPermissions([]string{"vm:read", "system:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}

	if _, err := client.RoleBinding.Create().
		SetID("rb-1").
		SetUser(user).
		SetRole(role).
		SetScopeType("global").
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("seed role binding: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	req = req.WithContext(middleware.SetUserContext(req.Context(), user.ID, user.Username, nil))
	c.Request = req

	server.GetCurrentUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var got generated.UserInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode user info: %v", err)
	}
	if len(got.Permissions) != 2 {
		t.Fatalf("unexpected permissions: %+v", got.Permissions)
	}
	if got.Permissions[0] != "system:read" || got.Permissions[1] != "vm:read" {
		t.Fatalf("permissions not sorted/stable: %+v", got.Permissions)
	}
}

func TestLogin_SetsSessionCookieAndNoStore(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_login_cookie")
	server := NewServer(ServerDeps{
		EntClient: client,
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-12345678901234567890"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		SessionConfig: config.SessionConfig{
			Cookie:   "shepherd_session",
			HTTPOnly: true,
			Secure:   true,
		},
	})

	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := client.User.Create().
		SetID("user-login").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed login user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"Passw0rd!Example"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	server.Login(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("Pragma = %q, want no-cache", got)
	}
	cookieHeader := w.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("missing auth cookie header: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "HttpOnly") {
		t.Fatalf("auth cookie should be HttpOnly: %s", cookieHeader)
	}
	if strings.Contains(cookieHeader, "Secure") {
		t.Fatalf("auth cookie should not mark Secure on plain HTTP test requests: %s", cookieHeader)
	}
}

func TestLogout_ClearsSessionCookie(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	server := NewServer(ServerDeps{
		SessionConfig: config.SessionConfig{
			Cookie:   "shepherd_session",
			HTTPOnly: true,
		},
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/auth/logout", http.NoBody)

	server.Logout(c)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	cookieHeader := w.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("missing cleared auth cookie header: %s", cookieHeader)
	}
	if !strings.Contains(cookieHeader, "Max-Age=0") {
		t.Fatalf("logout cookie should expire immediately: %s", cookieHeader)
	}
}

func TestLogin_SetsSecureCookieWhenPublicBaseURLUsesHTTPS(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_login_secure_public_base_url")
	server := NewServer(ServerDeps{
		EntClient:     client,
		PublicBaseURL: "https://console.example.com",
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-12345678901234567890"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		SessionConfig: config.SessionConfig{
			Cookie:   "shepherd_session",
			HTTPOnly: true,
			Secure:   true,
		},
	})

	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := client.User.Create().
		SetID("user-login-secure-public-base-url").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed login user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"Passw0rd!Example"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	server.Login(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if cookieHeader := w.Header().Get("Set-Cookie"); !strings.Contains(cookieHeader, "Secure") {
		t.Fatalf("auth cookie should mark Secure when public base URL is HTTPS: %s", cookieHeader)
	}
}

func TestSecureCookieByPolicy_ReleaseModeForcesSecure(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/login", http.NoBody)

	if !secureCookieByPolicyWithReleaseMode(c, true, "", true) {
		t.Fatal("secureCookieByPolicyWithReleaseMode() should force Secure in release mode")
	}
}

func TestSetConsoleBootstrapCookie_UsesSecurePolicy(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	server := NewServer(ServerDeps{
		PublicBaseURL: "https://console.example.com",
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/vms/vm-1/vnc/bootstrap", http.NoBody)

	server.setConsoleBootstrapCookie(c, "token-value", "/api/v1/vms/vm-1/vnc")

	if cookieHeader := w.Header().Get("Set-Cookie"); !strings.Contains(cookieHeader, "Secure") {
		t.Fatalf("console bootstrap cookie should mark Secure when public base URL is HTTPS: %s", cookieHeader)
	}
}

func TestLogin_ReturnsTooManyRequestsAfterRepeatedFailures(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_login_rate_limit")
	server := NewServer(ServerDeps{
		EntClient: client,
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-12345678901234567890"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		LoginRateLimitConfig: config.LoginRateLimit{
			Enabled:       true,
			MaxFailures:   2,
			Window:        time.Minute,
			BlockDuration: time.Minute,
		},
	})

	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := client.User.Create().
		SetID("user-login-rate-limit").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed login user: %v", err)
	}

	attempt := func(password string) (*httptest.ResponseRecorder, generated.Error) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"`+password+`"}`))
		req.Header.Set("Content-Type", "application/json")
		c.Request = req

		server.Login(c)

		var apiErr generated.Error
		if w.Body.Len() > 0 {
			if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
		}
		return w, apiErr
	}

	if w, apiErr := attempt("wrong-password"); w.Code != http.StatusUnauthorized || apiErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("first attempt status=%d code=%q", w.Code, apiErr.Code)
	}
	if w, apiErr := attempt("wrong-password"); w.Code != http.StatusUnauthorized || apiErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("second attempt status=%d code=%q", w.Code, apiErr.Code)
	}
	if w, apiErr := attempt("wrong-password"); w.Code != http.StatusTooManyRequests || apiErr.Code != loginRateLimitedErrorCode {
		t.Fatalf("third attempt status=%d code=%q", w.Code, apiErr.Code)
	} else if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header on rate-limited login response")
	}
}

func TestLogin_AuditIncludesClientIPAndRequestID(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_login_audit_details")
	server := NewServer(ServerDeps{
		EntClient: client,
		Audit:     audit.NewLogger(client),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-12345678901234567890"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		SessionConfig: config.SessionConfig{
			Cookie:   "shepherd_session",
			HTTPOnly: true,
		},
	})

	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, createErr := client.User.Create().
		SetID("user-login-audit").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("seed login user: %v", createErr)
	}

	router := gin.New()
	router.Use(middleware.RequestID())
	router.POST("/auth/login", server.Login)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"Passw0rd!Example"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(middleware.RequestIDHeader, "req-login-audit")
	req.RemoteAddr = "203.0.113.10:4321"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	row, err := client.AuditLog.Query().
		Where(auditlog.ActionEQ("user.login")).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if got := row.Details["username"]; got != "alice" {
		t.Fatalf("details.username = %v, want alice", got)
	}
	if got := row.Details["provider"]; got != "local" {
		t.Fatalf("details.provider = %v, want local", got)
	}
	if got := row.Details["client_ip"]; got != "203.0.113.10" {
		t.Fatalf("details.client_ip = %v, want 203.0.113.10", got)
	}
	if got := row.Details["request_id"]; got != "req-login-audit" {
		t.Fatalf("details.request_id = %v, want req-login-audit", got)
	}
}

func TestChangePassword_RollsBackWhenSessionRevocationFails(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_change_password_revoke_fail")
	server := NewServer(ServerDeps{
		EntClient:    client,
		AuthSessions: &service.AuthSessionManager{},
	})

	oldPassword := "Passw0rd!Example"
	hash, err := HashPassword(oldPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, createErr := client.User.Create().
		SetID("user-change-password-revoke-fail").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("seed user: %v", createErr)
	}

	changeCtx, changeW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/change-password",
		`{"old_password":"Passw0rd!Example","new_password":"NewPassw0rd!Example"}`,
		"user-change-password-revoke-fail",
		nil,
	)
	server.ChangePassword(changeCtx)
	if changeW.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", changeW.Code, changeW.Body.String())
	}

	reloaded, err := client.User.Get(t.Context(), "user-change-password-revoke-fail")
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(oldPassword)); err != nil {
		t.Fatalf("expected old password to remain valid after rollback: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("NewPassw0rd!Example")); err == nil {
		t.Fatal("expected new password to remain unapplied after rollback")
	}
}

func TestLogin_CookieOnlyModeSuppressesTokenInResponseBody(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_login_cookie_only")
	server := NewServer(ServerDeps{
		EntClient: client,
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-12345678901234567890"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		SessionConfig: config.SessionConfig{
			Cookie:   "shepherd_session",
			HTTPOnly: true,
			Secure:   true,
		},
	})

	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := client.User.Create().
		SetID("user-login-cookie-only").
		SetUsername("alice").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed login user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"Passw0rd!Example"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(authSessionModeHeader, authSessionModeCookieOnlyValue)
	c.Request = req

	server.Login(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var resp generated.LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Token != "" {
		t.Fatalf("token = %q, want empty in cookie-only mode", resp.Token)
	}
	if cookieHeader := w.Header().Get("Set-Cookie"); !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("missing auth cookie header: %s", cookieHeader)
	}
}
