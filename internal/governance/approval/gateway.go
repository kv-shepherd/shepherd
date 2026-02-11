// Package approval implements the approval workflow gateway.
//
// ADR-0005: Simple approval flow — PENDING → APPROVED or REJECTED.
// V1 scope: No multi-level chains, no timeout auto-processing.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/governance/approval
package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/approvalticket"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/governance/audit"
	"kv-shepherd.io/shepherd/internal/notification"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/service"
)

// AtomicApprovalWriter defines ADR-0012 atomic write operations for approval decisions.
type AtomicApprovalWriter interface {
	ApproveCreateAndEnqueue(
		ctx context.Context,
		ticketID, eventID, approver, clusterID, storageClass, serviceID, namespace, requesterID string,
	) (vmID, vmName string, err error)
	ApproveDeleteAndEnqueue(ctx context.Context, ticketID, eventID, approver, vmID string) error
}

// Gateway orchestrates approval decisions.
type Gateway struct {
	client       *ent.Client
	auditLogger  *audit.Logger
	validator    *service.ApprovalValidator
	atomicWriter AtomicApprovalWriter
	notifier     *notification.Triggers // Optional: nil-safe for backward compatibility
}

// NewGateway creates a new approval Gateway.
func NewGateway(client *ent.Client, auditLogger *audit.Logger, atomicWriter AtomicApprovalWriter) *Gateway {
	return &Gateway{
		client:       client,
		auditLogger:  auditLogger,
		validator:    service.NewApprovalValidator(client),
		atomicWriter: atomicWriter,
	}
}

// SetNotifier configures the notification trigger service.
// This is a setter to avoid breaking the existing constructor signature.
func (g *Gateway) SetNotifier(notifier *notification.Triggers) {
	g.notifier = notifier
}

// Approve approves a pending ticket. Admin-determined fields set here (ADR-0017).
// ADR-0012: ticket/domain/vm writes and River enqueue are committed atomically.
//
// Branching logic by operation_type:
//   - CREATE: ticket APPROVED + VM record CREATING → enqueue VMCreateArgs
//   - DELETE: ticket APPROVED + VM status DELETING → enqueue VMDeleteArgs
func (g *Gateway) Approve(ctx context.Context, ticketID, approver string, clusterID, storageClass string) error {
	ticket, err := g.client.ApprovalTicket.Get(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != approvalticket.StatusPENDING {
		return fmt.Errorf("ticket %s is not pending (current: %s)", ticketID, ticket.Status)
	}

	// Branch by operation_type (ADR-0015 §5.D).
	switch ticket.OperationType {
	case approvalticket.OperationTypeDELETE:
		return g.approveDelete(ctx, ticket, ticketID, approver)
	default:
		// CREATE is the default operation type.
		return g.approveCreate(ctx, ticket, ticketID, approver, clusterID, storageClass)
	}
}

// approveCreate handles approval of CREATE tickets (original flow).
func (g *Gateway) approveCreate(ctx context.Context, ticket *ent.ApprovalTicket, ticketID, approver, clusterID, storageClass string) error {
	if clusterID == "" {
		return fmt.Errorf("selected cluster is required for create approval")
	}

	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}

	payload, err := parseVMCreatePayload(event.Payload)
	if err != nil {
		return fmt.Errorf("parse create payload for ticket %s: %w", ticketID, err)
	}

	if g.validator != nil {
		if err := g.validator.ValidateApproval(ctx, clusterID, payload.InstanceSizeID); err != nil {
			return fmt.Errorf("approval validation failed for ticket %s: %w", ticketID, err)
		}
	}
	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}

	vmID, vmName, err := g.atomicWriter.ApproveCreateAndEnqueue(
		ctx,
		ticketID,
		ticket.EventID,
		approver,
		clusterID,
		storageClass,
		payload.ServiceID,
		payload.Namespace,
		payload.RequesterID,
	)
	if err != nil {
		return fmt.Errorf("approve create ticket %s atomically: %w", ticketID, err)
	}

	// Audit log (best-effort, outside transaction).
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "approved", approver)
	}

	// Notification trigger: APPROVAL_COMPLETED → notify requester (master-flow.md Stage 5.F).
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.RequesterID, approver)
	}

	logger.Info("CREATE ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", vmID),
		zap.String("vm_name", vmName),
		zap.String("event_id", ticket.EventID),
	)

	return nil
}

