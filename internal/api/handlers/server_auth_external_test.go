package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/config"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type testRuntimeAuthAdapter struct {
	typeKey         string
	startReq        provider.AuthStartRequest
	startResp       *provider.AuthStartResponse
	startErr        error
	credentialReq   provider.AuthCredentialRequest
	credentialResp  *provider.AuthResult
	credentialErr   error
	credentialReady chan<- string
	credentialGo    <-chan struct{}
	callbackReq     provider.AuthCallbackRequest
	callbackResp    *provider.AuthResult
	callbackErr     error
	callbackCalls   int
	callbackReady   chan<- string
	callbackGo      <-chan struct{}
	loginModes      []provider.AuthLoginMode
	callbackOrigins []string
}

func (a *testRuntimeAuthAdapter) Type() string { return a.typeKey }

func (a *testRuntimeAuthAdapter) ValidateConfig(map[string]interface{}) error { return nil }

func (a *testRuntimeAuthAdapter) TestConnection(_ context.Context, _ map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}

func (a *testRuntimeAuthAdapter) SampleFields(context.Context, map[string]interface{}) ([]provider.AuthProviderSampleField, error) {
	return nil, nil
}

func (a *testRuntimeAuthAdapter) Describe() provider.AuthProviderTypeDescriptor {
	return provider.AuthProviderTypeDescriptor{
		Type:         a.typeKey,
		DisplayName:  "Test Runtime Provider",
		Description:  "test runtime provider",
		BuiltIn:      false,
		ConfigSchema: map[string]interface{}{"type": "object"},
	}
}

func (a *testRuntimeAuthAdapter) DescribeRuntimeAuth() provider.AuthRuntimeDescriptor {
	loginModes := a.loginModes
	if len(loginModes) == 0 {
		loginModes = []provider.AuthLoginMode{
			{Key: "qr", DisplayName: "QR", Interaction: provider.AuthInteractionRedirect, Default: true},
		}
	}
	return provider.AuthRuntimeDescriptor{
		DisplayName: "Test Runtime Provider",
		LoginModes:  loginModes,
	}
}

func (a *testRuntimeAuthAdapter) StartLogin(_ context.Context, _ map[string]interface{}, req provider.AuthStartRequest) (*provider.AuthStartResponse, error) {
	a.startReq = req
	if a.startErr != nil {
		return nil, a.startErr
	}
	return a.startResp, nil
}

func (a *testRuntimeAuthAdapter) AuthenticateCredentials(ctx context.Context, runtimeConfig map[string]interface{}, req provider.AuthCredentialRequest) (*provider.AuthResult, error) {
	a.credentialReq = req
	if a.credentialReady != nil {
		tenant, _ := runtimeConfig["tenant"].(string)
		select {
		case a.credentialReady <- tenant:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.credentialGo != nil {
		select {
		case <-a.credentialGo:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.credentialErr != nil {
		return nil, a.credentialErr
	}
	return a.credentialResp, nil
}

func (a *testRuntimeAuthAdapter) CompleteLogin(ctx context.Context, runtimeConfig map[string]interface{}, req provider.AuthCallbackRequest) (*provider.AuthResult, error) {
	a.callbackCalls++
	a.callbackReq = req
	if a.callbackReady != nil {
		tenant, _ := runtimeConfig["tenant"].(string)
		select {
		case a.callbackReady <- tenant:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.callbackGo != nil {
		select {
		case <-a.callbackGo:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if a.callbackErr != nil {
		return nil, a.callbackErr
	}
	return a.callbackResp, nil
}

func (a *testRuntimeAuthAdapter) AllowedCallbackOrigins(map[string]interface{}) []string {
	return append([]string(nil), a.callbackOrigins...)
}

func TestListLoginAuthProviders_ListsEnabledRuntimeProviders(t *testing.T) {
	t.Parallel()

	srv, client := newExternalAuthTestServer(t, nil)
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{})

	if _, err := client.AuthProvider.Create().
		SetID("runtime-enabled").
		SetName("Runtime Enabled").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create enabled runtime provider: %v", err)
	}
	if _, err := client.AuthProvider.Create().
		SetID("runtime-disabled").
		SetName("Runtime Disabled").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(false).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create disabled runtime provider: %v", err)
	}
	if _, err := client.AuthProvider.Create().
		SetID("admin-only").
		SetName("Missing Runtime").
		SetAuthType("missing-provider").
		SetConfig(map[string]interface{}{"issuer_url": "https://issuer.example.com"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create admin-only provider: %v", err)
	}

	ctx, w := newPublicGinContext(t, http.MethodGet, "/auth/providers", "")
	srv.ListLoginAuthProviders(ctx)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.LoginAuthProviderList
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("login provider count = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].Id != "runtime-enabled" {
		t.Fatalf("login provider id = %q, want %q", resp.Items[0].Id, "runtime-enabled")
	}
}

func TestIsAllowedRequestOrigin_AllowsConfiguredProviderCallbackOrigin(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackOrigins: []string{"https://login.example.com"},
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})

	if _, err := client.AuthProvider.Create().
		SetID("runtime-callback-origin").
		SetName("Runtime Callback Origin").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	callbackPath := "/api/v1/auth/providers/runtime-callback-origin/callback"
	if !srv.IsAllowedRequestOrigin(t.Context(), callbackPath, "https://login.example.com") {
		t.Fatal("callback origin was rejected")
	}
	if srv.IsAllowedRequestOrigin(t.Context(), callbackPath, "https://denied.example.com") {
		t.Fatal("unexpectedly allowed denied callback origin")
	}
	if srv.IsAllowedRequestOrigin(t.Context(), "/api/v1/auth/me", "https://login.example.com") {
		t.Fatal("callback origin should not be allowed for unrelated API paths")
	}
	if !srv.IsAllowedRequestOrigin(t.Context(), "/api/v1/auth/me", "https://console.example.com") {
		t.Fatal("configured global origin was rejected")
	}
}

func TestStartLoginAuthProvider_RequiresConfiguredPublicBaseURL(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})

	if _, err := client.AuthProvider.Create().
		SetID("runtime-start").
		SetName("Runtime Start").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-start/login/start",
		`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
	)
	ctx.Request.Host = "api.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	srv.StartLoginAuthProvider(ctx, "runtime-start")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != "EXTERNAL_AUTH_PUBLIC_BASE_URL_REQUIRED" {
		t.Fatalf("code = %q", resp.Code)
	}
	if adapter.startReq.CallbackURL != "" {
		t.Fatalf("callback_url = %q, want empty", adapter.startReq.CallbackURL)
	}
}

func TestStartLoginAuthProvider_PrefersConfiguredPublicBaseURL(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://auth.example.com")

	if _, err := client.AuthProvider.Create().
		SetID("runtime-public-base-url").
		SetName("Runtime Public Base URL").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-public-base-url/login/start",
		`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
	)
	ctx.Request.Host = "internal-api:8080"
	ctx.Request.Header.Set("X-Forwarded-Proto", "http")
	srv.StartLoginAuthProvider(ctx, "runtime-public-base-url")
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if adapter.startReq.CallbackURL != "https://auth.example.com/api/v1/auth/providers/runtime-public-base-url/callback" {
		t.Fatalf("callback_url = %q", adapter.startReq.CallbackURL)
	}
}

func TestStartLoginAuthProvider_BindsProviderGenerationAndLoginModeInState(t *testing.T) {
	t.Parallel()

	const sensitiveTenant = "sentinel-private-tenant"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(
		t,
		[]string{"https://console.example.com"},
		"https://auth.example.com",
	)
	providerRow, err := client.AuthProvider.Create().
		SetID("runtime-generation-state").
		SetName("Runtime Generation State").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": sensitiveTenant}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-generation-state/login/start",
		`{"login_mode":" form_post ","return_to":"https://console.example.com/login"}`,
	)
	srv.StartLoginAuthProvider(ctx, providerRow.ID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if adapter.startReq.LoginMode != "form_post" {
		t.Fatalf("provider login mode = %q, want form_post", adapter.startReq.LoginMode)
	}

	claims, err := srv.validateExternalAuthState(adapter.startReq.State, providerRow.ID)
	if err != nil {
		t.Fatalf("validate issued state: %v", err)
	}
	if claims.LoginMode != "form_post" {
		t.Fatalf("state login mode = %q, want form_post", claims.LoginMode)
	}
	if claims.Issuer != srv.jwtCfg.Issuer+externalAuthStateIssuerSuffix {
		t.Fatalf("state issuer = %q, want versioned v2 issuer", claims.Issuer)
	}
	if _, parseErr := jwt.Parse(
		adapter.startReq.State,
		func(*jwt.Token) (interface{}, error) { return srv.jwtCfg.SigningKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(srv.jwtCfg.Issuer+"/external-auth"),
	); parseErr == nil {
		t.Fatal("v2 state was accepted by the legacy v1 issuer boundary")
	}
	providerGeneration, err := service.CaptureAuthProviderGeneration(providerRow)
	if err != nil {
		t.Fatalf("capture provider generation: %v", err)
	}
	if bindingErr := providerGeneration.ValidateStateBinding(srv.jwtCfg.SigningKey, claims.ProviderGeneration); bindingErr != nil {
		t.Fatalf("validate state provider generation: %v", bindingErr)
	}

	parts := strings.Split(adapter.startReq.State, ".")
	if len(parts) != 3 {
		t.Fatalf("state token segments = %d, want 3", len(parts))
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode state payload: %v", err)
	}
	encodedConfig, err := json.Marshal(providerRow.Config)
	if err != nil {
		t.Fatalf("encode provider config: %v", err)
	}
	rawConfigDigest := sha256.Sum256(encodedConfig)
	payloadText := string(payload)
	if strings.Contains(payloadText, sensitiveTenant) ||
		strings.Contains(payloadText, `"config_digest"`) ||
		strings.Contains(payloadText, hex.EncodeToString(rawConfigDigest[:])) ||
		strings.Contains(payloadText, base64.RawURLEncoding.EncodeToString(rawConfigDigest[:])) ||
		strings.Contains(payloadText, base64.StdEncoding.EncodeToString(rawConfigDigest[:])) {
		t.Fatalf("state payload exposed provider configuration: %s", payload)
	}
}

func TestStartLoginAuthProvider_PrefersPlatformSettingOverServerConfig(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://fallback.example.com")

	if _, err := client.PlatformSetting.Create().
		SetID("platform-setting-external-auth-test").
		SetKey(platformSettingKeyExternalAuth).
		SetValue(map[string]interface{}{"public_base_url": "https://auth.example.com"}).
		SetUpdatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create platform setting: %v", err)
	}
	srv.loadExternalAuthPlatformSetting(t.Context())
	if _, err := client.AuthProvider.Create().
		SetID("runtime-platform-setting").
		SetName("Runtime Platform Setting").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-platform-setting/login/start",
		`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
	)
	ctx.Request.Host = "internal-api:8080"
	srv.StartLoginAuthProvider(ctx, "runtime-platform-setting")
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if adapter.startReq.CallbackURL != "https://auth.example.com/api/v1/auth/providers/runtime-platform-setting/callback" {
		t.Fatalf("callback_url = %q", adapter.startReq.CallbackURL)
	}
}

func TestServerIsAllowedOrigin_AllowsPlatformSettingOrigin(t *testing.T) {
	t.Parallel()

	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "")
	if _, err := client.PlatformSetting.Create().
		SetID("platform-setting-origin-allow").
		SetKey(platformSettingKeyExternalAuth).
		SetValue(map[string]interface{}{"public_base_url": "https://auth.example.com"}).
		SetUpdatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create platform setting: %v", err)
	}
	srv.loadExternalAuthPlatformSetting(t.Context())

	if !srv.IsAllowedOrigin(t.Context(), "https://auth.example.com") {
		t.Fatal("expected platform setting origin to be allowed")
	}
	if srv.IsAllowedOrigin(t.Context(), "https://denied.example.com") {
		t.Fatal("expected denied origin to be rejected")
	}
}

func TestSanitizedAuthCallbackHeadersStripsCredentials(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("Authorization", "Bearer secret-token")
	header.Set("Cookie", "shepherd_session=secret-session")
	header.Set("User-Agent", "callback-agent")
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	header.Set("X-Request-ID", "req-1")
	header.Set("X-Forwarded-For", "203.0.113.10")

	got := sanitizedAuthCallbackHeaders(header)
	if _, ok := got["Authorization"]; ok {
		t.Fatalf("Authorization header leaked: %#v", got)
	}
	if _, ok := got["Cookie"]; ok {
		t.Fatalf("Cookie header leaked: %#v", got)
	}
	if got["User-Agent"][0] != "callback-agent" {
		t.Fatalf("User-Agent = %#v", got["User-Agent"])
	}
	if got["Content-Type"][0] != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %#v", got["Content-Type"])
	}
	if got["X-Request-Id"][0] != "req-1" {
		t.Fatalf("X-Request-Id = %#v", got["X-Request-Id"])
	}
	if _, ok := got["X-Forwarded-For"]; ok {
		t.Fatalf("X-Forwarded-For header should not be forwarded to auth provider plugins: %#v", got)
	}
}

func TestStartLoginAuthProvider_AllowsRelativeReturnToFromAllowedOrigin(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://auth.example.com")

	if _, err := client.AuthProvider.Create().
		SetID("runtime-relative-return-to").
		SetName("Runtime Relative Return To").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-relative-return-to/login/start",
		`{"login_mode":"qr","return_to":"/dashboard"}`,
	)
	ctx.Request.Host = "api.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	ctx.Request.Header.Set("Origin", "https://console.example.com")
	srv.StartLoginAuthProvider(ctx, "runtime-relative-return-to")
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if adapter.startReq.ReturnTo != "https://console.example.com/dashboard" {
		t.Fatalf("return_to = %q, want %q", adapter.startReq.ReturnTo, "https://console.example.com/dashboard")
	}
}

func TestStartLoginAuthProvider_AllowsLoopbackOriginAliases(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"http://localhost:3000"}, "https://auth.example.com")

	if _, err := client.AuthProvider.Create().
		SetID("runtime-loopback-alias").
		SetName("Runtime Loopback Alias").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-loopback-alias/login/start",
		`{"login_mode":"qr","return_to":"http://0.0.0.0:3000/dashboard"}`,
	)
	ctx.Request.Host = "api.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	srv.StartLoginAuthProvider(ctx, "runtime-loopback-alias")
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if adapter.startReq.ReturnTo != "http://0.0.0.0:3000/dashboard" {
		t.Fatalf("return_to = %q, want %q", adapter.startReq.ReturnTo, "http://0.0.0.0:3000/dashboard")
	}
}

func TestStartLoginAuthProvider_MapsStructuredStartErrors(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startErr: provider.NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "embedded login requires the dedicated client browser"),
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(t, []string{"https://console.example.com"}, "https://auth.example.com")

	if _, err := client.AuthProvider.Create().
		SetID("runtime-start-error").
		SetName("Runtime Start Error").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-start-error/login/start",
		`{"login_mode":"embedded","return_to":"https://console.example.com/login"}`,
	)
	ctx.Request.Host = "api.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	srv.StartLoginAuthProvider(ctx, "runtime-start-error")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	var resp generated.Error
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.Code != "AUTH_LOGIN_MODE_UNAVAILABLE" {
		t.Fatalf("code = %q, want %q", resp.Code, "AUTH_LOGIN_MODE_UNAVAILABLE")
	}
}

func TestStartLoginAuthProvider_DoesNotExposeUnstructuredProviderError(t *testing.T) {
	const (
		providerID    = "runtime-start-private-error"
		privateDetail = "oidc discovery failed for client_secret=sentinel-provider-secret"
	)
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startErr: errors.New(privateDetail),
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(
		t,
		[]string{"https://console.example.com"},
		"https://auth.example.com",
	)
	observedLogs := observeExternalAuthFailureLogs(t, srv)
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Start Private Error").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/start",
		`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
	)
	srv.StartLoginAuthProvider(ctx, providerID)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("start status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var response generated.Error
	mustDecodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Code != "EXTERNAL_AUTH_FAILED" || response.Message != "external authentication failed" {
		t.Fatalf("start response = %#v, want stable generic provider error", response)
	}
	if strings.Contains(recorder.Body.String(), privateDetail) ||
		strings.Contains(recorder.Body.String(), "sentinel-provider-secret") {
		t.Fatalf("start response exposed private provider error: %s", recorder.Body.String())
	}
	if cookie := recorder.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("failed login start unexpectedly set a session cookie: %s", cookie)
	}
	requireSafeExternalAuthFailureLog(t, observedLogs, providerID, "login_start", privateDetail)
}

