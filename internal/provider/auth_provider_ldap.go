package provider

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	ldap "github.com/go-ldap/ldap/v3"
)

const (
	defaultLDAPDirectoryFilter = "(objectClass=person)"
	defaultLDAPUsernameAttr    = "uid"
	defaultLDAPDisplayNameAttr = "cn"
	defaultLDAPEmailAttr       = "mail"
	defaultLDAPMemberOfAttr    = "memberOf"
	defaultLDAPGroupNameAttr   = "cn"
	defaultLDAPRequestTimeout  = 10 * time.Second
	defaultLDAPSampleLimit     = 20
	defaultLDAPSyncLimit       = 100
)

type ldapConnection interface {
	Bind(username, password string) error
	Search(searchRequest *ldap.SearchRequest) (*ldap.SearchResult, error)
	StartTLS(config *tls.Config) error
	Close() error
}

type ldapDialerFunc func(serverURL string, tlsEnabled, tlsSkipVerify bool, timeout time.Duration) (ldapConnection, error)

type ldapAuthProviderAdapter struct {
	configSchema map[string]interface{}
	dial         ldapDialerFunc
}

func newLDAPBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return newLDAPAuthProviderAdapter(ldapAuthProviderSchema())
}

func newLDAPAuthProviderAdapter(configSchema map[string]interface{}) *ldapAuthProviderAdapter {
	return &ldapAuthProviderAdapter{
		configSchema: configSchema,
		dial:         dialLDAPConnection,
	}
}

func (a *ldapAuthProviderAdapter) Type() string { return "ldap" }

func (a *ldapAuthProviderAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{
		Type:         a.Type(),
		DisplayName:  "LDAP",
		Description:  "LDAP directory provider with standardized directory sync and enrichment support",
		BuiltIn:      true,
		ConfigSchema: a.configSchema,
	}
}

func (a *ldapAuthProviderAdapter) DescribeRuntimeAuth() AuthRuntimeDescriptor {
	return AuthRuntimeDescriptor{
		DisplayName: "LDAP",
		Description: "Standard LDAP username/password login",
		LoginModes: []AuthLoginMode{
			{
				Key:         "credentials",
				DisplayName: "LDAP Login",
				Description: "Submit username and password directly to the LDAP provider",
				Interaction: AuthInteractionCredentials,
				RequestSchema: map[string]interface{}{
					"type":     "object",
					"required": []string{"username", "password"},
					"properties": map[string]interface{}{
						"username": map[string]interface{}{
							"type":        "string",
							"title":       "Username",
							"description": "LDAP username",
						},
						"password": map[string]interface{}{
							"type":        "string",
							"format":      "password",
							"title":       "Password",
							"description": "LDAP password",
						},
					},
					"additionalProperties": false,
				},
				Default: true,
			},
		},
	}
}

func (a *ldapAuthProviderAdapter) ValidateConfig(config map[string]interface{}) error {
	serverURL := strings.TrimSpace(configStringValue(config, "server_url"))
	if serverURL == "" {
		return fmt.Errorf("server_url is required")
	}
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return fmt.Errorf("server_url must be a valid URI")
	}
	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "ldap", "ldaps":
	default:
		return fmt.Errorf("server_url must use ldap:// or ldaps://")
	}
	if strings.TrimSpace(configStringValue(config, "bind_dn")) == "" {
		return fmt.Errorf("bind_dn is required")
	}
	if strings.TrimSpace(configStringValue(config, "base_dn")) == "" {
		return fmt.Errorf("base_dn is required")
	}
	if filter := strings.TrimSpace(configStringValue(config, "directory_filter")); filter != "" {
		if _, compileErr := ldap.CompileFilter(filter); compileErr != nil {
			return fmt.Errorf("directory_filter must be a valid LDAP filter")
		}
	}
	if filter := strings.TrimSpace(configStringValue(config, "group_filter")); filter != "" && strings.Count(filter, "%s") > 1 {
		return fmt.Errorf("group_filter may contain at most one %%s format token")
	}
	if tlsSkipVerify, _ := config["tls_skip_verify"].(bool); tlsSkipVerify {
		return fmt.Errorf("tls_skip_verify is not supported")
	}
	mode := DirectoryEnrichmentMode(strings.TrimSpace(configStringValue(config, "enrichment_mode")))
	if mode != "" && mode != DirectoryEnrichmentModeEnrichExistingOnly {
		return fmt.Errorf("unsupported enrichment_mode %q", mode)
	}
	joinKeyType := DirectoryJoinKeyType(strings.TrimSpace(configStringValue(config, "join_key_type")))
	if joinKeyType != "" && joinKeyType != DirectoryJoinKeyUsername {
		return fmt.Errorf("unsupported join_key_type %q", joinKeyType)
	}
	if _, _, _, err := ldapDirectoryRequestFromConfig(config, nil); err != nil {
		return err
	}
	return nil
}

