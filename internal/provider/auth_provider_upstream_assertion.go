package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	upstreamAssertionLoginMode           = "redirect"
	upstreamAssertionTrustModeUserInfo   = "token_userinfo"
	upstreamAssertionTrustModeIntrospect = "token_introspection"
	upstreamAssertionTrustModeHeaders    = "trusted_gateway_headers"

	upstreamTokenTransportBearer = "authorization_bearer"
	upstreamTokenTransportQuery  = "query"
	upstreamTokenTransportHeader = "header"
	upstreamTokenTransportCookie = "cookie"
	upstreamTokenTransportForm   = "form"

	defaultUpstreamAssertionTimeout = 10 * time.Second
)

type upstreamAssertionAuthProviderAdapter struct{}

func newUpstreamAssertionBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return newUpstreamAssertionAuthProviderAdapter()
}

func newUpstreamAssertionAuthProviderAdapter() *upstreamAssertionAuthProviderAdapter {
	return &upstreamAssertionAuthProviderAdapter{}
}

func (a *upstreamAssertionAuthProviderAdapter) Type() string { return "upstream_assertion" }

func (a *upstreamAssertionAuthProviderAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{
		Type:         a.Type(),
		DisplayName:  "Upstream Assertion",
		Description:  "Generic upstream token/userinfo, introspection, or trusted-gateway runtime login",
		BuiltIn:      true,
		ConfigSchema: upstreamAssertionAuthProviderSchema(),
	}
}

func (a *upstreamAssertionAuthProviderAdapter) DescribeRuntimeAuth() AuthRuntimeDescriptor {
	return AuthRuntimeDescriptor{
		DisplayName: "Upstream Assertion",
		Description: "Generic upstream token/userinfo, introspection, or trusted-gateway runtime login",
		LoginModes: []AuthLoginMode{
			{
				Key:         upstreamAssertionLoginMode,
				DisplayName: "Redirect Login",
				Description: "Redirect into the configured upstream entry URL",
				Interaction: AuthInteractionRedirect,
				Default:     true,
			},
		},
	}
}

func (a *upstreamAssertionAuthProviderAdapter) ValidateConfig(config map[string]interface{}) error {
	loginEntryURL := strings.TrimSpace(configStringValue(config, "login_entry_url"))
	if _, err := validateUpstreamAssertionURL(loginEntryURL, "login_entry_url"); err != nil {
		return err
	}

	trustMode := strings.TrimSpace(configStringValue(config, "trust_mode"))
	switch trustMode {
	case upstreamAssertionTrustModeUserInfo:
		if _, err := validateUpstreamAssertionURL(configStringValue(config, "userinfo_endpoint"), "userinfo_endpoint"); err != nil {
			return err
		}
		if strings.TrimSpace(configStringValue(config, "external_id_path")) == "" {
			return fmt.Errorf("external_id_path is required for token_userinfo mode")
		}
		if strings.TrimSpace(configStringValue(config, "username_path")) == "" {
			return fmt.Errorf("username_path is required for token_userinfo mode")
		}
	case upstreamAssertionTrustModeIntrospect:
		if _, err := validateUpstreamAssertionURL(configStringValue(config, "introspection_endpoint"), "introspection_endpoint"); err != nil {
			return err
		}
		if strings.TrimSpace(configStringValue(config, "external_id_path")) == "" {
			return fmt.Errorf("external_id_path is required for token_introspection mode")
		}
		if strings.TrimSpace(configStringValue(config, "username_path")) == "" {
			return fmt.Errorf("username_path is required for token_introspection mode")
		}
	case upstreamAssertionTrustModeHeaders:
		if len(configStringArrayValue(config, "trusted_gateway_cidrs")) == 0 {
			return fmt.Errorf("trusted_gateway_cidrs is required for trusted_gateway_headers mode")
		}
		if strings.TrimSpace(configStringValue(config, "trusted_header_external_id")) == "" {
			return fmt.Errorf("trusted_header_external_id is required for trusted_gateway_headers mode")
		}
		if strings.TrimSpace(configStringValue(config, "trusted_header_username")) == "" {
			return fmt.Errorf("trusted_header_username is required for trusted_gateway_headers mode")
		}
	default:
		return fmt.Errorf("unsupported trust_mode %q", trustMode)
	}

	if err := validateIncomingTokenTransport(strings.TrimSpace(configStringValue(config, "incoming_token_transport"))); err != nil {
		return err
	}
	if err := validateUpstreamTokenTransport(strings.TrimSpace(configStringValue(config, "upstream_token_transport"))); err != nil {
		return err
	}
	for _, cidr := range configStringArrayValue(config, "trusted_gateway_cidrs") {
		if _, err := parseTrustedPrefix(cidr); err != nil {
			return fmt.Errorf("invalid trusted_gateway_cidrs entry %q", cidr)
		}
	}

	return nil
}

func (a *upstreamAssertionAuthProviderAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "configuration accepted; runtime verification requires a real upstream assertion flow", nil
}

func (a *upstreamAssertionAuthProviderAdapter) SampleFields(context.Context, map[string]interface{}) ([]AuthProviderSampleField, error) {
	return nil, nil
}

func (a *upstreamAssertionAuthProviderAdapter) StartLogin(_ context.Context, config map[string]interface{}, req AuthStartRequest) (*AuthStartResponse, error) {
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

	redirectURL, err := url.Parse(strings.TrimSpace(configStringValue(config, "login_entry_url")))
	if err != nil {
		return nil, fmt.Errorf("invalid login_entry_url: %w", err)
	}
	query := redirectURL.Query()
	query.Set(firstNonEmptyString(configStringValue(config, "callback_param_name"), "redirect_uri"), callbackURL)
	query.Set(firstNonEmptyString(configStringValue(config, "state_param_name"), "state"), state)
	if returnToParam := strings.TrimSpace(configStringValue(config, "return_to_param_name")); returnToParam != "" && strings.TrimSpace(req.ReturnTo) != "" {
		query.Set(returnToParam, strings.TrimSpace(req.ReturnTo))
	}
	redirectURL.RawQuery = query.Encode()

	return &AuthStartResponse{RedirectURL: redirectURL.String()}, nil
}

func (a *upstreamAssertionAuthProviderAdapter) CompleteLogin(ctx context.Context, config map[string]interface{}, req AuthCallbackRequest) (*AuthResult, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}

	switch strings.TrimSpace(configStringValue(config, "trust_mode")) {
	case upstreamAssertionTrustModeUserInfo:
		token, err := extractIncomingAssertionToken(req, config)
		if err != nil {
			return nil, err
		}
		payload, err := callUpstreamAssertionEndpoint(ctx, config, configStringValue(config, "userinfo_endpoint"), http.MethodGet, token)
		if err != nil {
			return nil, err
		}
		return mapUpstreamPayloadToAuthResult(payload, config)
	case upstreamAssertionTrustModeIntrospect:
		token, err := extractIncomingAssertionToken(req, config)
		if err != nil {
			return nil, err
		}
		payload, err := callUpstreamAssertionEndpoint(ctx, config, configStringValue(config, "introspection_endpoint"), http.MethodPost, token)
		if err != nil {
			return nil, err
		}
		active, ok := lookupBoolByPath(payload, firstNonEmptyString(configStringValue(config, "active_path"), "active"))
		if !ok || !active {
			return nil, fmt.Errorf("upstream introspection reported inactive assertion")
		}
		return mapUpstreamPayloadToAuthResult(payload, config)
	case upstreamAssertionTrustModeHeaders:
		if err := verifyTrustedGatewayRemote(req.RemoteAddr, configStringArrayValue(config, "trusted_gateway_cidrs")); err != nil {
			return nil, err
		}
		return mapTrustedHeadersToAuthResult(req.Header, config)
	default:
		return nil, fmt.Errorf("unsupported trust_mode %q", strings.TrimSpace(configStringValue(config, "trust_mode")))
	}
}

