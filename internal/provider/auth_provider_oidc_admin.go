package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const defaultOIDCRequestTimeout = 10 * time.Second

type oidcProviderFactory func(ctx context.Context, issuerURL string) (*oidc.Provider, error)

type oidcAuthProviderAdapter struct {
	configSchema map[string]interface{}
	newProvider  oidcProviderFactory
}

func newOIDCBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return &oidcAuthProviderAdapter{
		configSchema: oidcAuthProviderSchema(),
		newProvider:  oidc.NewProvider,
	}
}

func (a *oidcAuthProviderAdapter) Type() string { return "oidc" }

func (a *oidcAuthProviderAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{
		Type:         a.Type(),
		DisplayName:  "OIDC",
		Description:  "OpenID Connect authorization-code login with verified ID tokens",
		BuiltIn:      true,
		ConfigSchema: a.configSchema,
	}
}

func (a *oidcAuthProviderAdapter) DescribeRuntimeAuth() AuthRuntimeDescriptor {
	return AuthRuntimeDescriptor{
		DisplayName: "OIDC",
		Description: "OpenID Connect authorization-code login",
		LoginModes: []AuthLoginMode{
			{
				Key:         "redirect",
				DisplayName: "OIDC Login",
				Description: "Redirect to the configured OpenID Connect provider",
				Interaction: AuthInteractionRedirect,
				Default:     true,
			},
		},
	}
}

func (a *oidcAuthProviderAdapter) ValidateConfig(config map[string]interface{}) error {
	issuerURL := oidcIssuerURL(config)
	if issuerURL == "" {
		return fmt.Errorf("issuer_url is required")
	}
	parsedIssuer, err := url.Parse(issuerURL)
	if err != nil || parsedIssuer.Scheme == "" || parsedIssuer.Host == "" {
		return fmt.Errorf("issuer_url must be a valid absolute URL")
	}
	if !oidcIssuerSchemeAllowed(parsedIssuer) {
		return fmt.Errorf("issuer_url must use https unless targeting a local development issuer")
	}
	if strings.TrimSpace(configStringValue(config, "client_id")) == "" {
		return fmt.Errorf("client_id is required")
	}
	if strings.TrimSpace(configStringValue(config, "client_secret")) == "" {
		return fmt.Errorf("client_secret is required")
	}
	if _, err := oidcScopes(config); err != nil {
		return err
	}
	if _, err := oidcClaimsMapping(config); err != nil {
		return err
	}
	if redirectURI := strings.TrimSpace(configStringValue(config, "redirect_uri")); redirectURI != "" {
		parsedRedirect, parseErr := url.Parse(redirectURI)
		if parseErr != nil || parsedRedirect.Scheme == "" || parsedRedirect.Host == "" {
			return fmt.Errorf("redirect_uri must be a valid absolute URL")
		}
	}
	if timeout := oidcRequestTimeout(config); timeout <= 0 {
		return fmt.Errorf("request_timeout_seconds must be positive")
	}
	return nil
}

func (a *oidcAuthProviderAdapter) TestConnection(ctx context.Context, config map[string]interface{}) (ok bool, message string, err error) {
	validationMessage := ""
	if validateErr := a.ValidateConfig(config); validateErr != nil {
		validationMessage = validateErr.Error()
	}
	if validationMessage != "" {
		return false, validationMessage, nil
	}

	connectionMessage := ""
	if _, discoverErr := a.discoverProvider(ctx, config); discoverErr != nil {
		connectionMessage = "oidc discovery failed: " + discoverErr.Error()
	}
	if connectionMessage != "" {
		return false, connectionMessage, nil
	}
	return true, "oidc discovery succeeded", nil
}

func (a *oidcAuthProviderAdapter) SampleFields(_ context.Context, config map[string]interface{}) ([]AuthProviderSampleField, error) {
	sampleUsers, ok := config["sample_users"].([]interface{})
	if !ok || len(sampleUsers) == 0 {
		return nil, nil
	}
	objects := make([]map[string]interface{}, 0, len(sampleUsers))
	for _, raw := range sampleUsers {
		if obj, ok := raw.(map[string]interface{}); ok {
			objects = append(objects, obj)
		}
	}
	return sampleFieldsFromObjects(objects), nil
}