func TestCompleteLoginAuthProviderGet_JITProvisionsUserAndRedirectsWithSessionCookie(t *testing.T) {
	t.Parallel()

	username := "alice.external." + strings.ToLower(uuid.NewString()[:8])
	email := username + "@example.com"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID:  "external-user-1",
			Username:    username,
			DisplayName: "Alice External",
			Email:       email,
			Enabled:     true,
			Cohorts: []provider.ExternalCohort{
				{Kind: "department", Key: "2", DisplayName: "Engineering"},
			},
			ProfileAttributes: provider.AuthProfileAttributes{
				"phone_number": "13800000000",
			},
		},
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})

	if _, err := client.AuthProvider.Create().
		SetID("runtime-callback").
		SetName("Runtime Callback").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	mappedRole, err := client.Role.Create().
		SetID("role-runtime-mapped").
		SetName("runtime_mapped_viewer").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create mapped role: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-runtime-1").
		SetProviderID("runtime-callback").
		SetCohortKind("department").
		SetCohortKey("2").
		SetRoleID(mappedRole.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create external cohort mapping: %v", mappingErr)
	}

	state, err := issueExternalAuthStateForTest(t, srv, client, "runtime-callback", "https://console.example.com/login", "qr")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}

	ctx, w := newPublicGinContext(t, http.MethodGet, "/auth/providers/runtime-callback/callback?code=code-1&state="+state, "")
	srv.CompleteLoginAuthProviderGet(ctx, "runtime-callback", generated.CompleteLoginAuthProviderGetParams{
		Code:  "code-1",
		State: state,
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "https://console.example.com/login" {
		t.Fatalf("callback Location = %q, want return_to", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("callback Cache-Control = %q, want no-store", got)
	}
	cookieHeader := w.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "shepherd_session=") {
		t.Fatalf("callback response missing auth session cookie: %s", cookieHeader)
	}
	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == srv.authSessionCookieName() {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || strings.TrimSpace(sessionCookie.Value) == "" {
		t.Fatalf("callback response missing non-empty session cookie: %s", cookieHeader)
	}
	claims, err := srv.jwtCfg.ValidateToken(t.Context(), sessionCookie.Value)
	if err != nil {
		t.Fatalf("validate callback token: %v", err)
	}
	if !slices.Contains(claims.Permissions, "vm:read") {
		t.Fatalf("callback permissions = %#v, want vm:read", claims.Permissions)
	}

	createdUser, queryErr := client.User.Query().
		Where(
			user.AuthProviderIDEQ("runtime-callback"),
			user.ExternalIDEQ("external-user-1"),
		).
		Only(t.Context())
	if queryErr != nil {
		t.Fatalf("query created external user: %v", queryErr)
	}
	if createdUser.Username != username {
		t.Fatalf("created username = %q, want %q", createdUser.Username, username)
	}
	if createdUser.LastLoginAt.IsZero() {
		t.Fatal("last_login_at was not updated")
	}

	binding, err := client.RoleBinding.Query().
		Where(
			rolebinding.HasUserWith(user.IDEQ(createdUser.ID)),
			rolebinding.HasRoleWith(role.IDEQ(mappedRole.ID)),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query managed role binding: %v", err)
	}
	grant, err := client.ExternalCohortGrant.Query().
		Where(
			externalcohortgrant.UserIDEQ(createdUser.ID),
			externalcohortgrant.RoleBindingIDEQ(binding.ID),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query external cohort grant: %v", err)
	}
	if grant.ProviderID != "runtime-callback" {
		t.Fatalf("grant provider_id = %q, want %q", grant.ProviderID, "runtime-callback")
	}
	if grant.BindingKey == "" {
		t.Fatal("grant binding_key is empty")
	}
	observedCohort, err := client.ExternalCohort.Query().
		Where(
			externalcohort.ProviderIDEQ("runtime-callback"),
			externalcohort.CohortKindEQ("department"),
			externalcohort.CohortKeyEQ("2"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query observed external cohort: %v", err)
	}
	if observedCohort.DisplayName != "Engineering" {
		t.Fatalf("observed cohort display_name = %q, want %q", observedCohort.DisplayName, "Engineering")
	}

	sampleCtx, sampleW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/runtime-callback/sample",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderSample(sampleCtx, "runtime-callback")
	if sampleW.Code != http.StatusOK {
		t.Fatalf("sample status = %d, want %d, body=%s", sampleW.Code, http.StatusOK, sampleW.Body.String())
	}
	var sampleResp generated.AuthProviderSampleResponse
	mustDecodeJSON(t, sampleW.Body.Bytes(), &sampleResp)
	fieldByName := make(map[string]generated.AuthProviderSampleField, len(sampleResp.Fields))
	for _, field := range sampleResp.Fields {
		fieldByName[field.Field] = field
	}
	if phoneField, ok := fieldByName["phone_number"]; !ok {
		t.Fatalf("sample fields missing phone_number: %#v", sampleResp.Fields)
	} else if !slices.Contains(phoneField.Sample, "13800000000") {
		t.Fatalf("phone_number samples = %#v, want 13800000000", phoneField.Sample)
	}
	if cohortField, ok := fieldByName["cohorts"]; !ok {
		t.Fatalf("sample fields missing cohorts: %#v", sampleResp.Fields)
	} else if !slices.Contains(cohortField.Sample, "department:2") {
		t.Fatalf("cohort samples = %#v, want department:2", cohortField.Sample)
	}
}

func TestSubmitLoginAuthProvider_CredentialMode_JITProvisionsUserAndReturnsLoginResponse(t *testing.T) {
	t.Parallel()

	username := "alice.ldap." + strings.ToLower(uuid.NewString()[:8])
	email := username + "@example.com"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "LDAP Login",
				Interaction: provider.AuthInteractionCredentials,
				RequestSchema: map[string]interface{}{
					"type": "object",
				},
				Default: true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "ldap-user-1",
			Username:    username,
			DisplayName: "Alice LDAP",
			Email:       email,
			Enabled:     true,
			Cohorts: []provider.ExternalCohort{
				{Kind: "group", Key: "cn=ops,ou=groups,dc=example,dc=com", DisplayName: "Ops"},
			},
		},
	})
	srv, client := newExternalAuthTestServer(t, nil)

	if _, err := client.AuthProvider.Create().
		SetID("runtime-credential").
		SetName("Runtime Credential").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	mappedRole, err := client.Role.Create().
		SetID("role-runtime-credential").
		SetName("runtime_credential_viewer").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create mapped role: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-runtime-credential").
		SetProviderID("runtime-credential").
		SetCohortKind("group").
		SetCohortKey("cn=ops,ou=groups,dc=example,dc=com").
		SetRoleID(mappedRole.ID).
		SetScopeType("global").
		SetAllowedEnvironments([]string{"test"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create external cohort mapping: %v", mappingErr)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-credential/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(ctx, "runtime-credential")
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.LoginResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if strings.TrimSpace(resp.Token) == "" {
		t.Fatalf("token = %q, want non-empty", resp.Token)
	}
	if !strings.Contains(w.Header().Get("Set-Cookie"), "shepherd_session=") {
		t.Fatalf("submit response missing auth session cookie: %s", w.Header().Get("Set-Cookie"))
	}
	claims, err := srv.jwtCfg.ValidateToken(t.Context(), resp.Token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	if !slices.Contains(claims.Permissions, "vm:read") {
		t.Fatalf("permissions = %#v, want vm:read", claims.Permissions)
	}
	if got := adapter.credentialReq.Credentials["username"]; got != "alice" {
		t.Fatalf("credential username = %#v, want alice", got)
	}

	createdUser, err := client.User.Query().
		Where(
			user.AuthProviderIDEQ("runtime-credential"),
			user.ExternalIDEQ("ldap-user-1"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query created external user: %v", err)
	}
	if createdUser.Username != username {
		t.Fatalf("created username = %q, want %q", createdUser.Username, username)
	}
}

func TestSubmitLoginAuthProvider_SuccessCreatesInitialIdleSessionSubject(t *testing.T) {
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "initial-session-user",
			Username:    "initial.session.user",
			DisplayName: "Initial Session User",
			Email:       "initial.session.user@example.com",
			Enabled:     true,
		},
	})
	srv, client, _ := newExternalAuthTestServerWithAuthSessions(t)
	const providerID = "runtime-initial-session"
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Initial Session").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ginCtx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(ginCtx, providerID)
	if recorder.Code != http.StatusOK {
		t.Fatalf("credential login status = %d, want %d body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	createdUser, queryErr := client.User.Query().
		Where(
			user.AuthProviderIDEQ(providerID),
			user.ExternalIDEQ("initial-session-user"),
		).
		Only(t.Context())
	if queryErr != nil {
		t.Fatalf("query created provider user: %v", queryErr)
	}
	var (
		version        int64
		lastActivityAt time.Time
	)
	if err := srv.pool.QueryRow(t.Context(), `
SELECT session_version, last_activity_at
FROM auth_session_subjects
WHERE user_id = $1
`, createdUser.ID).Scan(&version, &lastActivityAt); err != nil {
		t.Fatalf("read initial auth session subject: %v", err)
	}
	if version != 1 {
		t.Fatalf("initial session version = %d, want 1", version)
	}
	if lastActivityAt.IsZero() {
		t.Fatal("initial session activity is zero")
	}

	var loginResp generated.LoginResponse
	mustDecodeJSON(t, recorder.Body.Bytes(), &loginResp)
	claims, err := srv.jwtCfg.ValidateToken(t.Context(), loginResp.Token)
	if err != nil {
		t.Fatalf("validate external login token: %v", err)
	}
	if claims.SessionVersion != version {
		t.Fatalf("token session version = %d, want %d", claims.SessionVersion, version)
	}
}

func TestSubmitLoginAuthProvider_RejectsProviderChangedDuringCredentialAuthentication(t *testing.T) {
	providerID := "runtime-credential-generation"
	ready := make(chan string, 1)
	proceed := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case proceed <- struct{}{}:
		default:
		}
	})
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "credential-generation-user",
			Username:    "credential.generation.user",
			DisplayName: "Credential Generation User",
			Email:       "credential.generation.user@example.com",
			Enabled:     true,
		},
		credentialReady: ready,
		credentialGo:    proceed,
	})
	srv, client := newExternalAuthTestServer(t, nil)
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Credential Generation").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "A"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ginCtx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	ginCtx.Request = ginCtx.Request.WithContext(ctx)
	done := runHandlerAsync(func() {
		srv.SubmitLoginAuthProvider(ginCtx, providerID)
	})

	select {
	case tenant := <-ready:
		if tenant != "A" {
			t.Fatalf("credential authentication config tenant = %q, want A", tenant)
		}
	case <-ctx.Done():
		t.Fatalf("credential authentication did not start: %v", ctx.Err())
	}
	updateExternalAuthProviderConfigWithLock(t, client, providerID, map[string]interface{}{"tenant": "B"})
	proceed <- struct{}{}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("credential login did not finish: %v", ctx.Err())
	}

	if recorder.Code != http.StatusConflict {
		t.Fatalf("credential login status = %d, want %d, body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	var response generated.Error
	mustDecodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Code != "AUTH_PROVIDER_CHANGED" {
		t.Fatalf("credential login code = %q, want AUTH_PROVIDER_CHANGED", response.Code)
	}
	if recorder.Header().Get("Set-Cookie") != "" {
		t.Fatalf("credential login unexpectedly set a session cookie: %s", recorder.Header().Get("Set-Cookie"))
	}
	count, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(t.Context())
	if err != nil {
		t.Fatalf("count provider users: %v", err)
	}
	if count != 0 {
		t.Fatalf("provider user count = %d, want 0 after stale authentication", count)
	}
}

func TestSubmitLoginAuthProvider_ReturnsNotFoundWhenProviderIsDeletedDuringAuthentication(t *testing.T) {
	const providerID = "runtime-credential-deleted"
	ready := make(chan string, 1)
	proceed := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case proceed <- struct{}{}:
		default:
		}
	})
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "credential-deleted-user",
			Username:    "credential.deleted.user",
			DisplayName: "Credential Deleted User",
			Email:       "credential.deleted.user@example.com",
			Enabled:     true,
		},
		credentialReady: ready,
		credentialGo:    proceed,
	})
	srv, client, _ := newExternalAuthTestServerWithAuthSessions(t)
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Credential Deleted").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "A"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	loginCtx, loginW := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	loginCtx.Request = loginCtx.Request.WithContext(ctx)
	loginDone := runHandlerAsync(func() {
		srv.SubmitLoginAuthProvider(loginCtx, providerID)
	})

	select {
	case tenant := <-ready:
		if tenant != "A" {
			t.Fatalf("credential authentication config tenant = %q, want A", tenant)
		}
	case <-ctx.Done():
		t.Fatalf("credential authentication did not start: %v", ctx.Err())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	deleteCtx.Request = deleteCtx.Request.WithContext(middleware.SetUserContext(ctx, "admin-1", "admin-1", nil))
	srv.DeleteAuthProvider(deleteCtx, providerID)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete provider status = %d, want %d body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	proceed <- struct{}{}
	select {
	case <-loginDone:
	case <-ctx.Done():
		t.Fatalf("credential login did not finish: %v", ctx.Err())
	}

	if loginW.Code != http.StatusNotFound {
		t.Fatalf("credential login status = %d, want %d body=%s", loginW.Code, http.StatusNotFound, loginW.Body.String())
	}
	assertErrorCode(t, loginW.Body.Bytes(), "AUTH_PROVIDER_NOT_FOUND")
	if loginW.Header().Get("Set-Cookie") != "" {
		t.Fatalf("credential login unexpectedly set a session cookie: %s", loginW.Header().Get("Set-Cookie"))
	}
	if _, err := client.AuthProvider.Get(t.Context(), providerID); !ent.IsNotFound(err) {
		t.Fatalf("provider lookup error = %v, want not found", err)
	}
	if count, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("provider user count = %d/%v, want 0", count, err)
	}
}

