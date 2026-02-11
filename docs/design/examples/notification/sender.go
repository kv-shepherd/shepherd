//go:build ignore

// Package notification provides the notification system implementation.
//
// Reference: ADR-0015 §20, 04-governance.md §6.3
// Purpose: Notification data model and decoupled sender interface
// V1 Strategy: Platform-internal inbox only, no external push channels

package notification

import (
	"context"
	"errors"
	"time"
)

// NotificationType defines the types of notifications
type NotificationType string

const (
	TypeApprovalPending   NotificationType = "APPROVAL_PENDING"
	TypeApprovalCompleted NotificationType = "APPROVAL_COMPLETED"
	TypeApprovalRejected  NotificationType = "APPROVAL_REJECTED"
	TypeVMStatusChange    NotificationType = "VM_STATUS_CHANGE"
	TypeSystemAlert       NotificationType = "SYSTEM_ALERT"
)

// Notification represents a platform notification
type Notification struct {
	ID          string                 `json:"id"`
	RecipientID string                 `json:"recipient_id"`
	Type        NotificationType       `json:"type"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Read        bool                   `json:"read"`
	CreatedAt   time.Time              `json:"created_at"`
	ReadAt      *time.Time             `json:"read_at,omitempty"`
}

// NotificationSender defines the interface for sending notifications
// V1: Only InboxSender implementation
// V2+: Add EmailSender, WebhookSender, SlackSender via plugin
type NotificationSender interface {
	Send(ctx context.Context, notification *Notification) error
}

// NotificationRepository defines the data access interface for notifications
type NotificationRepository interface {
	Create(ctx context.Context, notification *Notification) error
	GetByRecipient(ctx context.Context, recipientID string, page, perPage int) ([]*Notification, error)
	GetUnreadCount(ctx context.Context, recipientID string) (int, error)
	MarkAsRead(ctx context.Context, id string) error
	MarkAllAsRead(ctx context.Context, recipientID string) error
}

// InboxSender is the V1 implementation that stores notifications to database
type InboxSender struct {
	repo NotificationRepository
}

// NewInboxSender creates a new inbox sender
func NewInboxSender(repo NotificationRepository) *InboxSender {
	return &InboxSender{repo: repo}
}

// Send stores the notification to the database
func (s *InboxSender) Send(ctx context.Context, n *Notification) error {
	return s.repo.Create(ctx, n)
}

// compile-time check
var _ NotificationSender = (*InboxSender)(nil)

// NotificationService orchestrates all notification senders
type NotificationService struct {
	senders []NotificationSender // V1: only InboxSender
	repo    NotificationRepository
}

// NewNotificationService creates a new notification service
func NewNotificationService(repo NotificationRepository, senders ...NotificationSender) *NotificationService {
	return &NotificationService{
		repo:    repo,
		senders: senders,
	}
}

// Notify sends a notification through all configured senders
func (s *NotificationService) Notify(ctx context.Context, n *Notification) error {
	var errs []error
	for _, sender := range s.senders {
		if err := sender.Send(ctx, n); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// GetUserNotifications returns notifications for a user (paginated)
func (s *NotificationService) GetUserNotifications(ctx context.Context, userID string, page, perPage int) ([]*Notification, error) {
	return s.repo.GetByRecipient(ctx, userID, page, perPage)
}

// GetUnreadCount returns the number of unread notifications for a user
func (s *NotificationService) GetUnreadCount(ctx context.Context, userID string) (int, error) {
	return s.repo.GetUnreadCount(ctx, userID)
}

// MarkAsRead marks a single notification as read
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID string) error {
	return s.repo.MarkAsRead(ctx, notificationID)
}

// MarkAllAsRead marks all notifications as read for a user
func (s *NotificationService) MarkAllAsRead(ctx context.Context, userID string) error {
	return s.repo.MarkAllAsRead(ctx, userID)
}
