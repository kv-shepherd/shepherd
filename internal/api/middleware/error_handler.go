// Package middleware provides HTTP middleware for KubeVirt Shepherd.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/api/middleware
package middleware

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	apperrors "kv-shepherd.io/shepherd/internal/pkg/errors"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// ErrorHandler is a Gin middleware that provides centralized error handling.
// It captures errors added via c.Error() and returns a consistent JSON response.
// Gin best practice: separate error handling from route handlers.
func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if len(c.Errors) == 0 {
			return
		}

		err := c.Errors.Last().Err

		// Check if it's an AppError with structured info
		var appErr *apperrors.AppError
		if errors.As(err, &appErr) {
			logger.Warn("Request error",
				zap.String("code", appErr.Code),
				zap.String("message", appErr.Message),
				zap.Int("status", appErr.HTTPStatus),
				zap.Error(appErr.Err),
			)
			c.JSON(appErr.HTTPStatus, gin.H{
				"code":    appErr.Code,
				"message": appErr.Message,
			})
			return
		}

		// Fallback: generic 500 error
		logger.Error("Unhandled request error", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "INTERNAL_ERROR",
			"message": "An internal error occurred",
		})
	}
}
