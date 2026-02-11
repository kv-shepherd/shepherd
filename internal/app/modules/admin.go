package modules

import (
	"context"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
)

// AdminModule is a domain boundary placeholder for admin capabilities
// (clusters/templates/instance sizes). Current handlers are centralized.
type AdminModule struct {
	infra *Infrastructure
}

func NewAdminModule(infra *Infrastructure) *AdminModule {
	return &AdminModule{infra: infra}
}

func (m *AdminModule) Name() string { return "admin" }

func (m *AdminModule) ContributeServerDeps(_ *handlers.ServerDeps) {}

func (m *AdminModule) RegisterWorkers(_ *river.Workers) {}

func (m *AdminModule) Shutdown(context.Context) error { return nil }
