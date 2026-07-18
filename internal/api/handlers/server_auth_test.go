package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/auditlog"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/governance/audit"
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

func TestLoadStableLoginAuthorizationSnapshotRetriesAfterSessionVersionChange(t *testing.T) {
	versions := []int64{1, 2, 2, 2}
	versionCall := 0
	authorizationCall := 0

	snapshot, err := loadStableLoginAuthorizationSnapshot(
		t.Context(),
		func(context.Context) (int64, error) {
			version := versions[versionCall]
			versionCall++
			return version, nil
		},
		func(context.Context) (*ent.User, []string, []string, error) {
			authorizationCall++
			if authorizationCall == 1 {
				return &ent.User{
					ID:                  "login-snapshot-user",
					Username:            "stale-name",
					ForcePasswordChange: false,
				}, []string{"stale-role"}, []string{"vm:read"}, nil
			}
			return &ent.User{
				ID:                  "login-snapshot-user",
				Username:            "fresh-name",
				ForcePasswordChange: true,
			}, []string{"fresh-role"}, []string{"vm:operate"}, nil
		},
	)
	if err != nil {
		t.Fatalf("loadStableLoginAuthorizationSnapshot() error = %v", err)
	}
	if authorizationCall != 2 {
		t.Fatalf("authorization loader calls = %d, want 2", authorizationCall)
	}
	if snapshot.SessionVersion != 2 || snapshot.User.Username != "fresh-name" || !snapshot.User.ForcePasswordChange {
		t.Fatalf("snapshot user/version = %+v/%d, want fresh user at version 2", snapshot.User, snapshot.SessionVersion)
	}
	if !slices.Equal(snapshot.RoleNames, []string{"fresh-role"}) ||
		!slices.Equal(snapshot.Permissions, []string{"vm:operate"}) {
		t.Fatalf("snapshot authorization = roles %v permissions %v, want fresh authorization", snapshot.RoleNames, snapshot.Permissions)
	}
}

func TestLoadStableLoginAuthorizationSnapshotRejectsRepeatedVersionChurn(t *testing.T) {
	versions := []int64{1, 2, 2, 3, 3, 4}
	versionCall := 0
	authorizationCall := 0

	snapshot, err := loadStableLoginAuthorizationSnapshot(
		t.Context(),
		func(context.Context) (int64, error) {
			version := versions[versionCall]
			versionCall++
			return version, nil
		},
		func(context.Context) (*ent.User, []string, []string, error) {
			authorizationCall++
			return &ent.User{ID: "login-churn-user", Username: "alice"}, nil, nil, nil
		},
	)
	if snapshot != nil {
		t.Fatalf("snapshot = %+v, want nil after repeated version churn", snapshot)
	}
	if err == nil || !strings.Contains(err.Error(), "changed repeatedly") {
		t.Fatalf("error = %v, want repeated authorization change error", err)
	}
	if authorizationCall != loginAuthorizationSnapshotMaxRetries {
		t.Fatalf("authorization loader calls = %d, want %d", authorizationCall, loginAuthorizationSnapshotMaxRetries)
	}
}

type localLoginCredentialRaceContextKey struct{}

