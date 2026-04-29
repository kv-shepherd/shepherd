package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"time"
)

type genericAuthProviderAdminAdapter struct {
	typeKey      string
	displayName  string
	description  string
	builtIn      bool
	configSchema map[string]interface{}
}

type genericDirectorySyncAdminAdapter struct {
	*genericAuthProviderAdminAdapter
}

func newGenericBuiltInAuthProviderAdapter() AuthProviderAdminAdapter {
	return &genericDirectorySyncAdminAdapter{
		genericAuthProviderAdminAdapter: &genericAuthProviderAdminAdapter{
			typeKey:      "generic",
			displayName:  "Generic",
			description:  "Provider plugin using Shepherd standard auth-provider contract",
			builtIn:      true,
			configSchema: genericAuthProviderSchema(),
		},
	}
}

func (a *genericAuthProviderAdminAdapter) Type() string { return a.typeKey }

func (a *genericAuthProviderAdminAdapter) Describe() AuthProviderTypeDescriptor {
	return AuthProviderTypeDescriptor{
		Type:         a.typeKey,
		DisplayName:  a.displayName,
		Description:  a.description,
		BuiltIn:      a.builtIn,
		ConfigSchema: a.configSchema,
	}
}

func (a *genericAuthProviderAdminAdapter) ValidateConfig(config map[string]interface{}) error {
	if len(config) == 0 {
		return fmt.Errorf("config must not be empty")
	}
	return nil
}

func (a *genericAuthProviderAdminAdapter) TestConnection(ctx context.Context, config map[string]interface{}) (ok bool, message string, err error) {
	endpoint := strings.TrimSpace(configStringValue(config, "test_endpoint", "healthcheck_url"))
	if endpoint == "" {
		return true, "configuration accepted (no healthcheck endpoint configured)", nil
	}

	parsedURL, err := validateGenericProviderHealthcheckEndpoint(ctx, endpoint)
	if err != nil {
		return false, err.Error(), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), http.NoBody)
	if err != nil {
		return false, "invalid healthcheck endpoint", nil
	}
	client := &http.Client{
		Timeout:       8 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				host, port, splitErr := net.SplitHostPort(addr)
				if splitErr != nil {
					return nil, splitErr
				}
				if resolveErr := validateGenericProviderDialTarget(ctx, host); resolveErr != nil {
					return nil, resolveErr
				}
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
			},
		},
	}
	//nolint:gosec // Request target is constrained to validated http(s) endpoints with private/reserved IPs rejected and redirects disabled.
	resp, err := client.Do(req)
	if err != nil {
		return false, "healthcheck request failed: " + err.Error(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("healthcheck status %d", resp.StatusCode), nil
	}
	return true, "healthcheck endpoint reachable", nil
}

var genericProviderDisallowedNets = mustParseNetipPrefixes(
	"0.0.0.0/8",
	"10.0.0.0/8",
	"100.64.0.0/10",
	"127.0.0.0/8",
	"169.254.0.0/16",
	"172.16.0.0/12",
	"192.0.0.0/24",
	"192.0.2.0/24",
	"192.168.0.0/16",
	"198.18.0.0/15",
	"198.51.100.0/24",
	"203.0.113.0/24",
	"224.0.0.0/4",
	"240.0.0.0/4",
	"::/128",
	"::1/128",
	"fc00::/7",
	"fe80::/10",
	"fec0::/10",
	"ff00::/8",
	"2001:db8::/32",
)