func TestSubmitLoginAuthProvider_RejectsProviderChangedBeforeTokenIssuance(t *testing.T) {
	providerID := "runtime-credential-token-generation"
	beforeToken := make(chan struct{}, 1)
	proceed := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case proceed <- struct{}{}:
		default:
		}
	})
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "credential-token-generation-user",
			Username:    "credential.token.generation.user",
			DisplayName: "Credential Token Generation User",
			Email:       "credential.token.generation.user@example.com",
			Enabled:     true,
		},
	})
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)
	srv.externalAuthBeforeTokenIssue = func(ctx context.Context) error {
		select {
		case beforeToken <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		select {
		case <-proceed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Credential Token Generation").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "A"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	const existingUserID = "credential-token-generation-existing-user"
	if _, err := client.User.Create().
		SetID(existingUserID).
		SetUsername("credential.token.generation.user").
		SetDisplayName("Credential Token Generation User").
		SetEmail("credential.token.generation.user@example.com").
		SetEnabled(true).
		SetAuthProviderID(providerID).
		SetExternalID("credential-token-generation-user").
		Save(t.Context()); err != nil {
		t.Fatalf("create existing provider user: %v", err)
	}
	if err := authSessions.ActivateUserSession(t.Context(), existingUserID, 1); err != nil {
		t.Fatalf("seed provider user session: %v", err)
	}
	wantActivity := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Microsecond)
	if _, err := srv.pool.Exec(t.Context(), `
UPDATE auth_session_subjects
SET last_activity_at = $2, updated_at = $2
WHERE user_id = $1
`, existingUserID, wantActivity); err != nil {
		t.Fatalf("seed provider user session activity: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ginCtx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	ginCtx.Request = ginCtx.Request.WithContext(ctx)
	done := runHandlerAsync(func() {
		srv.SubmitLoginAuthProvider(ginCtx, providerID)
	})

	select {
	case <-beforeToken:
	case <-ctx.Done():
		t.Fatalf("external login did not reach the pre-token boundary: %v", ctx.Err())
	}
	committedUsers, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(t.Context())
	if err != nil {
		t.Fatalf("count provisioned users: %v", err)
	}
	if committedUsers != 1 {
		t.Fatalf("provisioned user count = %d, want 1 before token issuance", committedUsers)
	}
	updateExternalAuthProviderConfigWithLock(t, client, providerID, map[string]interface{}{"tenant": "B"})
	proceed <- struct{}{}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("credential login did not finish: %v", ctx.Err())
	}

	if recorder.Code != http.StatusConflict {
		t.Fatalf("credential login status = %d, want %d, body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	var response generated.Error
	mustDecodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Code != "AUTH_PROVIDER_CHANGED" {
		t.Fatalf("credential login code = %q, want AUTH_PROVIDER_CHANGED", response.Code)
	}
	if recorder.Header().Get("Set-Cookie") != "" {
		t.Fatalf("credential login unexpectedly returned a usable session cookie: %s", recorder.Header().Get("Set-Cookie"))
	}
	var gotActivity time.Time
	if err := srv.pool.QueryRow(t.Context(), `
SELECT last_activity_at
FROM auth_session_subjects
WHERE user_id = $1
`, existingUserID).Scan(&gotActivity); err != nil {
		t.Fatalf("read provider user session activity: %v", err)
	}
	if !gotActivity.Equal(wantActivity) {
		t.Fatalf("last_activity_at = %s, want unchanged %s after provider generation failure", gotActivity, wantActivity)
	}
}

func TestCompleteLoginAuthProviderGet_RejectsProviderChangedSinceLoginStartBeforeAdapter(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *ent.Client, string)
	}{
		{
			name: "config",
			mutate: func(t *testing.T, client *ent.Client, providerID string) {
				updateExternalAuthProviderConfigWithLock(t, client, providerID, map[string]interface{}{"tenant": "B"})
			},
		},
		{
			name: "auth_type",
			mutate: func(t *testing.T, client *ent.Client, providerID string) {
				t.Helper()
				if err := WithTx(t.Context(), client, func(tx *ent.Tx) error {
					if lockErr := service.LockAuthProviderMutation(t.Context(), tx, providerID); lockErr != nil {
						return lockErr
					}
					_, updateErr := tx.Client().AuthProvider.UpdateOneID(providerID).
						SetAuthType("unregistered-runtime-auth-type").
						Save(t.Context())
					return updateErr
				}); err != nil {
					t.Fatalf("update auth provider type with mutation lock: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerID := "runtime-callback-start-generation-" + strings.ReplaceAll(tt.name, "_", "-")
			adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
				startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
				callbackResp: &provider.AuthResult{
					ExternalID: "must-not-authenticate",
					Username:   "must.not.authenticate",
					Enabled:    true,
				},
			})
			srv, client := newExternalAuthTestServerWithPublicBaseURL(
				t,
				[]string{"https://console.example.com"},
				"https://auth.example.com",
			)
			if _, err := client.AuthProvider.Create().
				SetID(providerID).
				SetName("Runtime Callback Start Generation " + tt.name).
				SetAuthType(adapter.typeKey).
				SetConfig(map[string]interface{}{"tenant": "A"}).
				SetEnabled(true).
				SetCreatedBy("admin-1").
				Save(t.Context()); err != nil {
				t.Fatalf("create runtime provider: %v", err)
			}
			startCtx, startRecorder := newPublicGinContext(
				t,
				http.MethodPost,
				"/auth/providers/"+providerID+"/login/start",
				`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
			)
			srv.StartLoginAuthProvider(startCtx, providerID)
			if startRecorder.Code != http.StatusOK {
				t.Fatalf("start status = %d, want %d, body=%s", startRecorder.Code, http.StatusOK, startRecorder.Body.String())
			}
			state := adapter.startReq.State
			if state == "" {
				t.Fatal("login start did not pass state to the provider adapter")
			}

			tt.mutate(t, client, providerID)
			ginCtx, recorder := newPublicGinContext(
				t,
				http.MethodGet,
				"/auth/providers/"+providerID+"/callback?code=code-1&state="+url.QueryEscape(state),
				"",
			)
			srv.CompleteLoginAuthProviderGet(ginCtx, providerID, generated.CompleteLoginAuthProviderGetParams{
				Code:  "code-1",
				State: state,
			})

			if recorder.Code != http.StatusConflict {
				t.Fatalf("callback status = %d, want %d, body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
			}
			if adapter.callbackCalls != 0 {
				t.Fatalf("adapter callback calls = %d, want 0 for stale start generation", adapter.callbackCalls)
			}
			if !strings.Contains(recorder.Body.String(), `"code":"AUTH_PROVIDER_CHANGED"`) {
				t.Fatalf("callback body does not require a fresh login: %s", recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), `"tenant":"A"`) || strings.Contains(recorder.Body.String(), `"tenant":"B"`) {
				t.Fatalf("callback body exposed provider configuration: %s", recorder.Body.String())
			}
			if got := recorder.Header().Values("Set-Cookie"); len(got) != 0 {
				t.Fatalf("stale callback emitted session cookie: %#v", got)
			}
			requireExternalAuthBridgeSecurityHeaders(t, recorder)
		})
	}
}

func TestCompleteLoginAuthProviderGet_RejectsProviderChangedDuringCallbackAuthentication(t *testing.T) {
	providerID := "runtime-callback-generation"
	ready := make(chan string, 1)
	proceed := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case proceed <- struct{}{}:
		default:
		}
	})
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID:  "callback-generation-user",
			Username:    "callback.generation.user",
			DisplayName: "Callback Generation User",
			Email:       "callback.generation.user@example.com",
			Enabled:     true,
		},
		callbackReady: ready,
		callbackGo:    proceed,
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Callback Generation").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "A"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	state, err := issueExternalAuthStateForTest(t, srv, client, providerID, "https://console.example.com/login", "qr")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	ginCtx, recorder := newPublicGinContext(
		t,
		http.MethodGet,
		"/auth/providers/"+providerID+"/callback?code=code-1&state="+state,
		"",
	)
	ginCtx.Request = ginCtx.Request.WithContext(ctx)
	done := runHandlerAsync(func() {
		srv.CompleteLoginAuthProviderGet(ginCtx, providerID, generated.CompleteLoginAuthProviderGetParams{
			Code:  "code-1",
			State: state,
		})
	})

	select {
	case tenant := <-ready:
		if tenant != "A" {
			t.Fatalf("callback authentication config tenant = %q, want A", tenant)
		}
	case <-ctx.Done():
		t.Fatalf("callback authentication did not start: %v", ctx.Err())
	}
	updateExternalAuthProviderConfigWithLock(t, client, providerID, map[string]interface{}{"tenant": "B"})
	proceed <- struct{}{}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("callback login did not finish: %v", ctx.Err())
	}

	if recorder.Code != http.StatusConflict {
		t.Fatalf("callback status = %d, want %d, body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "AUTH_PROVIDER_CHANGED") {
		t.Fatalf("callback body does not require reauthentication: %s", recorder.Body.String())
	}
	if recorder.Header().Get("Set-Cookie") != "" {
		t.Fatalf("callback unexpectedly set a session cookie: %s", recorder.Header().Get("Set-Cookie"))
	}
	count, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(t.Context())
	if err != nil {
		t.Fatalf("count provider users: %v", err)
	}
	if count != 0 {
		t.Fatalf("provider user count = %d, want 0 after stale callback", count)
	}
}

func TestCompleteLoginAuthProviderGet_ReturnsNotFoundWhenProviderIsDeletedDuringAuthentication(t *testing.T) {
	const providerID = "runtime-callback-deleted"
	ready := make(chan string, 1)
	proceed := make(chan struct{}, 1)
	t.Cleanup(func() {
		select {
		case proceed <- struct{}{}:
		default:
		}
	})
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
		callbackResp: &provider.AuthResult{
			ExternalID:  "callback-deleted-user",
			Username:    "callback.deleted.user",
			DisplayName: "Callback Deleted User",
			Email:       "callback.deleted.user@example.com",
			Enabled:     true,
		},
		callbackReady: ready,
		callbackGo:    proceed,
	})

	pool := testutil.OpenPGXPool(t, "external_auth_callback_deleted")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	srv := NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		ExternalAuth:  service.NewExternalAuthService(client),
		AuthSessions:  authSessions,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-0123456789abcdef"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
			CookieName: defaultAuthSessionCookieName,
		},
		SessionConfig: config.SessionConfig{Cookie: defaultAuthSessionCookieName, HTTPOnly: true},
		PublicBaseURL: "https://auth.example.com",
		AllowedOrigins: []string{
			"https://console.example.com",
		},
	})
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Callback Deleted").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "A"}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	startCtx, startW := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/start",
		`{"login_mode":"qr","return_to":"https://console.example.com/login"}`,
	)
	srv.StartLoginAuthProvider(startCtx, providerID)
	if startW.Code != http.StatusOK {
		t.Fatalf("start login status = %d, want %d body=%s", startW.Code, http.StatusOK, startW.Body.String())
	}
	state := adapter.startReq.State
	if state == "" {
		t.Fatal("start login did not send state to the provider adapter")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	callbackCtx, callbackW := newPublicGinContext(
		t,
		http.MethodGet,
		"/auth/providers/"+providerID+"/callback?code=code-1&state="+url.QueryEscape(state),
		"",
	)
	callbackCtx.Request = callbackCtx.Request.WithContext(ctx)
	callbackDone := runHandlerAsync(func() {
		srv.CompleteLoginAuthProviderGet(callbackCtx, providerID, generated.CompleteLoginAuthProviderGetParams{
			Code:  "code-1",
			State: state,
		})
	})

	select {
	case tenant := <-ready:
		if tenant != "A" {
			t.Fatalf("callback authentication config tenant = %q, want A", tenant)
		}
	case <-ctx.Done():
		t.Fatalf("callback authentication did not start: %v", ctx.Err())
	}

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerID,
		"",
		"admin-1",
		[]string{"auth_provider:delete"},
	)
	deleteCtx.Request = deleteCtx.Request.WithContext(middleware.SetUserContext(ctx, "admin-1", "admin-1", nil))
	srv.DeleteAuthProvider(deleteCtx, providerID)
	if got := deleteCtx.Writer.Status(); got != http.StatusNoContent {
		t.Fatalf("delete provider status = %d, want %d body=%s", got, http.StatusNoContent, deleteW.Body.String())
	}

	proceed <- struct{}{}
	select {
	case <-callbackDone:
	case <-ctx.Done():
		t.Fatalf("callback login did not finish: %v", ctx.Err())
	}

	if callbackW.Code != http.StatusNotFound {
		t.Fatalf("callback status = %d, want %d body=%s", callbackW.Code, http.StatusNotFound, callbackW.Body.String())
	}
	if !strings.Contains(callbackW.Body.String(), "AUTH_PROVIDER_NOT_FOUND") {
		t.Fatalf("callback body does not report deleted provider: %s", callbackW.Body.String())
	}
	if callbackW.Header().Get("Set-Cookie") != "" {
		t.Fatalf("callback unexpectedly set a session cookie: %s", callbackW.Header().Get("Set-Cookie"))
	}
	if _, err := client.AuthProvider.Get(t.Context(), providerID); !ent.IsNotFound(err) {
		t.Fatalf("provider lookup error = %v, want not found", err)
	}
	if count, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("provider user count = %d/%v, want 0", count, err)
	}
}

func TestSubmitLoginAuthProvider_CredentialMode_RBACChangeRevokesExistingSessions(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "LDAP Login",
				Interaction: provider.AuthInteractionCredentials,
				RequestSchema: map[string]interface{}{
					"type": "object",
				},
				Default: true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "ldap-user-rbac-revoke",
			Username:    "ldap.rbac.revoke",
			DisplayName: "LDAP RBAC Revoke",
			Email:       "ldap.rbac.revoke@example.com",
			Enabled:     true,
		},
	})
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)

	if _, err := client.AuthProvider.Create().
		SetID("runtime-credential-rbac-revoke").
		SetName("Runtime Credential RBAC Revoke").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	mappedRole, err := client.Role.Create().
		SetID("role-runtime-credential-rbac-revoke").
		SetName("runtime_credential_rbac_revoke").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create mapped role: %v", err)
	}
	existingUser, err := client.User.Create().
		SetID("user-runtime-credential-rbac-revoke").
		SetUsername("ldap.rbac.revoke").
		SetDisplayName("LDAP RBAC Revoke").
		SetEmail("ldap.rbac.revoke@example.com").
		SetAuthProviderID("runtime-credential-rbac-revoke").
		SetExternalID("ldap-user-rbac-revoke").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing external user: %v", err)
	}
	binding, err := client.RoleBinding.Create().
		SetID("rb-runtime-credential-rbac-revoke").
		SetUserID(existingUser.ID).
		SetRoleID(mappedRole.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing managed binding: %v", err)
	}
	_, err = client.ExternalCohortGrant.Create().
		SetID("grant-runtime-credential-rbac-revoke").
		SetUserID(existingUser.ID).
		SetProviderID("runtime-credential-rbac-revoke").
		SetBindingKey("stale-rbac-binding").
		SetRoleBindingID(binding.ID).
		SetSourceMappingIds([]string{"stale-mapping"}).
		SetLastAppliedAt(time.Now()).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing external cohort grant: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-credential-rbac-revoke/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(ctx, "runtime-credential-rbac-revoke")
	if w.Code != http.StatusOK {
		t.Fatalf("submit status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.LoginResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	claims, err := srv.jwtCfg.ValidateToken(t.Context(), resp.Token)
	if err != nil {
		t.Fatalf("validate token: %v", err)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("read session version after external login: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after RBAC-changing login = %d, want %d", afterVersion, beforeVersion+1)
	}
	if claims.SessionVersion != afterVersion {
		t.Fatalf("token session version = %d, want %d", claims.SessionVersion, afterVersion)
	}
	if slices.Contains(claims.Permissions, "vm:read") {
		t.Fatalf("permissions = %#v, want stale vm:read removed", claims.Permissions)
	}
}

func TestCompleteLoginAuthProviderGet_CallbackCookieAuthenticatesCurrentUser(t *testing.T) {
	t.Parallel()

	username := "alice.callback.cookie." + strings.ToLower(uuid.NewString()[:8])
	email := username + "@example.com"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID:  "callback-cookie-user",
			Username:    username,
			DisplayName: "Alice Callback Cookie",
			Email:       email,
			Enabled:     true,
			Cohorts: []provider.ExternalCohort{
				{Kind: "group", Key: "ops", DisplayName: "Ops"},
			},
		},
	})
	srv, client, _ := newExternalAuthTestServerWithAuthSessions(t)

	if _, err := client.AuthProvider.Create().
		SetID("runtime-callback-cookie").
		SetName("Runtime Callback Cookie").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	mappedRole, err := client.Role.Create().
		SetID("role-runtime-callback-cookie").
		SetName("runtime_callback_cookie").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create mapped role: %v", err)
	}
	if _, mappingErr := client.ExternalCohortMapping.Create().
		SetID("cohort-mapping-callback-cookie").
		SetProviderID("runtime-callback-cookie").
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(mappedRole.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); mappingErr != nil {
		t.Fatalf("create external cohort mapping: %v", mappingErr)
	}

	state, err := issueExternalAuthStateForTest(t, srv, client, "runtime-callback-cookie", "https://console.example.com/dashboard", "qr")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}
	ctx, w := newPublicGinContext(t, http.MethodGet, "/auth/providers/runtime-callback-cookie/callback?code=code-1&state="+state, "")
	srv.CompleteLoginAuthProviderGet(ctx, "runtime-callback-cookie", generated.CompleteLoginAuthProviderGetParams{
		Code:  "code-1",
		State: state,
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != "https://console.example.com/dashboard" {
		t.Fatalf("callback Location = %q, want return_to", got)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == srv.authSessionCookieName() {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || strings.TrimSpace(sessionCookie.Value) == "" {
		t.Fatalf("callback response missing non-empty session cookie: %s", w.Header().Get("Set-Cookie"))
	}

	router := gin.New()
	router.Use(middleware.JWTAuthWithConfig(srv.jwtCfg))
	router.GET("/auth/me", srv.GetCurrentUser)
	meReq := httptest.NewRequest(http.MethodGet, "/auth/me", http.NoBody)
	meReq.AddCookie(sessionCookie)
	meW := httptest.NewRecorder()
	router.ServeHTTP(meW, meReq)
	if meW.Code != http.StatusOK {
		t.Fatalf("auth/me status = %d, want %d, body=%s", meW.Code, http.StatusOK, meW.Body.String())
	}

	var me generated.UserInfo
	mustDecodeJSON(t, meW.Body.Bytes(), &me)
	if me.Username != username {
		t.Fatalf("auth/me username = %q, want %q", me.Username, username)
	}
	if !slices.Contains(me.Permissions, "vm:read") {
		t.Fatalf("auth/me permissions = %#v, want vm:read", me.Permissions)
	}
}

func TestExternalAuthSuccessRedirectTarget(t *testing.T) {
	t.Parallel()

	if got := externalAuthSuccessRedirectTarget(" https://console.example.com/dashboard?tab=vms#row ", false); got != "https://console.example.com/dashboard?tab=vms#row" {
		t.Fatalf("redirect target without password change = %q", got)
	}
	if got := externalAuthSuccessRedirectTarget("https://console.example.com/dashboard?tab=vms#row", true); got != "https://console.example.com/auth/change-password" {
		t.Fatalf("redirect target with password change = %q", got)
	}
}

func TestSubmitLoginAuthProvider_CredentialMode_DisabledResultPersistsAndRevokesSessions(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "LDAP Login",
				Interaction: provider.AuthInteractionCredentials,
				RequestSchema: map[string]interface{}{
					"type": "object",
				},
				Default: true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  "ldap-user-disabled",
			Username:    "ldap.disabled",
			DisplayName: "LDAP Disabled",
			Email:       "ldap.disabled@example.com",
			Enabled:     false,
			Cohorts: []provider.ExternalCohort{
				{Kind: "group", Key: "ops", DisplayName: "ops"},
			},
		},
	})
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)

	if _, err := client.AuthProvider.Create().
		SetID("runtime-credential-disabled").
		SetName("Runtime Credential Disabled").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}
	mappedRole, err := client.Role.Create().
		SetID("role-runtime-credential-disabled").
		SetName("runtime_credential_disabled").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create mapped role: %v", err)
	}
	existingUser, err := client.User.Create().
		SetID("user-runtime-credential-disabled").
		SetUsername("ldap.disabled").
		SetDisplayName("LDAP Disabled").
		SetEmail("ldap.disabled@example.com").
		SetAuthProviderID("runtime-credential-disabled").
		SetExternalID("ldap-user-disabled").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing external user: %v", err)
	}
	binding, err := client.RoleBinding.Create().
		SetID("rb-runtime-credential-disabled").
		SetUserID(existingUser.ID).
		SetRoleID(mappedRole.ID).
		SetScopeType("global").
		SetCreatedBy(externalCohortRoleBindingActor).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing managed binding: %v", err)
	}
	grant, err := client.ExternalCohortGrant.Create().
		SetID("grant-runtime-credential-disabled").
		SetUserID(existingUser.ID).
		SetProviderID("runtime-credential-disabled").
		SetBindingKey("stale-disabled-rbac-binding").
		SetRoleBindingID(binding.ID).
		SetSourceMappingIds([]string{"mapping-runtime-credential-disabled"}).
		SetLastAppliedAt(time.Now()).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing external cohort grant: %v", err)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	ctx, w := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/runtime-credential-disabled/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(ctx, "runtime-credential-disabled")
	if w.Code != http.StatusForbidden {
		t.Fatalf("submit status = %d, want %d, body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "USER_DISABLED")

	reloadedUser, err := client.User.Get(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("reload external user: %v", err)
	}
	if reloadedUser.Enabled {
		t.Fatal("external user Enabled = true after disabled login result, want false")
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("read session version after disabled login: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after disabled login = %d, want %d", afterVersion, beforeVersion+1)
	}
	if _, err := client.ExternalCohortGrant.Get(t.Context(), grant.ID); !ent.IsNotFound(err) {
		t.Fatalf("external cohort grant should be deleted, got err %v", err)
	}
	if _, err := client.RoleBinding.Get(t.Context(), binding.ID); !ent.IsNotFound(err) {
		t.Fatalf("managed role binding should be deleted, got err %v", err)
	}
}

func TestSubmitLoginAuthProvider_CredentialMode_RateLimitedAfterRepeatedInvalidCredentials(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{Key: "password", DisplayName: "Password", Interaction: provider.AuthInteractionCredentials, Default: true},
		},
		credentialErr: provider.NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials"),
	})
	srv, client := newExternalAuthTestServer(t, nil)
	srv.loginRateLimiter = newLoginAttemptLimiter(config.LoginRateLimit{
		Enabled:       true,
		MaxFailures:   2,
		Window:        time.Minute,
		BlockDuration: time.Minute,
	})

	if _, err := client.AuthProvider.Create().
		SetID("runtime-credential-rate-limit").
		SetName("Runtime Credential Rate Limit").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	attempt := func() (*httptest.ResponseRecorder, generated.Error) {
		ctx, w := newPublicGinContext(
			t,
			http.MethodPost,
			"/auth/providers/runtime-credential-rate-limit/login/submit",
			`{"login_mode":"password","credentials":{"username":"alice","password":"bad-password"}}`,
		)
		srv.SubmitLoginAuthProvider(ctx, "runtime-credential-rate-limit")

		var apiErr generated.Error
		if w.Body.Len() > 0 {
			if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
		}
		return w, apiErr
	}

	if w, apiErr := attempt(); w.Code != http.StatusUnauthorized || apiErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("first attempt status=%d code=%q", w.Code, apiErr.Code)
	}
	if w, apiErr := attempt(); w.Code != http.StatusUnauthorized || apiErr.Code != "INVALID_CREDENTIALS" {
		t.Fatalf("second attempt status=%d code=%q", w.Code, apiErr.Code)
	}
	if w, apiErr := attempt(); w.Code != http.StatusTooManyRequests || apiErr.Code != loginRateLimitedErrorCode {
		t.Fatalf("third attempt status=%d code=%q", w.Code, apiErr.Code)
	} else if got := w.Header().Get("Retry-After"); got == "" {
		t.Fatal("expected Retry-After header on rate-limited credential login response")
	}
}

func TestSubmitLoginAuthProvider_DoesNotExposeUnstructuredProviderError(t *testing.T) {
	const (
		providerID    = "runtime-credential-private-error"
		privateDetail = "ldap bind failed for password=sentinel-provider-secret"
	)
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{Key: "password", DisplayName: "Password", Interaction: provider.AuthInteractionCredentials, Default: true},
		},
		credentialErr: errors.New(privateDetail),
	})
	srv, client := newExternalAuthTestServer(t, nil)
	observedLogs := observeExternalAuthFailureLogs(t, srv)
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Credential Private Error").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime provider: %v", err)
	}

	ctx, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+providerID+"/login/submit",
		`{"login_mode":"password","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(ctx, providerID)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("submit status = %d, want %d body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	var response generated.Error
	mustDecodeJSON(t, recorder.Body.Bytes(), &response)
	if response.Code != "EXTERNAL_AUTH_FAILED" || response.Message != "external authentication failed" {
		t.Fatalf("submit response = %#v, want stable generic provider error", response)
	}
	if strings.Contains(recorder.Body.String(), privateDetail) ||
		strings.Contains(recorder.Body.String(), "sentinel-provider-secret") {
		t.Fatalf("submit response exposed private provider error: %s", recorder.Body.String())
	}
	if cookie := recorder.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("failed credential login unexpectedly set a session cookie: %s", cookie)
	}
	requireSafeExternalAuthFailureLog(t, observedLogs, providerID, "credential_authenticate", privateDetail)
}

func TestCompleteLoginAuthProviderPost_ForwardsSanitizedCallbackAndRedirects(t *testing.T) {
	t.Parallel()

	providerID := "runtime-post-callback"
	username := "alice.post.callback." + strings.ToLower(uuid.NewString()[:8])
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID:  "external-post-user-1",
			Username:    username,
			DisplayName: "Alice POST Callback",
			Email:       username + "@example.com",
			Enabled:     true,
		},
	})
	srv, client := newExternalAuthTestServerWithPublicBaseURL(
		t,
		[]string{"https://console.example.com"},
		"https://auth.example.com",
	)
	createExternalAuthCallbackProvider(t, client, providerID, adapter)

	returnTo := "https://console.example.com/login?from=post%20callback"
	state, err := issueExternalAuthStateForTest(t, srv, client, providerID, returnTo, "form_post")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}
	form := url.Values{
		"state": {state},
		"code":  {"form-code"},
		"scope": {"openid", "profile"},
	}
	ctx, w := newExternalAuthFormCallbackContext(
		t,
		"/auth/providers/"+providerID+"/callback?tenant=team-a&tenant=team-b&tracking=query-only",
		form,
	)
	ctx.Request.RemoteAddr = "203.0.113.20:43123"
	ctx.Request.Header.Set("Accept", " text/html ")
	ctx.Request.Header["Accept-Language"] = []string{" en-US ", "", " zh-CN "}
	ctx.Request.Header.Set("User-Agent", " callback-agent ")
	ctx.Request.Header.Set("X-Request-ID", " request-post-1 ")
	ctx.Request.Header.Set("Authorization", "Bearer must-not-leak")
	ctx.Request.Header.Set("Cookie", "upstream_session=must-not-leak")
	ctx.Request.Header.Set("Origin", "https://login.example.com")
	ctx.Request.Header.Set("X-Forwarded-For", "198.51.100.10")

	srv.CompleteLoginAuthProviderPost(ctx, providerID)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusSeeOther, w.Body.String())
	}
	if got := w.Header().Get("Location"); got != returnTo {
		t.Fatalf("callback Location = %q, want %q", got, returnTo)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("callback Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("callback Pragma = %q, want no-cache", got)
	}
	if adapter.callbackCalls != 1 {
		t.Fatalf("adapter callback calls = %d, want 1", adapter.callbackCalls)
	}
	if adapter.callbackReq.Method != http.MethodPost {
		t.Fatalf("adapter callback method = %q, want POST", adapter.callbackReq.Method)
	}
	wantQuery := map[string][]string{
		"tenant":   {"team-a", "team-b"},
		"tracking": {"query-only"},
	}
	if !reflect.DeepEqual(adapter.callbackReq.Query, wantQuery) {
		t.Fatalf("adapter callback query = %#v, want %#v", adapter.callbackReq.Query, wantQuery)
	}
	if !reflect.DeepEqual(adapter.callbackReq.Form, map[string][]string(form)) {
		t.Fatalf("adapter callback form = %#v, want %#v", adapter.callbackReq.Form, form)
	}
	wantHeaders := map[string][]string{
		"Accept":          {"text/html"},
		"Accept-Language": {"en-US", "zh-CN"},
		"Content-Type":    {"application/x-www-form-urlencoded"},
		"User-Agent":      {"callback-agent"},
		"X-Request-Id":    {"request-post-1"},
	}
	if !reflect.DeepEqual(adapter.callbackReq.Header, wantHeaders) {
		t.Fatalf("adapter callback headers = %#v, want only %#v", adapter.callbackReq.Header, wantHeaders)
	}
	if adapter.callbackReq.RemoteAddr != "203.0.113.20:43123" {
		t.Fatalf("adapter callback remote_addr = %q", adapter.callbackReq.RemoteAddr)
	}
	wantCallbackURL := "https://auth.example.com/api/v1/auth/providers/runtime-post-callback/callback"
	if adapter.callbackReq.CallbackURL != wantCallbackURL {
		t.Fatalf("adapter callback_url = %q, want %q", adapter.callbackReq.CallbackURL, wantCallbackURL)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == srv.authSessionCookieName() {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil || strings.TrimSpace(sessionCookie.Value) == "" {
		t.Fatalf("callback response missing non-empty session cookie: %s", w.Header().Get("Set-Cookie"))
	}
	if _, err := srv.jwtCfg.ValidateToken(t.Context(), sessionCookie.Value); err != nil {
		t.Fatalf("validate callback session cookie: %v", err)
	}
}

func TestCompleteLoginAuthProviderPost_RejectsInvalidStateBeforeAdapter(t *testing.T) {
	t.Parallel()

	providerID := "runtime-invalid-post-state"
	returnTo := "https://console.example.com/login"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID: "must-not-run",
			Username:   "must-not-run",
			Enabled:    true,
		},
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})
	createExternalAuthCallbackProvider(t, client, providerID, adapter)

	validState, err := issueExternalAuthStateForTest(t, srv, client, providerID, returnTo, "form_post")
	if err != nil {
		t.Fatalf("issue valid external auth state: %v", err)
	}
	expiredState := signExternalAuthStateForTest(
		t,
		srv,
		providerID,
		"test-provider-generation-binding",
		srv.jwtCfg.Issuer+externalAuthStateIssuerSuffix,
		time.Now().UTC().Add(-time.Minute),
	)
	wrongProviderState := signExternalAuthStateForTest(
		t,
		srv,
		"another-provider",
		"test-provider-generation-binding",
		srv.jwtCfg.Issuer+externalAuthStateIssuerSuffix,
		time.Now().UTC().Add(time.Minute),
	)
	missingGenerationState := signExternalAuthStateForTest(
		t,
		srv,
		providerID,
		"",
		srv.jwtCfg.Issuer+externalAuthStateIssuerSuffix,
		time.Now().UTC().Add(time.Minute),
	)
	legacyV1State := signExternalAuthStateForTest(
		t,
		srv,
		providerID,
		"test-provider-generation-binding",
		srv.jwtCfg.Issuer+"/external-auth",
		time.Now().UTC().Add(time.Minute),
	)

	tests := []struct {
		name string
		form url.Values
		code string
	}{
		{
			name: "missing state",
			form: url.Values{"code": {"form-code"}},
			code: "INVALID_REQUEST",
		},
		{
			name: "tampered state",
			form: url.Values{"state": {validState + "tampered"}, "code": {"form-code"}},
			code: "INVALID_STATE",
		},
		{
			name: "expired state",
			form: url.Values{"state": {expiredState}, "code": {"form-code"}},
			code: "INVALID_STATE",
		},
		{
			name: "state for another provider",
			form: url.Values{"state": {wrongProviderState}, "code": {"form-code"}},
			code: "INVALID_STATE",
		},
		{
			name: "v2 state without provider generation",
			form: url.Values{"state": {missingGenerationState}, "code": {"form-code"}},
			code: "INVALID_STATE",
		},
		{
			name: "legacy v1 state",
			form: url.Values{"state": {legacyV1State}, "code": {"form-code"}},
			code: "INVALID_STATE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callsBefore := adapter.callbackCalls
			ctx, w := newExternalAuthFormCallbackContext(
				t,
				"/auth/providers/"+providerID+"/callback",
				tt.form,
			)

			srv.CompleteLoginAuthProviderPost(ctx, providerID)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
			}
			if adapter.callbackCalls != callsBefore {
				t.Fatalf("adapter callback calls = %d, want unchanged at %d", adapter.callbackCalls, callsBefore)
			}
			if got := w.Header().Values("Set-Cookie"); len(got) != 0 {
				t.Fatalf("invalid state emitted session cookie: %#v", got)
			}
			if got := w.Header().Get("Location"); got != "" {
				t.Fatalf("invalid state emitted redirect Location %q", got)
			}
			requireExternalAuthBridgeSecurityHeaders(t, w)
			if !strings.Contains(w.Body.String(), `"code":"`+tt.code+`"`) {
				t.Fatalf("bridge body missing code %q: %s", tt.code, w.Body.String())
			}
		})
	}
}

