package modules

import (
	"context"
)

// AdminModule represents admin-domain composition
// (clusters/templates/instance sizes). Current handlers are centralized.
type AdminModule struct {
	infra *Infrastructure
}

func NewAdminModule(infra *Infrastructure) *AdminModule {
	return &AdminModule{infra: infra}
}

func (m *AdminModule) Name() string { return "admin" }

func (m *AdminModule) Shutdown(context.Context) error { return nil }