func validateGenericProviderHealthcheckEndpoint(ctx context.Context, raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("healthcheck endpoint must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("healthcheck endpoint must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("healthcheck endpoint host is required")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("healthcheck endpoint must not include userinfo")
	}
	if err := validateGenericProviderDialTarget(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	return parsed, nil
}

func validateGenericProviderDialTarget(ctx context.Context, host string) error {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if host == "" {
		return fmt.Errorf("healthcheck endpoint host is required")
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if genericProviderIPBlocked(ip) {
			return fmt.Errorf("healthcheck endpoint must not target private or reserved addresses")
		}
		return nil
	}

	lookupCtx := ctx
	if lookupCtx == nil {
		var cancel context.CancelFunc
		lookupCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}

	addrs, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", host)
	if err != nil {
		return fmt.Errorf("healthcheck endpoint host resolution failed")
	}
	if len(addrs) == 0 {
		return fmt.Errorf("healthcheck endpoint host did not resolve")
	}
	for _, ip := range addrs {
		if genericProviderIPBlocked(ip) {
			return fmt.Errorf("healthcheck endpoint must not target private or reserved addresses")
		}
	}
	return nil
}

func genericProviderIPBlocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.IsLoopback() || ip.IsMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}
	for _, prefix := range genericProviderDisallowedNets {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseNetipPrefixes(raw ...string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(raw))
	for _, item := range raw {
		prefix, err := netip.ParsePrefix(item)
		if err != nil {
			panic(err)
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes
}

func (a *genericAuthProviderAdminAdapter) SampleFields(_ context.Context, config map[string]interface{}) ([]AuthProviderSampleField, error) {
	sampleUsers, ok := config["sample_users"].([]interface{})
	if !ok {
		return nil, nil
	}

	type accumulator struct {
		valueType string
		values    map[string]struct{}
	}
	acc := map[string]*accumulator{}

	for _, userRaw := range sampleUsers {
		obj, ok := userRaw.(map[string]interface{})
		if !ok {
			continue
		}
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
			if typed, ok := raw.([]interface{}); ok {
				for _, item := range typed {
					v := strings.TrimSpace(fmt.Sprint(item))
					if v != "" {
						slot.values[v] = struct{}{}
					}
				}
				if slot.valueType == sampleValueTypeUnknown {
					slot.valueType = sampleValueTypeArray
				}
				continue
			}
			v := strings.TrimSpace(fmt.Sprint(raw))
			if v != "" {
				slot.values[v] = struct{}{}
			}
		}
	}

	fields := make([]AuthProviderSampleField, 0, len(acc))
	for field, slot := range acc {
		values := make([]string, 0, len(slot.values))
		for v := range slot.values {
			values = append(values, v)
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
	return fields, nil
}

func (a *genericDirectorySyncAdminAdapter) DescribeDirectorySync() DirectorySyncDescriptor {
	return DirectorySyncDescriptor{
		DisplayName:     "Generic Directory Sync",
		Description:     "Canonical directory sync over provider-configured sample users",
		SupportsPreview: true,
		RequestSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"selected_usernames": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
				},
				"limit": map[string]interface{}{
					"type":    "integer",
					"minimum": 1,
				},
			},
			"additionalProperties": true,
		},
	}
}

func (a *genericDirectorySyncAdminAdapter) PreviewDirectorySync(
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

func (a *genericDirectorySyncAdminAdapter) ListDirectoryUsers(
	_ context.Context,
	config map[string]interface{},
	providerRequest map[string]interface{},
) ([]DirectoryUserRecord, error) {
	selectedUsernames, limit, err := parseGenericDirectoryRequest(providerRequest)
	if err != nil {
		return nil, err
	}

	sampleUsers, ok := config["sample_users"].([]interface{})
	if !ok {
		return nil, nil
	}

	filtered := make([]DirectoryUserRecord, 0, len(sampleUsers))
	for _, sample := range sampleUsers {
		obj, ok := sample.(map[string]interface{})
		if !ok {
			continue
		}
		record, ok := directoryRecordFromGenericSample(obj)
		if !ok {
			continue
		}
		if len(selectedUsernames) > 0 {
			if _, exists := selectedUsernames[record.Username]; !exists {
				continue
			}
		}
		filtered = append(filtered, record)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return filtered, nil
}

func (a *genericDirectorySyncAdminAdapter) BuildScheduledDirectoryEnrichmentPlan(
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

	providerRequest := map[string]interface{}{}
	if raw, ok := config["scheduled_provider_request"]; ok && raw != nil {
		typed, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("scheduled_provider_request must be an object")
		}
		providerRequest = cloneGenericAttributes(typed)
	}

	return &ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             mode,
		JoinKeyType:      joinKeyType,
		ScheduleCron:     scheduleCron,
		ScheduleTimezone: scheduleTimezone,
		ProviderRequest:  providerRequest,
	}, nil
}