func TestCompleteLoginAuthProviderPost_AdapterFailureRendersSafeBridge(t *testing.T) {
	t.Parallel()

	const (
		providerID    = "runtime-post-adapter-failure"
		returnTo      = "https://console.example.com/login?source=external"
		privateDetail = "callback exchange failed for code=sentinel-provider-secret"
	)
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackErr: errors.New(privateDetail),
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})
	observedLogs := observeExternalAuthFailureLogs(t, srv)
	createExternalAuthCallbackProvider(t, client, providerID, adapter)
	state, err := issueExternalAuthStateForTest(t, srv, client, providerID, returnTo, "form_post")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}
	ctx, w := newExternalAuthFormCallbackContext(
		t,
		"/auth/providers/"+providerID+"/callback",
		url.Values{"state": {state}, "code": {"rejected-code"}},
	)

	srv.CompleteLoginAuthProviderPost(ctx, providerID)

	if adapter.callbackCalls != 1 {
		t.Fatalf("adapter callback calls = %d, want 1", adapter.callbackCalls)
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if got := w.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("failed callback emitted session cookie: %#v", got)
	}
	if got := w.Header().Get("Location"); got != "" {
		t.Fatalf("failed callback emitted redirect Location %q", got)
	}
	requireExternalAuthBridgeSecurityHeaders(t, w)
	if !strings.Contains(w.Body.String(), `"code":"EXTERNAL_AUTH_FAILED"`) {
		t.Fatalf("bridge body missing EXTERNAL_AUTH_FAILED: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"return_to":"https://console.example.com/login?source=external"`) {
		t.Fatalf("bridge body missing safe return_to: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), privateDetail) ||
		strings.Contains(w.Body.String(), "sentinel-provider-secret") {
		t.Fatalf("callback bridge exposed private provider error: %s", w.Body.String())
	}
	requireSafeExternalAuthFailureLog(t, observedLogs, providerID, "login_callback", privateDetail)
}

func TestRenderExternalAuthBridge_UsesNonceNoStoreAndEscapesScriptValues(t *testing.T) {
	t.Parallel()

	dangerous := `</script><script id="pwned">alert(1)</script><img src=x onerror=alert(2)>`
	returnTo := "https://console.example.com/return?payload=" + dangerous
	targetURL := "https://console.example.com/target?payload=" + dangerous
	ctx, w := newPublicGinContext(t, http.MethodGet, "/external-auth-bridge", "")
	(&Server{}).renderExternalAuthBridge(
		ctx,
		http.StatusBadRequest,
		returnTo,
		targetURL,
		externalAuthCallbackPayload{
			Type:     externalAuthBridgeMessageType,
			Success:  false,
			Code:     "FAILED" + dangerous,
			ReturnTo: returnTo,
		},
	)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("bridge status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	nonce := requireExternalAuthBridgeSecurityHeaders(t, w)
	body := w.Body.String()
	if strings.Count(body, "<script") != 1 {
		t.Fatalf("bridge body contains unexpected executable tags: %s", body)
	}
	for _, raw := range []string{`<script id="pwned">`, "<img src=x", dangerous} {
		if strings.Contains(body, raw) {
			t.Fatalf("bridge body contains unescaped attacker input %q: %s", raw, body)
		}
	}
	if !strings.Contains(body, `\u003c/script\u003e`) || !strings.Contains(body, `\u003cimg`) {
		t.Fatalf("bridge body does not contain JSON HTML escaping: %s", body)
	}
	if !strings.Contains(body, `<script nonce="`+nonce+`">`) {
		t.Fatalf("bridge script nonce does not match CSP nonce %q: %s", nonce, body)
	}
	if !strings.Contains(body, `const targetOrigin = "https://console.example.com";`) {
		t.Fatalf("bridge body target origin was not reduced to scheme+host: %s", body)
	}
}

func createExternalAuthCallbackProvider(
	t *testing.T,
	client *ent.Client,
	providerID string,
	adapter *testRuntimeAuthAdapter,
) {
	t.Helper()
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Runtime Callback Test").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create runtime auth provider: %v", err)
	}
}

func issueExternalAuthStateForTest(
	t *testing.T,
	srv *Server,
	client *ent.Client,
	providerID, returnTo, loginMode string,
) (string, error) {
	t.Helper()
	providerRow, err := client.AuthProvider.Get(t.Context(), providerID)
	if err != nil {
		return "", fmt.Errorf("load auth provider for external auth state: %w", err)
	}
	providerGeneration, err := service.CaptureAuthProviderGeneration(providerRow)
	if err != nil {
		return "", err
	}
	return srv.issueExternalAuthState(providerID, returnTo, loginMode, providerGeneration)
}

func newExternalAuthFormCallbackContext(
	t *testing.T,
	target string,
	form url.Values,
) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req
	return c, w
}

