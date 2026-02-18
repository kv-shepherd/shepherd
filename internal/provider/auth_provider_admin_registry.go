package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// AuthProviderTypeDescriptor describes a provider type exposed to admin UI/API.
type AuthProviderTypeDescriptor struct {
	Type         string
	DisplayName  string
	Description  string
	BuiltIn      bool
	ConfigSchema map[string]interface{}
}

// AuthProviderSampleField is the normalized sample-field contract exposed by plugins.
type AuthProviderSampleField struct {
	Field       string
	ValueType   string
	UniqueCount int
	Sample      []string
}

// AuthProviderAdminAdapter defines the plugin contract for auth provider management endpoints.
type AuthProviderAdminAdapter interface {
	// Type returns the provider type key used by auth_providers.auth_type.
	Type() string
	// ValidateConfig checks whether the provider config is structurally valid.
	ValidateConfig(config map[string]interface{}) error
	// TestConnection performs a provider-specific connectivity check.
	TestConnection(ctx context.Context, config map[string]interface{}) (bool, string, error)
	// SampleFields extracts sample fields for RBAC mapping configuration.
	SampleFields(ctx context.Context, config map[string]interface{}) ([]AuthProviderSampleField, error)
}

// AuthProviderAdminAdapterDescriber is an optional adapter extension for metadata exposure.
type AuthProviderAdminAdapterDescriber interface {
	Describe() AuthProviderTypeDescriptor
}

// AuthProviderAdminRegistry stores available adapter plugins.
type AuthProviderAdminRegistry struct {
	mu       sync.RWMutex
	adapters map[string]AuthProviderAdminAdapter
}

func newAuthProviderAdminRegistry() *AuthProviderAdminRegistry {
	r := &AuthProviderAdminRegistry{
		adapters: map[string]AuthProviderAdminAdapter{},
	}
	for _, builtin := range builtInAuthProviderAdapters() {
		_ = r.Register(builtin)
	}
	return r
}

// Register registers an adapter by type. Duplicate type keys are rejected.
func (r *AuthProviderAdminRegistry) Register(adapter AuthProviderAdminAdapter) error {
	if adapter == nil {
		return fmt.Errorf("adapter is nil")
	}
	t := strings.TrimSpace(strings.ToLower(adapter.Type()))
	if t == "" {
		return fmt.Errorf("adapter type is empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.adapters[t]; exists {
		return fmt.Errorf("adapter type already registered: %s", t)
	}
	r.adapters[t] = adapter
	return nil
}

// Resolve returns a typed adapter when available, otherwise nil.
func (r *AuthProviderAdminRegistry) Resolve(authType string) AuthProviderAdminAdapter {
	t := strings.TrimSpace(strings.ToLower(authType))
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.adapters[t]
}

// List returns all registered adapter descriptors sorted by type.
func (r *AuthProviderAdminRegistry) List() []AuthProviderTypeDescriptor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]AuthProviderTypeDescriptor, 0, len(r.adapters))
	for t, adapter := range r.adapters {
		if describer, ok := adapter.(AuthProviderAdminAdapterDescriber); ok {
			desc := describer.Describe()
			desc.Type = strings.TrimSpace(strings.ToLower(desc.Type))
			if desc.Type == "" {
				desc.Type = t
			}
			if strings.TrimSpace(desc.DisplayName) == "" {
				desc.DisplayName = strings.ToUpper(desc.Type)
			}
			items = append(items, desc)
			continue
		}
		items = append(items, AuthProviderTypeDescriptor{
			Type:        t,
			DisplayName: strings.ToUpper(t),
			BuiltIn:     false,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Type < items[j].Type })
	return items
}

var globalAuthProviderAdminRegistry = newAuthProviderAdminRegistry()

// RegisterAuthProviderAdminAdapter registers an adapter globally.
func RegisterAuthProviderAdminAdapter(adapter AuthProviderAdminAdapter) error {
	return globalAuthProviderAdminRegistry.Register(adapter)
}

// ResolveAuthProviderAdminAdapter resolves an adapter from global registry.
func ResolveAuthProviderAdminAdapter(authType string) AuthProviderAdminAdapter {
	return globalAuthProviderAdminRegistry.Resolve(authType)
}

// ListAuthProviderAdminAdapterTypes returns all registered provider type descriptors.
func ListAuthProviderAdminAdapterTypes() []AuthProviderTypeDescriptor {
	return globalAuthProviderAdminRegistry.List()
}

type genericAuthProviderAdminAdapter struct {
	typeKey      string
	displayName  string
	description  string
	builtIn      bool
	configSchema map[string]interface{}
}

func builtInAuthProviderAdapters() []AuthProviderAdminAdapter {
	schema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": true,
		"properties": map[string]interface{}{
			"test_endpoint": map[string]interface{}{"type": "string"},
			"healthcheck_url": map[string]interface{}{
				"type": "string",
			},
			"sample_users": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
		},
	}

	return []AuthProviderAdminAdapter{
		&genericAuthProviderAdminAdapter{
			typeKey:      "generic",
			displayName:  "Generic",
			description:  "Provider plugin using Shepherd standard auth-provider contract",
			builtIn:      true,
			configSchema: schema,
		},
		&genericAuthProviderAdminAdapter{
			typeKey:      "oidc",
			displayName:  "OIDC",
			description:  "OpenID Connect provider via standardized adapter contract",
			builtIn:      true,
			configSchema: schema,
		},
		&genericAuthProviderAdminAdapter{
			typeKey:      "ldap",
			displayName:  "LDAP",
			description:  "LDAP provider via standardized adapter contract",
			builtIn:      true,
			configSchema: schema,
		},
		&genericAuthProviderAdminAdapter{
			typeKey:      "sso",
			displayName:  "SSO",
			description:  "Enterprise SSO provider via standardized adapter contract",
			builtIn:      true,
			configSchema: schema,
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

func (a *genericAuthProviderAdminAdapter) TestConnection(ctx context.Context, config map[string]interface{}) (bool, string, error) {
	endpoint := strings.TrimSpace(configStringValue(config, "test_endpoint", "healthcheck_url"))
	if endpoint == "" {
		return true, "configuration accepted (no healthcheck endpoint configured)", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, "invalid healthcheck endpoint", nil
	}
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req) // #nosec G107 -- endpoint is admin-supplied configuration.
	if err != nil {
		return false, "healthcheck request failed: " + err.Error(), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("healthcheck status %d", resp.StatusCode), nil
	}
	return true, "healthcheck endpoint reachable", nil
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
			switch typed := raw.(type) {
			case []interface{}:
				for _, item := range typed {
					v := strings.TrimSpace(fmt.Sprint(item))
					if v != "" {
						slot.values[v] = struct{}{}
					}
				}
				if slot.valueType == "unknown" {
					slot.valueType = "array"
				}
			default:
				v := strings.TrimSpace(fmt.Sprint(typed))
				if v != "" {
					slot.values[v] = struct{}{}
				}
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
		return "array"
	default:
		return "unknown"
	}
}