func validateIncomingTokenTransport(transport string) error {
	switch firstNonEmptyString(strings.TrimSpace(transport), upstreamTokenTransportQuery) {
	case upstreamTokenTransportBearer, upstreamTokenTransportQuery, upstreamTokenTransportHeader, upstreamTokenTransportCookie:
		return nil
	default:
		return fmt.Errorf("unsupported incoming_token_transport %q", transport)
	}
}

func validateUpstreamTokenTransport(transport string) error {
	switch firstNonEmptyString(strings.TrimSpace(transport), upstreamTokenTransportBearer) {
	case upstreamTokenTransportBearer, upstreamTokenTransportQuery, upstreamTokenTransportHeader, upstreamTokenTransportForm:
		return nil
	default:
		return fmt.Errorf("unsupported upstream_token_transport %q", transport)
	}
}

func validateUpstreamAssertionURL(raw, fieldName string) (*url.URL, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("%s is required", fieldName)
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a valid URI", fieldName)
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "http":
	case "https":
	default:
		return nil, fmt.Errorf("%s must use http:// or https://", fieldName)
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("%s must include a host", fieldName)
	}
	return parsed, nil
}

func configStringArrayValue(config map[string]interface{}, key string) []string {
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func configStringMapValue(config map[string]interface{}, key string) map[string]string {
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil
	}
	items, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]string, len(items))
	for k, v := range items {
		k = strings.TrimSpace(k)
		value := strings.TrimSpace(fmt.Sprint(v))
		if k == "" || value == "" {
			continue
		}
		out[k] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func configDurationSeconds(config map[string]interface{}, key string, fallback time.Duration) time.Duration {
	raw, ok := config[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case int:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case int64:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case float64:
		if v > 0 {
			return time.Duration(v) * time.Second
		}
	case json.Number:
		if i, err := v.Int64(); err == nil && i > 0 {
			return time.Duration(i) * time.Second
		}
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && i > 0 {
			return time.Duration(i) * time.Second
		}
	}
	return fallback
}

func extractIncomingAssertionToken(req AuthCallbackRequest, config map[string]interface{}) (string, error) {
	transport := firstNonEmptyString(configStringValue(config, "incoming_token_transport"), upstreamTokenTransportQuery)
	name := firstNonEmptyString(configStringValue(config, "incoming_token_name"), "token")

	switch transport {
	case upstreamTokenTransportBearer:
		value := firstHeaderValue(req.Header, "Authorization")
		if strings.HasPrefix(strings.ToLower(value), "bearer ") {
			value = strings.TrimSpace(value[7:])
		}
		if value == "" {
			return "", fmt.Errorf("authorization bearer token is required")
		}
		return value, nil
	case upstreamTokenTransportQuery:
		value := firstQueryValue(req.Query, name)
		if value == "" {
			value = firstQueryValue(req.Form, name)
		}
		if value == "" {
			return "", fmt.Errorf("query token %q is required", name)
		}
		return value, nil
	case upstreamTokenTransportHeader:
		value := firstHeaderValue(req.Header, name)
		if value == "" {
			return "", fmt.Errorf("header token %q is required", name)
		}
		return value, nil
	case upstreamTokenTransportCookie:
		value := cookieValue(req.Header, name)
		if value == "" {
			return "", fmt.Errorf("cookie token %q is required", name)
		}
		return value, nil
	default:
		return "", fmt.Errorf("unsupported incoming_token_transport %q", transport)
	}
}

func callUpstreamAssertionEndpoint(ctx context.Context, config map[string]interface{}, endpoint, method, token string) (map[string]interface{}, error) {
	requestURL, err := validateUpstreamAssertionURL(endpoint, "upstream endpoint")
	if err != nil {
		return nil, err
	}

	transport := firstNonEmptyString(configStringValue(config, "upstream_token_transport"), upstreamTokenTransportBearer)
	tokenName := firstNonEmptyString(configStringValue(config, "upstream_token_name"), "token")

	var body io.Reader = http.NoBody
	switch transport {
	case upstreamTokenTransportBearer:
		// No-op; token set on header below.
	case upstreamTokenTransportQuery:
		query := requestURL.Query()
		query.Set(tokenName, token)
		requestURL.RawQuery = query.Encode()
	case upstreamTokenTransportHeader:
		// No-op; token set on header below.
	case upstreamTokenTransportForm:
		form := url.Values{}
		form.Set(tokenName, token)
		body = strings.NewReader(form.Encode())
	default:
		return nil, fmt.Errorf("unsupported upstream_token_transport %q", transport)
	}

	timeout := configDurationSeconds(config, "request_timeout_seconds", defaultUpstreamAssertionTimeout)
	client := &http.Client{Timeout: timeout}
	httpReq, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	if transport == upstreamTokenTransportBearer {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	if transport == upstreamTokenTransportHeader {
		httpReq.Header.Set(tokenName, token)
	}
	if transport == upstreamTokenTransportForm {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	httpReq.Header.Set("Accept", "application/json")

	//nolint:gosec // upstream endpoint is privileged admin configuration and validated as an absolute URL.
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call upstream endpoint: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream endpoint returned status %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode upstream JSON: %w", err)
	}
	return payload, nil
}

func mapUpstreamPayloadToAuthResult(payload, config map[string]interface{}) (*AuthResult, error) {
	externalID, ok := lookupStringByPath(payload, configStringValue(config, "external_id_path"))
	if !ok || externalID == "" {
		return nil, fmt.Errorf("external_id_path did not resolve to a non-empty string")
	}
	username, ok := lookupStringByPath(payload, configStringValue(config, "username_path"))
	if !ok || username == "" {
		return nil, fmt.Errorf("username_path did not resolve to a non-empty string")
	}
	displayName, _ := lookupStringByPath(payload, configStringValue(config, "display_name_path"))
	email, _ := lookupStringByPath(payload, configStringValue(config, "email_path"))
	enabled, ok := lookupBoolByPath(payload, configStringValue(config, "enabled_path"))
	if !ok {
		enabled = true
	}

	profileAttributes := AuthProfileAttributes{}
	for key, path := range configStringMapValue(config, "profile_attribute_paths") {
		if value, found := lookupPath(payload, path); found && value != nil {
			profileAttributes[key] = value
		}
	}

	return &AuthResult{
		ExternalID:        externalID,
		Username:          username,
		DisplayName:       firstNonEmptyString(displayName, username),
		Email:             email,
		Enabled:           enabled,
		Cohorts:           extractCohortsFromPayload(payload, config),
		ProfileAttributes: profileAttributes,
	}, nil
}

func mapTrustedHeadersToAuthResult(headers map[string][]string, config map[string]interface{}) (*AuthResult, error) {
	externalID := firstHeaderValue(headers, configStringValue(config, "trusted_header_external_id"))
	if externalID == "" {
		return nil, fmt.Errorf("trusted header external id is required")
	}
	username := firstHeaderValue(headers, configStringValue(config, "trusted_header_username"))
	if username == "" {
		return nil, fmt.Errorf("trusted header username is required")
	}
	displayName := firstHeaderValue(headers, configStringValue(config, "trusted_header_display_name"))
	email := firstHeaderValue(headers, configStringValue(config, "trusted_header_email"))
	enabledHeader := firstHeaderValue(headers, configStringValue(config, "trusted_header_enabled"))
	enabled, ok := parseBoolLoose(enabledHeader)
	if !ok {
		enabled = true
	}

	return &AuthResult{
		ExternalID:  externalID,
		Username:    username,
		DisplayName: firstNonEmptyString(displayName, username),
		Email:       email,
		Enabled:     enabled,
		Cohorts: extractCohortsFromHeader(
			headers,
			configStringValue(config, "trusted_header_cohorts"),
			firstNonEmptyString(configStringValue(config, "trusted_header_cohort_kind"), "group"),
		),
	}, nil
}

func extractCohortsFromPayload(payload, config map[string]interface{}) []ExternalCohort {
	path := strings.TrimSpace(configStringValue(config, "cohort_path"))
	if path == "" {
		return nil
	}
	value, ok := lookupPath(payload, path)
	if !ok {
		return nil
	}
	items := toStringSlice(value)
	if len(items) == 0 {
		return nil
	}
	kind := firstNonEmptyString(configStringValue(config, "cohort_kind"), "group")
	out := make([]ExternalCohort, 0, len(items))
	for _, item := range items {
		out = append(out, ExternalCohort{
			Kind:        kind,
			Key:         item,
			DisplayName: item,
		})
	}
	return out
}

func extractCohortsFromHeader(headers map[string][]string, headerName, cohortKind string) []ExternalCohort {
	if strings.TrimSpace(headerName) == "" {
		return nil
	}
	raw := firstHeaderValue(headers, headerName)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n'
	})
	out := make([]ExternalCohort, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, ExternalCohort{
			Kind:        cohortKind,
			Key:         part,
			DisplayName: part,
		})
	}
	return out
}

