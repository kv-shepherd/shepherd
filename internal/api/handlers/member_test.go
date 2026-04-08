package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/resourcerolebinding"
	"kv-shepherd.io/shepherd/internal/api/generated"
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
		member := toSystemMember(binding, nil, nil)
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
		member := toSystemMember(binding, user, nil)
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

	t.Run("projects requested profile attributes when available", func(t *testing.T) {
		t.Parallel()
		user := &ent.User{
			ID:          "user-1",
			Username:    "alice",
			Email:       "alice@example.com",
			DisplayName: "Alice",
			Edges: ent.UserEdges{
				DirectoryProfile: &ent.UserDirectoryProfile{
					Attributes: map[string]interface{}{
						"department": "Engineering",
						"section":    "Platform",
					},
				},
			},
		}
		member := toSystemMember(binding, user, []string{"department", "section"})
		if member.ProfileAttributes["department"] != "Engineering" {
			t.Fatalf("ProfileAttributes[department] = %#v, want %q", member.ProfileAttributes["department"], "Engineering")
		}
		if member.ProfileAttributes["section"] != "Platform" {
			t.Fatalf("ProfileAttributes[section] = %#v, want %q", member.ProfileAttributes["section"], "Platform")
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

func TestMemberHandler_ListSystemMembers_IncludesDirectoryProfileFields(t *testing.T) {
	t.Parallel()

	srv, client := newSystemBehaviorTestServer(t)
	systemID := "sys-members"
	mustCreateSystem(t, client, systemID, "shop-members", "seed")
	if _, err := client.User.Create().
		SetID("user-member-1").
		SetUsername("member.one").
		SetDisplayName("Member One").
		SetEmail("member.one@example.com").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := client.UserDirectoryProfile.Create().
		SetID("profile-member-1").
		SetUserID("user-member-1").
		SetAttributes(map[string]interface{}{
			"department": "Engineering",
			"section":    "Platform",
		}).
		SetLastSyncedAt(time.Now().UTC()).
		Save(t.Context()); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := client.ResourceRoleBinding.Create().
		SetID("binding-member-1").
		SetUserID("user-member-1").
		SetResourceType("system").
		SetResourceID(systemID).
		SetRole(resourcerolebinding.RoleMember).
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("create member binding: %v", err)
	}
	c, w := newAuthedGinContext(t, http.MethodGet, "/systems/"+systemID+"/members", "", "user-member-1", []string{"system:read"})
	srv.ListSystemMembers(c, systemID)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp generated.SystemMemberList
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.ProfileFields) == 0 {
		t.Fatal("expected profile_fields in response")
	}
	if len(resp.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].ProfileAttributes["department"] != "Engineering" {
		t.Fatalf("department = %#v, want Engineering", resp.Items[0].ProfileAttributes["department"])
	}
}
