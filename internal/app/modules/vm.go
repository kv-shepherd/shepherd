package modules

import (
	"context"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/jobs"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// VMModule wires VM domain use cases/services and workers.
type VMModule struct {
	infra      *Infrastructure
	vmService  *service.VMService
	createVMUC *usecase.CreateVMUseCase
	deleteVMUC *usecase.DeleteVMUseCase
}

// NewVMModule creates a VM module with explicit constructor wiring.
func NewVMModule(infra *Infrastructure) *VMModule {
	vmSvc := service.NewVMService(provider.NewMockProvider())
	createVM := usecase.NewCreateVMUseCase(infra.EntClient, vmSvc, service.NewInstanceSizeService(infra.EntClient))
	deleteVM := usecase.NewDeleteVMUseCase(infra.EntClient).WithAuditLogger(infra.AuditLogger)

	return &VMModule{
		infra:      infra,
		vmService:  vmSvc,
		createVMUC: createVM,
		deleteVMUC: deleteVM,
	}
}

func (m *VMModule) Name() string { return "vm" }

func (m *VMModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	if deps == nil {
		return
	}
	deps.VMService = m.vmService
	deps.CreateVMUC = m.createVMUC
	deps.DeleteVMUC = m.deleteVMUC
}

func (m *VMModule) RegisterWorkers(workers *river.Workers) {
	if workers == nil || m == nil || m.infra == nil {
		return
	}
	river.AddWorker(workers, jobs.NewVMCreateWorker(m.infra.EntClient, m.vmService, m.infra.AuditLogger))
	river.AddWorker(workers, jobs.NewVMDeleteWorker(m.infra.EntClient, m.vmService, m.infra.AuditLogger))
	river.AddWorker(workers, jobs.NewVMPowerWorker(m.infra.EntClient, m.vmService, m.infra.AuditLogger))
}

func (m *VMModule) Shutdown(context.Context) error { return nil }
