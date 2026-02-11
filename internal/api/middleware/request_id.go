package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type contextKey string

const (
	// RequestIDHeader is the HTTP header for request tracing.
	RequestIDHeader = "X-Request-ID"

	ctxKeyRequestID contextKey = "request_id"
	ctxKeyUserID    contextKey = "user_id"
	ctxKeyUsername  contextKey = "username"
	ctxKeyRoles     contextKey = "roles"
)

// RequestID injects a unique request ID into the context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			id, _ := uuid.NewV7()
			rid = id.String()
		}
		c.Set(string(ctxKeyRequestID), rid)
		c.Writer.Header().Set(RequestIDHeader, rid)
		c.Request = c.Request.WithContext(
			context.WithValue(c.Request.Context(), ctxKeyRequestID, rid),
		)
		c.Next()
	}
}

// GetRequestID extracts request ID from context.
func GetRequestID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// SetUserContext stores authenticated user info in context.
func SetUserContext(ctx context.Context, userID, username string, roles []string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyUserID, userID)
	ctx = context.WithValue(ctx, ctxKeyUsername, username)
	ctx = context.WithValue(ctx, ctxKeyRoles, roles)
	return ctx
}

// GetUserID extracts user ID from context.
func GetUserID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUserID).(string); ok {
		return v
	}
	return ""
}

// GetUsername extracts username from context.
func GetUsername(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyUsername).(string); ok {
		return v
	}
	return ""
}

// GetRoles extracts user roles from context.
func GetRoles(ctx context.Context) []string {
	if v, ok := ctx.Value(ctxKeyRoles).([]string); ok {
		return v
	}
	return nil
}
