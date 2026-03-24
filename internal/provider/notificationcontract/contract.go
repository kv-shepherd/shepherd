package notificationcontract

import "context"

// NotificationProvider defines the notification interface.
// Phase 1: Log (noop). Future: Email, Webhook, etc.
type NotificationProvider interface {
	Send(ctx context.Context, notification *Notification) error
	Type() string
}

// Notification represents a notification message.
type Notification struct {
	RecipientID string                 `json:"recipient_id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Data        map[string]interface{} `json:"data,omitempty"`
}