func parseGenericDirectoryRequest(providerRequest map[string]interface{}) (selectedUsernames map[string]struct{}, limit int, err error) {
	selectedUsernames = map[string]struct{}{}
	if raw, ok := providerRequest["selected_usernames"]; ok && raw != nil {
		items, ok := raw.([]interface{})
		if !ok {
			return nil, 0, NewDirectorySyncRequestError("selected_usernames must be an array of strings")
		}
		for _, item := range items {
			username := strings.TrimSpace(fmt.Sprint(item))
			if username == "" {
				continue
			}
			selectedUsernames[username] = struct{}{}
		}
	}

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
			return nil, 0, NewDirectorySyncRequestError("limit must be a positive integer")
		}
		if limit < 1 {
			return nil, 0, NewDirectorySyncRequestError("limit must be a positive integer")
		}
	}
	return selectedUsernames, limit, nil
}

func directoryRecordFromGenericSample(sample map[string]interface{}) (DirectoryUserRecord, bool) {
	record := DirectoryUserRecord{
		ExternalID:  configStringValue(sample, "external_id", "id", "sub", "user_id", "uid", "username"),
		Username:    configStringValue(sample, "username", "login", "uid", "user_name"),
		DisplayName: configStringValue(sample, "display_name", "name"),
		Email:       configStringValue(sample, "email", "mail"),
		Cohorts:     genericSampleCohorts(sample),
		Attributes:  cloneGenericAttributes(sample),
	}
	if record.Username == "" {
		record.Username = record.ExternalID
	}
	if record.DisplayName == "" {
		record.DisplayName = record.Username
	}
	if record.ExternalID == "" || record.Username == "" || record.DisplayName == "" {
		return DirectoryUserRecord{}, false
	}
	return record, true
}

func genericSampleCohorts(sample map[string]interface{}) []ExternalCohort {
	if len(sample) == 0 {
		return nil
	}

	if raw, ok := sample["cohorts"]; ok {
		if typed, ok := raw.([]interface{}); ok {
			items := make([]ExternalCohort, 0, len(typed))
			for _, item := range typed {
				switch cohort := item.(type) {
				case map[string]interface{}:
					parsed := ExternalCohort{
						Kind:        strings.TrimSpace(fmt.Sprint(cohort["kind"])),
						Key:         strings.TrimSpace(fmt.Sprint(cohort["key"])),
						DisplayName: strings.TrimSpace(fmt.Sprint(cohort["display_name"])),
					}
					if parsed.Kind == "" {
						parsed.Kind = externalCohortKindGroup
					}
					if parsed.DisplayName == "" {
						parsed.DisplayName = parsed.Key
					}
					if parsed.Key != "" {
						items = append(items, parsed)
					}
				default:
					value := strings.TrimSpace(fmt.Sprint(item))
					if value != "" {
						items = append(items, ExternalCohort{
							Kind:        externalCohortKindGroup,
							Key:         value,
							DisplayName: value,
						})
					}
				}
			}
			if len(items) > 0 {
				return items
			}
		}
	}

	return externalCohortsFromStringValues(externalCohortKindGroup, genericStringSlice(sample["groups"]))
}

func cloneGenericAttributes(sample map[string]interface{}) map[string]interface{} {
	if len(sample) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(sample))
	for key, value := range sample {
		cloned[key] = value
	}
	return cloned
}

func genericStringSlice(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		value := strings.TrimSpace(fmt.Sprint(item))
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
}

func configStringValue(config map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		v, ok := config[key]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s != "" {
				return s
			}
			continue
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s != "" {
			return s
		}
	}
	return ""
}

func detectSampleValueType(raw interface{}) string {
	switch raw.(type) {
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	case json.Number:
		return "number"
	case map[string]interface{}:
		return "object"
	case []interface{}, []string:
		return sampleValueTypeArray
	default:
		return sampleValueTypeUnknown
	}
}
