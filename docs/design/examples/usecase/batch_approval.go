//go:build ignore

// Package usecase provides batch operation examples.
//
// Reference: ADR-0015 §19, 04-governance.md §5.6
// Model: parent-child tickets + two-layer rate limiting + atomic submission.

package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BatchSubmitRequest is the canonical parent-ticket submission payload.
type BatchSubmitRequest struct {
	BatchType       string            `json:"batch_type"` // BATCH_CREATE, BATCH_DELETE, BATCH_APPROVE, BATCH_POWER
	Operation       string            `json:"operation"`  // create, delete, approve, start, stop, restart
	Items           []BatchTargetItem `json:"items"`
	RequestID       string            `json:"request_id"` // idempotency key (required)
	Reason          string            `json:"reason"`
	TargetClusterID string            `json:"target_cluster_id,omitempty"`
}

// BatchTargetItem is one child ticket target.
type BatchTargetItem struct {
	ResourceType string `json:"resource_type"` // vm, approval_ticket
	ResourceID   string `json:"resource_id"`
	Payload      any    `json:"payload,omitempty"`
}

// BatchSubmitResult is returned immediately with tracking metadata.
type BatchSubmitResult struct {
	BatchID           string `json:"batch_id"`
	Status            string `json:"status"` // PENDING_APPROVAL
	StatusURL         string `json:"status_url"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`
}

// BatchActionResult reports how many child tickets were affected by a mutation.
type BatchActionResult struct {
	BatchID       string `json:"batch_id"`
	AffectedCount int    `json:"affected_count"`
}

// BatchApprovalUseCase demonstrates ADR-0015-compliant submission flow.
type BatchApprovalUseCase struct {
	rateLimitSvc RateLimitService
	batchRepo    BatchRepository
	txManager    TxManager
}

func NewBatchApprovalUseCase(
	rateLimitSvc RateLimitService,
	batchRepo BatchRepository,
	txManager TxManager,
) *BatchApprovalUseCase {
	return &BatchApprovalUseCase{
		rateLimitSvc: rateLimitSvc,
		batchRepo:    batchRepo,
		txManager:    txManager,
	}
}

// Execute enforces two-layer rate limiting and creates parent/child tickets atomically.
func (u *BatchApprovalUseCase) Execute(ctx context.Context, req *BatchSubmitRequest, userID string) (*BatchSubmitResult, error) {
	if err := validateBatchRequest(req, userID); err != nil {
		return nil, err
	}

	// Idempotency fast-path: duplicate submission returns existing batch immediately.
	if existing, ok, err := u.batchRepo.FindByRequestID(ctx, userID, req.BatchType, req.RequestID); err != nil {
		return nil, err
	} else if ok {
		return buildSubmitResult(existing.BatchID), nil
	}

	// Layer 1: global protection.
	if err := u.rateLimitSvc.CheckGlobalBatchLimit(ctx); err != nil {
		return nil, err
	}

	// Layer 2: per-user fairness.
	if err := u.rateLimitSvc.CheckUserBatchLimit(ctx, userID, len(req.Items)); err != nil {
		return nil, err
	}

	batchID, err := u.txManager.WithTx(ctx, func(txCtx context.Context) (string, error) {
		// Idempotency: return existing batch if same request already submitted.
		if existing, ok, err := u.batchRepo.FindByRequestID(txCtx, userID, req.BatchType, req.RequestID); err != nil {
			return "", err
		} else if ok {
			return existing.BatchID, nil
		}

		parent := &BatchTicket{
			BatchID:       newBatchID(),
			BatchType:     req.BatchType,
			Status:        "PENDING_APPROVAL",
			CreatedBy:     userID,
			ChildCount:    len(req.Items),
			PendingCount:  len(req.Items),
			RequestID:     req.RequestID,
			RetryAfterSec: 2,
		}
		if err := u.batchRepo.CreateParent(txCtx, parent); err != nil {
			return "", err
		}

		for i, item := range req.Items {
			child := &ChildTicket{
				ParentBatchID: parent.BatchID,
				SequenceNo:    i + 1,
				ResourceType:  item.ResourceType,
				ResourceID:    item.ResourceID,
				Status:        "PENDING",
			}
			if err := u.batchRepo.CreateChild(txCtx, child); err != nil {
				return "", err // rollback entire submission
			}
		}

		return parent.BatchID, nil
	})
	if err != nil {
		return nil, err
	}

	return buildSubmitResult(batchID), nil
}

