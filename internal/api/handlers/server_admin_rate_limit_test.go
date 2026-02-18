package handlers

import (
	"net/http"
	"testing"

	"kv-shepherd.io/shepherd/internal/api/generated"
)

func TestRateLimitStatus_RequiresRateLimitManagePermission(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/rate-limits/status", "", "user-a", []string{"cluster:read"})

	srv.ListRateLimitStatus(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusForbidden, w.Body.String())
	}
	assertErrorCode(t, w.Body.Bytes(), "FORBIDDEN")
}

func TestRateLimitStatus_AllowsRateLimitManagerWithoutPlatformAdmin(t *testing.T) {
	t.Parallel()

	srv, _ := newAdminIdentityTestServer(t)
	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/rate-limits/status", "", "user-rate", []string{"rate_limit:manage"})

	srv.ListRateLimitStatus(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var list generated.RateLimitStatusList
	mustDecodeJSON(t, w.Body.Bytes(), &list)
}
