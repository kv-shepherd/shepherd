package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestOIDCAuthProviderSupportsRedirectRuntime(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("oidc")
	if adapter == nil {
		t.Fatal("oidc auth provider adapter is nil")
	}
	if _, ok := adapter.(AuthRuntimeCapability); !ok {
		t.Fatal("oidc auth provider adapter does not implement AuthRuntimeCapability")
	}
	descriptor := adapter.(AuthRuntimeDescriber).DescribeRuntimeAuth()
	if len(descriptor.LoginModes) != 1 || descriptor.LoginModes[0].Interaction != AuthInteractionRedirect {
		t.Fatalf("runtime descriptor = %#v, want one redirect mode", descriptor)
	}
}

func TestOIDCAuthProviderStartLoginBuildsStandardAuthCodeURL(t *testing.T) {
	server, _ := newTestOIDCProvider(t)
	adapter := newOIDCBuiltInAuthProviderAdapter().(*oidcAuthProviderAdapter)
	config := testOIDCConfig(server.URL)

	resp, err := adapter.StartLogin(t.Context(), config, AuthStartRequest{
		CallbackURL: "https://shepherd.example.com/api/v1/auth/providers/oidc-main/callback",
		State:       "state-token",
	})
	if err != nil {
		t.Fatalf("StartLogin() error = %v", err)
	}
	parsed, err := url.Parse(resp.RedirectURL)
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}
	if got := parsed.Query().Get("response_type"); got != "code" {
		t.Fatalf("response_type = %q, want code", got)
	}
	if got := parsed.Query().Get("scope"); !strings.Contains(got, "openid") {
		t.Fatalf("scope = %q, want openid", got)
	}
	if got := parsed.Query().Get("state"); got != "state-token" {
		t.Fatalf("state = %q, want state-token", got)
	}
	if got := parsed.Query().Get("nonce"); got != oidcNonce(config, "state-token") {
		t.Fatalf("nonce = %q, want derived nonce", got)
	}
	if got := parsed.Query().Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", got)
	}
}

func TestOIDCAuthProviderCompleteLoginVerifiesIDTokenAndMapsClaims(t *testing.T) {
	server, tokenRequests := newTestOIDCProvider(t)
	adapter := newOIDCBuiltInAuthProviderAdapter().(*oidcAuthProviderAdapter)
	config := testOIDCConfig(server.URL)

	result, err := adapter.CompleteLogin(t.Context(), config, AuthCallbackRequest{
		CallbackURL: "https://shepherd.example.com/api/v1/auth/providers/oidc-main/callback",
		Query: map[string][]string{
			"code":  {"auth-code"},
			"state": {"state-token"},
		},
	})
	if err != nil {
		t.Fatalf("CompleteLogin() error = %v", err)
	}
	if len(*tokenRequests) != 1 {
		t.Fatalf("token request count = %d, want 1", len(*tokenRequests))
	}
	if got := (*tokenRequests)[0].Get("code_verifier"); got != oidcPKCEVerifier(config, "state-token") {
		t.Fatalf("code_verifier = %q, want derived verifier", got)
	}
	if result.ExternalID != "oidc-subject-1" {
		t.Fatalf("ExternalID = %q", result.ExternalID)
	}
	if result.Username != "alice" {
		t.Fatalf("Username = %q", result.Username)
	}
	if result.DisplayName != "Alice Example" {
		t.Fatalf("DisplayName = %q", result.DisplayName)
	}
	if result.Email != "alice@example.com" {
		t.Fatalf("Email = %q", result.Email)
	}
	if len(result.Cohorts) != 2 {
		t.Fatalf("Cohorts = %#v, want 2 groups", result.Cohorts)
	}
	if got := result.ProfileAttributes["phone_number"]; got != "+10000000000" {
		t.Fatalf("phone_number = %#v", got)
	}
}

func TestOIDCAuthProviderCompleteLoginRejectsMismatchedUserInfoSubject(t *testing.T) {
	server, _ := newTestOIDCProviderWithUserInfoSubject(t, "other-subject")
	adapter := newOIDCBuiltInAuthProviderAdapter().(*oidcAuthProviderAdapter)
	config := testOIDCConfig(server.URL)

	_, err := adapter.CompleteLogin(t.Context(), config, AuthCallbackRequest{
		CallbackURL: "https://shepherd.example.com/api/v1/auth/providers/oidc-main/callback",
		Query: map[string][]string{
			"code":  {"auth-code"},
			"state": {"state-token"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "userinfo subject mismatch") {
		t.Fatalf("CompleteLogin() error = %v, want userinfo subject mismatch", err)
	}
}

func testOIDCConfig(issuer string) map[string]interface{} {
	return map[string]interface{}{
		"issuer_url":    issuer,
		"client_id":     "shepherd-client",
		"client_secret": "client-secret",
		"scopes":        []interface{}{"openid", "profile", "email"},
	}
}

func newTestOIDCProvider(t *testing.T) (*httptest.Server, *[]url.Values) {
	return newTestOIDCProviderWithUserInfoSubject(t, "oidc-subject-1")
}

func newTestOIDCProviderWithUserInfoSubject(t *testing.T, userInfoSubject string) (*httptest.Server, *[]url.Values) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	tokenRequests := []url.Values{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			writeTestJSON(t, w, map[string]interface{}{
				"issuer":                                server.URL,
				"authorization_endpoint":                server.URL + "/authorize",
				"token_endpoint":                        server.URL + "/token",
				"jwks_uri":                              server.URL + "/jwks",
				"userinfo_endpoint":                     server.URL + "/userinfo",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/jwks":
			writeTestJSON(t, w, map[string]interface{}{
				"keys": []map[string]string{testJWKFromRSAKey(&key.PublicKey)},
			})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			tokenRequests = append(tokenRequests, r.PostForm)
			state := "state-token"
			idToken := testSignedOIDCIDToken(t, key, server.URL, oidcNonce(testOIDCConfig(server.URL), state))
			writeTestJSON(t, w, map[string]interface{}{
				"access_token": "access-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
				"id_token":     idToken,
			})
		case "/userinfo":
			writeTestJSON(t, w, map[string]interface{}{
				"sub":          userInfoSubject,
				"phone_number": "+10000000000",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &tokenRequests
}

func testSignedOIDCIDToken(t *testing.T, key *rsa.PrivateKey, issuer, nonce string) string {
	t.Helper()
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":                issuer,
		"sub":                "oidc-subject-1",
		"aud":                "shepherd-client",
		"exp":                now.Add(5 * time.Minute).Unix(),
		"iat":                now.Unix(),
		"nonce":              nonce,
		"preferred_username": "alice",
		"name":               "Alice Example",
		"email":              "alice@example.com",
		"groups":             []string{"dev", "ops"},
	})
	token.Header["kid"] = "test-key"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign id token: %v", err)
	}
	return signed
}

func testJWKFromRSAKey(key *rsa.PublicKey) map[string]string {
	return map[string]string{
		"kty": "RSA",
		"use": "sig",
		"kid": "test-key",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, payload interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