func (a *ldapAuthProviderAdapter) TestConnection(ctx context.Context, config map[string]interface{}) (ok bool, message string, err error) {
	if validateErr := a.ValidateConfig(config); validateErr != nil {
		return false, validateErr.Error(), nil
	}
	conn, err := a.connect(ctx, config)
	if err != nil {
		return false, err.Error(), nil
	}
	defer conn.Close()
	if bindErr := conn.Bind(strings.TrimSpace(configStringValue(config, "bind_dn")), configStringValue(config, "bind_password")); bindErr != nil {
		return false, fmt.Sprintf("ldap bind failed: %v", bindErr), nil
	}
	return true, "ldap bind succeeded", nil
}

func (a *ldapAuthProviderAdapter) AuthenticateCredentials(
	ctx context.Context,
	config map[string]interface{},
	req AuthCredentialRequest,
) (*AuthResult, error) {
	if err := a.ValidateConfig(config); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(configStringValue(req.Credentials, "username"))
	password := strings.TrimSpace(configStringValue(req.Credentials, "password"))
	if username == "" || password == "" {
		return nil, NewAuthCredentialError("INVALID_CREDENTIALS", "username and password are required")
	}

	conn, err := a.connect(ctx, config)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if bindErr := conn.Bind(strings.TrimSpace(configStringValue(config, "bind_dn")), configStringValue(config, "bind_password")); bindErr != nil {
		return nil, fmt.Errorf("ldap bind failed: %w", bindErr)
	}

	entry, err := a.lookupLoginEntry(ctx, conn, config, username)
	if err != nil {
		return nil, err
	}
	if entry == nil || strings.TrimSpace(entry.DN) == "" {
		return nil, NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials")
	}

	if err := conn.Bind(strings.TrimSpace(entry.DN), password); err != nil {
		return nil, NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials")
	}

	record := a.directoryRecordFromEntry(ctx, conn, config, entry)
	if record.ExternalID == "" || record.Username == "" || record.DisplayName == "" {
		return nil, fmt.Errorf("ldap user entry cannot be mapped to canonical auth result")
	}

	return &AuthResult{
		ExternalID:        record.ExternalID,
		Username:          record.Username,
		DisplayName:       record.DisplayName,
		Email:             record.Email,
		Enabled:           true,
		Cohorts:           record.Cohorts,
		ProfileAttributes: AuthProfileAttributes(record.Attributes),
	}, nil
}

func (a *ldapAuthProviderAdapter) SampleFields(ctx context.Context, config map[string]interface{}) ([]AuthProviderSampleField, error) {
	records, rawObjects, err := a.searchDirectory(ctx, config, map[string]interface{}{
		"limit": float64(defaultLDAPSampleLimit),
	})
	if err != nil {
		return nil, err
	}
	if len(rawObjects) == 0 {
		rawObjects = make([]map[string]interface{}, 0, len(records))
		for _, record := range records {
			rawObjects = append(rawObjects, cloneGenericAttributes(record.Attributes))
		}
	}
	return sampleFieldsFromObjects(rawObjects), nil
}

func (a *ldapAuthProviderAdapter) DescribeDirectorySync() DirectorySyncDescriptor {
	return DirectorySyncDescriptor{
		DisplayName:     "LDAP Directory Sync",
		Description:     "Canonical directory sync over LDAP search results",
		SupportsPreview: true,
		RequestSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"search_base": map[string]interface{}{
					"type":        "string",
					"description": "Optional LDAP search base override",
				},
				"directory_filter": map[string]interface{}{
					"type":        "string",
					"description": "Optional LDAP directory filter override",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"minimum":     1,
					"description": "Maximum number of LDAP entries to return",
				},
			},
			"additionalProperties": true,
		},
	}
}

