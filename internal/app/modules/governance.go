package modules

import (
	"context"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
)

// GovernanceModule is a domain boundary placeholder for system/service/RBAC composition.
// Current HTTP server implementation is centralized in handlers.Server, so this module
// contributes through shared server deps and remains a no-op for workers.
type GovernanceModule struct {
	infra *Infrastructure
}

func NewGovernanceModule(infra *Infrastructure) *GovernanceModule {
	return &GovernanceModule{infra: infra}
}

func (m *GovernanceModule) Name() string { return "governance" }

func (m *GovernanceModule) ContributeServerDeps(_ *handlers.ServerDeps) {}

func (m *GovernanceModule) RegisterWorkers(_ *river.Workers) {}

func (m *GovernanceModule) Shutdown(context.Context) error { return nil }
