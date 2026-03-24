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

	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

// ApprovalProviderRouter owns the active approval provider for this runtime.
// V1 always wires the built-in provider here, which keeps handlers decoupled
// from provider implementation details.
type ApprovalProviderRouter struct {
	provider approvalcontract.ApprovalProvider
}

// NewApprovalProviderRouter creates a router pre-wired with the active approval
// provider. V1 uses the built-in provider exclusively.
func NewApprovalProviderRouter(activeProvider approvalcontract.ApprovalProvider) *ApprovalProviderRouter {
	if activeProvider == nil {
		panic("approval: ApprovalProviderRouter requires a non-nil provider")
	}
	return &ApprovalProviderRouter{provider: activeProvider}
}

// SubmitForApproval delegates ticket submission to the active provider.
func (r *ApprovalProviderRouter) SubmitForApproval(ctx context.Context, req *approvalcontract.ApprovalRequest) (*approvalcontract.ApprovalResponse, error) {
	if r == nil || r.provider == nil {
		return nil, fmt.Errorf("approval router: provider is not configured")
	}
	return r.provider.SubmitForApproval(ctx, req)
}

// ProcessApproval delegates the provider decision path to the active provider.
func (r *ApprovalProviderRouter) ProcessApproval(ctx context.Context, ticketID string, decision approvalcontract.ApprovalDecision) error {
	if r == nil || r.provider == nil {
		return fmt.Errorf("approval router: provider is not configured")
	}
	return r.provider.ProcessApproval(ctx, ticketID, decision)
}
