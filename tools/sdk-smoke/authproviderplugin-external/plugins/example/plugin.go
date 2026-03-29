package example

import (
	"context"

	"kv-shepherd.io/shepherd/pkg/authproviderplugin"
)

type Adapter struct{}

func (a *Adapter) Type() string { return "external-smoke" }

func (a *Adapter) Describe() authproviderplugin.AdminTypeDescriptor {
	return authproviderplugin.AdminTypeDescriptor{
		Type:        a.Type(),
		DisplayName: "External Smoke",
		Description: "Compile-only external provider smoke adapter",
		BuiltIn:     false,
		ConfigSchema: map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		},
	}
}

func (a *Adapter) ValidateConfig(map[string]interface{}) error { return nil }

func (a *Adapter) TestConnection(context.Context, map[string]interface{}) (bool, string, error) {
	return true, "ok", nil
}

func (a *Adapter) SampleFields(context.Context, map[string]interface{}) ([]authproviderplugin.AdminSampleField, error) {
	return nil, nil
}

func init() {
	authproviderplugin.MustRegisterAdminAdapter(&Adapter{})
}
