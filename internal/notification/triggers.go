package notification

import (
	"context"
	"fmt"
	"slices"

	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// Triggers encapsulates notification trigger logic for approval and VM lifecycle events.
// master-flow.md Stage 5.F defines three trigger points:
//  1. APPROVAL_PENDING — notify approvers when a request is submitted
//  2. APPROVAL_COMPLETED / APPROVAL_REJECTED — notify requester on decision
//  3. VM_STATUS_CHANGE — notify resource owner on VM state transitions
//
// ADR-0015 §20: Notifications are synchronous writes within the same DB
// transaction as business operations.
type Triggers struct {
	sender Sender
	client *ent.Client
}

// NewTriggers creates a new notification trigger service.
func NewTriggers(sender Sender, client *ent.Client) *Triggers {
	return &Triggers{sender: sender, client: client}
}

// OnTicketSubmitted fires when a VM request is submitted and needs approval.
// Notifies all users who have the "approval:approve" permission.
//
// master-flow.md Stage 5.F / Event: VM Request Submitted:
//
//	INSERT INTO notifications ... SELECT user_id FROM role_bindings
//	WHERE role_id IN (SELECT id FROM roles WHERE permissions @> 'approval:approve')
func (t *Triggers) OnTicketSubmitted(ctx context.Context, ticketID, requesterName, namespace string) {
	approverIDs, err := t.findApproverUserIDs(ctx)
	if err != nil {
		logger.Error("failed to find approvers for notification",
			zap.String("ticket_id", ticketID),
			zap.Error(err),
		)
		return
	}

	if len(approverIDs) == 0 {
		logger.Warn("no approvers found for notification", zap.String("ticket_id", ticketID))
		return
	}

	params := Params{
		Type:         TypeApprovalPending,
		Title:        "New VM request pending approval",
		Message:      fmt.Sprintf("User %s submitted a VM request in namespace %s", requesterName, namespace),
		ResourceType: "approval_ticket",
		ResourceID:   ticketID,
	}

	if err := t.sender.SendToMany(ctx, approverIDs, params); err != nil {
		// master-flow.md: "Notification write must not be dropped silently;
		// failures must be observable."
		logger.Error("failed to send APPROVAL_PENDING notifications",
			zap.String("ticket_id", ticketID),
			zap.Int("approver_count", len(approverIDs)),
			zap.Error(err),
		)
	}
}

// OnTicketApproved fires when a ticket is approved.
// Notifies the requester that their request was approved.
//
// master-flow.md Stage 5.F / Event: Request Approved/Rejected:
//
//	INSERT INTO notifications (recipient_id, type, title, metadata)
//	VALUES (ticket.requested_by, 'APPROVAL_COMPLETED', ...)
func (t *Triggers) OnTicketApproved(ctx context.Context, ticketID, requesterID, approver string) {
	params := Params{
		RecipientID:  requesterID,
		Type:         TypeApprovalCompleted,
		Title:        "Your VM request has been approved",
		Message:      fmt.Sprintf("Your request (ticket %s) was approved by %s", ticketID, approver),
		ResourceType: "approval_ticket",
		ResourceID:   ticketID,
	}

	if err := t.sender.Send(ctx, params); err != nil {
		logger.Error("failed to send APPROVAL_COMPLETED notification",
			zap.String("ticket_id", ticketID),
			zap.String("requester", requesterID),
			zap.Error(err),
		)
	}
}

// OnTicketRejected fires when a ticket is rejected.
// Notifies the requester that their request was rejected.
func (t *Triggers) OnTicketRejected(ctx context.Context, ticketID, requesterID, approver, reason string) {
	msg := fmt.Sprintf("Your request (ticket %s) was rejected by %s", ticketID, approver)
	if reason != "" {
		msg += fmt.Sprintf(": %s", reason)
	}

	params := Params{
		RecipientID:  requesterID,
		Type:         TypeApprovalRejected,
		Title:        "Your VM request has been rejected",
		Message:      msg,
		ResourceType: "approval_ticket",
		ResourceID:   ticketID,
	}

	if err := t.sender.Send(ctx, params); err != nil {
		logger.Error("failed to send APPROVAL_REJECTED notification",
			zap.String("ticket_id", ticketID),
			zap.String("requester", requesterID),
			zap.Error(err),
		)
	}
}

// OnVMStatusChanged fires when a VM changes runtime state.
// Notifies the resource owner about the state transition.
//
// master-flow.md Stage 5.F / Event: VM State Changed:
//
//	INSERT INTO notifications (recipient_id, type, title, metadata)
//	VALUES (vm.owner_id, 'VM_STATUS_CHANGE', 'VM vm-name-01 is now Running', ...)
func (t *Triggers) OnVMStatusChanged(ctx context.Context, vmID, vmName, ownerID, newState string) {
	params := Params{
		RecipientID:  ownerID,
		Type:         TypeVMStatusChange,
		Title:        fmt.Sprintf("VM %s is now %s", vmName, newState),
		Message:      fmt.Sprintf("Virtual machine %s has transitioned to state: %s", vmName, newState),
		ResourceType: "vm",
		ResourceID:   vmID,
	}

	if err := t.sender.Send(ctx, params); err != nil {
		logger.Error("failed to send VM_STATUS_CHANGE notification",
			zap.String("vm_id", vmID),
			zap.String("owner", ownerID),
			zap.String("new_state", newState),
			zap.Error(err),
		)
	}
}

// findApproverUserIDs queries all user IDs that have the "approval:approve" permission.
// Ent JSON array fields don't generate DB-level Contains predicates, so we
// query all roles with their bindings+users and filter in Go.
//
// master-flow.md Stage 5.F:
//
//	FROM role_bindings WHERE role_id IN (SELECT id FROM roles WHERE permissions @> 'approval:approve')
func (t *Triggers) findApproverUserIDs(ctx context.Context) ([]string, error) {
	// Query all roles eager-loading bindings → users.
	roles, err := t.client.Role.Query().
		WithRoleBindings(func(q *ent.RoleBindingQuery) {
			q.WithUser()
		}).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("query roles with bindings: %w", err)
	}

	seen := make(map[string]struct{})
	var userIDs []string

	for _, r := range roles {
		// Filter: role must contain "approval:approve" permission.
		if !slices.Contains(r.Permissions, "approval:approve") {
			continue
		}
		for _, b := range r.Edges.RoleBindings {
			if b.Edges.User != nil {
				uid := b.Edges.User.ID
				if _, ok := seen[uid]; !ok {
					seen[uid] = struct{}{}
					userIDs = append(userIDs, uid)
				}
			}
		}
	}

	return userIDs, nil
}
