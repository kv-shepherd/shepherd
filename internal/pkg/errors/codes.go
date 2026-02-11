package errors

import "net/http"

// Error code constants (ADR-0023).
// Errors contain code + params only, no hardcoded messages.
// Frontend handles i18n translation. Backend logs always in English.

// VM error codes.
const (
	CodeVMNotFound   = "VM_NOT_FOUND"
	CodeVMCreateFail = "VM_CREATION_FAILED"
	CodeVMDeleteFail = "VM_DELETION_FAILED"
	CodeVMModifyFail = "VM_MODIFY_FAILED"
)

// System/Service error codes.
const (
	CodeSystemNotFound  = "SYSTEM_NOT_FOUND"
	CodeServiceNotFound = "SERVICE_NOT_FOUND"
	CodeSystemExists    = "SYSTEM_ALREADY_EXISTS"
	CodeServiceExists   = "SERVICE_ALREADY_EXISTS"
)

// Cluster error codes.
const (
	CodeClusterUnhealthy = "CLUSTER_UNHEALTHY"
	CodeClusterNotFound  = "CLUSTER_NOT_FOUND"
)

// Approval error codes.
const (
	CodeApprovalRequired = "APPROVAL_REQUIRED"
	CodeApprovalNotFound = "APPROVAL_NOT_FOUND"
	CodeDuplicateRequest = "DUPLICATE_PENDING_REQUEST"
)

// Namespace error codes (ADR-0023).
const (
	CodeNamespacePermDenied    = "NAMESPACE_PERMISSION_DENIED"
	CodeNamespaceQuotaExceeded = "NAMESPACE_QUOTA_EXCEEDED"
	CodeNamespaceCreateFailed  = "NAMESPACE_CREATION_FAILED"
)

// Quota error codes.
const (
	CodeQuotaExceeded = "QUOTA_EXCEEDED" // V2+ reserved
)

// Auth error codes.
const (
	CodeAuthFailed        = "AUTH_FAILED"
	CodeTokenExpired      = "TOKEN_EXPIRED"
	CodeTokenInvalid      = "TOKEN_INVALID"
	CodePasswordChangeReq = "PASSWORD_CHANGE_REQUIRED"
)

// Validation error codes.
const (
	CodeInvalidRequestField = "INVALID_REQUEST_FIELD"
	CodeValidationFailed    = "VALIDATION_FAILED"
	CodeNameInvalid         = "NAME_INVALID"
)

// Convenience constructors using predefined codes.

// ErrVMNotFound creates a VM not found error.
func ErrVMNotFoundf(vmID string) *AppError {
	return &AppError{
		Code:       CodeVMNotFound,
		Message:    "virtual machine not found",
		HTTPStatus: http.StatusNotFound,
	}
}

// ErrClusterUnhealthy creates a cluster unhealthy error.
func ErrClusterUnhealthyf(clusterID string) *AppError {
	return &AppError{
		Code:       CodeClusterUnhealthy,
		Message:    "target cluster is unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
	}
}

// ErrApprovalRequired creates an approval required error (202 Accepted).
func ErrApprovalRequiredf(ticketID string) *AppError {
	return &AppError{
		Code:       CodeApprovalRequired,
		Message:    "request pending approval",
		HTTPStatus: http.StatusAccepted,
	}
}

// ErrInvalidRequestField creates a bad request error for forbidden fields (ADR-0017).
func ErrInvalidRequestFieldf(fieldName string) *AppError {
	return &AppError{
		Code:       CodeInvalidRequestField,
		Message:    "request contains forbidden field: " + fieldName,
		HTTPStatus: http.StatusBadRequest,
	}
}
