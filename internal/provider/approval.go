package provider

import approvalcontract "kv-shepherd.io/shepherd/internal/provider/approvalcontract"

// ApprovalProvider defines the approval workflow interface.
type ApprovalProvider = approvalcontract.ApprovalProvider

// ApprovalRequest represents a canonical approval submission.
type ApprovalRequest = approvalcontract.ApprovalRequest

// ApprovalResponse represents an approval submission response.
type ApprovalResponse = approvalcontract.ApprovalResponse

// ApprovalDecision represents an approval decision.
type ApprovalDecision = approvalcontract.ApprovalDecision

// ApprovalExecutionOptions are canonical core-owned fields needed to execute an
// approved work order after a provider reaches a final decision.
type ApprovalExecutionOptions = approvalcontract.ApprovalExecutionOptions