func TestLogin_ConcurrentPasswordChangeRejectsPreviouslyValidatedPassword(t *testing.T) {
	server, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "auth_login_password_rotation_race")
	server.jwtCfg.SigningKey = []byte("test-signing-key-12345678901234567890")
	server.jwtCfg.Issuer = "shepherd-test"
	server.jwtCfg.ExpiresIn = time.Hour
	server.passwordHashGenerator = func(password string) (string, error) {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		return string(hash), hashErr
	}

	const (
		userID      = "login-password-rotation-user"
		username    = "login-password-rotation"
		oldPassword = "Passw0rd!BeforeRotation"
		newPassword = "Passw0rd!AfterRotation"
	)
	hash, err := server.generatePasswordHash(oldPassword)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if _, err := client.User.Create().
		SetID(userID).
		SetUsername(username).
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("seed login user: %v", err)
	}

	freshUserReturned := make(chan struct{})
	releaseFreshUser := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFreshUser) }) }
	t.Cleanup(release)
	markedQueryCount := 0
	client.User.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
		return ent.QuerierFunc(func(ctx context.Context, query ent.Query) (ent.Value, error) {
			value, queryErr := next.Query(ctx, query)
			if ctx.Value(localLoginCredentialRaceContextKey{}) == true {
				markedQueryCount++
				if markedQueryCount == 2 {
					close(freshUserReturned)
					<-releaseFreshUser
				}
			}
			return value, queryErr
		})
	}))

	loginCtx, loginResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/login",
		`{"username":"`+username+`","password":"`+oldPassword+`"}`,
		"",
		nil,
	)
	loginRequestContext := context.WithValue(
		loginCtx.Request.Context(),
		localLoginCredentialRaceContextKey{},
		true,
	)
	loginCtx.Request = loginCtx.Request.WithContext(loginRequestContext)
	loginDone := runHandlerAsync(func() { server.Login(loginCtx) })

	select {
	case <-freshUserReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("login did not pause after loading the pre-rotation credential")
	}

	changeCtx, changeResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/change-password",
		`{"old_password":"`+oldPassword+`","new_password":"`+newPassword+`"}`,
		userID,
		nil,
	)
	server.ChangePassword(changeCtx)
	if changeResponse.Code != http.StatusNoContent {
		t.Fatalf("password change status = %d, want %d body=%s", changeResponse.Code, http.StatusNoContent, changeResponse.Body.String())
	}

	release()
	select {
	case <-loginDone:
	case <-time.After(5 * time.Second):
		t.Fatal("login did not finish after password rotation")
	}
	if loginResponse.Code != http.StatusUnauthorized {
		t.Fatalf("stale-password login status = %d, want %d body=%s", loginResponse.Code, http.StatusUnauthorized, loginResponse.Body.String())
	}
	assertErrorCode(t, loginResponse.Body.Bytes(), "INVALID_CREDENTIALS")
}

func TestLogin_ActivatesSessionOnlyAfterAuthenticationAndSigningSucceed(t *testing.T) {
	tests := []struct {
		name           string
		password       string
		signingKey     []byte
		wantStatus     int
		wantActivation bool
	}{
		{
			name:       "wrong password",
			password:   "Passw0rd!Wrong",
			signingKey: []byte("test-signing-key-12345678901234567890"),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token signing failure",
			password:   "Passw0rd!Correct",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:           "successful login",
			password:       "Passw0rd!Correct",
			signingKey:     []byte("test-signing-key-12345678901234567890"),
			wantStatus:     http.StatusOK,
			wantActivation: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
				t,
				"auth_login_activation_"+strings.ReplaceAll(test.name, " ", "_"),
			)
			server.jwtCfg.SigningKey = test.signingKey
			server.jwtCfg.Issuer = "shepherd-test"
			server.jwtCfg.ExpiresIn = time.Hour

			userID := "login-activation-" + strings.ReplaceAll(test.name, " ", "-")
			username := "login.activation." + strings.ReplaceAll(test.name, " ", ".")
			hash, err := HashPassword("Passw0rd!Correct")
			if err != nil {
				t.Fatalf("HashPassword() error = %v", err)
			}
			if _, err := client.User.Create().
				SetID(userID).
				SetUsername(username).
				SetPasswordHash(hash).
				SetEnabled(true).
				Save(t.Context()); err != nil {
				t.Fatalf("seed login user: %v", err)
			}
			if err := authSessions.ActivateUserSession(t.Context(), userID, 1); err != nil {
				t.Fatalf("seed auth session: %v", err)
			}
			beforeActivity := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
			if _, err := server.pool.Exec(t.Context(), `
UPDATE auth_session_subjects
SET last_activity_at = $2, updated_at = $2
WHERE user_id = $1
`, userID, beforeActivity); err != nil {
				t.Fatalf("seed auth session activity: %v", err)
			}

			loginCtx, loginResponse := newAuthedGinContext(
				t,
				http.MethodPost,
				"/auth/login",
				`{"username":"`+username+`","password":"`+test.password+`"}`,
				"",
				nil,
			)
			server.Login(loginCtx)
			if loginResponse.Code != test.wantStatus {
				t.Fatalf("login status = %d, want %d body=%s", loginResponse.Code, test.wantStatus, loginResponse.Body.String())
			}

			var afterActivity time.Time
			if err := server.pool.QueryRow(t.Context(), `
SELECT last_activity_at
FROM auth_session_subjects
WHERE user_id = $1
`, userID).Scan(&afterActivity); err != nil {
				t.Fatalf("read auth session activity: %v", err)
			}
			if test.wantActivation {
				if !afterActivity.After(beforeActivity) {
					t.Fatalf("last_activity_at = %s, want after %s", afterActivity, beforeActivity)
				}
				if loginResponse.Header().Get("Set-Cookie") == "" {
					t.Fatal("successful login did not set an auth session cookie")
				}
				return
			}
			if !afterActivity.Equal(beforeActivity) {
				t.Fatalf("last_activity_at = %s, want unchanged %s", afterActivity, beforeActivity)
			}
			if loginResponse.Header().Get("Set-Cookie") != "" {
				t.Fatalf("failed login unexpectedly set a session cookie: %s", loginResponse.Header().Get("Set-Cookie"))
			}
		})
	}
}

