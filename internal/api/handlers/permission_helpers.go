package handlers

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
)

// requireGlobalPermission enforces explicit global RBAC permission checks.
// Fail-closed policy:
// - unauthenticated => 401
// - missing/invalid permissions context => 403
// - missing required permission => 403
func requireGlobalPermission(c *gin.Context, permission string) bool {
	return requireAnyGlobalPermission(c, permission)
}

// requireAnyGlobalPermission allows one of the provided permissions.
func requireAnyGlobalPermission(c *gin.Context, permissions ...string) bool {
	actor := middleware.GetUserID(c.Request.Context())
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return false
	}
	if len(permissions) == 0 {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return false
	}

	permsRaw, exists := c.Get("permissions")
	if !exists {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return false
	}
	permList, ok := permsRaw.([]string)
	if !ok {
		c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
		return false
	}

	if slices.Contains(permList, "platform:admin") {
		return true
	}

	for _, permission := range permissions {
		if slices.Contains(permList, permission) {
			return true
		}
	}

	c.JSON(http.StatusForbidden, generated.Error{Code: "FORBIDDEN"})
	return false
}

// requireActorWithAnyGlobalPermission returns request context and actor ID after permission check.
func requireActorWithAnyGlobalPermission(c *gin.Context, permissions ...string) (context.Context, string, bool) {
	if !requireAnyGlobalPermission(c, permissions...) {
		return nil, "", false
	}
	ctx := c.Request.Context()
	actor := middleware.GetUserID(ctx)
	if strings.TrimSpace(actor) == "" {
		c.JSON(http.StatusUnauthorized, generated.Error{Code: "UNAUTHORIZED"})
		return nil, "", false
	}
	return ctx, actor, true
}