func (a *ldapAuthProviderAdapter) PreviewDirectorySync(
	ctx context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) (*DirectorySyncPreview, error) {
	records, _, err := a.searchDirectory(ctx, config, providerRequest)
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

func (a *ldapAuthProviderAdapter) ListDirectoryUsers(
	ctx context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) ([]DirectoryUserRecord, error) {
	records, _, err := a.searchDirectory(ctx, config, providerRequest)
	return records, err
}

func (a *ldapAuthProviderAdapter) BuildScheduledDirectoryEnrichmentPlan(
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

	scheduleCron := strings.TrimSpace(configStringValue(config, "schedule_cron"))
	if scheduleCron == "" {
		scheduleCron = defaultScheduledEnrichmentCron
	}

	scheduleTimezone := strings.TrimSpace(configStringValue(config, "schedule_timezone"))
	if scheduleTimezone == "" {
		scheduleTimezone = defaultScheduleTimezoneUTC
	}

	providerRequest := map[string]interface{}{
		"search_base":      ldapDirectoryBaseDN(config, nil),
		"directory_filter": ldapDirectoryFilter(config, nil),
		"limit":            float64(defaultLDAPSyncLimit),
	}
	if raw, ok := config["scheduled_provider_request"]; ok && raw != nil {
		typed, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("scheduled_provider_request must be an object")
		}
		for key, value := range typed {
			providerRequest[key] = value
		}
	}
	if _, _, _, err := ldapDirectoryRequestFromConfig(config, providerRequest); err != nil {
		return nil, err
	}

	return &ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             mode,
		JoinKeyType:      joinKeyType,
		ScheduleCron:     scheduleCron,
		ScheduleTimezone: scheduleTimezone,
		ProviderRequest:  CloneDirectoryAttributes(providerRequest),
	}, nil
}

func (a *ldapAuthProviderAdapter) connect(_ context.Context, config map[string]interface{}) (ldapConnection, error) {
	if a.dial == nil {
		return nil, fmt.Errorf("ldap dialer is not configured")
	}
	timeout := ldapTimeout(config)
	return a.dial(
		strings.TrimSpace(configStringValue(config, "server_url")),
		ldapTLSEnabled(config),
		false,
		timeout,
	)
}

func (a *ldapAuthProviderAdapter) searchDirectory(
	ctx context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) (records []DirectoryUserRecord, rawObjects []map[string]interface{}, err error) {
	if validateErr := a.ValidateConfig(config); validateErr != nil {
		return nil, nil, validateErr
	}
	searchBase, searchFilter, limit, err := ldapDirectoryRequestFromConfig(config, providerRequest)
	if err != nil {
		return nil, nil, err
	}

	conn, err := a.connect(ctx, config)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	if bindErr := conn.Bind(strings.TrimSpace(configStringValue(config, "bind_dn")), configStringValue(config, "bind_password")); bindErr != nil {
		return nil, nil, fmt.Errorf("ldap bind failed: %w", bindErr)
	}

	searchResult, err := conn.Search(ldap.NewSearchRequest(
		searchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		limit,
		int(ldapTimeout(config).Seconds()),
		false,
		searchFilter,
		ldapRequestedAttributes(config),
		nil,
	))
	if err != nil {
		return nil, nil, fmt.Errorf("ldap search failed: %w", err)
	}

	records = make([]DirectoryUserRecord, 0, len(searchResult.Entries))
	rawObjects = make([]map[string]interface{}, 0, len(searchResult.Entries))
	for _, entry := range searchResult.Entries {
		if entry == nil {
			continue
		}
		record := a.directoryRecordFromEntry(ctx, conn, config, entry)
		if record.ExternalID == "" || record.Username == "" || record.DisplayName == "" {
			continue
		}
		records = append(records, record)
		rawObjects = append(rawObjects, cloneGenericAttributes(record.Attributes))
	}
	return records, rawObjects, nil
}

