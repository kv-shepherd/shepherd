// Package approval — Stage 2.E: Approval Provider Router
//
// The router is the seam between core work-order semantics and provider-owned
// approval workflow. V1 wires one built-in provider behind this interface so
// submission and final decision handling do not couple handlers directly to the
// core ticket service.
//
// Go interface compliance: compile-time guard at bottom of file.
// ADR-0013: manual DI, no Wire.
// ADR-0016: import path kv-shepherd.io/shepherd/internal/governance/approval

package approval

import (
	"context"
	"fmt"
	"sync"

	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

// ApprovalProviderRouter owns the active approval provider for this runtime.
// It keeps handlers decoupled from built-in and external provider
// implementation details.
type ApprovalProviderRouter struct {
	mu       sync.RWMutex
	provider approvalcontract.ApprovalProvider
}

// NewApprovalProviderRouter creates a router pre-wired with the active approval
// provider.
func NewApprovalProviderRouter(activeProvider approvalcontract.ApprovalProvider) *ApprovalProviderRouter {
	if activeProvider == nil {
		panic("approval: ApprovalProviderRouter requires a non-nil provider")
	}
	return &ApprovalProviderRouter{provider: activeProvider}
}

// SetActiveProvider swaps the runtime provider used for new approval calls.
func (r *ApprovalProviderRouter) SetActiveProvider(activeProvider approvalcontract.ApprovalProvider) error {
	if activeProvider == nil {
		return fmt.Errorf("approval router: provider is not configured")
	}
	if r == nil {
		return fmt.Errorf("approval router: router is not configured")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.provider = activeProvider
	return nil
}

// ActiveProviderType returns the currently configured provider type.
func (r *ApprovalProviderRouter) ActiveProviderType() string {
	if r == nil {
		return ""
	}
	r.mu.RLock()
	provider := r.provider
	r.mu.RUnlock()
	if provider == nil {
		return ""
	}
	return provider.Type()
}

// SubmitForApproval delegates ticket submission to the active provider.
func (r *ApprovalProviderRouter) SubmitForApproval(ctx context.Context, req *approvalcontract.ApprovalRequest) (*approvalcontract.ApprovalResponse, error) {
	provider := r.activeProvider()
	if provider == nil {
		return nil, fmt.Errorf("approval router: provider is not configured")
	}
	return provider.SubmitForApproval(ctx, req)
}

// ProcessApproval delegates the provider decision path to the active provider.
func (r *ApprovalProviderRouter) ProcessApproval(ctx context.Context, ticketID string, decision approvalcontract.ApprovalDecision) error {
	provider := r.activeProvider()
	if provider == nil {
		return fmt.Errorf("approval router: provider is not configured")
	}
	return provider.ProcessApproval(ctx, ticketID, decision)
}

func (r *ApprovalProviderRouter) activeProvider() approvalcontract.ApprovalProvider {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.provider
}