func TestLogin_RetriesActivationAfterSessionVersionChange(t *testing.T) {
	server, client, authSessions := newAdminIdentityTestServerWithAuthSessions(t, "auth_login_activation_retry")
	server.jwtCfg.SigningKey = []byte("test-signing-key-12345678901234567890")
	server.jwtCfg.Issuer = "shepherd-test"
	server.jwtCfg.ExpiresIn = time.Hour

	const (
		userID   = "login-activation-retry-user"
		username = "login.activation.retry"
		password = "Passw0rd!ActivationRetry"
	)
	hash, hashErr := HashPassword(password)
	if hashErr != nil {
		t.Fatalf("HashPassword() error = %v", hashErr)
	}
	if _, createErr := client.User.Create().
		SetID(userID).
		SetUsername(username).
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("seed login user: %v", createErr)
	}
	if activationErr := authSessions.ActivateUserSession(t.Context(), userID, 1); activationErr != nil {
		t.Fatalf("seed auth session: %v", activationErr)
	}

	var (
		revokeOnce         sync.Once
		revokeErr          error
		activationVersions []int64
	)
	server.authSessionBeforeActivate = func(ctx context.Context, gotUserID string, version int64) error {
		if gotUserID != userID {
			return nil
		}
		activationVersions = append(activationVersions, version)
		revokeOnce.Do(func() {
			revokeErr = authSessions.RevokeUserSessions(ctx, userID, "test_activation_race")
		})
		return revokeErr
	}

	loginCtx, loginResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/auth/login",
		`{"username":"`+username+`","password":"`+password+`"}`,
		"",
		nil,
	)
	server.Login(loginCtx)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d body=%s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}
	if !slices.Equal(activationVersions, []int64{1, 2}) {
		t.Fatalf("activation versions = %v, want [1 2]", activationVersions)
	}

	var loginResp generated.LoginResponse
	mustDecodeJSON(t, loginResponse.Body.Bytes(), &loginResp)
	claims, err := server.jwtCfg.ValidateToken(t.Context(), loginResp.Token)
	if err != nil {
		t.Fatalf("validate returned login token: %v", err)
	}
	if claims.SessionVersion != 2 {
		t.Fatalf("returned token session version = %d, want 2", claims.SessionVersion)
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
		SetPermissions([]string{"vm:read", "system:read", " vm:read ", "", "legacy:compat"}).
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

func TestGetCurrentUser_DeduplicatesAndSortsActiveRoleNames(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_me_role_names")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-role-names").
		SetUsername("role.names").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	alphaRole, err := client.Role.Create().
		SetID("role-alpha").
		SetName("alpha").
		SetPermissions([]string{"system:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed alpha role: %v", err)
	}
	betaRole, err := client.Role.Create().
		SetID("role-beta").
		SetName("beta").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed beta role: %v", err)
	}
	disabledRole, err := client.Role.Create().
		SetID("role-disabled").
		SetName("disabled").
		SetPermissions([]string{"platform:admin"}).
		SetEnabled(false).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed disabled role: %v", err)
	}

	bindings := []struct {
		id        string
		role      *ent.Role
		scopeType string
		scopeID   string
	}{
		{id: "rb-beta-global", role: betaRole, scopeType: scopeTypeGlobal},
		{id: "rb-alpha-global", role: alphaRole, scopeType: scopeTypeGlobal},
		{id: "rb-beta-system", role: betaRole, scopeType: scopeTypeSystem, scopeID: "system-a"},
		{id: "rb-disabled-global", role: disabledRole, scopeType: scopeTypeGlobal},
	}
	for _, binding := range bindings {
		create := client.RoleBinding.Create().
			SetID(binding.id).
			SetUser(user).
			SetRole(binding.role).
			SetScopeType(binding.scopeType).
			SetCreatedBy("seed")
		if binding.scopeID != "" {
			create = create.SetScopeID(binding.scopeID)
		}
		if _, createErr := create.Save(t.Context()); createErr != nil {
			t.Fatalf("seed role binding %s: %v", binding.id, createErr)
		}
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
	wantRoles := []string{"alpha", "beta"}
	if !slices.Equal(got.Roles, wantRoles) {
		t.Fatalf("roles = %#v, want %#v", got.Roles, wantRoles)
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

	server, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"auth_handler_change_password_revoke_fail",
	)

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
	beforeVersions := installAuthSessionVersionBumpFailure(
		t,
		server,
		authSessions,
		"user-change-password-revoke-fail",
	)

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
	assertAuthSessionVersionBumpFailureTriggered(t, server)

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
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func TestChangePassword_ConcurrentOldPasswordRequestsAllowOnlyOneCommit(t *testing.T) {
	server, client, authSessions := newAdminIdentityTestServerWithAuthSessions(
		t,
		"auth_handler_change_password_concurrent",
	)
	// The transaction and advisory-lock behavior is the subject of this test.
	// Password-strength hashing is covered separately; using bcrypt's minimum
	// cost here keeps the race test deterministic under full-suite CPU load.
	server.passwordHashGenerator = func(password string) (string, error) {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
		return string(hash), hashErr
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()

	const (
		userID      = "user-change-password-concurrent"
		oldPassword = "Passw0rd!Example"
	)
	hash, hashErr := server.generatePasswordHash(oldPassword)
	if hashErr != nil {
		t.Fatalf("generatePasswordHash() error = %v", hashErr)
	}
	if _, createErr := client.User.Create().
		SetID(userID).
		SetUsername("concurrent.password").
		SetPasswordHash(hash).
		SetEnabled(true).
		Save(ctx); createErr != nil {
		t.Fatalf("seed user: %v", createErr)
	}
	beforeVersion, beforeVersionErr := authSessions.CurrentSessionVersion(ctx, userID)
	if beforeVersionErr != nil {
		t.Fatalf("seed session version: %v", beforeVersionErr)
	}

	lockConn, acquireErr := server.pool.Acquire(ctx)
	if acquireErr != nil {
		t.Fatalf("acquire user mutation lock connection: %v", acquireErr)
	}
	lockHeld := false
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if lockHeld {
			if _, unlockErr := lockConn.Exec(
				cleanupCtx,
				`SELECT pg_advisory_unlock(hashtextextended($1 || ':' || current_schema(), 0))`,
				userMutationAdvisoryLockKey(userID),
			); unlockErr != nil {
				t.Errorf("release user mutation lock during cleanup: %v", unlockErr)
				rawConn := lockConn.Hijack()
				closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer closeCancel()
				if closeErr := rawConn.Close(closeCtx); closeErr != nil {
					t.Errorf("close user mutation lock connection after cleanup failure: %v", closeErr)
				}
				return
			}
		}
		lockConn.Release()
	})
	if _, lockErr := lockConn.Exec(
		ctx,
		`SELECT pg_advisory_lock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(userID),
	); lockErr != nil {
		t.Fatalf("hold user mutation advisory lock: %v", lockErr)
	}
	lockHeld = true
	var blockerPID int32
	if pidErr := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); pidErr != nil {
		t.Fatalf("query user mutation blocker PID: %v", pidErr)
	}

	type changeResult struct {
		newPassword string
		status      int
		body        []byte
	}
	requests := []struct {
		newPassword string
		body        string
	}{
		{
			newPassword: "NewPassw0rd!Alpha",
			body:        `{"old_password":"Passw0rd!Example","new_password":"NewPassw0rd!Alpha"}`,
		},
		{
			newPassword: "NewPassw0rd!Bravo",
			body:        `{"old_password":"Passw0rd!Example","new_password":"NewPassw0rd!Bravo"}`,
		},
	}
	results := make(chan changeResult, len(requests))
	start := make(chan struct{})
	done := make([]<-chan struct{}, 0, len(requests))
	for _, request := range requests {
		requestCase := request
		changeCtx, changeW := newAuthedGinContext(
			t,
			http.MethodPost,
			"/auth/change-password",
			requestCase.body,
			userID,
			nil,
		)
		done = append(done, runHandlerAsync(func() {
			<-start
			server.ChangePassword(changeCtx)
			results <- changeResult{
				newPassword: requestCase.newPassword,
				status:      changeW.Code,
				body:        append([]byte(nil), changeW.Body.Bytes()...),
			}
		}))
	}
	close(start)

	blockedCalls := 0
	var blockedQueryErr error
	require.Eventually(t, func() bool {
		blockedQueryErr = server.pool.QueryRow(ctx, `
WITH RECURSIVE blocked(pid) AS (
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
    AND $1 = ANY(pg_blocking_pids(activity.pid))
  UNION
  SELECT activity.pid
  FROM pg_stat_activity AS activity
  JOIN blocked AS upstream
    ON upstream.pid = ANY(pg_blocking_pids(activity.pid))
  WHERE activity.datname = current_database()
    AND activity.state = 'active'
)
SELECT count(*) FROM blocked
`, blockerPID).Scan(&blockedCalls)
		return blockedQueryErr != nil || blockedCalls == len(requests)
	}, 25*time.Second, 10*time.Millisecond, "password changes did not all block on the user lock")
	require.NoError(t, blockedQueryErr, "query blocked password changes")
	if blockedCalls != len(requests) {
		completed := make([]changeResult, 0, len(requests))
		for len(completed) < len(requests) {
			select {
			case result := <-results:
				completed = append(completed, result)
			default:
				t.Fatalf("password changes blocked on user lock = %d, want %d; early results=%+v", blockedCalls, len(requests), completed)
			}
		}
		t.Fatalf("password changes blocked on user lock = %d, want %d; early results=%+v", blockedCalls, len(requests), completed)
	}
	if _, unlockErr := lockConn.Exec(
		ctx,
		`SELECT pg_advisory_unlock(hashtextextended($1 || ':' || current_schema(), 0))`,
		userMutationAdvisoryLockKey(userID),
	); unlockErr != nil {
		t.Fatalf("release user mutation advisory lock: %v", unlockErr)
	}
	lockHeld = false

	var winnerPassword string
	for range requests {
		result := <-results
		switch result.status {
		case http.StatusNoContent:
			if winnerPassword != "" {
				t.Fatalf("multiple concurrent password changes succeeded: %q and %q", winnerPassword, result.newPassword)
			}
			winnerPassword = result.newPassword
		case http.StatusBadRequest:
			assertErrorCode(t, result.body, "INVALID_CURRENT_PASSWORD")
		default:
			t.Fatalf("password change for %q status = %d body=%s", result.newPassword, result.status, result.body)
		}
	}
	for _, requestDone := range done {
		waitForHandlerCompletion(t, requestDone, "concurrent password change")
	}
	if winnerPassword == "" {
		t.Fatal("no concurrent password change succeeded")
	}

	reloaded, reloadErr := client.User.Get(ctx, userID)
	if reloadErr != nil {
		t.Fatalf("reload user: %v", reloadErr)
	}
	if compareErr := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(winnerPassword)); compareErr != nil {
		t.Fatalf("persisted password does not match winning request: %v", compareErr)
	}
	if compareErr := bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(oldPassword)); compareErr == nil {
		t.Fatal("old password remains valid after winning concurrent change")
	}
	afterVersion, afterVersionErr := authSessions.CurrentSessionVersion(ctx, userID)
	if afterVersionErr != nil {
		t.Fatalf("read session version after concurrent changes: %v", afterVersionErr)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after concurrent changes = %d, want %d", afterVersion, beforeVersion+1)
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
