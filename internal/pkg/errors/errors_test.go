package errors

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestAppError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *AppError
		want string
	}{
		{
			name: "without wrapped error",
			err:  New("VM_NOT_FOUND", "VM not found", http.StatusNotFound),
			want: "VM_NOT_FOUND: VM not found",
		},
		{
			name: "with wrapped error",
			err:  Wrap(fmt.Errorf("db error"), "DB_ERROR", "database failure", http.StatusInternalServerError),
			want: "DB_ERROR: database failure: db error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	appErr := Wrap(inner, "CODE", "msg", 500)

	if !errors.Is(appErr, inner) {
		t.Error("errors.Is should match inner error")
	}
}

func TestIsAppError(t *testing.T) {
	appErr := NotFound("NOT_FOUND", "resource not found")
	wrapped := fmt.Errorf("wrapped: %w", appErr)

	got, ok := IsAppError(wrapped)
	if !ok {
		t.Fatal("IsAppError should return true for wrapped AppError")
	}
	if got.Code != "NOT_FOUND" {
		t.Errorf("Code = %q, want NOT_FOUND", got.Code)
	}
}

func TestErrorConstructors(t *testing.T) {
	tests := []struct {
		name       string
		err        *AppError
		wantStatus int
	}{
		{"NotFound", NotFound("NF", "not found"), http.StatusNotFound},
		{"BadRequest", BadRequest("BR", "bad request"), http.StatusBadRequest},
		{"Unauthorized", Unauthorized("UA", "unauthorized"), http.StatusUnauthorized},
		{"Forbidden", Forbidden("FB", "forbidden"), http.StatusForbidden},
		{"Conflict", Conflict("CF", "conflict"), http.StatusConflict},
		{"Internal", Internal("IE", "internal"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.HTTPStatus != tt.wantStatus {
				t.Errorf("HTTPStatus = %d, want %d", tt.err.HTTPStatus, tt.wantStatus)
			}
		})
	}
}
