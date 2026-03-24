package approvalcontract

import "context"

// ApprovalProvider defines the approval workflow interface.
// V1: built-in approval provider is implemented behind this seam so the core
// only depends on canonical approval submission/decision contracts.
type ApprovalProvider interface {
	// SubmitForApproval submits a request for approval.
	SubmitForApproval(ctx context.Context, req *ApprovalRequest) (*ApprovalResponse, error)

	// ProcessApproval processes an approval decision.
	ProcessApproval(ctx context.Context, ticketID string, decision ApprovalDecision) error

	// Type returns the provider type identifier.
	Type() string
}

// ApprovalRequest represents a canonical approval submission.
type ApprovalRequest struct {
	EventID   string                 `json:"event_id"`
	Requester string                 `json:"requester"`
	Action    string                 `json:"action"`
	Reason    string                 `json:"reason"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"` // Canonical work-order context for provider-owned workflow.
}

// ApprovalResponse represents an approval submission response.
type ApprovalResponse struct {
	TicketID string `json:"ticket_id"`
	Status   string `json:"status"`
}

// ApprovalDecision represents an approval decision.
type ApprovalDecision struct {
	Approved     bool                     `json:"approved"`
	Approver     string                   `json:"approver"`
	RejectReason string                   `json:"reject_reason,omitempty"`
	Execution    ApprovalExecutionOptions `json:"execution,omitempty"`
}

// ApprovalExecutionOptions are canonical core-owned fields needed to execute an
// approved work order after a provider reaches a final decision.
type ApprovalExecutionOptions struct {
	ClusterID     string   `json:"cluster_id,omitempty"`
	StorageClass  string   `json:"storage_class,omitempty"`
	DVAccessModes []string `json:"dv_access_modes,omitempty"`
	DVVolumeMode  string   `json:"dv_volume_mode,omitempty"`

	EnableOverride  bool    `json:"enable_override,omitempty"`
	CPURequest      float64 `json:"cpu_request,omitempty"`
	CPULimit        float64 `json:"cpu_limit,omitempty"`
	MemoryRequestGi float64 `json:"memory_request_gi,omitempty"`
	MemoryLimitGi   float64 `json:"memory_limit_gi,omitempty"`
	DiskGB          int     `json:"disk_gb,omitempty"`
}
