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

func TestRateLimitStatus_IncludesUserSummaryFields(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	_, err := client.User.Create().
		SetID("user-rate-1").
		SetUsername("ops-alice").
		SetDisplayName("Alice Ops").
		SetEmail("alice.ops@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, err = client.RateLimitExemption.Create().
		SetID("user-rate-1").
		SetExemptedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create exemption: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/rate-limits/status", "", "user-rate", []string{"rate_limit:manage"})
	srv.ListRateLimitStatus(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var list generated.RateLimitStatusList
	mustDecodeJSON(t, w.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(list.Items))
	}
	item := list.Items[0]
	if item.UserId != "user-rate-1" {
		t.Fatalf("user_id = %q, want %q", item.UserId, "user-rate-1")
	}
	if item.Username != "ops-alice" {
		t.Fatalf("username = %q, want %q", item.Username, "ops-alice")
	}
	if item.DisplayName != "Alice Ops" {
		t.Fatalf("display_name = %q, want %q", item.DisplayName, "Alice Ops")
	}
	if item.Email != "alice.ops@example.com" {
		t.Fatalf("email = %q, want %q", item.Email, "alice.ops@example.com")
	}
}

func TestListRateLimitExemptions_IncludesUserSummaryFields(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	_, err := client.User.Create().
		SetID("user-rate-exempt").
		SetUsername("ops-bob").
		SetDisplayName("Bob Ops").
		SetEmail("bob.ops@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_, err = client.RateLimitExemption.Create().
		SetID("user-rate-exempt").
		SetExemptedBy("admin-1").
		SetReason("CI automation").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create exemption: %v", err)
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/rate-limits/exemptions?page=1&per_page=20", "", "user-rate", []string{"rate_limit:manage"})
	srv.ListRateLimitExemptions(c, generated.ListRateLimitExemptionsParams{
		Page:    generated.Page(1),
		PerPage: generated.PerPage(20),
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var list generated.RateLimitExemptionList
	mustDecodeJSON(t, w.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(list.Items))
	}
	item := list.Items[0]
	if item.UserId != "user-rate-exempt" {
		t.Fatalf("user_id = %q, want %q", item.UserId, "user-rate-exempt")
	}
	if item.Username != "ops-bob" {
		t.Fatalf("username = %q, want %q", item.Username, "ops-bob")
	}
	if item.DisplayName != "Bob Ops" {
		t.Fatalf("display_name = %q, want %q", item.DisplayName, "Bob Ops")
	}
	if item.Email != "bob.ops@example.com" {
		t.Fatalf("email = %q, want %q", item.Email, "bob.ops@example.com")
	}
}
