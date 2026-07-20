package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestRoleCanPerform_Stage4Matrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		role   ResourceRole
		action string
		want   bool
	}{
		{"owner can transfer ownership", ResourceRoleOwner, "transfer_ownership", true},
		{"owner can manage members", ResourceRoleOwner, "manage_members", true},
		{"admin cannot transfer ownership", ResourceRoleAdmin, "transfer_ownership", false},
		{"admin can manage members", ResourceRoleAdmin, "manage_members", true},
		{"member can view", ResourceRoleMember, "view", true},
		{"member can create", ResourceRoleMember, "create", true},
		{"member cannot manage members", ResourceRoleMember, "manage_members", false},
		{"viewer can view", ResourceRoleViewer, "view", true},
		{"viewer cannot create", ResourceRoleViewer, "create", false},
		{"unknown role denied", ResourceRole("unknown"), "view", false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := RoleCanPerform(tc.role, tc.action); got != tc.want {
				t.Fatalf("RoleCanPerform(%q,%q) = %v, want %v", tc.role, tc.action, got, tc.want)
			}
		})
	}
}

func TestRequirePermission(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)

	run := func(perms interface{}, required string) (int, bool) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		if perms != nil {
			c.Set("permissions", perms)
		}

		called := false
		RequirePermission(required)(c)
		if !c.IsAborted() {
			called = true
		}
		return w.Code, called
	}

	t.Run("platform admin bypasses required permission", func(t *testing.T) {
		t.Parallel()
		status, called := run([]string{"platform:admin"}, "system:delete")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
		if !called {
			t.Fatal("middleware unexpectedly aborted for platform:admin")
		}
	})

	t.Run("specific permission allowed", func(t *testing.T) {
		t.Parallel()
		status, called := run([]string{"system:read"}, "system:read")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want %d", status, http.StatusOK)
		}
		if !called {
			t.Fatal("middleware unexpectedly aborted with matching permission")
		}
	})

	t.Run("missing permission forbidden", func(t *testing.T) {
		t.Parallel()
		status, called := run([]string{"system:read"}, "system:delete")
		if status != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", status, http.StatusForbidden)
		}
		if called {
			t.Fatal("middleware should abort when permission missing")
		}
	})
}

func TestResourceRoleChecker_WalksNearestResourceBinding(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "resource_role_checker_hierarchy")
	systemID, serviceID, vmID := createResourceRoleHierarchy(t, client)

	createResourceRoleBinding(t, client, "direct-vm", "vm", vmID, resourcerolebinding.RoleAdmin)
	createResourceRoleBinding(t, client, "service-member", "service", serviceID, resourcerolebinding.RoleMember)
	createResourceRoleBinding(t, client, "system-viewer", "system", systemID, resourcerolebinding.RoleViewer)
	createResourceRoleBinding(t, client, "nearest-wins", "system", systemID, resourcerolebinding.RoleOwner)
	createResourceRoleBinding(t, client, "nearest-wins", "service", serviceID, resourcerolebinding.RoleViewer)

	checker := NewResourceRoleChecker(client)
	tests := []struct {
		name         string
		userID       string
		resourceType string
		resourceID   string
		wantRole     ResourceRole
		wantFound    bool
	}{
		{
			name:         "returns direct VM binding",
			userID:       "direct-vm",
			resourceType: "vm",
			resourceID:   vmID,
			wantRole:     ResourceRoleAdmin,
			wantFound:    true,
		},
		{
			name:         "inherits service binding from VM",
			userID:       "service-member",
			resourceType: "vm",
			resourceID:   vmID,
			wantRole:     ResourceRoleMember,
			wantFound:    true,
		},
		{
			name:         "inherits system binding through service",
			userID:       "system-viewer",
			resourceType: "vm",
			resourceID:   vmID,
			wantRole:     ResourceRoleViewer,
			wantFound:    true,
		},
		{
			name:         "nearest binding overrides stronger ancestor binding",
			userID:       "nearest-wins",
			resourceType: "vm",
			resourceID:   vmID,
			wantRole:     ResourceRoleViewer,
			wantFound:    true,
		},
		{
			name:         "returns no role for an existing unbound hierarchy",
			userID:       "unbound-user",
			resourceType: "vm",
			resourceID:   vmID,
			wantFound:    false,
		},
		{
			name:         "does not confuse a missing resource with authorization",
			userID:       "system-viewer",
			resourceType: "vm",
			resourceID:   "missing-vm",
			wantFound:    false,
		},
		{
			name:         "unknown resource type without a direct binding is denied",
			userID:       "system-viewer",
			resourceType: "unknown",
			resourceID:   vmID,
			wantFound:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			role, found, err := checker.CheckResourceRole(
				t.Context(),
				tc.userID,
				tc.resourceType,
				tc.resourceID,
			)
			if err != nil {
				t.Fatalf("CheckResourceRole() error = %v", err)
			}
			if found != tc.wantFound {
				t.Fatalf("CheckResourceRole() found = %v, want %v", found, tc.wantFound)
			}
			if role != tc.wantRole {
				t.Fatalf("CheckResourceRole() role = %q, want %q", role, tc.wantRole)
			}
		})
	}
}

