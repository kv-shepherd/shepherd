// Package notification implements the platform notification system.
//
// ADR-0015 §20: V1 notifications are synchronous DB writes within the same
// transaction as business operations — NOT via River Queue.
// V2+: External push channels (email, webhook) via River Queue.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/notification
package notification

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// Type constants matching ent/schema/notification.go enum values.
const (
	TypeApprovalPending   = "APPROVAL_PENDING"
	TypeApprovalCompleted = "APPROVAL_COMPLETED"
	TypeApprovalRejected  = "APPROVAL_REJECTED"
	TypeVMStatusChange    = "VM_STATUS_CHANGE"
)

// Params holds the required fields for creating a notification.
type Params struct {
	RecipientID  string // User ID of the recipient
	Type         string // One of Type* constants above
	Title        string // Human-readable title
	Message      string // Body text
	ResourceType string // e.g. "approval_ticket", "vm"
	ResourceID   string // ID of the related resource for navigation
}

// Sender defines the interface for sending notifications.
// V1: Only InboxSender implementation (synchronous DB write).
// V2+: Add EmailSender, WebhookSender via plugin pattern.
type Sender interface {
	// Send creates a notification for a single recipient.
	Send(ctx context.Context, params Params) error

	// SendToMany creates notifications for multiple recipients.
	// Best-effort: logs errors but does not abort on individual failures.
	SendToMany(ctx context.Context, recipientIDs []string, params Params) error
}

// InboxSender is the V1 implementation that writes notifications to the
// database synchronously within the caller's context.
//
// ADR-0015 §20: In-app inbox only, synchronous write.
// master-flow.md Stage 5.F: notification write must not be dropped silently.
type InboxSender struct {
	client *ent.Client
}

// NewInboxSender creates a new inbox sender.
func NewInboxSender(client *ent.Client) *InboxSender {
	return &InboxSender{client: client}
}

// Send stores a single notification to the database.
func (s *InboxSender) Send(ctx context.Context, params Params) error {
	if err := validateParams(params); err != nil {
		return fmt.Errorf("notification params invalid: %w", err)
	}

	notifType, err := toEntType(params.Type)
	if err != nil {
		return err
	}

	_, err = s.client.Notification.Create().
		SetID(uuid.NewString()).
		SetType(notifType).
		SetTitle(params.Title).
		SetMessage(params.Message).
		SetResourceType(params.ResourceType).
		SetResourceID(params.ResourceID).
		SetRead(false).
		SetUserID(params.RecipientID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create notification for user %s: %w", params.RecipientID, err)
	}

	logger.Debug("notification sent",
		zap.String("recipient", params.RecipientID),
		zap.String("type", params.Type),
		zap.String("title", params.Title),
	)

	return nil
}

// SendToMany creates notifications for multiple recipients (best-effort).
// Failures are logged but do not prevent delivery to other recipients.
func (s *InboxSender) SendToMany(ctx context.Context, recipientIDs []string, params Params) error {
	if len(recipientIDs) == 0 {
		return nil
	}

	var failCount int
	for _, recipientID := range recipientIDs {
		p := params
		p.RecipientID = recipientID
		if err := s.Send(ctx, p); err != nil {
			failCount++
			logger.Error("notification delivery failed",
				zap.String("recipient", recipientID),
				zap.String("type", params.Type),
				zap.Error(err),
			)
		}
	}

	if failCount > 0 {
		return fmt.Errorf("notification delivery failed for %d/%d recipients", failCount, len(recipientIDs))
	}
	return nil
}

// compile-time check
var _ Sender = (*InboxSender)(nil)

// --- Helpers ---

func validateParams(p Params) error {
	if p.RecipientID == "" {
		return fmt.Errorf("recipient_id is required")
	}
	if p.Title == "" {
		return fmt.Errorf("title is required")
	}
	if p.Message == "" {
		return fmt.Errorf("message is required")
	}
	return nil
}

func toEntType(t string) (entnotification.Type, error) {
	switch t {
	case TypeApprovalPending:
		return entnotification.TypeAPPROVAL_PENDING, nil
	case TypeApprovalCompleted:
		return entnotification.TypeAPPROVAL_COMPLETED, nil
	case TypeApprovalRejected:
		return entnotification.TypeAPPROVAL_REJECTED, nil
	case TypeVMStatusChange:
		return entnotification.TypeVM_STATUS_CHANGE, nil
	default:
		return "", fmt.Errorf("unknown notification type: %s", t)
	}
}
