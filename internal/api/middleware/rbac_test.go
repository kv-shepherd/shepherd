package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
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
