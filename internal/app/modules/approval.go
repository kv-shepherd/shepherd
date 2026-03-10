package modules

import (
	"context"
	"fmt"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ApprovalModule wires governance approval gateway with ADR-0012 atomic writer.
type ApprovalModule struct {
	gateway        *approval.Gateway
	providerRouter *approval.ApprovalProviderRouter
	requirements   *service.ApprovalRequirementService
	notifier       *notification.Triggers
}

// NewApprovalModule creates the approval module after River client is initialized.
//
// Stage 2.E wiring:
//  1. Gateway is created (the atomic approval executor).
//  2. BuiltinApprovalProvider wraps Gateway — implements provider.ApprovalProvider.
//  3. ApprovalProviderRouter is created with the builtin provider as the default.
//     V2+ external providers can be registered on the router after this point.
//
// P1-A: vmSvc may be nil (backward compatible). When non-nil, DryRun Pre-flight Gate
// is enabled in Gateway.approveCreate (ADR-0006 Addendum).
func NewApprovalModule(infra *Infrastructure, vmSvc *service.VMService) (*ApprovalModule, error) {
	if infra == nil || infra.EntClient == nil || infra.Pool == nil || infra.RiverClient == nil {
		return nil, fmt.Errorf("approval module requires ent client, pgx pool, and river client")
	}

	atomicWriter := usecase.NewApprovalAtomicWriter(infra.Pool, infra.RiverClient)
	gateway := approval.NewGateway(infra.EntClient, infra.AuditLogger, atomicWriter)
	requirements := service.NewApprovalRequirementService(infra.EntClient)

	// Wire notification system (ADR-0015 §20, master-flow.md Stage 5.F).
	inboxSender := notification.NewInboxSender(infra.EntClient)
	notifier := notification.NewTriggers(inboxSender, infra.EntClient)
	gateway.SetNotifier(notifier)

	// P1-A: Wire vmService for DryRun Pre-flight Gate (ADR-0006 Addendum).
	// vmSvc is nil-safe: if nil, DryRun gate is skipped (backward compatible).
	if vmSvc != nil {
		gateway.SetVMService(vmSvc)
	}

	// Stage 2.E: wire the provider router over the gateway.
	// V1: only the built-in provider is registered; the router always selects it.
	// V2+: external providers can be added via router.Register(externalProvider).
	builtinProvider := approval.NewBuiltinApprovalProvider(gateway)
	providerRouter := approval.NewApprovalProviderRouter(builtinProvider)

	return &ApprovalModule{
		gateway:        gateway,
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
	deps.Gateway = m.gateway
	deps.Notifier = m.notifier
	deps.ApprovalReqs = m.requirements
	// Stage 2.E: expose the provider router so handlers can route submissions
	// through the correct backend (builtin in V1, external in V2+).
	deps.ApprovalRouter = m.providerRouter
}

func (m *ApprovalModule) Shutdown(context.Context) error { return nil }