func (a *ldapAuthProviderAdapter) lookupLoginEntry(
	_ context.Context,
	conn ldapConnection,
	config map[string]interface{},
	username string,
) (*ldap.Entry, error) {
	baseDN := strings.TrimSpace(configStringValue(config, "base_dn"))
	userFilter := strings.TrimSpace(configStringValue(config, "user_filter"))
	if userFilter == "" {
		userFilter = "(uid=%s)"
	}
	if strings.Count(userFilter, "%s") > 1 {
		return nil, fmt.Errorf("user_filter may contain at most one %%s format token")
	}
	if strings.Contains(userFilter, "%s") {
		userFilter = fmt.Sprintf(userFilter, ldap.EscapeFilter(username))
	}
	if _, err := ldap.CompileFilter(userFilter); err != nil {
		return nil, fmt.Errorf("user_filter must be a valid LDAP filter")
	}

	searchResult, err := conn.Search(ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		2,
		int(ldapTimeout(config).Seconds()),
		false,
		userFilter,
		ldapRequestedAttributes(config),
		nil,
	))
	if err != nil {
		return nil, fmt.Errorf("ldap user lookup failed: %w", err)
	}
	if len(searchResult.Entries) != 1 {
		return nil, NewAuthCredentialError("INVALID_CREDENTIALS", "invalid credentials")
	}
	return searchResult.Entries[0], nil
}

func (a *ldapAuthProviderAdapter) directoryRecordFromEntry(
	ctx context.Context,
	conn ldapConnection,
	config map[string]interface{},
	entry *ldap.Entry,
) DirectoryUserRecord {
	usernameAttr := firstNonEmpty(strings.TrimSpace(configStringValue(config, "username_attribute")), defaultLDAPUsernameAttr)
	displayNameAttr := firstNonEmpty(strings.TrimSpace(configStringValue(config, "display_name_attribute")), defaultLDAPDisplayNameAttr)
	emailAttr := firstNonEmpty(strings.TrimSpace(configStringValue(config, "email_attribute")), defaultLDAPEmailAttr)
	externalIDAttr := strings.TrimSpace(configStringValue(config, "external_id_attribute"))
	memberOfAttr := firstNonEmpty(strings.TrimSpace(configStringValue(config, "member_of_attribute")), defaultLDAPMemberOfAttr)

	username := firstNonEmpty(
		strings.TrimSpace(entry.GetEqualFoldAttributeValue(usernameAttr)),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("sAMAccountName")),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("uid")),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("cn")),
	)
	displayName := firstNonEmpty(
		strings.TrimSpace(entry.GetEqualFoldAttributeValue(displayNameAttr)),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("displayName")),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("cn")),
		username,
	)
	email := firstNonEmpty(
		strings.TrimSpace(entry.GetEqualFoldAttributeValue(emailAttr)),
		strings.TrimSpace(entry.GetEqualFoldAttributeValue("mail")),
	)

	externalID := strings.TrimSpace(entry.DN)
	if externalIDAttr != "" && !strings.EqualFold(externalIDAttr, "dn") {
		externalID = firstNonEmpty(
			strings.TrimSpace(entry.GetEqualFoldAttributeValue(externalIDAttr)),
			externalID,
		)
	}

	groupValues := ldapNormalizeGroupValues(entry.GetEqualFoldAttributeValues(memberOfAttr))
	if len(groupValues) == 0 {
		groupValues = a.lookupGroupsByFilter(ctx, conn, config, entry, username)
	}

	attributes := ldapAttributesFromEntry(entry)
	return DirectoryUserRecord{
		ExternalID:  externalID,
		Username:    username,
		DisplayName: displayName,
		Email:       email,
		Cohorts:     externalCohortsFromStringValues("group", groupValues),
		Attributes:  attributes,
	}
}

