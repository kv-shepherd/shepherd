package errors_test

import (
	stderrors "errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
)

func TestAppError_ErrorAndUnwrap(t *testing.T) {
	root := stderrors.New("root cause")
	err := apperrors.Wrap(root, "VM_CREATE_FAILED", "create vm failed", http.StatusBadGateway)

	require.Equal(t, "VM_CREATE_FAILED: create vm failed: root cause", err.Error())
	require.ErrorIs(t, err, root)
}

func TestAppError_WithParams(t *testing.T) {
	err := apperrors.New("VM_NOT_FOUND", "vm missing", http.StatusNotFound).WithParams(map[string]interface{}{
		"vm_id": "vm-1",
		"scope": "service-a",
	})

	require.NotNil(t, err)
	require.Equal(t, "vm-1", err.Params["vm_id"])
	require.Equal(t, "service-a", err.Params["scope"])
}

func TestAppError_WithFieldErrors(t *testing.T) {
	err := apperrors.BadRequest("INVALID_REQUEST", "validation failed").WithFieldErrors([]apperrors.FieldError{
		{Field: "name", Code: "REQUIRED"},
		{Field: "namespace", Code: "INVALID_FORMAT", Message: "must be RFC-1035"},
	})

	require.NotNil(t, err)
	require.Len(t, err.FieldErrors, 2)
	require.Equal(t, "name", err.FieldErrors[0].Field)
	require.Equal(t, "REQUIRED", err.FieldErrors[0].Code)
	require.Equal(t, "must be RFC-1035", err.FieldErrors[1].Message)
}

func TestAppError_ConstructorsAndTypeCheck(t *testing.T) {
	notFound := apperrors.NotFound("SYS_NOT_FOUND", "system missing")
	require.Equal(t, http.StatusNotFound, notFound.HTTPStatus)

	conflict := apperrors.Conflict("DUPLICATE_NAME", "duplicate")
	require.Equal(t, http.StatusConflict, conflict.HTTPStatus)

	internal := apperrors.Internal("UNKNOWN", "unknown error")
	require.Equal(t, http.StatusInternalServerError, internal.HTTPStatus)

	got, ok := apperrors.IsAppError(conflict)
	require.True(t, ok)
	require.Equal(t, "DUPLICATE_NAME", got.Code)

	_, ok = apperrors.IsAppError(stderrors.New("plain"))
	require.False(t, ok)
}