func verifyTrustedGatewayRemote(remoteAddr string, cidrs []string) error {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return fmt.Errorf("remote_addr is required for trusted_gateway_headers mode")
	}
	addr, err := parseRemoteAddr(remoteAddr)
	if err != nil {
		return err
	}
	for _, raw := range cidrs {
		prefix, err := parseTrustedPrefix(raw)
		if err == nil && prefix.Contains(addr) {
			return nil
		}
	}
	return fmt.Errorf("remote address %q is outside trusted gateway cidrs", remoteAddr)
}

func parseRemoteAddr(remoteAddr string) (netip.Addr, error) {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		remoteAddr = host
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(remoteAddr))
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid remote_addr %q", remoteAddr)
	}
	return addr.Unmap(), nil
}

func parseTrustedPrefix(raw string) (netip.Prefix, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Prefix{}, fmt.Errorf("empty trusted prefix")
	}
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Prefix{}, err
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Prefix{}, err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	return netip.PrefixFrom(addr.Unmap(), bits), nil
}

func lookupStringByPath(payload map[string]interface{}, path string) (string, bool) {
	value, ok := lookupPath(payload, path)
	if !ok {
		return "", false
	}
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		return v, v != ""
	default:
		s := strings.TrimSpace(fmt.Sprint(v))
		return s, s != ""
	}
}

