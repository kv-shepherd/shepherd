// Package approval — Stage 2.E: Approval Provider Router
//
// master-flow.md §Stage 2.E defines a canonical provider-router contract with pluggable backends.
//
//   V1 go-live path (required):
//     1) User submits request → approval_tickets=PENDING
//     2) Router selects built-in provider ("builtin-default", only provider in V1)
//     3) Built-in approver decides APPROVED / REJECTED
//     4) Shepherd executes decision path and appends audit logs
//
//   V2+ external plugin route (roadmap):
//     1) External adapter is registered via router.Register(provider)
//     2) Router delegates ticket via ExternalApprovalProvider.SubmitForApproval
//     3) Callback/polling maps external decision to canonical APPROVED/REJECTED
//     4) Provider timeout/unavailable → controlled fallback to built-in queue
//
// Design:
//   - ApprovalProviderRouter wraps Gateway; it selects the right backend per ticket.
//   - V1 ships with BuiltinApprovalProvider only (direct Gateway delegation).
//   - V2+ registers external providers; a "type" field on the ticket (or policy)
//     determines routing.  The router falls back to the built-in on error or timeout.
//
// Go interface compliance: compile-time guard at bottom of file.
// ADR-0013: manual DI, no Wire.
// ADR-0016: import path kv-shepherd.io/shepherd/internal/governance/approval

package approval

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider"
)

const builtinProviderType = "builtin-default"

// ─── Built-in Provider (V1) ───────────────────────────────────────────────────

// BuiltinApprovalProvider is the V1 mandatory provider that delegates all
// approval decisions to the Gateway's internal logic.
//
// It implements provider.ApprovalProvider so that the router can treat it as
// one of N possible backends without the router knowing any approval details.
//
// This design lets V2+ external adapters slot in alongside the built-in
// without modifying the Gateway or existing call sites.
type BuiltinApprovalProvider struct {
	gateway *Gateway
}

// Compile-time interface compliance check (Go FAQ recommended pattern).
var _ provider.ApprovalProvider = (*BuiltinApprovalProvider)(nil)

// NewBuiltinApprovalProvider creates the V1 built-in provider backed by gateway.
func NewBuiltinApprovalProvider(gw *Gateway) *BuiltinApprovalProvider {
	if gw == nil {
		panic("approval: BuiltinApprovalProvider requires a non-nil Gateway")
	}
	return &BuiltinApprovalProvider{gateway: gw}
}

// Type returns the canonical identifier for this provider.
// "builtin-default" matches the label in master-flow.md §Stage 2.E.
func (p *BuiltinApprovalProvider) Type() string { return builtinProviderType }

// SubmitForApproval creates a PENDING ticket and routes to the Gateway's built-in
// approval queue.  In V1 this is a no-op because ticket creation happens upstream
// (at the request-submission handler), so this method simply validates the request.
//
// V2+ external providers will POST to an external queue here instead.
func (p *BuiltinApprovalProvider) SubmitForApproval(ctx context.Context, req *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("approval: SubmitForApproval: request must not be nil")
	}
	if strings.TrimSpace(req.EventID) == "" {
		return nil, fmt.Errorf("approval: SubmitForApproval: event_id is required")
	}

	// V1: ticket is already created by the handler before this is called.
	// The built-in provider delegates to the internal queue (admin UI) — no external
	// call needed.  We return PENDING to signal that the ticket is in the queue.
	logger.Info("builtin approval provider: ticket submitted to internal queue",
		zap.String("event_id", req.EventID),
		zap.String("requester", req.Requester),
		zap.String("action", req.Action),
	)
	return &provider.ApprovalResponse{
		TicketID: req.EventID, // echoed; actual ticket ID is set by the handler upstream
		Status:   "PENDING",
	}, nil
}