func signExternalAuthStateForTest(
	t *testing.T,
	srv *Server,
	providerID string,
	providerGeneration string,
	issuer string,
	expiresAt time.Time,
) string {
	t.Helper()
	now := time.Now().UTC()
	claims := externalAuthStateClaims{
		ProviderID:         providerID,
		ProviderGeneration: providerGeneration,
		ReturnTo:           "https://console.example.com/login",
		LoginMode:          "form_post",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   providerID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now.Add(-time.Minute)),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Minute)),
			ID:        uuid.NewString(),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(srv.jwtCfg.SigningKey)
	if err != nil {
		t.Fatalf("sign external auth state: %v", err)
	}
	return signed
}

func requireExternalAuthBridgeSecurityHeaders(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("bridge Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("bridge Pragma = %q, want no-cache", got)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("bridge Content-Type = %q, want text/html; charset=utf-8", got)
	}

	csp := w.Header().Get("Content-Security-Policy")
	const nonceMarker = "script-src 'nonce-"
	markerIndex := strings.Index(csp, nonceMarker)
	if markerIndex < 0 {
		t.Fatalf("bridge CSP is missing nonce-bound script-src: %q", csp)
	}
	nonceStart := markerIndex + len(nonceMarker)
	nonceEndOffset := strings.Index(csp[nonceStart:], "'")
	if nonceEndOffset <= 0 {
		t.Fatalf("bridge CSP contains an invalid nonce: %q", csp)
	}
	nonce := csp[nonceStart : nonceStart+nonceEndOffset]
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("bridge CSP unexpectedly permits unsafe-inline: %q", csp)
	}
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("bridge CSP is missing deny-by-default directives: %q", csp)
	}
	if !strings.Contains(w.Body.String(), `<script nonce="`+nonce+`">`) {
		t.Fatalf("bridge body nonce does not match CSP nonce %q: %s", nonce, w.Body.String())
	}
	return nonce
}