func lookupBoolByPath(payload map[string]interface{}, path string) (value, ok bool) {
	if strings.TrimSpace(path) == "" {
		return false, false
	}
	resolved, ok := lookupPath(payload, path)
	if !ok {
		return false, false
	}
	return parseBoolLoose(resolved)
}

func parseBoolLoose(value interface{}) (parsed, ok bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "enabled", "active":
			return true, true
		case "0", "false", "no", "n", "disabled", "inactive":
			return false, true
		default:
			return false, false
		}
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case float64:
		return v != 0, true
	case json.Number:
		i, err := v.Int64()
		return i != 0, err == nil
	default:
		return false, false
	}
}

func lookupPath(payload map[string]interface{}, path string) (interface{}, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	current := interface{}(payload)
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return nil, false
		}
		switch node := current.(type) {
		case map[string]interface{}:
			next, ok := node[segment]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(node) {
				return nil, false
			}
			current = node[index]
		default:
			return nil, false
		}
	}
	return current, true
}

func toStringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s := strings.TrimSpace(fmt.Sprint(item))
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		return []string{s}
	default:
		return nil
	}
}

func firstHeaderValue(headers map[string][]string, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	for headerKey, values := range headers {
		if !strings.EqualFold(headerKey, key) || len(values) == 0 {
			continue
		}
		return strings.TrimSpace(values[0])
	}
	return ""
}

func firstQueryValue(values map[string][]string, key string) string {
	if len(values) == 0 {
		return ""
	}
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0])
}

func cookieValue(headers map[string][]string, name string) string {
	raw := firstHeaderValue(headers, "Cookie")
	if raw == "" {
		return ""
	}
	cookies := strings.Split(raw, ";")
	for _, item := range cookies {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == strings.TrimSpace(name) {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
