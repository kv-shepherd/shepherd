package modules

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	approvalbuiltin "kv-shepherd.io/shepherd/internal/governance/approval/builtin"
	approvalregistry "kv-shepherd.io/shepherd/internal/governance/approval/registry"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ApprovalModule wires the core ticket service and the built-in approval
// provider seam with ADR-0012 atomic execution.
type ApprovalModule struct {
	ticketService  *ticketing.Service
	providerRouter *approval.ApprovalProviderRouter
	registry       *approvalregistry.Service
	requirements   *service.ApprovalRequirementService
	notifier       *notification.Triggers
}

// NewApprovalModule creates the approval module after River client is initialized.
//
// Stage 2.E wiring:
//  1. Core ticket service is created (canonical decision executor).
//  2. Built-in provider plugin wraps that service behind provider.ApprovalProvider.
//  3. ApprovalProviderRouter hosts that provider so handlers only depend on the
//     canonical provider seam, not the built-in workflow implementation.
//
// P1-A: vmSvc may be nil (backward compatible). When non-nil, DryRun Pre-flight Gate
// is enabled in ticketing.Service.approveCreate (ADR-0006 Addendum).
func NewApprovalModule(infra *Infrastructure, vmSvc *service.VMService) (*ApprovalModule, error) {
	if infra == nil || infra.EntClient == nil || infra.Pool == nil || infra.RiverClient == nil {
		return nil, fmt.Errorf("approval module requires ent client, pgx pool, and river client")
	}

	atomicWriter := usecase.NewApprovalAtomicWriter(infra.Pool, infra.RiverClient)
	ticketService := ticketing.NewService(infra.EntClient, infra.AuditLogger, atomicWriter)
	requirements := service.NewApprovalRequirementService(infra.EntClient)

	// Wire notification system (ADR-0015 §20, master-flow.md Stage 5.F).
	inboxSender := notification.NewInboxSender(infra.EntClient)
	notifier := notification.NewTriggers(inboxSender, infra.EntClient)
	ticketService.SetNotifier(notifier)

	// P1-A: Wire vmService for DryRun Pre-flight Gate (ADR-0006 Addendum).
	// vmSvc is nil-safe: if nil, DryRun gate is skipped (backward compatible).
	if vmSvc != nil {
		ticketService.SetVMService(vmSvc)
	}

	// Stage 2.E: host the built-in approval provider behind the provider seam.
	builtinProvider := approvalbuiltin.NewProvider(ticketService)
	registry := approvalregistry.NewService(infra.EntClient, infra.EncryptionKey)
	activeProvider := approvalProviderOrFallback(registry, builtinProvider)
	providerRouter := approval.NewApprovalProviderRouter(activeProvider)

	return &ApprovalModule{
		ticketService:  ticketService,
		providerRouter: providerRouter,
		registry:       registry,
		requirements:   requirements,
		notifier:       notifier,
	}, nil
}

func (m *ApprovalModule) Name() string { return "approval" }

// TicketService exposes the initialized dispatcher target to the composition
// root before River workers start consuming jobs.
func (m *ApprovalModule) TicketService() *ticketing.Service {
	if m == nil {
		return nil
	}
	return m.ticketService
}

func (m *ApprovalModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	if deps == nil {
		return
	}
	deps.TicketService = m.ticketService
	deps.Notifier = m.notifier
	deps.ApprovalReqs = m.requirements
	deps.ExternalApprovalRegistry = m.registry
	// Stage 2.E: expose the provider router so handlers route submissions and
	// decisions through the provider seam instead of coupling to the built-in flow.
	deps.ApprovalRouter = m.providerRouter
}

func (m *ApprovalModule) Shutdown(context.Context) error { return nil }

func approvalProviderOrFallback(
	registry *approvalregistry.Service,
	fallback approvalcontract.ApprovalProvider,
) approvalcontract.ApprovalProvider {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider, err := registry.ActiveProvider(ctx, fallback)
	if err != nil {
		logger.Warn("external approval registry unavailable; using built-in approval provider", zap.Error(err))
		return fallback
	}
	return provider
}