func (a *oidcAuthProviderAdapter) StartLogin(ctx context.Context, config map[string]interface{}, req AuthStartRequest) (*AuthStartResponse, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}
	callbackURL := strings.TrimSpace(req.CallbackURL)
	if callbackURL == "" {
		return nil, fmt.Errorf("callback_url is required")
	}
	state := strings.TrimSpace(req.State)
	if state == "" {
		return nil, fmt.Errorf("state is required")
	}

	provider, err := a.discoverProvider(ctx, config)
	if err != nil {
		return nil, err
	}
	scopes, err := oidcScopes(config)
	if err != nil {
		return nil, err
	}
	oauthConfig := oidcOAuth2Config(provider, config, callbackURL, scopes)
	redirectURL := oauthConfig.AuthCodeURL(
		state,
		oauth2.S256ChallengeOption(oidcPKCEVerifier(config, state)),
		oauth2.SetAuthURLParam("nonce", oidcNonce(config, state)),
	)
	return &AuthStartResponse{RedirectURL: redirectURL}, nil
}

func (a *oidcAuthProviderAdapter) CompleteLogin(ctx context.Context, config map[string]interface{}, req AuthCallbackRequest) (*AuthResult, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}
	if providerErr := firstFormValue(req.Query, "error"); providerErr != "" {
		description := firstNonEmpty(firstFormValue(req.Query, "error_description"), providerErr)
		return nil, fmt.Errorf("oidc provider returned error: %s", description)
	}
	code := firstNonEmpty(firstFormValue(req.Query, "code"), firstFormValue(req.Form, "code"))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	state := firstNonEmpty(firstFormValue(req.Query, "state"), firstFormValue(req.Form, "state"))
	if state == "" {
		return nil, fmt.Errorf("state is required")
	}
	callbackURL := firstNonEmpty(strings.TrimSpace(req.CallbackURL), strings.TrimSpace(configStringValue(config, "redirect_uri")))
	if callbackURL == "" {
		return nil, fmt.Errorf("callback_url is required")
	}

	provider, err := a.discoverProvider(ctx, config)
	if err != nil {
		return nil, err
	}
	scopes, err := oidcScopes(config)
	if err != nil {
		return nil, err
	}
	oauthConfig := oidcOAuth2Config(provider, config, callbackURL, scopes)
	exchangeCtx, cancel := context.WithTimeout(ctx, oidcRequestTimeout(config))
	defer cancel()
	token, err := oauthConfig.Exchange(exchangeCtx, code, oauth2.VerifierOption(oidcPKCEVerifier(config, state)))
	if err != nil {
		return nil, fmt.Errorf("oidc token exchange failed: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || strings.TrimSpace(rawIDToken) == "" {
		return nil, fmt.Errorf("oidc token response did not include id_token")
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: configStringValue(config, "client_id")})
	idToken, err := verifier.Verify(exchangeCtx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("oidc id_token verification failed: %w", err)
	}
	if expectedNonce := oidcNonce(config, state); idToken.Nonce != expectedNonce {
		return nil, fmt.Errorf("oidc nonce verification failed")
	}

	claims := map[string]interface{}{}
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("decode oidc id_token claims: %w", err)
	}
	if idToken.Subject != "" {
		claims["sub"] = idToken.Subject
	}
	if userInfo, infoErr := provider.UserInfo(exchangeCtx, oauth2.StaticTokenSource(token)); infoErr == nil && userInfo != nil {
		if userInfo.Subject != "" && idToken.Subject != "" && userInfo.Subject != idToken.Subject {
			return nil, fmt.Errorf("oidc userinfo subject mismatch")
		}
		userInfoClaims := map[string]interface{}{}
		if claimsErr := userInfo.Claims(&userInfoClaims); claimsErr == nil {
			for key, value := range userInfoClaims {
				if key == "sub" {
					continue
				}
				claims[key] = value
			}
		}
	}

	return oidcAuthResultFromClaims(config, claims)
}

