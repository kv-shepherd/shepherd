package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/externalcohort"
	"kv-shepherd.io/shepherd/ent/externalcohortgrant"
	"kv-shepherd.io/shepherd/ent/role"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type testRuntimeAuthAdapter struct {
	typeKey        string
	startReq       provider.AuthStartRequest
	startResp      *provider.AuthStartResponse
	startErr       error
	credentialReq  provider.AuthCredentialRequest
	credentialResp *provider.AuthResult
	credentialErr  error
	callbackReq    provider.AuthCallbackRequest
	callbackResp   *provider.AuthResult
	callbackErr    error
	loginModes     []provider.AuthLoginMode
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

func (a *testRuntimeAuthAdapter) AuthenticateCredentials(_ context.Context, _ map[string]interface{}, req provider.AuthCredentialRequest) (*provider.AuthResult, error) {
	a.credentialReq = req
	if a.credentialErr != nil {
		return nil, a.credentialErr
	}
	return a.credentialResp, nil
}

func (a *testRuntimeAuthAdapter) CompleteLogin(_ context.Context, _ map[string]interface{}, req provider.AuthCallbackRequest) (*provider.AuthResult, error) {
	a.callbackReq = req
	if a.callbackErr != nil {
		return nil, a.callbackErr
	}
	return a.callbackResp, nil
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
		SetName("Admin Only").
		SetAuthType("oidc").
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

func TestStartLoginAuthProvider_PassesStateAndCallbackURLToRuntimeProvider(t *testing.T) {
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
	if w.Code != http.StatusOK {
		t.Fatalf("start status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.AuthProviderLoginStartResponse
	mustDecodeJSON(t, w.Body.Bytes(), &resp)
	if resp.RedirectUrl != "https://login.example.com/start" {
		t.Fatalf("redirect_url = %q, want %q", resp.RedirectUrl, "https://login.example.com/start")
	}
	if adapter.startReq.State == "" {
		t.Fatal("provider start request state is empty")
	}
	if adapter.startReq.CallbackURL != "https://api.example.com/api/v1/auth/providers/runtime-start/callback" {
		t.Fatalf("callback_url = %q", adapter.startReq.CallbackURL)
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

func TestStartLoginAuthProvider_AllowsRelativeReturnToFromAllowedOrigin(t *testing.T) {
	t.Parallel()

	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		startResp: &provider.AuthStartResponse{RedirectURL: "https://login.example.com/start"},
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})

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
	srv, client := newExternalAuthTestServer(t, []string{"http://localhost:3000"})

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
		startErr: provider.NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "in_wecom login requires the WeCom client browser"),
	})
	srv, client := newExternalAuthTestServer(t, []string{"https://console.example.com"})

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
		`{"login_mode":"in_wecom","return_to":"https://console.example.com/login"}`,
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

func TestCompleteLoginAuthProviderGet_JITProvisionsUserAndReturnsBridge(t *testing.T) {
	t.Parallel()

	username := "alice.wecom." + strings.ToLower(uuid.NewString()[:8])
	email := username + "@example.com"
	adapter := registerRuntimeAuthTestAdapter(t, &testRuntimeAuthAdapter{
		callbackResp: &provider.AuthResult{
			ExternalID:  "wecom-user-1",
			Username:    username,
			DisplayName: "Alice WeCom",
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

	state, err := srv.issueExternalAuthState("runtime-callback", "https://console.example.com/login", "qr")
	if err != nil {
		t.Fatalf("issue external auth state: %v", err)
	}

	ctx, w := newPublicGinContext(t, http.MethodGet, "/auth/providers/runtime-callback/callback?code=code-1&state="+state, "")
	srv.CompleteLoginAuthProviderGet(ctx, "runtime-callback", generated.CompleteLoginAuthProviderGetParams{
		Code:  "code-1",
		State: state,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("callback status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "shepherd.external_auth.complete") {
		t.Fatalf("callback body missing bridge payload: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"return_to":"https://console.example.com/login"`) {
		t.Fatalf("callback body missing return_to payload: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"username":"`+username+`"`) {
		t.Fatalf("callback body missing user payload: %s", w.Body.String())
	}
	tokenMatch := regexp.MustCompile(`"token":"([^"]+)"`).FindStringSubmatch(w.Body.String())
	if len(tokenMatch) != 2 {
		t.Fatalf("callback body missing token payload: %s", w.Body.String())
	}
	claims, err := srv.jwtCfg.ValidateToken(t.Context(), tokenMatch[1])
	if err != nil {
		t.Fatalf("validate callback token: %v", err)
	}
	if !slices.Contains(claims.Permissions, "vm:read") {
		t.Fatalf("callback permissions = %#v, want vm:read", claims.Permissions)
	}

	createdUser, err := client.User.Query().
		Where(
			user.AuthProviderIDEQ("runtime-callback"),
			user.ExternalIDEQ("wecom-user-1"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query created external user: %v", err)
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

func newExternalAuthTestServer(t *testing.T, allowedOrigins []string) (*Server, *ent.Client) {
	return newExternalAuthTestServerWithPublicBaseURL(t, allowedOrigins, "")
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

func registerRuntimeAuthTestAdapter(t *testing.T, adapter *testRuntimeAuthAdapter) *testRuntimeAuthAdapter {
	t.Helper()
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
	if adapter.typeKey == "" {
		adapter.typeKey = "test-runtime-auth-" + uuid.NewString()
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register runtime auth adapter: %v", err)
	}
	return adapter
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
