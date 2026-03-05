package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
)

func TestHasPlatformAdmin(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	t.Run("returns true when permission exists", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("permissions", []string{"system:read", "platform:admin"})
		if !hasPlatformAdmin(c) {
			t.Fatal("hasPlatformAdmin() = false, want true")
		}
	})

	t.Run("returns false when context missing permissions", func(t *testing.T) {
		t.Parallel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		if hasPlatformAdmin(c) {
			t.Fatal("hasPlatformAdmin() = true, want false")
		}
	})
}

func TestIsValidMemberRole(t *testing.T) {
	t.Parallel()

	valid := []string{
		resourcerolebinding.RoleOwner.String(),
		resourcerolebinding.RoleAdmin.String(),
		resourcerolebinding.RoleMember.String(),
		resourcerolebinding.RoleViewer.String(),
	}
	for _, role := range valid {
		if !isValidMemberRole(role) {
			t.Fatalf("isValidMemberRole(%q) = false, want true", role)
		}
	}

	invalid := []string{"", "platform-admin", "operator", "read-only"}
	for _, role := range invalid {
		if isValidMemberRole(role) {
			t.Fatalf("isValidMemberRole(%q) = true, want false", role)
		}
	}
}

func TestToSystemMember(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC().Round(time.Second)
	binding := &ent.ResourceRoleBinding{
		UserID:    "user-1",
		Role:      resourcerolebinding.RoleMember,
		CreatedAt: createdAt,
	}

	t.Run("falls back to user id when user entity absent", func(t *testing.T) {
		t.Parallel()
		member := toSystemMember(binding, nil)
		if member.UserId != "user-1" {
			t.Fatalf("UserId = %q, want %q", member.UserId, "user-1")
		}
		if member.Username != "user-1" {
			t.Fatalf("Username = %q, want fallback user id", member.Username)
		}
		if member.Role != "member" {
			t.Fatalf("Role = %q, want %q", member.Role, "member")
		}
	})

	t.Run("fills user profile fields when user present", func(t *testing.T) {
		t.Parallel()
		user := &ent.User{
			ID:          "user-1",
			Username:    "alice",
			Email:       "alice@example.com",
			DisplayName: "Alice",
		}
		member := toSystemMember(binding, user)
		if member.Username != "alice" {
			t.Fatalf("Username = %q, want %q", member.Username, "alice")
		}
		if member.Email != "alice@example.com" {
			t.Fatalf("Email = %q, want %q", member.Email, "alice@example.com")
		}
		if member.DisplayName != "Alice" {
			t.Fatalf("DisplayName = %q, want %q", member.DisplayName, "Alice")
		}
	})
}

func TestMemberHandler_ListSystemMembers_RequestContextCanceled(t *testing.T) {
	t.Parallel()

	srv, _ := newSystemBehaviorTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/sys-cancelled/members", "", "user-a", []string{"system:read"})
	reqCtx, cancel := context.WithCancel(c.Request.Context())
	cancel()
	c.Request = c.Request.WithContext(reqCtx)

	srv.ListSystemMembers(c, "sys-cancelled")

	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body for canceled request, got %q", w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d for canceled request", w.Code, http.StatusOK)
	}
}
