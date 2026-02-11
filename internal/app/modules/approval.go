package modules

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/internal/api/handlers"
	"kv-shepherd.io/shepherd/internal/governance/approval"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/usecase"
)

// ApprovalModule wires governance approval gateway with ADR-0012 atomic writer.
type ApprovalModule struct {
	gateway  *approval.Gateway
	notifier *notification.Triggers
}

// NewApprovalModule creates the approval module after River client is initialized.
func NewApprovalModule(infra *Infrastructure) (*ApprovalModule, error) {
	if infra == nil || infra.EntClient == nil || infra.Pool == nil || infra.RiverClient == nil {
		return nil, fmt.Errorf("approval module requires ent client, pgx pool, and river client")
	}

	atomicWriter := usecase.NewApprovalAtomicWriter(infra.Pool, infra.RiverClient)
	gateway := approval.NewGateway(infra.EntClient, infra.AuditLogger, atomicWriter)

	// Wire notification system (ADR-0015 §20, master-flow.md Stage 5.F).
	inboxSender := notification.NewInboxSender(infra.EntClient)
	notifier := notification.NewTriggers(inboxSender, infra.EntClient)
	gateway.SetNotifier(notifier)

	return &ApprovalModule{gateway: gateway, notifier: notifier}, nil
}

func (m *ApprovalModule) Name() string { return "approval" }

func (m *ApprovalModule) ContributeServerDeps(deps *handlers.ServerDeps) {
	if deps == nil {
		return
	}
	deps.Gateway = m.gateway
	deps.Notifier = m.notifier
}

func (m *ApprovalModule) RegisterWorkers(_ *river.Workers) {}

func (m *ApprovalModule) Shutdown(context.Context) error { return nil }
