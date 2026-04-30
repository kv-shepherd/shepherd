package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	apivalidator "kv-shepherd.io/shepherd/internal/api/validator"
	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

func bindAndValidateJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, generated.Error{
				Code:    "REQUEST_TOO_LARGE",
				Message: "request body is too large",
			})
			return false
		}
		c.JSON(http.StatusBadRequest, generated.Error{Code: "INVALID_REQUEST"})
		return false
	}

	if shouldSkipStructValidation(req) {
		return true
	}

	if err := apivalidator.ValidateStruct(req); err != nil {
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			if appErr.HTTPStatus >= http.StatusInternalServerError {
				logger.Error("request validation failed due to invalid validator input", zap.Error(err))
			}
			c.JSON(appErr.HTTPStatus, toGeneratedError(appErr))
			return false
		}

		logger.Error("unexpected request validation failure", zap.Error(err))
		c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
		return false
	}

	return true
}

func shouldSkipStructValidation(req any) bool {
	switch req.(type) {
	case *generated.VMBatchSubmitRequest,
		*generated.VMBatchPowerRequest,
		*generated.VMPowerRequest,
		*generated.SystemMemberCreateRequest,
		*generated.SystemMemberRoleUpdateRequest:
		// These handlers return dedicated business error codes for semantic checks
		// (for example INVALID_BATCH_SIZE / INVALID_BATCH_OPERATION / INVALID_ROLE),
		// so we keep handler-level validation as the source of truth for now.
		return true
	default:
		return false
	}
}

func toGeneratedError(appErr *apperrors.AppError) generated.Error {
	if appErr == nil {
		return generated.Error{Code: "INTERNAL_ERROR"}
	}

	fieldErrors := make([]generated.FieldError, 0, len(appErr.FieldErrors))
	for _, fe := range appErr.FieldErrors {
		fieldErrors = append(fieldErrors, generated.FieldError{
			Field:   fe.Field,
			Code:    fe.Code,
			Message: fe.Message,
		})
	}

	return generated.Error{
		Code:        appErr.Code,
		Message:     appErr.Message,
		Params:      appErr.Params,
		FieldErrors: fieldErrors,
	}
}
