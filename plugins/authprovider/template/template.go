package template

import (
	"context"
	"fmt"
	"strings"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
)

// Adapter is a reusable skeleton for custom auth-provider plugins.
type Adapter struct {
	TypeKey string
	Name    string
}

// New creates a template adapter instance with a normalized type key.
func New(typeKey, displayName string) *Adapter {
	return &Adapter{TypeKey: strings.ToLower(strings.TrimSpace(typeKey)), Name: strings.TrimSpace(displayName)}
}

func (a *Adapter) Type() string {
	return a.TypeKey
}

func (a *Adapter) Describe() authproviderplugin.AdminTypeDescriptor {
	name := a.Name
	if name == "" {
		name = strings.ToUpper(a.TypeKey)
	}
	return authproviderplugin.AdminTypeDescriptor{
		Type:        a.Type(),
		DisplayName: name,
		Description: "Custom auth-provider plugin",
		BuiltIn:     false,
		ConfigSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
}

func (a *Adapter) ValidateConfig(config map[string]interface{}) error {
	if len(config) == 0 {
		return fmt.Errorf("config must not be empty")
	}
	return nil
}

func (a *Adapter) TestConnection(_ context.Context, config map[string]interface{}) (bool, string, error) {
	if err := a.ValidateConfig(config); err != nil {
		return false, err.Error(), nil
	}
	return true, "configuration accepted", nil
}

func (a *Adapter) SampleFields(_ context.Context, _ map[string]interface{}) ([]authproviderplugin.AdminSampleField, error) {
	return nil, nil
}
