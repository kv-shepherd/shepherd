package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/api/middleware"
	"kv-shepherd.io/shepherd/internal/testutil"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_UsesConfiguredCost(t *testing.T) {
	hash, err := HashPassword("Passw0rd!Example")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}

	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		t.Fatalf("bcrypt.Cost() error = %v", err)
	}

	if cost != passwordHashCost {
		t.Fatalf("bcrypt cost = %d, want %d", cost, passwordHashCost)
	}
}

func TestGetCurrentUser_IncludesPermissions(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	client := testutil.OpenEntPostgres(t, "auth_handler_me_permissions")
	server := NewServer(ServerDeps{EntClient: client})

	user, err := client.User.Create().
		SetID("user-1").
		SetUsername("alice").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	role, err := client.Role.Create().
		SetID("role-1").
		SetName("Operator").
		SetPermissions([]string{"vm:read", "system:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}

	if _, err := client.RoleBinding.Create().
		SetID("rb-1").
		SetUser(user).
		SetRole(role).
		SetScopeType("global").
		SetCreatedBy("seed").
		Save(t.Context()); err != nil {
		t.Fatalf("seed role binding: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(middleware.SetUserContext(req.Context(), user.ID, user.Username, nil))
	c.Request = req

	server.GetCurrentUser(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}

	var got generated.UserInfo
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode user info: %v", err)
	}
	if len(got.Permissions) != 2 {
		t.Fatalf("unexpected permissions: %+v", got.Permissions)
	}
	if got.Permissions[0] != "system:read" || got.Permissions[1] != "vm:read" {
		t.Fatalf("permissions not sorted/stable: %+v", got.Permissions)
	}
}