// ProcessApproval executes an approval decision through the Gateway.
// opts is passed as the zero-value ApproveOpts; callers that need cluster/storage-class
// selection should call Gateway.Approve directly (the admin approval handler).
//
// This method exists to satisfy the ApprovalProvider interface so that the
// router can dispatch decisions uniformly across providers.
func (p *BuiltinApprovalProvider) ProcessApproval(ctx context.Context, ticketID string, decision provider.ApprovalDecision) error {
	if strings.TrimSpace(ticketID) == "" {
		return fmt.Errorf("approval: ProcessApproval: ticket_id is required")
	}
	if decision.Approved {
		// Built-in provider uses zero-value ApproveOpts for router-initiated approvals.
		// Admin-initiated approvals with full opts go through Gateway.Approve directly.
		return p.gateway.Approve(ctx, ticketID, decision.Approver, ApproveOpts{})
	}
	return p.gateway.Reject(ctx, ticketID, decision.Approver, decision.RejectReason)
}

// ─── Approval Provider Router (Stage 2.E) ────────────────────────────────────

// ApprovalProviderRouter selects the active approval backend for each ticket.
//
// V1: always routes to the built-in provider.
// V2+: consults the registered provider registry; falls back to built-in on error
//
// Thread-safe; providers are registered at startup and immutable during operation.
type ApprovalProviderRouter struct {
	mu       sync.RWMutex
	builtin  provider.ApprovalProvider
	external map[string]provider.ApprovalProvider // key = provider.Type()
}

// NewApprovalProviderRouter creates a router pre-wired with the V1 built-in provider.
// The built-in provider is always present and acts as the fallback for V2+.
func NewApprovalProviderRouter(builtinProvider provider.ApprovalProvider) *ApprovalProviderRouter {
	if builtinProvider == nil {
		panic("approval: ApprovalProviderRouter requires a non-nil builtin provider")
	}
	return &ApprovalProviderRouter{
		builtin:  builtinProvider,
		external: make(map[string]provider.ApprovalProvider),
	}
}

// Register adds an external provider to the router (V2+ extension point).
// Returns an error if the type key is already registered or is the reserved
// "builtin-default" type used exclusively by the built-in provider.
func (r *ApprovalProviderRouter) Register(p provider.ApprovalProvider) error {
	if p == nil {
		return fmt.Errorf("approval router: provider must not be nil")
	}
	key := strings.TrimSpace(strings.ToLower(p.Type()))
	if key == "" {
		return fmt.Errorf("approval router: provider.Type() must not be empty")
	}
	if key == builtinProviderType {
		return fmt.Errorf("approval router: %q is reserved for the built-in provider", builtinProviderType)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.external[key]; exists {
		return fmt.Errorf("approval router: provider type %q is already registered", key)
	}
	r.external[key] = p
	logger.Info("approval router: external provider registered", zap.String("type", key))
	return nil
}

// Resolve returns the provider for the given type key, or the built-in fallback.
// This implements the master-flow.md §Stage 2.E routing contract:
//
//	"Router selects built-in provider (`builtin-default`, only provider in V1)"
//
// V2+: if a matching external provider is registered, it is returned instead.
// If the type is unknown, the built-in provider is returned as a safe fallback.
func (r *ApprovalProviderRouter) Resolve(providerType string) provider.ApprovalProvider {
	key := strings.TrimSpace(strings.ToLower(providerType))
	if key == "" || key == builtinProviderType {
		return r.builtin
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.external[key]; ok {
		return p
	}
	// Unknown type → controlled fallback to built-in (Stage 2.E §4).
	logger.Warn("approval router: unknown provider type, falling back to builtin",
		zap.String("requested_type", key),
	)
	return r.builtin
}

// SubmitForApproval routes a submission through the resolved provider.
// Uses the built-in provider (V1), or an external provider if registered (V2+).
func (r *ApprovalProviderRouter) SubmitForApproval(ctx context.Context, req *provider.ApprovalRequest) (*provider.ApprovalResponse, error) {
	// V1: always builtin.  V2+: providerType would come from policy/config.
	p := r.Resolve(builtinProviderType)
	resp, err := p.SubmitForApproval(ctx, req)
	if err != nil {
		// Stage 2.E fallback: provider error → route to built-in queue.
		if p.Type() != r.builtin.Type() {
			logger.Warn("approval router: external provider SubmitForApproval failed, falling back",
				zap.String("provider", p.Type()),
				zap.Error(err),
			)
			return r.builtin.SubmitForApproval(ctx, req)
		}
		return nil, err
	}
	return resp, nil
}
