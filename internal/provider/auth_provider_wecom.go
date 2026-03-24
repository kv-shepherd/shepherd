package provider

import (
	"context"
	"encoding/json"
	"fmt"
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

func (a *wecomAuthProviderAdapter) DescribeDirectorySync() DirectorySyncDescriptor {
	return DirectorySyncDescriptor{
		DisplayName:     "WeCom Directory Sync",
		Description:     "Preview and import WeCom users from selected departments",
		SupportsPreview: true,
		RequestSchema: map[string]interface{}{
			"type": "object",
			"required": []string{
				"department_ids",
			},
			"properties": map[string]interface{}{
				"department_ids": map[string]interface{}{
					"type":        "array",
					"title":       "Department IDs",
					"description": "WeCom department IDs to enumerate",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"include_nested": map[string]interface{}{
					"type":        "boolean",
					"title":       "Include Nested Departments",
					"description": "Whether descendant departments should be included",
					"default":     false,
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"minimum":     1,
					"title":       "Limit",
					"description": "Optional maximum number of returned users after provider-side deduplication",
				},
			},
			"additionalProperties": true,
		},
	}
}

func (a *wecomAuthProviderAdapter) PreviewDirectorySync(
	ctx context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) (*DirectorySyncPreview, error) {
	records, err := a.ListDirectoryUsers(ctx, config, providerRequest)
	if err != nil {
		return nil, err
	}
	items := make([]DirectoryPreviewItem, 0, len(records))
	for _, record := range records {
		items = append(items, DirectoryPreviewItem{Record: record})
	}
	return &DirectorySyncPreview{
		TotalCount: len(items),
		Items:      items,
	}, nil
}

func (a *wecomAuthProviderAdapter) ListDirectoryUsers(
	ctx context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) ([]DirectoryUserRecord, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}

	departmentIDs, includeNested, limit, err := wecomDirectoryRequest(providerRequest)
	if err != nil {
		return nil, err
	}

	accessToken, err := a.getAccessToken(ctx, config)
	if err != nil {
		return nil, err
	}

	users, err := a.listDirectoryUsers(ctx, accessToken, departmentIDs, includeNested)
	if err != nil {
		return nil, err
	}
	departmentNames, err := a.getDepartmentNames(ctx, accessToken, departmentIDs)
	if err != nil {
		return nil, err
	}

	records := make([]DirectoryUserRecord, 0, len(users))
	for i := range users {
		item := users[i]
		record := wecomDirectoryRecordFromUser(item, departmentNames)
		if record.ExternalID == "" || record.Username == "" || record.DisplayName == "" {
			continue
		}
		records = append(records, record)
		if limit > 0 && len(records) >= limit {
			break
		}
	}
	return records, nil
}

func (a *wecomAuthProviderAdapter) BuildScheduledDirectoryEnrichmentPlan(
	_ context.Context,
	config map[string]interface{},
) (*ScheduledDirectoryEnrichmentPlan, error) {
	enabled, _ := config["enrichment_enabled"].(bool)
	if !enabled {
		return &ScheduledDirectoryEnrichmentPlan{Enabled: false}, nil
	}

	mode := DirectoryEnrichmentMode(strings.TrimSpace(configStringValue(config, "enrichment_mode")))
	if mode == "" {
		mode = DirectoryEnrichmentModeEnrichExistingOnly
	}
	if mode != DirectoryEnrichmentModeEnrichExistingOnly {
		return nil, fmt.Errorf("unsupported enrichment_mode %q", mode)
	}

	joinKeyType := DirectoryJoinKeyType(strings.TrimSpace(configStringValue(config, "join_key_type")))
	if joinKeyType == "" {
		joinKeyType = DirectoryJoinKeyUsername
	}
	if joinKeyType != DirectoryJoinKeyUsername {
		return nil, fmt.Errorf("unsupported join_key_type %q", joinKeyType)
	}

	departmentIDs, err := stringArrayConfigValue(config, "scheduled_department_ids")
	if err != nil {
		return nil, err
	}
	if len(departmentIDs) == 0 {
		return nil, fmt.Errorf("scheduled_department_ids is required when scheduled enrichment is enabled")
	}

	scheduleCron := strings.TrimSpace(configStringValue(config, "schedule_cron"))
	if scheduleCron == "" {
		scheduleCron = defaultScheduledEnrichmentCron
	}
	scheduleTimezone := strings.TrimSpace(configStringValue(config, "schedule_timezone"))
	if scheduleTimezone == "" {
		scheduleTimezone = defaultScheduleTimezoneUTC
	}
	includeNested, _ := config["scheduled_include_nested"].(bool)

	return &ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             mode,
		JoinKeyType:      joinKeyType,
		ScheduleCron:     scheduleCron,
		ScheduleTimezone: scheduleTimezone,
		ProviderRequest: map[string]interface{}{
			"department_ids": departmentIDs,
			"include_nested": includeNested,
		},
	}, nil
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

func (a *wecomAuthProviderAdapter) listDirectoryUsers(
	ctx context.Context,
	accessToken string,
	departmentIDs []int,
	includeNested bool,
) ([]wecomDirectoryUser, error) {
	seen := make(map[string]struct{})
	users := make([]wecomDirectoryUser, 0)
	fetchChild := "0"
	if includeNested {
		fetchChild = "1"
	}

	for _, departmentID := range departmentIDs {
		values := url.Values{}
		values.Set("access_token", accessToken)
		values.Set("department_id", strconv.Itoa(departmentID))
		values.Set("fetch_child", fetchChild)

		var payload wecomUserListResponse
		if err := a.getWeComJSON(ctx, "/cgi-bin/user/list", values, &payload, "user list"); err != nil {
			return nil, err
		}
		if err := payload.Err(); err != nil {
			return nil, err
		}
		for i := range payload.UserList {
			item := payload.UserList[i]
			userID := strings.TrimSpace(item.UserID)
			if userID == "" {
				continue
			}
			if _, ok := seen[userID]; ok {
				continue
			}
			seen[userID] = struct{}{}
			users = append(users, item)
		}
	}
	return users, nil
}

func decodeWeComJSONResponse(resp *http.Response, out interface{}) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("wecom request returned status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
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

	resp, err := a.doWeComRequest(req)
	if err != nil {
		return fmt.Errorf("wecom %s request failed: %w", action, err)
	}
	defer resp.Body.Close()

	if err := decodeWeComJSONResponse(resp, out); err != nil {
		return err
	}
	return nil
}

func (a *wecomAuthProviderAdapter) doWeComRequest(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("wecom request url is required")
	}
	if !strings.EqualFold(req.URL.Hostname(), "qyapi.weixin.qq.com") {
		return nil, fmt.Errorf("unexpected wecom host %q", req.URL.Hostname())
	}

	client := a.httpClient
	if client == nil {
		client = &http.Client{Timeout: wecomDefaultRequestTimeout}
	}
	if client.Timeout > 0 {
		ctx, cancel := context.WithTimeout(req.Context(), client.Timeout)
		defer cancel()
		req = req.Clone(ctx)
	}

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

type wecomDirectoryUser struct {
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

type wecomUserListResponse struct {
	WeComError
	UserList []wecomDirectoryUser `json:"userlist"`
}

func wecomDirectoryRecordFromUser(item wecomDirectoryUser, departmentNames map[int]string) DirectoryUserRecord {
	cohorts := make([]ExternalCohort, 0, len(item.DepartmentIDs))
	departmentDisplayNames := make([]string, 0, len(item.DepartmentIDs))
	for _, departmentID := range item.DepartmentIDs {
		displayName := strings.TrimSpace(departmentNames[departmentID])
		if displayName == "" {
			displayName = "Department " + strconv.Itoa(departmentID)
		}
		cohorts = append(cohorts, ExternalCohort{
			Kind:        "department",
			Key:         strconv.Itoa(departmentID),
			DisplayName: displayName,
		})
		departmentDisplayNames = append(departmentDisplayNames, displayName)
	}

	attributes := map[string]interface{}{
		"wecom_status": item.Status,
	}
	if alias := strings.TrimSpace(item.Alias); alias != "" {
		attributes["preferred_name"] = alias
	}
	if mobile := strings.TrimSpace(item.Mobile); mobile != "" {
		attributes["phone_number"] = mobile
	}
	if position := strings.TrimSpace(item.Position); position != "" {
		attributes["job_title"] = position
	}
	if avatarURL := strings.TrimSpace(item.Avatar); avatarURL != "" {
		attributes["avatar_url"] = avatarURL
	}
	if englishName := strings.TrimSpace(item.EnglishName); englishName != "" {
		attributes["given_name"] = englishName
	}
	if len(departmentDisplayNames) > 0 {
		attributes["organization_unit"] = departmentDisplayNames
	}

	return DirectoryUserRecord{
		ExternalID:  strings.TrimSpace(item.UserID),
		Username:    firstNonEmpty(strings.TrimSpace(item.EnglishName), strings.TrimSpace(item.UserID)),
		DisplayName: firstNonEmpty(strings.TrimSpace(item.Name), strings.TrimSpace(item.UserID)),
		Email:       firstNonEmpty(strings.TrimSpace(item.Email), strings.TrimSpace(item.BizMail)),
		Cohorts:     cohorts,
		Attributes:  attributes,
	}
}

func wecomDirectoryRequest(providerRequest map[string]interface{}) (departmentIDs []int, includeNested bool, limit int, err error) {
	if len(providerRequest) == 0 {
		return nil, false, 0, NewDirectorySyncRequestError("department_ids is required")
	}

	departmentIDs, err = integerArrayValue(providerRequest["department_ids"], "department_ids")
	if err != nil {
		return nil, false, 0, err
	}
	if len(departmentIDs) == 0 {
		return nil, false, 0, NewDirectorySyncRequestError("department_ids is required")
	}

	includeNested = false
	if raw, ok := providerRequest["include_nested"]; ok && raw != nil {
		typed, ok := raw.(bool)
		if !ok {
			return nil, false, 0, NewDirectorySyncRequestError("include_nested must be a boolean")
		}
		includeNested = typed
	}

	limit = 0
	if raw, ok := providerRequest["limit"]; ok && raw != nil {
		switch typed := raw.(type) {
		case int:
			limit = typed
		case int32:
			limit = int(typed)
		case int64:
			limit = int(typed)
		case float64:
			limit = int(typed)
		default:
			return nil, false, 0, NewDirectorySyncRequestError("limit must be a positive integer")
		}
		if limit < 1 {
			return nil, false, 0, NewDirectorySyncRequestError("limit must be a positive integer")
		}
	}
	return departmentIDs, includeNested, limit, nil
}

func integerArrayValue(raw interface{}, field string) ([]int, error) {
	switch typed := raw.(type) {
	case []string:
		values := make([]int, 0, len(typed))
		for _, item := range typed {
			parsed, err := strconv.Atoi(strings.TrimSpace(item))
			if err != nil {
				return nil, NewDirectorySyncRequestError(field + " must contain integers")
			}
			values = append(values, parsed)
		}
		return values, nil
	case []interface{}:
		values := make([]int, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case float64:
				values = append(values, int(value))
			case int:
				values = append(values, value)
			case int32:
				values = append(values, int(value))
			case int64:
				values = append(values, int(value))
			case string:
				parsed, err := strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					return nil, NewDirectorySyncRequestError(field + " must contain integers")
				}
				values = append(values, parsed)
			default:
				return nil, NewDirectorySyncRequestError(field + " must contain integers")
			}
		}
		return values, nil
	default:
		return nil, NewDirectorySyncRequestError(field + " must be an array of integers")
	}
}

func stringArrayConfigValue(config map[string]interface{}, key string) ([]string, error) {
	raw, ok := config[key]
	if !ok || raw == nil {
		return nil, nil
	}

	switch typed := raw.(type) {
	case []string:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			item = strings.TrimSpace(item)
			if item != "" {
				values = append(values, item)
			}
		}
		return values, nil
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			value := strings.TrimSpace(fmt.Sprint(item))
			if value != "" {
				values = append(values, value)
			}
		}
		return values, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
