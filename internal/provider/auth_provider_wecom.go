package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWeComAPIBaseURL     = "https://qyapi.weixin.qq.com"
	defaultWeComOpenBaseURL    = "https://open.work.weixin.qq.com"
	defaultWeComOAuthBaseURL   = "https://open.weixin.qq.com"
	wecomLoginModeQR           = "qr"
	wecomLoginModeInWeCom      = "in_wecom"
	wecomDefaultRequestTimeout = 10 * time.Second
	wecomMaxResponseBodyBytes  = int64(1024 * 1024)
)

type wecomAuthProviderAdapter struct {
	openBaseURL  string
	oauthBaseURL string
	httpClient   *http.Client
}

func newWeComBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return newWeComAuthProviderAdapter()
}

func newWeComAuthProviderAdapter() *wecomAuthProviderAdapter {
	return &wecomAuthProviderAdapter{
		openBaseURL:  defaultWeComOpenBaseURL,
		oauthBaseURL: defaultWeComOAuthBaseURL,
		httpClient: &http.Client{
			Timeout: wecomDefaultRequestTimeout,
		},
	}
}

func (a *wecomAuthProviderAdapter) Type() string { return "wecom" }

func (a *wecomAuthProviderAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{
		Type:         a.Type(),
		DisplayName:  "WeCom",
		Description:  "WeCom QR login and in-app authorization login",
		BuiltIn:      true,
		ConfigSchema: weComAuthProviderSchema(),
	}
}

func (a *wecomAuthProviderAdapter) DescribeRuntimeAuth() AuthRuntimeDescriptor {
	return AuthRuntimeDescriptor{
		DisplayName: "WeCom",
		Description: "WeCom QR login and in-app authorization login",
		LoginModes: []AuthLoginMode{
			{
				Key:         wecomLoginModeQR,
				DisplayName: "QR Login",
				Description: "Desktop browser QR login",
				Interaction: AuthInteractionRedirect,
				Default:     true,
			},
			{
				Key:         wecomLoginModeInWeCom,
				DisplayName: "In-WeCom Login",
				Description: "In-WeCom web authorization login",
				Interaction: AuthInteractionRedirect,
			},
		},
	}
}

func (a *wecomAuthProviderAdapter) ValidateConfig(config map[string]interface{}) error {
	corpID := strings.TrimSpace(configStringValue(config, "corp_id"))
	agentID := strings.TrimSpace(configStringValue(config, "agent_id"))
	agentSecret := strings.TrimSpace(configStringValue(config, "agent_secret"))
	switch {
	case corpID == "":
		return fmt.Errorf("corp_id is required")
	case agentID == "":
		return fmt.Errorf("agent_id is required")
	case agentSecret == "":
		return fmt.Errorf("agent_secret is required")
	}
	return nil
}

func (a *wecomAuthProviderAdapter) TestConnection(ctx context.Context, config map[string]interface{}) (ok bool, message string, err error) {
	validationMessage := ""
	if validateErr := a.ValidateConfig(config); validateErr != nil {
		validationMessage = validateErr.Error()
	}
	if validationMessage != "" {
		return false, validationMessage, nil
	}

	connectionMessage := ""
	if _, tokenErr := a.getAccessToken(ctx, config); tokenErr != nil {
		connectionMessage = tokenErr.Error()
	}
	if connectionMessage != "" {
		return false, connectionMessage, nil
	}
	return true, "WeCom connection succeeded. This validates CorpID and Agent Secret only; sample fields and departments appear after the first real WeCom login.", nil
}

func (a *wecomAuthProviderAdapter) SampleFields(context.Context, map[string]interface{}) ([]AuthProviderSampleField, error) {
	return nil, nil
}

func (a *wecomAuthProviderAdapter) StartLogin(ctx context.Context, config map[string]interface{}, req AuthStartRequest) (*AuthStartResponse, error) {
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

	mode := strings.TrimSpace(req.LoginMode)
	if mode == "" {
		mode = wecomLoginModeQR
	}

	corpID := configStringValue(config, "corp_id")
	agentID := configStringValue(config, "agent_id")
	switch mode {
	case wecomLoginModeQR:
		query := url.Values{}
		query.Set("appid", corpID)
		query.Set("agentid", agentID)
		query.Set("redirect_uri", callbackURL)
		query.Set("state", state)
		return &AuthStartResponse{
			RedirectURL: strings.TrimRight(a.openBaseURL, "/") + "/wwopen/sso/qrConnect?" + query.Encode(),
		}, nil
	case wecomLoginModeInWeCom:
		if !wecomSupportsEmbeddedBrowser(req.UserAgent) {
			return nil, NewAuthStartError("AUTH_LOGIN_MODE_UNAVAILABLE", "in_wecom login requires the WeCom client browser")
		}
		query := url.Values{}
		query.Set("appid", corpID)
		query.Set("redirect_uri", callbackURL)
		query.Set("response_type", "code")
		query.Set("scope", "snsapi_privateinfo")
		query.Set("agentid", agentID)
		query.Set("state", state)
		return &AuthStartResponse{
			RedirectURL: strings.TrimRight(a.oauthBaseURL, "/") + "/connect/oauth2/authorize?" + query.Encode() + "#wechat_redirect",
		}, nil
	default:
		return nil, fmt.Errorf("unsupported login_mode %q", mode)
	}
}