func newExternalAuthTestServer(t *testing.T, allowedOrigins []string) (*Server, *ent.Client) {
	return newExternalAuthTestServerWithPublicBaseURL(t, allowedOrigins, "")
}

func TestFinalizeExternalAuthLoginProviderMigrationInvalidatesOldClaims(t *testing.T) {
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)
	existingUser, destinationGeneration := seedExternalAuthProviderMigration(t, client)

	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("read session version before provider migration: %v", err)
	}
	oldClaims := &middleware.JWTClaims{
		UserID:         existingUser.ID,
		Username:       existingUser.Username,
		SessionVersion: beforeVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: existingUser.ID,
		},
	}
	if validateErr := authSessions.ValidateClaims(t.Context(), oldClaims); validateErr != nil {
		t.Fatalf("validate claims before provider migration: %v", validateErr)
	}

	loginResp, result, err := srv.finalizeExternalAuthLogin(
		t.Context(),
		"provider-identity-destination",
		destinationGeneration,
		&provider.AuthResult{
			ExternalID:  "destination-external-id",
			Username:    existingUser.Username,
			DisplayName: existingUser.DisplayName,
			Email:       existingUser.Email,
			Enabled:     true,
		},
	)
	if err != nil {
		t.Fatalf("finalize provider migration login: %v", err)
	}
	if result == nil || result.User == nil {
		t.Fatal("provider migration result has no user")
	}
	if !result.IdentityStateChanged {
		t.Fatal("provider migration IdentityStateChanged = false, want true")
	}
	if result.RBACChanged {
		t.Fatal("provider migration RBACChanged = true, want false without cohort grants")
	}

	reloadedUser, err := client.User.Get(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("reload migrated user: %v", err)
	}
	if reloadedUser.AuthProviderID != "provider-identity-destination" {
		t.Fatalf("migrated auth provider = %q, want destination", reloadedUser.AuthProviderID)
	}
	if reloadedUser.ExternalID != "destination-external-id" {
		t.Fatalf("migrated external id = %q, want destination identity", reloadedUser.ExternalID)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("read session version after provider migration: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after provider migration = %d, want %d", afterVersion, beforeVersion+1)
	}
	if validateErr := authSessions.ValidateClaims(t.Context(), oldClaims); !errors.Is(validateErr, middleware.ErrJWTSessionStale) {
		t.Fatalf("validate pre-migration claims error = %v, want %v", validateErr, middleware.ErrJWTSessionStale)
	}
	newClaims, err := srv.jwtCfg.ValidateToken(t.Context(), loginResp.Token)
	if err != nil {
		t.Fatalf("validate post-migration token: %v", err)
	}
	if newClaims.SessionVersion != afterVersion {
		t.Fatalf("post-migration token session version = %d, want %d", newClaims.SessionVersion, afterVersion)
	}

	matchingLoginResp, matchingResult, err := srv.finalizeExternalAuthLogin(
		t.Context(),
		"provider-identity-destination",
		destinationGeneration,
		&provider.AuthResult{
			ExternalID:  "destination-external-id",
			Username:    existingUser.Username,
			DisplayName: existingUser.DisplayName,
			Email:       existingUser.Email,
			Enabled:     true,
		},
	)
	if err != nil {
		t.Fatalf("finalize matching provider identity login: %v", err)
	}
	if matchingResult == nil || matchingResult.IdentityStateChanged || matchingResult.RBACChanged {
		t.Fatalf("matching identity result = %#v, want unchanged identity and RBAC state", matchingResult)
	}
	matchingClaims, err := srv.jwtCfg.ValidateToken(t.Context(), matchingLoginResp.Token)
	if err != nil {
		t.Fatalf("validate matching-identity token: %v", err)
	}
	if matchingClaims.SessionVersion != afterVersion {
		t.Fatalf("matching-identity token session version = %d, want unchanged %d", matchingClaims.SessionVersion, afterVersion)
	}
	stillBoundUser, err := client.User.Get(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("reload matching-identity user: %v", err)
	}
	if stillBoundUser.AuthProviderID != "provider-identity-destination" ||
		stillBoundUser.ExternalID != "destination-external-id" {
		t.Fatalf(
			"matching login changed identity ownership to %q/%q",
			stillBoundUser.AuthProviderID,
			stillBoundUser.ExternalID,
		)
	}
}

