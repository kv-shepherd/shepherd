package provider

import "context"

// AuthProvider defines the authentication provider interface.
// Phase 1: JWT implementation. Future: OIDC, LDAP adapters.
type AuthProvider interface {
	// Authenticate validates credentials and returns a user identity.
	Authenticate(ctx context.Context, credentials interface{}) (*AuthResult, error)

	// ValidateToken validates a token and returns claims.
	ValidateToken(ctx context.Context, token string) (*TokenClaims, error)

	// Type returns the provider type identifier.
	Type() string
}

// AuthResult represents an authentication result.
type AuthResult struct {
	UserID      string            `json:"user_id"`
	Username    string            `json:"username"`
	Email       string            `json:"email,omitempty"`
	DisplayName string            `json:"display_name,omitempty"`
	Groups      []string          `json:"groups,omitempty"`
	ProviderID  string            `json:"provider_id,omitempty"`
	ExternalID  string            `json:"external_id,omitempty"`
	RawClaims   map[string]interface{} `json:"raw_claims,omitempty"`
}

// TokenClaims represents JWT token claims.
type TokenClaims struct {
	UserID   string   `json:"user_id"`
	Username string   `json:"username"`
	Roles    []string `json:"roles,omitempty"`
}

// ApprovalProvider defines the approval workflow interface.
// Phase 1: Internal (built-in). V2+: External adapters (RFC-0004).
type ApprovalProvider interface {
	// SubmitForApproval submits a request for approval.
	SubmitForApproval(ctx context.Context, req *ApprovalRequest) (*ApprovalResponse, error)

	// ProcessApproval processes an approval decision.
	ProcessApproval(ctx context.Context, ticketID string, decision ApprovalDecision) error

	// Type returns the provider type identifier.
	Type() string
}

// ApprovalRequest represents an approval request.
type ApprovalRequest struct {
	EventID   string `json:"event_id"`
	Requester string `json:"requester"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}

// ApprovalResponse represents an approval submission response.
type ApprovalResponse struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"`
}

// ApprovalDecision represents an approval decision.
type ApprovalDecision struct {
	Approved     bool   `json:"approved"`
	Approver     string `json:"approver"`
	RejectReason string `json:"reject_reason,omitempty"`
}

// NotificationProvider defines the notification interface.
// Phase 1: Log (noop). Future: Email, Webhook, etc.
type NotificationProvider interface {
	// Send sends a notification.
	Send(ctx context.Context, notification *Notification) error

	// Type returns the provider type identifier.
	Type() string
}

// Notification represents a notification message.
type Notification struct {
	RecipientID string                 `json:"recipient_id"`
	Type        string                 `json:"type"` // e.g. "approval_required", "vm_created"
	Title       string                 `json:"title"`
	Body        string                 `json:"body"`
	Data        map[string]interface{} `json:"data,omitempty"`
}
