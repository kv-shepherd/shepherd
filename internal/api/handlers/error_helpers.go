package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

// respondInternalUnlessCanceled centralizes the common handler pattern:
// request-canceled errors should quietly stop work, while true server errors
// should be logged and returned as a stable INTERNAL_ERROR contract.
func respondInternalUnlessCanceled(c *gin.Context, err error, msg string, fields ...zap.Field) bool {
	if err == nil {
		return false
	}
	if isRequestContextCanceled(err) {
		logger.Debug("request canceled",
			append(fields,
				zap.Error(err),
				zap.String("method", c.Request.Method),
				zap.String("path", c.FullPath()),
			)...,
		)
		return true
	}

	logger.Error(msg, append(fields, zap.Error(err))...)
	c.JSON(http.StatusInternalServerError, generated.Error{Code: "INTERNAL_ERROR"})
	return true
}