func (a *ldapAuthProviderAdapter) lookupGroupsByFilter(
	_ context.Context,
	conn ldapConnection,
	config map[string]interface{},
	entry *ldap.Entry,
	username string,
) []string {
	groupFilter := strings.TrimSpace(configStringValue(config, "group_filter"))
	if groupFilter == "" || !strings.Contains(groupFilter, "%s") {
		return nil
	}
	subject := firstNonEmpty(username, strings.TrimSpace(entry.DN))
	if subject == "" {
		return nil
	}
	filter := fmt.Sprintf(groupFilter, ldap.EscapeFilter(subject))
	if _, err := ldap.CompileFilter(filter); err != nil {
		return nil
	}
	searchBase := firstNonEmpty(
		strings.TrimSpace(configStringValue(config, "group_base_dn")),
		ldapDirectoryBaseDN(config, nil),
	)
	groupNameAttr := firstNonEmpty(strings.TrimSpace(configStringValue(config, "group_name_attribute")), defaultLDAPGroupNameAttr)
	result, err := conn.Search(ldap.NewSearchRequest(
		searchBase,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		defaultLDAPSyncLimit,
		int(ldapTimeout(config).Seconds()),
		false,
		filter,
		[]string{groupNameAttr},
		nil,
	))
	if err != nil {
		return nil
	}
	values := make([]string, 0, len(result.Entries))
	for _, groupEntry := range result.Entries {
		if groupEntry == nil {
			continue
		}
		value := firstNonEmpty(
			strings.TrimSpace(groupEntry.GetEqualFoldAttributeValue(groupNameAttr)),
			strings.TrimSpace(groupEntry.DN),
		)
		if value != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	return values
}

func ldapDirectoryRequestFromConfig(config, providerRequest map[string]interface{}) (searchBase, searchFilter string, limit int, err error) {
	searchBase = ldapDirectoryBaseDN(config, providerRequest)
	if searchBase == "" {
		return "", "", 0, fmt.Errorf("base_dn is required")
	}
	searchFilter = ldapDirectoryFilter(config, providerRequest)
	if searchFilter == "" {
		searchFilter = defaultLDAPDirectoryFilter
	}
	if _, compileErr := ldap.CompileFilter(searchFilter); compileErr != nil {
		return "", "", 0, NewDirectorySyncRequestError("directory_filter must be a valid LDAP filter")
	}
	limit = defaultLDAPSyncLimit
	if providerRequest != nil {
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
				return "", "", 0, NewDirectorySyncRequestError("limit must be a positive integer")
			}
			if limit < 1 {
				return "", "", 0, NewDirectorySyncRequestError("limit must be a positive integer")
			}
		}
	}
	return searchBase, searchFilter, limit, nil
}

func ldapDirectoryBaseDN(config, providerRequest map[string]interface{}) string {
	if providerRequest != nil {
		if value := strings.TrimSpace(configStringValue(providerRequest, "search_base")); value != "" {
			return value
		}
	}
	return firstNonEmpty(
		strings.TrimSpace(configStringValue(config, "directory_base_dn")),
		strings.TrimSpace(configStringValue(config, "base_dn")),
	)
}

func ldapDirectoryFilter(config, providerRequest map[string]interface{}) string {
	if providerRequest != nil {
		if value := strings.TrimSpace(configStringValue(providerRequest, "directory_filter")); value != "" {
			return value
		}
	}
	return firstNonEmpty(
		strings.TrimSpace(configStringValue(config, "directory_filter")),
		defaultLDAPDirectoryFilter,
	)
}

func ldapRequestedAttributes(config map[string]interface{}) []string {
	attrs := []string{
		firstNonEmpty(strings.TrimSpace(configStringValue(config, "username_attribute")), defaultLDAPUsernameAttr),
		firstNonEmpty(strings.TrimSpace(configStringValue(config, "display_name_attribute")), defaultLDAPDisplayNameAttr),
		firstNonEmpty(strings.TrimSpace(configStringValue(config, "email_attribute")), defaultLDAPEmailAttr),
		firstNonEmpty(strings.TrimSpace(configStringValue(config, "member_of_attribute")), defaultLDAPMemberOfAttr),
		"telephoneNumber",
		"title",
		"department",
		"ou",
	}
	if attr := strings.TrimSpace(configStringValue(config, "external_id_attribute")); attr != "" && !strings.EqualFold(attr, "dn") {
		attrs = append(attrs, attr)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		attr = strings.TrimSpace(attr)
		if attr == "" {
			continue
		}
		key := strings.ToLower(attr)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, attr)
	}
	return out
}