// RequeueFailed requeues FAILED children only; successful children are untouched.
func (u *BatchApprovalUseCase) RequeueFailed(ctx context.Context, batchID string, userID string) (*BatchActionResult, error) {
	affected, err := u.batchRepo.RequeueFailedChildren(ctx, batchID, userID, time.Now())
	if err != nil {
		return nil, err
	}
	return &BatchActionResult{BatchID: batchID, AffectedCount: affected}, nil
}

// CancelPending marks not-yet-executed children as CANCELLED.
func (u *BatchApprovalUseCase) CancelPending(ctx context.Context, batchID string, userID string) (*BatchActionResult, error) {
	affected, err := u.batchRepo.CancelPendingChildren(ctx, batchID, userID)
	if err != nil {
		return nil, err
	}
	return &BatchActionResult{BatchID: batchID, AffectedCount: affected}, nil
}

// --- Example-only placeholders ---

type RateLimitService interface {
	CheckGlobalBatchLimit(ctx context.Context) error
	CheckUserBatchLimit(ctx context.Context, userID string, newChildCount int) error
}

type TxManager interface {
	WithTx(ctx context.Context, fn func(txCtx context.Context) (string, error)) (string, error)
}

type BatchRepository interface {
	FindByRequestID(ctx context.Context, userID, batchType, requestID string) (*BatchTicket, bool, error)
	CreateParent(ctx context.Context, parent *BatchTicket) error
	CreateChild(ctx context.Context, child *ChildTicket) error
	RequeueFailedChildren(ctx context.Context, batchID, actorID string, ts time.Time) (int, error)
	CancelPendingChildren(ctx context.Context, batchID, actorID string) (int, error)
}

type BatchTicket struct {
	BatchID       string
	BatchType     string
	Status        string
	CreatedBy     string
	ChildCount    int
	SuccessCount  int
	FailedCount   int
	PendingCount  int
	RequestID     string
	RetryAfterSec int
}

var ErrInvalidBatchRequest = errors.New("invalid batch request")
var ErrBatchTooLarge = errors.New("batch size exceeds operation limit")

func newBatchID() string {
	return "BAT-" + uuid.NewString()
}

type ChildTicket struct {
	ChildTicketID string
	ParentBatchID string
	SequenceNo    int
	ResourceType  string
	ResourceID    string
	Status        string
	AttemptCount  int
}

func buildSubmitResult(batchID string) *BatchSubmitResult {
	return &BatchSubmitResult{
		BatchID:           batchID,
		Status:            "PENDING_APPROVAL",
		StatusURL:         "/api/v1/vms/batch/" + batchID,
		RetryAfterSeconds: 2,
	}
}

func validateBatchRequest(req *BatchSubmitRequest, userID string) error {
	if req == nil || userID == "" {
		return ErrInvalidBatchRequest
	}
	if req.RequestID == "" || req.BatchType == "" || req.Operation == "" {
		return ErrInvalidBatchRequest
	}
	if !isSupportedBatchType(req.BatchType) || !isSupportedOperation(req.Operation) {
		return ErrInvalidBatchRequest
	}
	if len(req.Items) == 0 {
		return ErrInvalidBatchRequest
	}

	maxBatchSize := maxBatchSizeFor(req.BatchType, req.Operation)
	if len(req.Items) > maxBatchSize {
		return ErrBatchTooLarge
	}

	for i, item := range req.Items {
		if item.ResourceType == "" || item.ResourceID == "" {
			return fmt.Errorf("%w: item %d missing resource_type/resource_id", ErrInvalidBatchRequest, i)
		}
	}
	return nil
}

func maxBatchSizeFor(batchType, operation string) int {
	switch {
	case batchType == "BATCH_CREATE" || batchType == "BATCH_DELETE":
		return 10
	case batchType == "BATCH_APPROVE":
		return 20
	case batchType == "BATCH_POWER":
		return 50
	case operation == "create" || operation == "delete":
		return 10
	case operation == "approve":
		return 20
	case operation == "start" || operation == "stop" || operation == "restart":
		return 50
	default:
		return 50
	}
}

func isSupportedBatchType(batchType string) bool {
	switch batchType {
	case "BATCH_CREATE", "BATCH_DELETE", "BATCH_APPROVE", "BATCH_POWER":
		return true
	default:
		return false
	}
}

func isSupportedOperation(operation string) bool {
	switch operation {
	case "create", "delete", "approve", "start", "stop", "restart":
		return true
	default:
		return false
	}
}