func TestSubmitLoginAuthProviderRejectsTokenAfterAnotherProviderTakesIdentity(t *testing.T) {
	const (
		sourceProviderID = "provider-interleaved-source"
		firstProviderID  = "provider-interleaved-b"
		secondProviderID = "provider-interleaved-c"
		userID           = "user-interleaved-provider-takeover"
		username         = "interleaved.provider.takeover"
		email            = "interleaved.provider.takeover@example.com"
		firstExternalID  = "interleaved-external-b"
		secondExternalID = "interleaved-external-c"
	)
	firstAdapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		loginModes: []provider.AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "Credential Login",
				Interaction: provider.AuthInteractionCredentials,
				Default:     true,
			},
		},
		credentialResp: &provider.AuthResult{
			ExternalID:  firstExternalID,
			Username:    username,
			DisplayName: "Interleaved Provider Takeover",
			Email:       email,
			Enabled:     true,
		},
	})
	srv, client, _ := newExternalAuthTestServerWithAuthSessions(t)

	for _, providerSeed := range []struct {
		id       string
		name     string
		authType string
	}{
		{id: sourceProviderID, name: "Interleaved Source", authType: "oidc"},
		{id: firstProviderID, name: "Interleaved Provider B", authType: firstAdapter.typeKey},
		{id: secondProviderID, name: "Interleaved Provider C", authType: "oidc"},
	} {
		if _, err := client.AuthProvider.Create().
			SetID(providerSeed.id).
			SetName(providerSeed.name).
			SetAuthType(providerSeed.authType).
			SetConfig(map[string]interface{}{}).
			SetEnabled(true).
			SetCreatedBy("admin-1").
			Save(t.Context()); err != nil {
			t.Fatalf("create %s: %v", providerSeed.name, err)
		}
	}
	secondProvider, err := client.AuthProvider.Get(t.Context(), secondProviderID)
	if err != nil {
		t.Fatalf("load provider C: %v", err)
	}
	secondGeneration, err := service.CaptureAuthProviderGeneration(secondProvider)
	if err != nil {
		t.Fatalf("capture provider C generation: %v", err)
	}
	if _, seedUserErr := client.User.Create().
		SetID(userID).
		SetUsername(username).
		SetDisplayName("Interleaved Provider Takeover").
		SetEmail(email).
		SetAuthProviderID(sourceProviderID).
		SetExternalID("interleaved-external-source").
		SetEnabled(true).
		Save(t.Context()); seedUserErr != nil {
		t.Fatalf("create source-linked user: %v", seedUserErr)
	}

	var (
		observedFirstProvisioning bool
		secondLoginResp           generated.LoginResponse
		secondLoginResult         *service.ExternalAuthUpsertResult
		secondLoginErr            error
	)
	srv.authSessionBeforeActivate = func(ctx context.Context, activatingUserID string, _ int64) error {
		if activatingUserID != userID {
			return fmt.Errorf("provider B activation user = %q, want %q", activatingUserID, userID)
		}
		firstProvisionedUser, loadErr := client.User.Get(ctx, userID)
		if loadErr != nil {
			return fmt.Errorf("load provider B provisioned user: %w", loadErr)
		}
		observedFirstProvisioning = firstProvisionedUser.AuthProviderID == firstProviderID &&
			firstProvisionedUser.ExternalID == firstExternalID
		if !observedFirstProvisioning {
			return fmt.Errorf(
				"provider B provisioning = %q/%q, want %q/%q",
				firstProvisionedUser.AuthProviderID,
				firstProvisionedUser.ExternalID,
				firstProviderID,
				firstExternalID,
			)
		}

		// Provider B has already signed against a matching, locked user snapshot.
		// Clear the process-local hook before C enters the same activation path;
		// C's version bump must make B's pending activation fail and retry.
		srv.authSessionBeforeActivate = nil
		secondLoginResp, secondLoginResult, secondLoginErr = srv.finalizeExternalAuthLogin(
			ctx,
			secondProviderID,
			secondGeneration,
			&provider.AuthResult{
				ExternalID:  secondExternalID,
				Username:    username,
				DisplayName: "Interleaved Provider Takeover",
				Email:       email,
				Enabled:     true,
			},
		)
		return secondLoginErr
	}

	requestContext, recorder := newPublicGinContext(
		t,
		http.MethodPost,
		"/auth/providers/"+firstProviderID+"/login/submit",
		`{"login_mode":"credentials","credentials":{"username":"alice","password":"secret"}}`,
	)
	srv.SubmitLoginAuthProvider(requestContext, firstProviderID)

	if !observedFirstProvisioning {
		t.Fatal("provider B login did not reach the post-upsert token boundary")
	}
	if secondLoginErr != nil || secondLoginResult == nil || secondLoginResult.User == nil {
		t.Fatalf("provider C login result = %#v, error = %v, want success", secondLoginResult, secondLoginErr)
	}
	if recorder.Code != http.StatusConflict {
		t.Fatalf("provider B login status = %d, want %d body=%s", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	assertErrorCode(t, recorder.Body.Bytes(), "AUTH_PROVIDER_CHANGED")
	if cookie := recorder.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("provider B login unexpectedly set a session cookie: %s", cookie)
	}
	var firstResponse map[string]interface{}
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &firstResponse); decodeErr != nil {
		t.Fatalf("decode provider B conflict response: %v", decodeErr)
	}
	if token, exists := firstResponse["token"]; exists {
		t.Fatalf("provider B conflict response contains token %#v", token)
	}

	secondClaims, err := srv.jwtCfg.ValidateToken(t.Context(), secondLoginResp.Token)
	if err != nil {
		t.Fatalf("validate provider C token: %v", err)
	}
	if secondClaims.UserID != userID {
		t.Fatalf("provider C token user = %q, want %q", secondClaims.UserID, userID)
	}
	finalUser, err := client.User.Get(t.Context(), userID)
	if err != nil {
		t.Fatalf("reload user after provider C takeover: %v", err)
	}
	if finalUser.AuthProviderID != secondProviderID || finalUser.ExternalID != secondExternalID {
		t.Fatalf(
			"final identity = %q/%q, want provider C %q/%q",
			finalUser.AuthProviderID,
			finalUser.ExternalID,
			secondProviderID,
			secondExternalID,
		)
	}
}

func TestFinalizeExternalAuthLoginProviderMigrationRollsBackWhenSessionInvalidationFails(t *testing.T) {
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)
	existingUser, destinationGeneration := seedExternalAuthProviderMigration(t, client)
	beforeVersions := installAuthSessionVersionBumpFailure(t, srv, authSessions, existingUser.ID)

	_, result, err := srv.finalizeExternalAuthLogin(
		t.Context(),
		"provider-identity-destination",
		destinationGeneration,
		&provider.AuthResult{
			ExternalID:  "destination-external-id",
			Username:    existingUser.Username,
			DisplayName: existingUser.DisplayName,
			Email:       existingUser.Email,
			Enabled:     true,
		},
	)
	if err == nil || !strings.Contains(err.Error(), "forced auth session version bump failure") {
		t.Fatalf("finalize provider migration error = %v, want forced session invalidation failure", err)
	}
	if result == nil || !result.IdentityStateChanged {
		t.Fatalf("provider migration result = %#v, want detected identity-state change", result)
	}
	assertAuthSessionVersionBumpFailureTriggered(t, srv)

	reloadedUser, err := client.User.Get(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("reload user after provider migration rollback: %v", err)
	}
	if reloadedUser.AuthProviderID != existingUser.AuthProviderID {
		t.Fatalf("auth provider after rollback = %q, want %q", reloadedUser.AuthProviderID, existingUser.AuthProviderID)
	}
	if reloadedUser.ExternalID != existingUser.ExternalID {
		t.Fatalf("external id after rollback = %q, want %q", reloadedUser.ExternalID, existingUser.ExternalID)
	}
	if reloadedUser.Enabled != existingUser.Enabled {
		t.Fatalf("enabled after rollback = %t, want %t", reloadedUser.Enabled, existingUser.Enabled)
	}
	if !reloadedUser.UpdatedAt.Equal(existingUser.UpdatedAt.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("updated_at after rollback = %s, want %s", reloadedUser.UpdatedAt, existingUser.UpdatedAt)
	}
	assertAuthSessionVersionsUnchanged(t, authSessions, beforeVersions)
}

