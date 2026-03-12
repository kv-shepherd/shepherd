package modules

import (
	"context"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/service"
)

// AdminModule represents admin-domain composition
// (clusters/templates/instance sizes/policies).
type AdminModule struct {
	infra         *Infrastructure
	clusterPolicy *service.ClusterPolicyService
	approvalReqs  *service.ApprovalRequirementService
}

func NewAdminModule(infra *Infrastructure) *AdminModule {
	return &AdminModule{
		infra:         infra,
		clusterPolicy: service.NewClusterPolicyService(infra.EntClient),
		approvalReqs:  service.NewApprovalRequirementService(infra.EntClient),
	}
}

func (m *AdminModule) Name() string { return "admin" }

func (m *AdminModule) Shutdown(context.Context) error { return nil }

// ContributeServerDeps wires admin-domain services into the HTTP server.
// ADR-0013: All service construction must happen in the composition root.
func (m *AdminModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	deps.ClusterPolicy = m.clusterPolicy
	if deps.ApprovalReqs == nil {
		deps.ApprovalReqs = m.approvalReqs
	}
}
