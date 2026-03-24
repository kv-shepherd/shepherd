package modules

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/service"
)

// AdminModule represents admin-domain composition
// (clusters/templates/instance sizes/policies).
type AdminModule struct {
	infra         *Infrastructure
	clusterPolicy *service.ClusterPolicyService
	approvalReqs  *service.ApprovalRequirementService
	directorySync *service.DirectorySyncService
}

func NewAdminModule(infra *Infrastructure) *AdminModule {
	return &AdminModule{
		infra:         infra,
		clusterPolicy: service.NewClusterPolicyService(infra.EntClient),
		approvalReqs:  service.NewApprovalRequirementService(infra.EntClient),
		directorySync: service.NewDirectorySyncService(infra.EntClient),
	}
}

func (m *AdminModule) Name() string { return "admin" }

func (m *AdminModule) Shutdown(context.Context) error { return nil }

func (m *AdminModule) RegisterWorkers(workers *river.Workers) {
	if workers == nil || m == nil || m.infra == nil || m.infra.EntClient == nil || m.directorySync == nil {
		return
	}
	river.AddWorker(workers, jobs.NewDirectorySyncWorker(m.infra.EntClient, m.directorySync, m.infra.AuditLogger, m.infra.EncryptionKey))
	river.AddWorker(workers, jobs.NewDirectoryEnrichmentScheduleScanWorker(m.infra.EntClient, func() *river.Client[pgx.Tx] {
		return m.infra.RiverClient
	}, m.infra.AuditLogger, m.infra.EncryptionKey))
}

// ContributeServerDeps wires admin-domain services into the HTTP server.
// ADR-0013: All service construction must happen in the composition root.
func (m *AdminModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	deps.ClusterPolicy = m.clusterPolicy
	if deps.ApprovalReqs == nil {
		deps.ApprovalReqs = m.approvalReqs
	}
	if deps.DirectorySync == nil {
		deps.DirectorySync = m.directorySync
	}
}
