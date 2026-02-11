package middleware

import (
	"context"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	rrb "kv-shepherd.io/shepherd/ent/resourcerolebinding"
)

// RequirePermission returns middleware that checks if the authenticated user
// has a specific global permission (from their platform role).
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		perms, exists := c.Get("permissions")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "FORBIDDEN", "message": "no permissions in context",
			})
			return
		}
		permList, ok := perms.([]string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "FORBIDDEN", "message": "invalid permissions type",
			})
			return
		}

		// platform:admin is the explicit super-admin permission (ADR-0019).
		if slices.Contains(permList, "platform:admin") {
			c.Next()
			return
		}

		if slices.Contains(permList, permission) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code": "FORBIDDEN", "message": "insufficient permissions",
		})
	}
}

// ResourceRole represents a user's role on a specific resource.
type ResourceRole string

const (
	ResourceRoleOwner  ResourceRole = "owner"
	ResourceRoleAdmin  ResourceRole = "admin"
	ResourceRoleMember ResourceRole = "member"
	ResourceRoleViewer ResourceRole = "viewer"
)

// ResourceRoleChecker provides hierarchical resource permission checking.
// Implements the permission check algorithm from master-flow.md Stage 4.A+:
//
//	func checkPermission(user, resource) Role:
//	    current = resource
//	    while current != nil:
//	        binding = findBinding(user, current)
//	        if binding != nil:
//	            return binding.role
//	        current = current.parent  // VM→Service→System→nil
//	    return nil  // no permission, resource invisible
type ResourceRoleChecker struct {
	client *ent.Client
}

// NewResourceRoleChecker creates a new checker.
func NewResourceRoleChecker(client *ent.Client) *ResourceRoleChecker {
	return &ResourceRoleChecker{client: client}
}

// CheckResourceRole walks the resource hierarchy to find the user's role.
// Returns the role and whether any binding was found.
func (c *ResourceRoleChecker) CheckResourceRole(ctx context.Context, userID string, resourceType string, resourceID string) (ResourceRole, bool, error) {
	// 1. Check direct binding on this resource.
	binding, err := c.findBinding(ctx, userID, resourceType, resourceID)
	if err != nil {
		return "", false, err
	}
	if binding != nil {
		return ResourceRole(binding.Role.String()), true, nil
	}

	// 2. Walk up the hierarchy.
	switch resourceType {
	case "vm":
		// VM → Service (via service edge)
		vmEnt, err := c.client.VM.Get(ctx, resourceID)
		if err != nil {
			return "", false, nil
		}
		svc, err := vmEnt.QueryService().Only(ctx)
		if err != nil {
			return "", false, nil
		}
		return c.CheckResourceRole(ctx, userID, "service", svc.ID)

	case "service":
		// Service → System (via system edge)
		svcEnt, err := c.client.Service.Get(ctx, resourceID)
		if err != nil {
			return "", false, nil
		}
		sys, err := svcEnt.QuerySystem().Only(ctx)
		if err != nil {
			return "", false, nil
		}
		return c.CheckResourceRole(ctx, userID, "system", sys.ID)

	case "system":
		// System is the top level, no parent.
		return "", false, nil
	}

	return "", false, nil
}

// findBinding queries for a direct ResourceRoleBinding for the user on the resource.
func (c *ResourceRoleChecker) findBinding(ctx context.Context, userID, resourceType, resourceID string) (*ent.ResourceRoleBinding, error) {
	binding, err := c.client.ResourceRoleBinding.Query().
		Where(
			rrb.UserIDEQ(userID),
			rrb.ResourceTypeEQ(resourceType),
			rrb.ResourceIDEQ(resourceID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return binding, nil
}

// RoleCanPerform checks if a resource role can perform the given action.
// Permission matrix from master-flow.md Stage 4.A+.
func RoleCanPerform(role ResourceRole, action string) bool {
	switch role {
	case ResourceRoleOwner:
		return true
	case ResourceRoleAdmin:
		return action != "transfer_ownership"
	case ResourceRoleMember:
		return action == "view" || action == "create"
	case ResourceRoleViewer:
		return action == "view"
	default:
		return false
	}
}

// RequireResourceAccess returns middleware that checks resource-level permissions.
// It first checks global permissions, then falls back to resource role hierarchy.
func RequireResourceAccess(checker *ResourceRoleChecker, resourceType string, action string, paramName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Global permission check: platform:admin allows everything.
		perms, _ := c.Get("permissions")
		if permList, ok := perms.([]string); ok && slices.Contains(permList, "platform:admin") {
			c.Next()
			return
		}

		userID := GetUserID(c.Request.Context())
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "FORBIDDEN", "message": "not authenticated",
			})
			return
		}

		resourceID := c.Param(paramName)
		if resourceID == "" {
			c.Next()
			return
		}

		// 2. Resource-level permission check (walk inheritance chain).
		role, found, err := checker.CheckResourceRole(c.Request.Context(), userID, resourceType, resourceID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code": "INTERNAL_ERROR", "message": "permission check failed",
			})
			return
		}

		if !found || !RoleCanPerform(role, action) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "FORBIDDEN", "message": "insufficient resource permissions",
			})
			return
		}

		c.Next()
	}
}