func ldapAttributesFromEntry(entry *ldap.Entry) map[string]interface{} {
	if entry == nil {
		return map[string]interface{}{}
	}
	attributes := map[string]interface{}{
		"ldap_dn": strings.TrimSpace(entry.DN),
	}
	for _, attr := range entry.Attributes {
		if attr == nil {
			continue
		}
		key := strings.TrimSpace(attr.Name)
		if key == "" || len(attr.Values) == 0 {
			continue
		}
		if len(attr.Values) == 1 {
			attributes[key] = strings.TrimSpace(attr.Values[0])
			continue
		}
		values := make([]string, 0, len(attr.Values))
		for _, value := range attr.Values {
			value = strings.TrimSpace(value)
			if value != "" {
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			attributes[key] = values
		}
	}
	return attributes
}

func ldapNormalizeGroupValues(values []string) []string {
	if len(values) == 0 {
		return nil
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
	sort.Strings(out)
	return out
}

func sampleFieldsFromObjects(objects []map[string]interface{}) []AuthProviderSampleField {
	type accumulator struct {
		valueType string
		values    map[string]struct{}
	}
	acc := map[string]*accumulator{}
	for _, obj := range objects {
		for field, raw := range obj {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			slot, exists := acc[field]
			if !exists {
				slot = &accumulator{valueType: detectSampleValueType(raw), values: map[string]struct{}{}}
				acc[field] = slot
			}
			switch typed := raw.(type) {
			case []string:
				for _, item := range typed {
					item = strings.TrimSpace(item)
					if item != "" {
						slot.values[item] = struct{}{}
					}
				}
				if slot.valueType == sampleValueTypeUnknown {
					slot.valueType = sampleValueTypeArray
				}
			case []interface{}:
				for _, item := range typed {
					value := strings.TrimSpace(fmt.Sprint(item))
					if value != "" {
						slot.values[value] = struct{}{}
					}
				}
				if slot.valueType == sampleValueTypeUnknown {
					slot.valueType = sampleValueTypeArray
				}
			default:
				value := strings.TrimSpace(fmt.Sprint(typed))
				if value != "" {
					slot.values[value] = struct{}{}
				}
			}
		}
	}
	fields := make([]AuthProviderSampleField, 0, len(acc))
	for field, slot := range acc {
		values := make([]string, 0, len(slot.values))
		for value := range slot.values {
			values = append(values, value)
		}
		sort.Strings(values)
		if len(values) > 10 {
			values = values[:10]
		}
		fields = append(fields, AuthProviderSampleField{
			Field:       field,
			ValueType:   slot.valueType,
			UniqueCount: len(slot.values),
			Sample:      values,
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return fields
}

func ldapTLSEnabled(config map[string]interface{}) bool {
	if value, ok := config["tls_enabled"].(bool); ok {
		return value
	}
	return true
}

func ldapTimeout(config map[string]interface{}) time.Duration {
	if raw, ok := config["request_timeout_seconds"]; ok && raw != nil {
		switch typed := raw.(type) {
		case int:
			if typed > 0 {
				return time.Duration(typed) * time.Second
			}
		case int32:
			if typed > 0 {
				return time.Duration(typed) * time.Second
			}
		case int64:
			if typed > 0 {
				return time.Duration(typed) * time.Second
			}
		case float64:
			if typed > 0 {
				return time.Duration(int(typed)) * time.Second
			}
		}
	}
	return defaultLDAPRequestTimeout
}

func dialLDAPConnection(serverURL string, tlsEnabled, _ bool, timeout time.Duration) (ldapConnection, error) {
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse ldap server_url: %w", err)
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: parsedURL.Hostname(),
	}
	conn, err := ldap.DialURL(
		serverURL,
		ldap.DialWithDialer(&net.Dialer{Timeout: timeout}),
		ldap.DialWithTLSConfig(tlsConfig),
	)
	if err != nil {
		return nil, fmt.Errorf("dial ldap server: %w", err)
	}
	if strings.EqualFold(parsedURL.Scheme, "ldap") && tlsEnabled {
		if err := conn.StartTLS(tlsConfig); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("ldap starttls failed: %w", err)
		}
	}
	return conn, nil
}
