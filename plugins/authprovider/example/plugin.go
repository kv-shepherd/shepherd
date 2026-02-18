package example

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
)

// Adapter demonstrates a minimal third-party auth-provider plugin implementation.
type Adapter struct{}

func (a *Adapter) Type() string {
	return "example-sso"
}

func (a *Adapter) Describe() authproviderplugin.AdminTypeDescriptor {
	return authproviderplugin.AdminTypeDescriptor{
		Type:        a.Type(),
		DisplayName: "Example SSO",
		Description: "Example third-party plugin showing minimal registration contract",
		BuiltIn:     false,
		ConfigSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
			"required":             []string{"issuer"},
			"properties": map[string]interface{}{
				"issuer": map[string]interface{}{
					"type": "string",
				},
				"client_id": map[string]interface{}{
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
		},
	}
}

func (a *Adapter) ValidateConfig(config map[string]interface{}) error {
	issuer := strings.TrimSpace(toString(config["issuer"]))
	if issuer == "" {
		return fmt.Errorf("issuer is required")
	}
	return nil
}

func (a *Adapter) TestConnection(_ context.Context, config map[string]interface{}) (bool, string, error) {
	if err := a.ValidateConfig(config); err != nil {
		return false, err.Error(), nil
	}
	return true, "example plugin configuration accepted", nil
}

func (a *Adapter) SampleFields(_ context.Context, config map[string]interface{}) ([]authproviderplugin.AdminSampleField, error) {
	rawUsers, ok := config["sample_users"].([]interface{})
	if !ok {
		return nil, nil
	}

	type slot struct {
		vals map[string]struct{}
	}
	byField := map[string]*slot{}
	for _, raw := range rawUsers {
		obj, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		for key, value := range obj {
			field := strings.TrimSpace(key)
			if field == "" {
				continue
			}
			entry := byField[field]
			if entry == nil {
				entry = &slot{vals: map[string]struct{}{}}
				byField[field] = entry
			}
			s := strings.TrimSpace(toString(value))
			if s != "" {
				entry.vals[s] = struct{}{}
			}
		}
	}

	out := make([]authproviderplugin.AdminSampleField, 0, len(byField))
	for field, slot := range byField {
		samples := make([]string, 0, len(slot.vals))
		for v := range slot.vals {
			samples = append(samples, v)
		}
		sort.Strings(samples)
		if len(samples) > 10 {
			samples = samples[:10]
		}
		out = append(out, authproviderplugin.AdminSampleField{
			Field:       field,
			ValueType:   "string",
			UniqueCount: len(slot.vals),
			Sample:      samples,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out, nil
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func init() {
	authproviderplugin.MustRegisterAdminAdapter(&Adapter{})
}
