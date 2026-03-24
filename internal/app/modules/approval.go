package modules

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	approvalbuiltin "kv-shepherd.io/shepherd/internal/governance/approval/builtin"
	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ApprovalModule wires the core ticket service and the built-in approval
// provider seam with ADR-0012 atomic execution.
type ApprovalModule struct {
	ticketService  *ticketing.Service
	providerRouter *approval.ApprovalProviderRouter
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
	providerRouter := approval.NewApprovalProviderRouter(builtinProvider)

	return &ApprovalModule{
		ticketService:  ticketService,
		providerRouter: providerRouter,
		requirements:   requirements,
		notifier:       notifier,
	}, nil
}

func (m *ApprovalModule) Name() string { return "approval" }

func (m *ApprovalModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	if deps == nil {
		return
	}
	deps.TicketService = m.ticketService
	deps.Notifier = m.notifier
	deps.ApprovalReqs = m.requirements
	// Stage 2.E: expose the provider router so handlers route submissions and
	// decisions through the provider seam instead of coupling to the built-in flow.
	deps.ApprovalRouter = m.providerRouter
}

func (m *ApprovalModule) Shutdown(context.Context) error { return nil }
