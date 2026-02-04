// Package usecase provides batch operation use cases.
//
// Reference: ADR-0015 §19, 04-governance.md §5.6
// Purpose: Batch Approval Use Case
// V1 Strategy: UX convenience - batch is not atomic, each item queued independently

package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// BatchApprovalRequest represents a batch approval request
type BatchApprovalRequest struct {
	TicketIDs    []string `json:"ticket_ids"`
	ClusterID    string   `json:"cluster_id"`
	InstanceSize string   `json:"instance_size,omitempty"`
	Reason       string   `json:"reason"`
}

// BatchResult represents the result of a batch operation
type BatchResult struct {
	BatchID  string      `json:"batch_id"`
	Total    int         `json:"total"`
	Accepted int         `json:"accepted"`
	Rejected int         `json:"rejected"`
	Items    []BatchItem `json:"items"`
}

// BatchItem represents a single item in the batch result
type BatchItem struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"` // "queued", "rejected", "error"
	JobID    string `json:"job_id,omitempty"`
	Error    string `json:"error,omitempty"`
}

// ApprovalJobArgs represents the arguments for a river approval job
type ApprovalJobArgs struct {
	TicketID     string `json:"ticket_id"`
	ClusterID    string `json:"cluster_id"`
	InstanceSize string `json:"instance_size,omitempty"`
	ApprovedBy   string `json:"approved_by"`
}

func (ApprovalJobArgs) Kind() string { return "approval_job" }

// BatchApprovalUseCase handles batch approval operations
type BatchApprovalUseCase struct {
	riverClient   *river.Client[any]
	ticketRepo    TicketRepository
	clusterRepo   ClusterRepository
	permissionSvc PermissionService
}

// NewBatchApprovalUseCase creates a new batch approval use case
func NewBatchApprovalUseCase(
	riverClient *river.Client[any],
	ticketRepo TicketRepository,
	clusterRepo ClusterRepository,
	permissionSvc PermissionService,
) *BatchApprovalUseCase {
	return &BatchApprovalUseCase{
		riverClient:   riverClient,
		ticketRepo:    ticketRepo,
		clusterRepo:   clusterRepo,
		permissionSvc: permissionSvc,
	}
}

// Execute processes a batch approval request
// Each ticket is validated independently and queued as a separate River job
func (u *BatchApprovalUseCase) Execute(ctx context.Context, req *BatchApprovalRequest) (*BatchResult, error) {
	result := &BatchResult{
		BatchID: uuid.NewString(),
		Items:   make([]BatchItem, len(req.TicketIDs)),
	}

	userID := ctx.Value("user_id").(string)

	for i, ticketID := range req.TicketIDs {
		// Validate each ticket individually
		if err := u.validateTicket(ctx, ticketID, req); err != nil {
			result.Items[i] = BatchItem{
				TicketID: ticketID,
				Status:   "rejected",
				Error:    err.Error(),
			}
			result.Rejected++
			continue
		}

		// Enqueue individual River job
		job, err := u.riverClient.Insert(ctx, &ApprovalJobArgs{
			TicketID:     ticketID,
			ClusterID:    req.ClusterID,
			InstanceSize: req.InstanceSize,
			ApprovedBy:   userID,
		}, nil)

		if err != nil {
			result.Items[i] = BatchItem{TicketID: ticketID, Status: "error", Error: err.Error()}
			result.Rejected++
		} else {
			result.Items[i] = BatchItem{
				TicketID: ticketID,
				Status:   "queued",
				JobID:    job.ID,
			}
			result.Accepted++
		}
	}

	result.Total = len(req.TicketIDs)
	return result, nil
}

// validateTicket checks if a ticket can be approved
func (u *BatchApprovalUseCase) validateTicket(ctx context.Context, ticketID string, req *BatchApprovalRequest) error {
	// 1. Check ticket exists and is pending
	ticket, err := u.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return err
	}
	if ticket.Status != "PENDING_APPROVAL" {
		return ErrTicketNotPending
	}

	// 2. Check user has approval permission
	userID := ctx.Value("user_id").(string)
	hasPermission, err := u.permissionSvc.HasPermission(ctx, userID, "approval:approve", "ticket", ticketID)
	if err != nil || !hasPermission {
		return ErrNoApprovalPermission
	}

	// 3. Check cluster environment matches ticket
	cluster, err := u.clusterRepo.GetByID(ctx, req.ClusterID)
	if err != nil {
		return err
	}
	if cluster.Environment != ticket.Environment {
		return ErrEnvironmentMismatch
	}

	return nil
}

// Placeholder interfaces and errors for compilation
type TicketRepository interface {
	GetByID(ctx context.Context, id string) (*Ticket, error)
}

type ClusterRepository interface {
	GetByID(ctx context.Context, id string) (*Cluster, error)
}

type PermissionService interface {
	HasPermission(ctx context.Context, userID, permission, resourceType, resourceID string) (bool, error)
}

type Ticket struct {
	ID          string
	Status      string
	Environment string
}

type Cluster struct {
	ID          string
	Environment string
}

var (
	ErrTicketNotPending     = &AppError{Code: "TICKET_NOT_PENDING"}
	ErrNoApprovalPermission = &AppError{Code: "NO_APPROVAL_PERMISSION"}
	ErrEnvironmentMismatch  = &AppError{Code: "ENVIRONMENT_MISMATCH"}
)

type AppError struct {
	Code string
}

func (e *AppError) Error() string { return e.Code }