func wecomSupportsEmbeddedBrowser(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	return strings.Contains(ua, "wxwork")
}

func (a *wecomAuthProviderAdapter) CompleteLogin(ctx context.Context, config map[string]interface{}, req AuthCallbackRequest) (*AuthResult, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}

	code := firstFormValue(req.Query, "code")
	if code == "" {
		code = firstFormValue(req.Form, "code")
	}
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	accessToken, err := a.getAccessToken(ctx, config)
	if err != nil {
		return nil, err
	}

	authUser, err := a.getAuthUserInfo(ctx, accessToken, code)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(authUser.UserID) == "" {
		return nil, fmt.Errorf("enterprise contacts without UserId are not supported")
	}

	profile, err := a.getUserProfile(ctx, accessToken, authUser.UserID)
	if err != nil {
		return nil, err
	}

	departmentNames, err := a.getDepartmentNames(ctx, accessToken, profile.DepartmentIDs)
	if err != nil {
		return nil, err
	}
	cohorts := make([]ExternalCohort, 0, len(profile.DepartmentIDs))
	for _, departmentID := range profile.DepartmentIDs {
		cohort := ExternalCohort{
			Kind:        "department",
			Key:         strconv.Itoa(departmentID),
			DisplayName: departmentNames[departmentID],
		}
		if cohort.DisplayName == "" {
			cohort.DisplayName = "Department " + strconv.Itoa(departmentID)
		}
		cohorts = append(cohorts, cohort)
	}

	profileAttributes := AuthProfileAttributes{}
	if alias := strings.TrimSpace(profile.Alias); alias != "" {
		profileAttributes["preferred_name"] = alias
	}
	if mobile := strings.TrimSpace(profile.Mobile); mobile != "" {
		profileAttributes["phone_number"] = mobile
	}
	if position := strings.TrimSpace(profile.Position); position != "" {
		profileAttributes["job_title"] = position
	}
	if avatarURL := strings.TrimSpace(profile.Avatar); avatarURL != "" {
		profileAttributes["avatar_url"] = avatarURL
	}
	if englishName := strings.TrimSpace(profile.EnglishName); englishName != "" {
		profileAttributes["given_name"] = englishName
	}
	if len(departmentNames) > 0 {
		displayDepartments := make([]string, 0, len(profile.DepartmentIDs))
		for _, departmentID := range profile.DepartmentIDs {
			if name := strings.TrimSpace(departmentNames[departmentID]); name != "" {
				displayDepartments = append(displayDepartments, name)
			}
		}
		if len(displayDepartments) > 0 {
			profileAttributes["organization_unit"] = displayDepartments
		}
	}

	enabled := profile.Status == 0 || profile.Status == 1
	return &AuthResult{
		ExternalID:        strings.TrimSpace(profile.UserID),
		Username:          firstNonEmpty(strings.TrimSpace(profile.EnglishName), strings.TrimSpace(profile.UserID)),
		DisplayName:       firstNonEmpty(strings.TrimSpace(profile.Name), strings.TrimSpace(profile.UserID)),
		Email:             firstNonEmpty(strings.TrimSpace(profile.Email), strings.TrimSpace(profile.BizMail)),
		Enabled:           enabled,
		Cohorts:           cohorts,
		ProfileAttributes: profileAttributes,
	}, nil
}

func (a *wecomAuthProviderAdapter) getAccessToken(ctx context.Context, config map[string]interface{}) (string, error) {
	values := url.Values{}
	values.Set("corpid", configStringValue(config, "corp_id"))
	values.Set("corpsecret", configStringValue(config, "agent_secret"))

	var payload map[string]interface{}
	if err := a.getWeComJSON(ctx, "/cgi-bin/gettoken", values, &payload, "gettoken"); err != nil {
		return "", err
	}
	if err := wecomErrorFromPayload(payload); err != nil {
		return "", err
	}
	token := strings.TrimSpace(fmt.Sprint(payload["access_token"]))
	if token == "" {
		return "", fmt.Errorf("wecom gettoken returned empty access_token")
	}
	return token, nil
}

