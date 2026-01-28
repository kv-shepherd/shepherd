// Package domain provides example domain entities for KubeVirt Shepherd.
//
// This file defines resource-level RBAC entities per ADR-0018 §Stage 2.A+, §Stage 4.A+.
// Dual-layer permission model: Global RBAC + Resource-level RBAC.
//
// Reference: docs/adr/ADR-0018-instance-size-abstraction.md

package domain

import "time"

// ResourceRoleBinding represents a resource-level permission grant.
// This supplements the global RBAC (RoleBinding) with fine-grained resource permissions.
//
// Example Use Cases:
// - User A can only manage VMs in System "shop"
// - User B can only view (not modify) Service "redis"
// - Team lead grants VM access to team members
//
// Permission Inheritance:
// - System permission → inherits to all Services and VMs under it
// - Service permission → inherits to all VMs under it
type ResourceRoleBinding struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`       // Target user
	Role         string     `json:"role"`          // viewer, editor, admin
	ResourceType string     `json:"resource_type"` // system, service, vm, namespace
	ResourceID   string     `json:"resource_id"`   // The specific resource ID
	GrantedBy    string     `json:"granted_by"`    // Who granted this permission
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"` // Optional expiration
}

// ResourceRole defines the available roles for resource-level RBAC.
type ResourceRole string

const (
	ResourceRoleViewer ResourceRole = "viewer" // Read-only access
	ResourceRoleEditor ResourceRole = "editor" // Read + Modify (no delete)
	ResourceRoleAdmin  ResourceRole = "admin"  // Full access including grant permissions
)

// ResourceType defines the resource types that support resource-level RBAC.
type ResourceType string

const (
	ResourceTypeSystem       ResourceType = "system"
	ResourceTypeService      ResourceType = "service"
	ResourceTypeVM           ResourceType = "vm"
	ResourceTypeNamespace    ResourceType = "namespace"
	ResourceTypeTemplate     ResourceType = "template"
	ResourceTypeInstanceSize ResourceType = "instance_size"
)

// Permission represents a permission check result.
type Permission struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"` // Why allowed/denied
	Source  string `json:"source,omitempty"` // global_rbac, resource_rbac, inheritance
}

// PermissionChecker interface for checking permissions.
// Implementation should check both global RBAC and resource-level RBAC.
type PermissionChecker interface {
	// CheckPermission checks if user has specified permission on resource.
	// Returns Permission with allowed=true if:
	// 1. Global RBAC grants the permission, OR
	// 2. Resource-level RBAC grants the permission (direct or inherited)
	CheckPermission(userID, action, resourceType, resourceID string) (*Permission, error)

	// CanGrant checks if user can grant the specified role to another user.
	// Only users with "admin" role on the resource can grant permissions.
	CanGrant(granterID, resourceType, resourceID, role string) (bool, error)
}