func TestRequireResourceAccess_EnforcesInheritedRoleAndFailureModes(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "require_resource_access")
	systemID, _, vmID := createResourceRoleHierarchy(t, client)
	createResourceRoleBinding(t, client, "owner-user", "system", systemID, resourcerolebinding.RoleOwner)
	createResourceRoleBinding(t, client, "admin-user", "system", systemID, resourcerolebinding.RoleAdmin)
	createResourceRoleBinding(t, client, "member-user", "system", systemID, resourcerolebinding.RoleMember)
	createResourceRoleBinding(t, client, "viewer-user", "system", systemID, resourcerolebinding.RoleViewer)

	checker := NewResourceRoleChecker(client)
	tests := []struct {
		name          string
		userID        string
		permissions   []string
		action        string
		cancelContext bool
		wantStatus    int
		wantMessage   string
	}{
		{
			name:        "platform admin bypasses resource lookup without user context",
			permissions: []string{"platform:admin"},
			action:      "transfer_ownership",
			wantStatus:  http.StatusNoContent,
		},
		{
			name:       "owner can transfer ownership through inherited system role",
			userID:     "owner-user",
			action:     "transfer_ownership",
			wantStatus: http.StatusNoContent,
		},
		{
			name:        "admin cannot transfer ownership",
			userID:      "admin-user",
			action:      "transfer_ownership",
			wantStatus:  http.StatusForbidden,
			wantMessage: "insufficient resource permissions",
		},
		{
			name:       "member can create through inherited system role",
			userID:     "member-user",
			action:     "create",
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "viewer can view through inherited system role",
			userID:     "viewer-user",
			action:     "view",
			wantStatus: http.StatusNoContent,
		},
		{
			name:        "viewer cannot create",
			userID:      "viewer-user",
			action:      "create",
			wantStatus:  http.StatusForbidden,
			wantMessage: "insufficient resource permissions",
		},
		{
			name:        "authenticated user without a binding is denied",
			userID:      "unbound-user",
			action:      "view",
			wantStatus:  http.StatusForbidden,
			wantMessage: "insufficient resource permissions",
		},
		{
			name:        "missing authenticated user is denied before lookup",
			action:      "view",
			wantStatus:  http.StatusForbidden,
			wantMessage: "not authenticated",
		},
		{
			name:          "lookup failure does not degrade to an authorization decision",
			userID:        "viewer-user",
			action:        "view",
			cancelContext: true,
			wantStatus:    http.StatusInternalServerError,
			wantMessage:   "permission check failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tc.permissions != nil {
					c.Set("permissions", tc.permissions)
				}
				c.Next()
			})
			router.GET(
				"/vms/:vm_id",
				RequireResourceAccess(checker, "vm", tc.action, "vm_id"),
				func(c *gin.Context) { c.Status(http.StatusNoContent) },
			)

			req := httptest.NewRequest(http.MethodGet, "/vms/"+vmID, http.NoBody)
			requestCtx := req.Context()
			if tc.userID != "" {
				requestCtx = SetUserContext(requestCtx, tc.userID, tc.userID, nil)
			}
			if tc.cancelContext {
				var cancel context.CancelFunc
				requestCtx, cancel = context.WithCancel(requestCtx)
				cancel()
			}
			req = req.WithContext(requestCtx)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", w.Code, tc.wantStatus, w.Body.String())
			}
			if tc.wantMessage != "" && !strings.Contains(w.Body.String(), tc.wantMessage) {
				t.Fatalf("body = %q, want message %q", w.Body.String(), tc.wantMessage)
			}
		})
	}
}