func (a *wecomAuthProviderAdapter) getAuthUserInfo(ctx context.Context, accessToken, code string) (*wecomAuthUserInfoResponse, error) {
	values := url.Values{}
	values.Set("access_token", accessToken)
	values.Set("code", code)

	var payload wecomAuthUserInfoResponse
	if err := a.getWeComJSON(ctx, "/cgi-bin/auth/getuserinfo", values, &payload, "getuserinfo"); err != nil {
		return nil, err
	}
	if err := payload.Err(); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (a *wecomAuthProviderAdapter) getUserProfile(ctx context.Context, accessToken, userID string) (*wecomUserGetResponse, error) {
	values := url.Values{}
	values.Set("access_token", accessToken)
	values.Set("userid", userID)

	var payload wecomUserGetResponse
	if err := a.getWeComJSON(ctx, "/cgi-bin/user/get", values, &payload, "user get"); err != nil {
		return nil, err
	}
	if err := payload.Err(); err != nil {
		return nil, err
	}
	return &payload, nil
}

func (a *wecomAuthProviderAdapter) getDepartmentNames(ctx context.Context, accessToken string, departmentIDs []int) (map[int]string, error) {
	if len(departmentIDs) == 0 {
		return nil, nil
	}
	values := url.Values{}
	values.Set("access_token", accessToken)

	var payload wecomDepartmentListResponse
	if err := a.getWeComJSON(ctx, "/cgi-bin/department/list", values, &payload, "department list"); err != nil {
		return nil, err
	}
	if err := payload.Err(); err != nil {
		return nil, err
	}

	names := make(map[int]string, len(payload.Department))
	for i := range payload.Department {
		department := payload.Department[i]
		if department.ID == 0 {
			continue
		}
		names[department.ID] = strings.TrimSpace(department.Name)
	}
	return names, nil
}

func decodeWeComJSONResponse(resp *http.Response, out interface{}) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wecom request returned status %d", resp.StatusCode)
	}
	payload, err := io.ReadAll(&io.LimitedReader{R: resp.Body, N: wecomMaxResponseBodyBytes + 1})
	if err != nil {
		return fmt.Errorf("read wecom response: %w", err)
	}
	if int64(len(payload)) > wecomMaxResponseBodyBytes {
		return fmt.Errorf("wecom response exceeds %d bytes", wecomMaxResponseBodyBytes)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("decode wecom response: %w", err)
	}
	return nil
}

func (a *wecomAuthProviderAdapter) getWeComJSON(
	ctx context.Context,
	path string,
	values url.Values,
	out interface{},
	action string,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultWeComAPIBaseURL+path, http.NoBody)
	if err != nil {
		return fmt.Errorf("build wecom %s request: %w", action, err)
	}
	req.URL.RawQuery = values.Encode()

	resp, cancel, err := a.doWeComRequest(req)
	if err != nil {
		return fmt.Errorf("wecom %s request failed: %w", action, err)
	}
	if cancel != nil {
		defer cancel()
	}
	defer resp.Body.Close()

	if err := decodeWeComJSONResponse(resp, out); err != nil {
		return err
	}
	return nil
}

func (a *wecomAuthProviderAdapter) doWeComRequest(req *http.Request) (*http.Response, context.CancelFunc, error) {
	if req == nil || req.URL == nil {
		return nil, nil, fmt.Errorf("wecom request url is required")
	}
	if !strings.EqualFold(req.URL.Hostname(), "qyapi.weixin.qq.com") {
		return nil, nil, fmt.Errorf("unexpected wecom host %q", req.URL.Hostname())
	}

	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: wecomDefaultRequestTimeout}
	}
	if client.Timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), client.Timeout)
		req = req.Clone(ctx)
		resp, err := roundTripWeComRequest(client, req)
		if err != nil {
			cancel()
			return nil, nil, err
		}
		return resp, cancel, nil
	}

	resp, err := roundTripWeComRequest(client, req)
	return resp, nil, err
}

func roundTripWeComRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}

func wecomErrorFromPayload(payload map[string]interface{}) error {
	if len(payload) == 0 {
		return nil
	}
	errCode := 0
	switch raw := payload["errcode"].(type) {
	case float64:
		errCode = int(raw)
	case int:
		errCode = raw
	case int32:
		errCode = int(raw)
	case int64:
		errCode = int(raw)
	case json.Number:
		if parsed, err := raw.Int64(); err == nil {
			errCode = int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			errCode = parsed
		}
	}
	if errCode == 0 {
		return nil
	}
	return fmt.Errorf("wecom error %d: %s", errCode, strings.TrimSpace(fmt.Sprint(payload["errmsg"])))
}

type WeComError struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
}

func (e WeComError) Err() error {
	if e.ErrCode == 0 {
		return nil
	}
	return fmt.Errorf("wecom error %d: %s", e.ErrCode, strings.TrimSpace(e.ErrMsg))
}

type wecomAuthUserInfoResponse struct {
	WeComError
	UserID string `json:"UserId"`
	OpenID string `json:"OpenId"`
}

type wecomUserGetResponse struct {
	WeComError
	UserID        string `json:"userid"`
	Name          string `json:"name"`
	Alias         string `json:"alias"`
	Mobile        string `json:"mobile"`
	Email         string `json:"email"`
	BizMail       string `json:"biz_mail"`
	Position      string `json:"position"`
	Avatar        string `json:"avatar"`
	EnglishName   string `json:"english_name"`
	Status        int    `json:"status"`
	DepartmentIDs []int  `json:"department"`
}

type wecomDepartmentListResponse struct {
	WeComError
	Department []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"department"`
}

func firstFormValue(values map[string][]string, key string) string {
	if len(values) == 0 {
		return ""
	}
	items := values[key]
	if len(items) == 0 {
		return ""
	}
	return strings.TrimSpace(items[0])
}
