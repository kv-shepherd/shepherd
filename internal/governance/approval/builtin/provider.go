package builtin

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/governance/ticketing"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"
)

const providerType = "builtin-default"

// Provider implements the built-in approval workflow behind the shared
// approvalcontract.ApprovalProvider seam.
type Provider struct {
	ticketService *ticketing.Service
}

var _ approvalcontract.ApprovalProvider = (*Provider)(nil)

func NewProvider(ticketService *ticketing.Service) *Provider {
	if ticketService == nil {
		panic("approval builtin provider requires a non-nil ticket service")
	}
	return &Provider{ticketService: ticketService}
}

func (p *Provider) Type() string { return providerType }

func (p *Provider) SubmitForApproval(ctx context.Context, req *approvalcontract.ApprovalRequest) (*approvalcontract.ApprovalResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("approval builtin provider: request must not be nil")
	}
	if strings.TrimSpace(req.EventID) == "" {
		return nil, fmt.Errorf("approval builtin provider: event_id is required")
	}

	logger.Info("builtin approval provider: ticket submitted to internal queue",
		zap.String("event_id", req.EventID),
		zap.String("requester", req.Requester),
		zap.String("action", req.Action),
	)
	return &approvalcontract.ApprovalResponse{
		TicketID: req.EventID,
		Status:   "PENDING",
	}, nil
}

func (p *Provider) ProcessApproval(ctx context.Context, ticketID string, decision approvalcontract.ApprovalDecision) error {
	if strings.TrimSpace(ticketID) == "" {
		return fmt.Errorf("approval builtin provider: ticket_id is required")
	}
	if decision.Approved {
		return p.ticketService.Approve(ctx, ticketID, decision.Approver, ticketing.ExecutionOptions{
			ClusterID:       decision.Execution.ClusterID,
			StorageClass:    decision.Execution.StorageClass,
			DVAccessModes:   decision.Execution.DVAccessModes,
			DVVolumeMode:    decision.Execution.DVVolumeMode,
			EnableOverride:  decision.Execution.EnableOverride,
			CPURequest:      decision.Execution.CPURequest,
			CPULimit:        decision.Execution.CPULimit,
			MemoryRequestGi: decision.Execution.MemoryRequestGi,
			MemoryLimitGi:   decision.Execution.MemoryLimitGi,
			DiskGB:          decision.Execution.DiskGB,
		})
	}
	return p.ticketService.Reject(ctx, ticketID, decision.Approver, decision.RejectReason)
}