func (a *oidcAuthProviderAdapter) discoverProvider(ctx context.Context, config map[string]interface{}) (*oidc.Provider, error) {
	if a.newProvider == nil {
		return nil, fmt.Errorf("oidc provider factory is not configured")
	}
	discoveryCtx, cancel := context.WithTimeout(ctx, oidcRequestTimeout(config))
	defer cancel()
	provider, err := a.newProvider(discoveryCtx, oidcIssuerURL(config))
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func oidcOAuth2Config(provider *oidc.Provider, config map[string]interface{}, callbackURL string, scopes []string) oauth2.Config {
	return oauth2.Config{
		ClientID:     strings.TrimSpace(configStringValue(config, "client_id")),
		ClientSecret: configStringValue(config, "client_secret"),
		RedirectURL:  strings.TrimSpace(callbackURL),
		Endpoint:     provider.Endpoint(),
		Scopes:       scopes,
	}
}

func oidcIssuerURL(config map[string]interface{}) string {
	return firstNonEmpty(configStringValue(config, "issuer_url"), configStringValue(config, "issuer"))
}

func oidcIssuerSchemeAllowed(parsed *url.URL) bool {
	if strings.EqualFold(parsed.Scheme, "https") {
		return true
	}
	if !strings.EqualFold(parsed.Scheme, "http") || providerReleaseMode() {
		return false
	}
	host := strings.Trim(strings.ToLower(parsed.Hostname()), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func oidcScopes(config map[string]interface{}) ([]string, error) {
	raw, exists := config["scopes"]
	if !exists || raw == nil {
		return []string{oidc.ScopeOpenID, "profile", "email"}, nil
	}
	var scopes []string
	switch typed := raw.(type) {
	case []string:
		scopes = append(scopes, typed...)
	case []interface{}:
		for _, item := range typed {
			scope := strings.TrimSpace(fmt.Sprint(item))
			if scope != "" {
				scopes = append(scopes, scope)
			}
		}
	default:
		return nil, fmt.Errorf("scopes must be an array of strings")
	}
	if !slices.Contains(scopes, oidc.ScopeOpenID) {
		scopes = append([]string{oidc.ScopeOpenID}, scopes...)
	}
	return scopes, nil
}

func oidcClaimsMapping(config map[string]interface{}) (map[string]string, error) {
	raw, ok := config["claims_mapping"]
	if !ok || raw == nil {
		return nil, nil
	}
	typed, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("claims_mapping must be an object")
	}
	out := make(map[string]string, len(typed))
	for key, value := range typed {
		source := strings.TrimSpace(key)
		target := strings.TrimSpace(fmt.Sprint(value))
		if source == "" || target == "" {
			continue
		}
		out[source] = target
	}
	return out, nil
}

func oidcMappedClaimString(claims map[string]interface{}, mapping map[string]string, target string) string {
	target = strings.TrimSpace(target)
	for source, mappedTarget := range mapping {
		if strings.EqualFold(strings.TrimSpace(mappedTarget), target) {
			if value := oidcClaimString(claims, source); value != "" {
				return value
			}
		}
	}
	if source, ok := mapping[target]; ok {
		return oidcClaimString(claims, source)
	}
	return ""
}

func oidcMappedClaimValues(claims map[string]interface{}, mapping map[string]string, target string) []string {
	target = strings.TrimSpace(target)
	for source, mappedTarget := range mapping {
		if strings.EqualFold(strings.TrimSpace(mappedTarget), target) {
			if values := oidcClaimStrings(claims, source); len(values) > 0 {
				return values
			}
		}
	}
	if source, ok := mapping[target]; ok {
		return oidcClaimStrings(claims, source)
	}
	return nil
}

func oidcAuthResultFromClaims(config, claims map[string]interface{}) (*AuthResult, error) {
	mapping, err := oidcClaimsMapping(config)
	if err != nil {
		return nil, err
	}
	externalID := firstNonEmpty(
		oidcMappedClaimString(claims, mapping, "external_id"),
		oidcClaimString(claims, "sub"),
	)
	username := firstNonEmpty(
		oidcMappedClaimString(claims, mapping, "username"),
		oidcClaimString(claims, "preferred_username"),
		oidcClaimString(claims, "email"),
		oidcClaimString(claims, "nickname"),
		externalID,
	)
	displayName := firstNonEmpty(
		oidcMappedClaimString(claims, mapping, "display_name"),
		oidcClaimString(claims, "name"),
		oidcClaimString(claims, "preferred_username"),
		oidcClaimString(claims, "email"),
		username,
	)
	email := firstNonEmpty(
		oidcMappedClaimString(claims, mapping, "email"),
		oidcClaimString(claims, "email"),
	)
	groupValues := firstNonEmptySlice(
		oidcMappedClaimValues(claims, mapping, "cohorts"),
		oidcClaimStrings(claims, firstNonEmpty(configStringValue(config, "groups_claim"), "groups")),
		oidcClaimStrings(claims, "roles"),
	)

	result := &AuthResult{
		ExternalID:        externalID,
		Username:          username,
		DisplayName:       displayName,
		Email:             email,
		Enabled:           true,
		Cohorts:           externalCohortsFromStringValues("group", groupValues),
		ProfileAttributes: oidcProfileAttributes(claims),
	}
	if result.ExternalID == "" || result.Username == "" {
		return nil, fmt.Errorf("oidc claims cannot be mapped to canonical auth result")
	}
	return result, nil
}

func oidcProfileAttributes(claims map[string]interface{}) AuthProfileAttributes {
	attributes := AuthProfileAttributes{}
	if value := oidcClaimString(claims, "given_name"); value != "" {
		attributes["given_name"] = value
	}
	if value := oidcClaimString(claims, "family_name"); value != "" {
		attributes["family_name"] = value
	}
	if value := firstNonEmpty(oidcClaimString(claims, "nickname"), oidcClaimString(claims, "preferred_username")); value != "" {
		attributes["preferred_name"] = value
	}
	if value := oidcClaimString(claims, "phone_number"); value != "" {
		attributes["phone_number"] = value
	}
	if value := oidcClaimString(claims, "locale"); value != "" {
		attributes["locale"] = value
	}
	if value := oidcClaimString(claims, "picture"); value != "" {
		attributes["avatar_url"] = value
	}
	if value := oidcClaimString(claims, "organization"); value != "" {
		attributes["organization"] = value
	}
	if values := oidcClaimStrings(claims, "department"); len(values) > 0 {
		if len(values) == 1 {
			attributes["organization_unit"] = values[0]
		} else {
			attributes["organization_unit"] = values
		}
	}
	return attributes
}

func oidcClaimString(claims map[string]interface{}, key string) string {
	key = strings.TrimSpace(key)
	if key == "" || claims == nil {
		return ""
	}
	raw, ok := claims[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return strings.TrimSpace(typed.String())
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func oidcClaimStrings(claims map[string]interface{}, key string) []string {
	key = strings.TrimSpace(key)
	if key == "" || claims == nil {
		return nil
	}
	raw, ok := claims[key]
	if !ok || raw == nil {
		return nil
	}
	var values []string
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []interface{}:
		for _, item := range typed {
			values = append(values, strings.TrimSpace(fmt.Sprint(item)))
		}
	case string:
		for _, item := range strings.Split(typed, ",") {
			values = append(values, strings.TrimSpace(item))
		}
	default:
		values = append(values, strings.TrimSpace(fmt.Sprint(typed)))
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func oidcPKCEVerifier(config map[string]interface{}, state string) string {
	return oidcHMACBase64(config, "oidc-pkce-verifier", state)
}

func oidcNonce(config map[string]interface{}, state string) string {
	return oidcHMACBase64(config, "oidc-nonce", state)
}

func oidcHMACBase64(config map[string]interface{}, purpose, state string) string {
	key := []byte(configStringValue(config, "client_secret"))
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(purpose))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.TrimSpace(state)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func oidcRequestTimeout(config map[string]interface{}) time.Duration {
	if raw, ok := config["request_timeout_seconds"]; ok && raw != nil {
		switch typed := raw.(type) {
		case int:
			return time.Duration(typed) * time.Second
		case int32:
			return time.Duration(typed) * time.Second
		case int64:
			return time.Duration(typed) * time.Second
		case float64:
			return time.Duration(int(typed)) * time.Second
		}
	}
	return defaultOIDCRequestTimeout
}