// approveDelete handles approval of DELETE tickets.
// ADR-0012: decision write + domain state + River enqueue are one atomic commit.
func (g *Gateway) approveDelete(ctx context.Context, ticket *ent.ApprovalTicket, ticketID, approver string) error {
	// Parse the event payload to extract VM info for the delete job.
	event, err := g.client.DomainEvent.Get(ctx, ticket.EventID)
	if err != nil {
		return fmt.Errorf("get domain event %s: %w", ticket.EventID, err)
	}

	var payload struct {
		VMID      string `json:"vm_id"`
		VMName    string `json:"vm_name"`
		ClusterID string `json:"cluster_id"`
		Namespace string `json:"namespace"`
		Actor     string `json:"actor"`
	}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("parse delete event payload: %w", err)
	}

	if g.atomicWriter == nil {
		return fmt.Errorf("atomic approval writer is not configured")
	}
	if err := g.atomicWriter.ApproveDeleteAndEnqueue(ctx, ticketID, ticket.EventID, approver, payload.VMID); err != nil {
		return fmt.Errorf("approve delete ticket %s atomically: %w", ticketID, err)
	}

	// Audit log (best-effort).
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "delete_approved", approver)
	}

	// Notification trigger: APPROVAL_COMPLETED for delete → notify requester.
	if g.notifier != nil {
		g.notifier.OnTicketApproved(ctx, ticketID, payload.Actor, approver)
	}

	logger.Info("DELETE ticket approved and job enqueued",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("vm_id", payload.VMID),
		zap.String("vm_name", payload.VMName),
		zap.String("event_id", ticket.EventID),
	)

	return nil
}

// Reject rejects a pending ticket.
func (g *Gateway) Reject(ctx context.Context, ticketID, approver, reason string) error {
	ticket, err := g.client.ApprovalTicket.Get(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != approvalticket.StatusPENDING {
		return fmt.Errorf("ticket %s is not pending (current: %s)", ticketID, ticket.Status)
	}

	if _, err := g.client.ApprovalTicket.UpdateOneID(ticketID).
		SetStatus(approvalticket.StatusREJECTED).
		SetApprover(approver).
		SetRejectReason(reason).
		Save(ctx); err != nil {
		return fmt.Errorf("reject ticket %s: %w", ticketID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(ticket.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set domain event CANCELLED for rejected ticket %s: %w", ticketID, err)
	}

	// Audit log (master-flow.md Stage 5.B)
	if g.auditLogger != nil {
		_ = g.auditLogger.LogApproval(ctx, ticketID, "rejected", approver)
	}

	// Notification trigger: APPROVAL_REJECTED → notify requester (master-flow.md Stage 5.F).
	if g.notifier != nil {
		g.notifier.OnTicketRejected(ctx, ticketID, ticket.Requester, approver, reason)
	}

	logger.Info("Ticket rejected",
		zap.String("ticket_id", ticketID),
		zap.String("approver", approver),
		zap.String("reason", reason),
	)

	return nil
}

// Cancel allows a user to cancel their own pending request (ADR-0015 §10).
func (g *Gateway) Cancel(ctx context.Context, ticketID, requester string) error {
	ticket, err := g.client.ApprovalTicket.Get(ctx, ticketID)
	if err != nil {
		return fmt.Errorf("get ticket %s: %w", ticketID, err)
	}

	if ticket.Status != approvalticket.StatusPENDING {
		return fmt.Errorf("ticket %s is not pending", ticketID)
	}

	if ticket.Requester != requester {
		return fmt.Errorf("only requester can cancel ticket %s", ticketID)
	}

	if _, err := g.client.ApprovalTicket.UpdateOneID(ticketID).
		SetStatus(approvalticket.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("cancel ticket %s: %w", ticketID, err)
	}
	if _, err := g.client.DomainEvent.UpdateOneID(ticket.EventID).
		SetStatus(domainevent.StatusCANCELLED).
		Save(ctx); err != nil {
		return fmt.Errorf("set domain event CANCELLED for canceled ticket %s: %w", ticketID, err)
	}

	return nil
}

// ListPending returns pending tickets sorted by creation time (oldest first).
func (g *Gateway) ListPending(ctx context.Context) ([]*ent.ApprovalTicket, error) {
	return g.client.ApprovalTicket.Query().
		Where(approvalticket.StatusEQ(approvalticket.StatusPENDING)).
		Order(ent.Asc(approvalticket.FieldCreatedAt)).
		All(ctx)
}

type vmCreatePayload struct {
	ServiceID      string `json:"service_id"`
	Namespace      string `json:"namespace"`
	RequesterID    string `json:"requester_id"`
	InstanceSizeID string `json:"instance_size_id"`
}

func parseVMCreatePayload(raw json.RawMessage) (*vmCreatePayload, error) {
	var payload vmCreatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload.ServiceID == "" || payload.Namespace == "" || payload.RequesterID == "" || payload.InstanceSizeID == "" {
		return nil, fmt.Errorf("invalid create payload: missing required fields")
	}
	return &payload, nil
}

// PriorityTier calculates urgency tier based on pending duration (ADR-0015 §11).
func PriorityTier(createdAt time.Time) string {
	days := int(time.Since(createdAt).Hours() / 24)
	switch {
	case days >= 7:
		return "urgent" // Red
	case days >= 4:
		return "warning" // Yellow
	default:
		return "normal" // Default
	}
}
