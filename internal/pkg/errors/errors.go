// Package errors provides domain-specific error types for KubeVirt Shepherd.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/pkg/errors
package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Sentinel errors for common failure scenarios.
var (
	ErrNotFound       = errors.New("not found")
	ErrAlreadyExists  = errors.New("already exists")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrBadRequest     = errors.New("bad request")
	ErrInternal       = errors.New("internal error")
	ErrConflict       = errors.New("conflict")
	ErrServiceUnavail = errors.New("service unavailable")
)

// AppError is a structured application error with HTTP status and error code.
type AppError struct {
	// Code is a machine-readable error code (e.g., "VM_NOT_FOUND").
	Code string `json:"code"`

	// Message is a human-readable error message.
	Message string `json:"message"`

	// HTTPStatus is the corresponding HTTP status code.
	HTTPStatus int `json:"-"`

	// Params carries structured context for frontend/i18n interpolation.
	Params map[string]interface{} `json:"params,omitempty"`

	// FieldErrors carries field-level validation details for form binding.
	FieldErrors []FieldError `json:"field_errors,omitempty"`

	// Err is the wrapped underlying error.
	Err error `json:"-"`
}

// FieldError describes a field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError.
func New(code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Wrap wraps an existing error into an AppError.
func Wrap(err error, code, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        err,
	}
}

// WithParams attaches structured parameters to the error.
func (e *AppError) WithParams(params map[string]interface{}) *AppError {
	if e == nil || len(params) == 0 {
		return e
	}
	e.Params = params
	return e
}

// WithFieldErrors attaches field-level errors to the AppError.
func (e *AppError) WithFieldErrors(fieldErrors []FieldError) *AppError {
	if e == nil || len(fieldErrors) == 0 {
		return e
	}
	e.FieldErrors = fieldErrors
	return e
}

// Common error constructors.

// NotFound creates a 404 error.
func NotFound(code, message string) *AppError {
	return New(code, message, http.StatusNotFound)
}

// BadRequest creates a 400 error.
func BadRequest(code, message string) *AppError {
	return New(code, message, http.StatusBadRequest)
}

// Unauthorized creates a 401 error.
func Unauthorized(code, message string) *AppError {
	return New(code, message, http.StatusUnauthorized)
}

// Forbidden creates a 403 error.
func Forbidden(code, message string) *AppError {
	return New(code, message, http.StatusForbidden)
}

// Conflict creates a 409 error.
func Conflict(code, message string) *AppError {
	return New(code, message, http.StatusConflict)
}

// Internal creates a 500 error.
func Internal(code, message string) *AppError {
	return New(code, message, http.StatusInternalServerError)
}

// IsAppError checks if an error is an AppError and returns it.
func IsAppError(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