func seedExternalAuthProviderMigration(
	t *testing.T,
	client *ent.Client,
) (*ent.User, service.AuthProviderGeneration) {
	t.Helper()

	if _, err := client.AuthProvider.Create().
		SetID("provider-identity-source").
		SetName("Identity Source").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create source auth provider: %v", err)
	}
	destination, err := client.AuthProvider.Create().
		SetID("provider-identity-destination").
		SetName("Identity Destination").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create destination auth provider: %v", err)
	}
	destinationGeneration, err := service.CaptureAuthProviderGeneration(destination)
	if err != nil {
		t.Fatalf("capture destination auth provider generation: %v", err)
	}
	existingUser, err := client.User.Create().
		SetID("user-provider-identity-migration").
		SetUsername("provider.identity.migration").
		SetDisplayName("Provider Identity Migration").
		SetEmail("provider.identity.migration@example.com").
		SetAuthProviderID("provider-identity-source").
		SetExternalID("source-external-id").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create source-linked external user: %v", err)
	}
	return existingUser, destinationGeneration
}

func TestFinalizeExternalAuthLoginSerializesWithProviderDelete(t *testing.T) {
	srv, client, authSessions := newExternalAuthTestServerWithAuthSessions(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	const providerID = "provider-delete-jit-serialization"
	providerRow, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Provider Delete JIT Serialization").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{}).
		SetEnabled(true).
		SetCreatedBy("admin-1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	providerGeneration, err := service.CaptureAuthProviderGeneration(providerRow)
	if err != nil {
		t.Fatalf("capture auth provider generation: %v", err)
	}
	if schemaErr := authSessions.EnsureSchema(ctx); schemaErr != nil {
		t.Fatalf("initialize auth session schema: %v", schemaErr)
	}

	lockConn, acquireErr := srv.pool.Acquire(ctx)
	if acquireErr != nil {
		t.Fatalf("acquire provider lock connection: %v", acquireErr)
	}
	const lockSQL = `
SELECT pg_advisory_lock(
  hashtextextended(current_schema() || ':auth_provider:' || $1, 0)
)
`
	const unlockSQL = `
SELECT pg_advisory_unlock(
  hashtextextended(current_schema() || ':auth_provider:' || $1, 0)
)
`
	if _, lockErr := lockConn.Exec(ctx, lockSQL, providerID); lockErr != nil {
		t.Fatalf("hold provider mutation lock: %v", lockErr)
	}
	var blockerPID int32
	if pidErr := lockConn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); pidErr != nil {
		t.Fatalf("query provider mutation blocker PID: %v", pidErr)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cleanupCancel()
			if _, unlockErr := lockConn.Exec(cleanupCtx, unlockSQL, providerID); unlockErr != nil {
				t.Errorf("release provider mutation lock during cleanup: %v", unlockErr)
				rawConn := lockConn.Hijack()
				closeCtx, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer closeCancel()
				if closeErr := rawConn.Close(closeCtx); closeErr != nil {
					t.Errorf("close provider lock connection after cleanup failure: %v", closeErr)
				}
				return
			}
		}
		lockConn.Release()
	}()

	deleteCtx, deleteW := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/auth-providers/"+providerID,
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	deleteCtx.Request = deleteCtx.Request.WithContext(middleware.SetUserContext(ctx, "admin-1", "admin-1", nil))
	deleteDone := runHandlerAsync(func() {
		srv.DeleteAuthProvider(deleteCtx, providerID)
	})

	type loginResult struct {
		result *service.ExternalAuthUpsertResult
		err    error
	}
	var login loginResult
	loginDone := runHandlerAsync(func() {
		_, result, loginErr := srv.finalizeExternalAuthLogin(ctx, providerID, providerGeneration, &provider.AuthResult{
			ExternalID:  "jit-user-delete-race",
			Username:    "jit.delete.race",
			DisplayName: "JIT Delete Race",
			Email:       "jit.delete.race@example.com",
			Enabled:     true,
		})
		login = loginResult{result: result, err: loginErr}
	})

	var (
		waiting  int
		queryErr error
	)
	require.Eventually(t, func() bool {
		queryErr = lockConn.QueryRow(ctx, `
SELECT count(*)
FROM pg_locks AS waiting
JOIN pg_locks AS blocker
  ON blocker.locktype = waiting.locktype
 AND blocker.database IS NOT DISTINCT FROM waiting.database
 AND blocker.classid IS NOT DISTINCT FROM waiting.classid
 AND blocker.objid IS NOT DISTINCT FROM waiting.objid
 AND blocker.objsubid IS NOT DISTINCT FROM waiting.objsubid
WHERE blocker.pid = $1
  AND blocker.locktype = 'advisory'
  AND blocker.granted
  AND NOT waiting.granted
`, blockerPID).Scan(&waiting)
		return queryErr != nil || waiting >= 2
	}, 5*time.Second, 10*time.Millisecond, "provider delete and JIT login did not both wait for the shared lock; waiting=%d", waiting)
	require.NoError(t, queryErr, "count waiting advisory locks")

	if _, unlockErr := lockConn.Exec(ctx, unlockSQL, providerID); unlockErr != nil {
		t.Fatalf("release provider mutation lock: %v", unlockErr)
	}
	lockHeld = false

	select {
	case <-deleteDone:
	case <-ctx.Done():
		t.Fatalf("provider delete did not complete: %v", ctx.Err())
	}
	select {
	case <-loginDone:
	case <-ctx.Done():
		t.Fatalf("JIT login did not complete: %v", ctx.Err())
	}

	linkedUsers, err := client.User.Query().Where(user.AuthProviderIDEQ(providerID)).Count(ctx)
	if err != nil {
		t.Fatalf("count provider-linked users: %v", err)
	}
	_, providerErr := client.AuthProvider.Get(ctx, providerID)
	deleteStatus := deleteCtx.Writer.Status()
	switch deleteStatus {
	case http.StatusNoContent:
		if !errors.Is(login.err, errExternalAuthProviderUnavailable) {
			t.Fatalf("JIT login error = %v, want provider unavailable after successful delete", login.err)
		}
		if !ent.IsNotFound(providerErr) {
			t.Fatalf("provider lookup error = %v, want not found", providerErr)
		}
		if linkedUsers != 0 {
			t.Fatalf("linked user count = %d, want 0 after provider delete", linkedUsers)
		}
	case http.StatusConflict:
		if login.err != nil || login.result == nil || login.result.User == nil {
			t.Fatalf("JIT login result = %#v, error = %v, want committed user before delete conflict", login.result, login.err)
		}
		if providerErr != nil {
			t.Fatalf("provider lookup after delete conflict: %v", providerErr)
		}
		if linkedUsers != 1 {
			t.Fatalf("linked user count = %d, want 1 after JIT wins serialization", linkedUsers)
		}
	default:
		t.Fatalf("delete status = %d, want %d or %d, body=%s", deleteStatus, http.StatusNoContent, http.StatusConflict, deleteW.Body.String())
	}
	if ent.IsNotFound(providerErr) && linkedUsers != 0 {
		t.Fatalf("provider was deleted with %d orphan linked users", linkedUsers)
	}
}

func newExternalAuthTestServerWithPublicBaseURL(t *testing.T, allowedOrigins []string, publicBaseURL string) (*Server, *ent.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbName := strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(strings.ToLower(t.Name()))
	client := testutil.OpenEntPostgres(t, "external_auth_"+dbName)
	return NewServer(ServerDeps{
		EntClient:     client,
		ExternalAuth:  service.NewExternalAuthService(client),
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-0123456789abcdef"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
		},
		PublicBaseURL:  publicBaseURL,
		AllowedOrigins: allowedOrigins,
	}), client
}

func newExternalAuthTestServerWithAuthSessions(t *testing.T) (*Server, *ent.Client, *service.AuthSessionManager) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbName := strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(strings.ToLower(t.Name()))
	pool := testutil.OpenPGXPool(t, "external_auth_sessions_"+dbName)
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)
	return NewServer(ServerDeps{
		EntClient:     client,
		Pool:          pool,
		ExternalAuth:  service.NewExternalAuthService(client),
		AuthSessions:  authSessions,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
		JWTCfg: middleware.JWTConfig{
			SigningKey: []byte("test-signing-key-0123456789abcdef"),
			Issuer:     "shepherd-test",
			ExpiresIn:  time.Hour,
			CookieName: defaultAuthSessionCookieName,
		},
		SessionConfig: config.SessionConfig{Cookie: defaultAuthSessionCookieName, HTTPOnly: true},
	}), client, authSessions
}

func updateExternalAuthProviderConfigWithLock(
	t *testing.T,
	client *ent.Client,
	providerID string,
	storedConfig map[string]interface{},
) {
	t.Helper()
	if err := WithTx(t.Context(), client, func(tx *ent.Tx) error {
		if lockErr := service.LockAuthProviderMutation(t.Context(), tx, providerID); lockErr != nil {
			return lockErr
		}
		_, updateErr := tx.Client().AuthProvider.UpdateOneID(providerID).
			SetConfig(storedConfig).
			Save(t.Context())
		return updateErr
	}); err != nil {
		t.Fatalf("update auth provider config with mutation lock: %v", err)
	}
}

func registerRuntimeAuthTestAdapter(t *testing.T, adapter *testRuntimeAuthAdapter) *testRuntimeAuthAdapter {
	t.Helper()
	if adapter == nil {
		t.Fatal("adapter is nil")
		return adapter
	}
	if adapter.typeKey == "" {
		adapter.typeKey = "test-runtime-auth-" + uuid.NewString()
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register runtime auth adapter: %v", err)
	}
	return adapter
}

func observeExternalAuthFailureLogs(t *testing.T, srv *Server) *observer.ObservedLogs {
	t.Helper()
	if srv == nil {
		t.Fatal("server is nil")
	}
	core, observed := observer.New(zap.DebugLevel)
	observedLogger := zap.New(core)
	srv.externalAuthFailureLog = func(message string, fields ...zap.Field) {
		observedLogger.Warn(message, fields...)
	}
	return observed
}

func requireSafeExternalAuthFailureLog(
	t *testing.T,
	observed *observer.ObservedLogs,
	providerID, operation, privateDetail string,
) {
	t.Helper()
	if observed == nil {
		t.Fatal("observed logs are nil")
	}
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("external auth failure log count = %d, want 1", len(entries))
	}
	entry := entries[0]
	contextFields := entry.ContextMap()
	encodedLog := entry.Message + " " + fmt.Sprint(contextFields)
	if strings.Contains(encodedLog, privateDetail) || strings.Contains(encodedLog, "sentinel-provider-secret") {
		t.Fatalf("external auth failure log exposed private provider detail: %s", encodedLog)
	}
	if entry.Message != "external auth provider operation failed" {
		t.Fatalf("external auth failure log message = %q, want stable classification", entry.Message)
	}
	for key, want := range map[string]string{
		"provider_id":   providerID,
		"operation":     operation,
		"failure_class": "provider_operation_failed",
	} {
		if got, _ := contextFields[key].(string); got != want {
			t.Fatalf("external auth failure log %s = %q, want %q", key, got, want)
		}
	}
	if errorType, _ := contextFields["error_type"].(string); strings.TrimSpace(errorType) == "" {
		t.Fatal("external auth failure log error_type is empty")
	}
	for _, forbiddenField := range []string{"error", "credentials", "runtime_config"} {
		if _, exists := contextFields[forbiddenField]; exists {
			t.Fatalf("external auth failure log contains forbidden field %q", forbiddenField)
		}
	}
}

func newPublicGinContext(t *testing.T, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	var req *http.Request
	if strings.TrimSpace(body) == "" {
		req = httptest.NewRequest(method, target, http.NoBody)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	return c, w
}
