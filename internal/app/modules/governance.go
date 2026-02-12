package modules

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/jobs"
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

func (m *GovernanceModule) RegisterWorkers(workers *river.Workers) {
	if workers == nil || m == nil || m.infra == nil || m.infra.EntClient == nil {
		return
	}
	river.AddWorker(workers, jobs.NewNotificationCleanupWorker(m.infra.EntClient, 90*24*time.Hour))
}

func (m *GovernanceModule) Shutdown(context.Context) error { return nil }