func TestRequireResourceAccess_FailsClosedOnInvalidConfiguration(t *testing.T) {
	t.Parallel()

	client := testutil.OpenEntPostgres(t, "require_resource_access_config")
	checker := NewResourceRoleChecker(client)
	tests := []struct {
		name         string
		checker      *ResourceRoleChecker
		resourceType string
		action       string
		paramName    string
	}{
		{name: "nil checker", resourceType: "vm", action: "view", paramName: "vm_id"},
		{name: "empty resource type", checker: checker, action: "view", paramName: "vm_id"},
		{name: "empty action", checker: checker, resourceType: "vm", paramName: "vm_id"},
		{name: "empty parameter name", checker: checker, resourceType: "vm", action: "view"},
		{name: "wrong parameter name", checker: checker, resourceType: "vm", action: "view", paramName: "wrong_id"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			downstreamCalled := false
			router := gin.New()
			router.GET(
				"/vms/:vm_id",
				RequireResourceAccess(test.checker, test.resourceType, test.action, test.paramName),
				func(c *gin.Context) {
					downstreamCalled = true
					c.Status(http.StatusNoContent)
				},
			)
			req := httptest.NewRequest(http.MethodGet, "/vms/vm-1", http.NoBody)
			req = req.WithContext(SetUserContext(req.Context(), "member-user", "member-user", nil))
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
			}
			if downstreamCalled {
				t.Fatal("misconfigured resource middleware executed downstream handler")
			}
			if !strings.Contains(w.Body.String(), `"code":"INTERNAL_ERROR"`) {
				t.Fatalf("body = %q, want INTERNAL_ERROR", w.Body.String())
			}
		})
	}
}

func createResourceRoleHierarchy(t *testing.T, client *ent.Client) (systemID, serviceID, vmID string) {
	t.Helper()

	suffix := strings.ToLower(uuid.NewString()[:8])
	systemID = "system-" + suffix
	serviceID = "service-" + suffix
	vmID = "vm-" + suffix

	if _, err := client.System.Create().
		SetID(systemID).
		SetName("sys" + suffix[:6]).
		SetCreatedBy("test-seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create system: %v", err)
	}
	if _, err := client.Service.Create().
		SetID(serviceID).
		SetName("svc" + suffix[:6]).
		SetSystemID(systemID).
		Save(t.Context()); err != nil {
		t.Fatalf("create service: %v", err)
	}
	if _, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-" + suffix).
		SetInstance("01").
		SetNamespace("ns-" + suffix).
		SetCreatedBy("test-seed").
		SetServiceID(serviceID).
		Save(t.Context()); err != nil {
		t.Fatalf("create VM: %v", err)
	}

	return systemID, serviceID, vmID
}

func createResourceRoleBinding(
	t *testing.T,
	client *ent.Client,
	userID string,
	resourceType string,
	resourceID string,
	role resourcerolebinding.Role,
) {
	t.Helper()

	if _, err := client.ResourceRoleBinding.Create().
		SetID(uuid.NewString()).
		SetUserID(userID).
		SetResourceType(resourceType).
		SetResourceID(resourceID).
		SetRole(role).
		SetCreatedBy("test-seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create resource role binding: %v", err)
	}
}
