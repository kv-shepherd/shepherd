package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/testutil"
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

func TestRateLimitStatus_CountsModifyAndPowerParentsAcrossActiveStates(t *testing.T) {
	t.Parallel()

	srv, client := newAdminIdentityTestServer(t)
	testCases := []struct {
		userID    string
		eventType domain.EventType
		status    domainevent.Status
	}{
		{
			userID:    "user-rate-modify",
			eventType: domain.EventBatchModifyRequested,
			status:    domainevent.StatusPENDING,
		},
		{
			userID:    "user-rate-power",
			eventType: domain.EventBatchPowerRequested,
			status:    domainevent.StatusPROCESSING,
		},
	}
	for _, tc := range testCases {
		if _, err := client.User.Create().
			SetID(tc.userID).
			SetUsername(tc.userID).
			SetEnabled(true).
			Save(t.Context()); err != nil {
			t.Fatalf("create rate-limit status user %q: %v", tc.userID, err)
		}
		if _, err := client.DomainEvent.Create().
			SetID("event-" + tc.userID).
			SetEventType(string(tc.eventType)).
			SetAggregateType("batch").
			SetAggregateID("batch-" + tc.userID).
			SetPayload([]byte(`{}`)).
			SetStatus(tc.status).
			SetCreatedBy(tc.userID).
			Save(t.Context()); err != nil {
			t.Fatalf("create %s batch event for %q: %v", tc.eventType, tc.userID, err)
		}
	}

	c, w := newAuthedGinContext(t, http.MethodGet, "/admin/rate-limits/status", "", "rate-admin", []string{"rate_limit:manage"})
	srv.ListRateLimitStatus(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var list generated.RateLimitStatusList
	mustDecodeJSON(t, w.Body.Bytes(), &list)
	byUserID := make(map[string]generated.RateLimitUserStatus, len(list.Items))
	for _, item := range list.Items {
		byUserID[item.UserId] = item
	}
	for _, tc := range testCases {
		item, ok := byUserID[tc.userID]
		if !ok {
			t.Fatalf("rate-limit status omitted %s batch user %q: %+v", tc.eventType, tc.userID, list.Items)
		}
		if item.CurrentPendingParents != 1 {
			t.Fatalf("%s current_pending_parents = %d, want 1", tc.eventType, item.CurrentPendingParents)
		}
		if item.CooldownRemainingSeconds <= 0 {
			t.Fatalf("%s cooldown_remaining_seconds = %d, want active cooldown", tc.eventType, item.CooldownRemainingSeconds)
		}
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

func TestCreateRateLimitExemption_UpsertPersistsTrimmedValuesAndClearsOptionalFields(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "exemption_upsert")
	mustCreateRateLimitUser(t, client, "rate-user-exemption")
	futureExpiry := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)

	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		mustJSON(t, map[string]any{
			"user_id":    "  rate-user-exemption  ",
			"reason":     "  approved maintenance  ",
			"expires_at": futureExpiry,
		}),
		"rate-admin-create",
		[]string{"rate_limit:manage"},
	)
	srv.CreateRateLimitExemption(createContext)
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d body=%s", createResponse.Code, http.StatusOK, createResponse.Body.String())
	}

	var createdResponse generated.RateLimitExemption
	mustDecodeJSON(t, createResponse.Body.Bytes(), &createdResponse)
	if createdResponse.UserId != "rate-user-exemption" {
		t.Fatalf("create response user_id = %q, want rate-user-exemption", createdResponse.UserId)
	}
	if createdResponse.Username != "rate-user-exemption" || createdResponse.DisplayName != "Rate User" || createdResponse.Email != "rate-user-exemption@example.com" {
		t.Fatalf("create response user summary = (%q, %q, %q), want persisted user fields", createdResponse.Username, createdResponse.DisplayName, createdResponse.Email)
	}
	if createdResponse.ExemptedBy != "rate-admin-create" {
		t.Fatalf("create response exempted_by = %q, want rate-admin-create", createdResponse.ExemptedBy)
	}
	if createdResponse.Reason != "approved maintenance" {
		t.Fatalf("create response reason = %q, want trimmed reason", createdResponse.Reason)
	}
	if !createdResponse.ExpiresAt.Equal(futureExpiry) {
		t.Fatalf("create response expires_at = %s, want %s", createdResponse.ExpiresAt, futureExpiry)
	}

	createdRow, err := client.RateLimitExemption.Get(t.Context(), "rate-user-exemption")
	if err != nil {
		t.Fatalf("query created exemption: %v", err)
	}
	if createdRow.Reason != "approved maintenance" || createdRow.ExemptedBy != "rate-admin-create" {
		t.Fatalf("created row reason/exempted_by = (%q, %q), want trimmed reason and actor", createdRow.Reason, createdRow.ExemptedBy)
	}
	if createdRow.ExpiresAt == nil || !createdRow.ExpiresAt.Equal(futureExpiry) {
		t.Fatalf("created row expires_at = %v, want %s", createdRow.ExpiresAt, futureExpiry)
	}

	updateContext, updateResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		mustJSON(t, map[string]any{
			"user_id":    " rate-user-exemption ",
			"reason":     "   ",
			"expires_at": nil,
		}),
		"rate-admin-update",
		[]string{"rate_limit:manage"},
	)
	srv.CreateRateLimitExemption(updateContext)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d body=%s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}

	var updatedResponse generated.RateLimitExemption
	mustDecodeJSON(t, updateResponse.Body.Bytes(), &updatedResponse)
	if updatedResponse.Reason != "" || !updatedResponse.ExpiresAt.IsZero() {
		t.Fatalf("update response reason/expires_at = (%q, %s), want cleared values", updatedResponse.Reason, updatedResponse.ExpiresAt)
	}
	if updatedResponse.ExemptedBy != "rate-admin-update" {
		t.Fatalf("update response exempted_by = %q, want rate-admin-update", updatedResponse.ExemptedBy)
	}

	updatedRow, err := client.RateLimitExemption.Get(t.Context(), "rate-user-exemption")
	if err != nil {
		t.Fatalf("query updated exemption: %v", err)
	}
	if updatedRow.Reason != "" || updatedRow.ExpiresAt != nil {
		t.Fatalf("updated row reason/expires_at = (%q, %v), want cleared values", updatedRow.Reason, updatedRow.ExpiresAt)
	}
	if updatedRow.ExemptedBy != "rate-admin-update" {
		t.Fatalf("updated row exempted_by = %q, want rate-admin-update", updatedRow.ExemptedBy)
	}
	if !updatedRow.CreatedAt.Equal(createdRow.CreatedAt) {
		t.Fatalf("upsert replaced row: created_at = %s, want %s", updatedRow.CreatedAt, createdRow.CreatedAt)
	}
	count, err := client.RateLimitExemption.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count exemptions after upsert: %v", err)
	}
	if count != 1 {
		t.Fatalf("exemption count after upsert = %d, want 1", count)
	}
}

func TestCreateRateLimitExemption_RejectsInvalidExpiryAndBlankUserWithoutWriting(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "exemption_validation")
	now := time.Now().UTC()
	tests := []struct {
		name      string
		userID    string
		expiresAt time.Time
		seedUser  bool
	}{
		{name: "expired", userID: "rate-user-expired", expiresAt: now.Add(-time.Minute), seedUser: true},
		{name: "current_time_boundary", userID: "rate-user-current", expiresAt: now, seedUser: true},
		{name: "blank_user_after_trim", userID: "   ", expiresAt: now.Add(time.Hour), seedUser: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.seedUser {
				mustCreateRateLimitUser(t, client, test.userID)
			}
			requestContext, response := newAuthedGinContext(
				t,
				http.MethodPost,
				"/admin/rate-limits/exemptions",
				mustJSON(t, map[string]any{"user_id": test.userID, "expires_at": test.expiresAt}),
				"rate-admin",
				[]string{"rate_limit:manage"},
			)
			srv.CreateRateLimitExemption(requestContext)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), "INVALID_REQUEST")
			count, err := client.RateLimitExemption.Query().Count(t.Context())
			if err != nil {
				t.Fatalf("count exemptions: %v", err)
			}
			if count != 0 {
				t.Fatalf("exemption count = %d, want 0 after rejected request", count)
			}
		})
	}
}

func TestCreateRateLimitExemption_UserNotFoundDoesNotWrite(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "exemption_user_not_found")
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"missing-rate-user","reason":"support"}`,
		"rate-admin",
		[]string{"rate_limit:manage"},
	)
	srv.CreateRateLimitExemption(requestContext)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "USER_NOT_FOUND")
	count, err := client.RateLimitExemption.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count exemptions: %v", err)
	}
	if count != 0 {
		t.Fatalf("exemption count = %d, want 0", count)
	}
}

func TestDeleteRateLimitExemption_TrimsIDAndDeletesPersistedRow(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "exemption_delete")
	mustCreateRateLimitUser(t, client, "rate-user-delete")
	_, err := client.RateLimitExemption.Create().
		SetID("rate-user-delete").
		SetExemptedBy("seed-admin").
		SetReason("temporary").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed exemption: %v", err)
	}

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/rate-limits/exemptions/rate-user-delete",
		"",
		"rate-admin",
		[]string{"rate_limit:manage"},
	)
	srv.DeleteRateLimitExemption(requestContext, "  rate-user-delete  ")
	requestContext.Writer.WriteHeaderNow()
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if response.Body.Len() != 0 {
		t.Fatalf("204 response body = %q, want empty", response.Body.String())
	}
	if _, err := client.RateLimitExemption.Get(t.Context(), "rate-user-delete"); !ent.IsNotFound(err) {
		t.Fatalf("deleted exemption query error = %v, want not found", err)
	}
}

func TestDeleteRateLimitExemption_RejectsBlankAndMissingIDs(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "exemption_delete_missing")
	tests := []struct {
		name       string
		userID     string
		wantStatus int
		wantCode   string
	}{
		{name: "blank", userID: "  ", wantStatus: http.StatusBadRequest, wantCode: "INVALID_REQUEST"},
		{name: "missing", userID: "missing-rate-user", wantStatus: http.StatusNotFound, wantCode: "RATE_LIMIT_EXEMPTION_NOT_FOUND"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestContext, response := newAuthedGinContext(
				t,
				http.MethodDelete,
				"/admin/rate-limits/exemptions/test-user-id",
				"",
				"rate-admin",
				[]string{"rate_limit:manage"},
			)
			srv.DeleteRateLimitExemption(requestContext, test.userID)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), test.wantCode)
			count, err := client.RateLimitExemption.Query().Count(t.Context())
			if err != nil {
				t.Fatalf("count exemptions: %v", err)
			}
			if count != 0 {
				t.Fatalf("exemption count = %d, want 0", count)
			}
		})
	}
}

func TestDeleteRateLimitExemption_WaitsForUserMutationGuard(t *testing.T) {
	t.Parallel()

	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "exemption_delete_guard")
	mustCreateRateLimitUser(t, client, "rate-user-delete-guard")
	if _, err := client.RateLimitExemption.Create().
		SetID("rate-user-delete-guard").
		SetExemptedBy("seed-admin").
		Save(t.Context()); err != nil {
		t.Fatalf("seed guarded exemption: %v", err)
	}
	releaseGuard, blockerPID := holdUserMutationGuard(t, srv.pool, "rate-user-delete-guard")

	requestContext, response := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/rate-limits/exemptions/rate-user-delete-guard",
		"",
		"rate-admin",
		[]string{"rate_limit:manage"},
	)
	done := runHandlerAsync(func() {
		srv.DeleteRateLimitExemption(requestContext, "rate-user-delete-guard")
	})

	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, 1)
	releaseGuard()
	waitForHandlerCompletion(t, done, "guarded rate-limit exemption delete")
	requestContext.Writer.WriteHeaderNow()
	if response.Code != http.StatusNoContent {
		t.Fatalf("guarded delete status = %d, want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if _, err := client.RateLimitExemption.Get(t.Context(), "rate-user-delete-guard"); !ent.IsNotFound(err) {
		t.Fatalf("guarded deleted exemption query error = %v, want not found", err)
	}
}

func TestUpdateRateLimitUserOverrides_CreateAndPartialUpdatePersistValues(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "override_upsert")
	mustCreateRateLimitUser(t, client, "rate-user-override")

	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-override",
		`{"max_pending_parents":1,"max_pending_children":2,"cooldown_seconds":0,"reason":"  elevated support window  "}`,
		"rate-admin-create",
		[]string{"rate_limit:manage"},
	)
	srv.UpdateRateLimitUserOverrides(createContext, "  rate-user-override  ")
	if createResponse.Code != http.StatusOK {
		t.Fatalf("create status = %d, want %d body=%s", createResponse.Code, http.StatusOK, createResponse.Body.String())
	}

	var createdResponse generated.RateLimitUserOverride
	mustDecodeJSON(t, createResponse.Body.Bytes(), &createdResponse)
	if createdResponse.UserId != "rate-user-override" || createdResponse.UpdatedBy != "rate-admin-create" {
		t.Fatalf("create response user_id/updated_by = (%q, %q), want persisted ID and actor", createdResponse.UserId, createdResponse.UpdatedBy)
	}
	if createdResponse.MaxPendingParents != 1 ||
		createdResponse.MaxPendingChildren != 2 ||
		createdResponse.CooldownSeconds != 0 {
		t.Fatalf(
			"create response limits = (%d, %d, %d), want (1, 2, 0)",
			createdResponse.MaxPendingParents,
			createdResponse.MaxPendingChildren,
			createdResponse.CooldownSeconds,
		)
	}
	if createdResponse.Reason != "elevated support window" {
		t.Fatalf("create response reason = %q, want trimmed reason", createdResponse.Reason)
	}

	createdRow, err := client.RateLimitUserOverride.Get(t.Context(), "rate-user-override")
	if err != nil {
		t.Fatalf("query created override: %v", err)
	}
	assertRateLimitOverrideValues(t, createdRow, 1, 2, 0, "elevated support window", "rate-admin-create")

	updateContext, updateResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-override",
		`{"max_pending_parents":4,"reason":"   "}`,
		"rate-admin-update",
		[]string{"rate_limit:manage"},
	)
	srv.UpdateRateLimitUserOverrides(updateContext, " rate-user-override ")
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d body=%s", updateResponse.Code, http.StatusOK, updateResponse.Body.String())
	}

	updatedRow, err := client.RateLimitUserOverride.Get(t.Context(), "rate-user-override")
	if err != nil {
		t.Fatalf("query updated override: %v", err)
	}
	assertRateLimitOverrideValues(t, updatedRow, 4, 2, 0, "", "rate-admin-update")
	if !updatedRow.CreatedAt.Equal(createdRow.CreatedAt) {
		t.Fatalf("upsert replaced row: created_at = %s, want %s", updatedRow.CreatedAt, createdRow.CreatedAt)
	}
	count, err := client.RateLimitUserOverride.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count overrides after upsert: %v", err)
	}
	if count != 1 {
		t.Fatalf("override count after upsert = %d, want 1", count)
	}
}

func TestUpdateRateLimitUserOverrides_RejectsInvalidBoundsWithoutWriting(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "override_validation")
	tests := []struct {
		name string
		body string
	}{
		{name: "empty_body", body: `{}`},
		{name: "zero_parent_limit", body: `{"max_pending_parents":0}`},
		{name: "zero_child_limit", body: `{"max_pending_children":0}`},
		{name: "negative_cooldown", body: `{"cooldown_seconds":-1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			userID := "rate-user-" + test.name
			mustCreateRateLimitUser(t, client, userID)
			requestContext, response := newAuthedGinContext(
				t,
				http.MethodPut,
				"/admin/rate-limits/users/"+userID,
				test.body,
				"rate-admin",
				[]string{"rate_limit:manage"},
			)
			srv.UpdateRateLimitUserOverrides(requestContext, userID)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			assertErrorCode(t, response.Body.Bytes(), "INVALID_REQUEST")
			if _, err := client.RateLimitUserOverride.Get(t.Context(), userID); !ent.IsNotFound(err) {
				t.Fatalf("override query error = %v, want not found after rejected request", err)
			}
		})
	}
}

func TestUpdateRateLimitUserOverrides_UserNotFoundDoesNotWrite(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "override_user_not_found")
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/missing-rate-user",
		`{"max_pending_parents":2}`,
		"rate-admin",
		[]string{"rate_limit:manage"},
	)
	srv.UpdateRateLimitUserOverrides(requestContext, " missing-rate-user ")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	assertErrorCode(t, response.Body.Bytes(), "USER_NOT_FOUND")
	count, err := client.RateLimitUserOverride.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count overrides: %v", err)
	}
	if count != 0 {
		t.Fatalf("override count = %d, want 0", count)
	}
}

func TestRateLimitMutationHandlers_ForbiddenRequestsHaveNoSideEffects(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "mutation_forbidden")
	mustCreateRateLimitUser(t, client, "rate-user-forbidden")
	exemption, err := client.RateLimitExemption.Create().
		SetID("rate-user-forbidden").
		SetExemptedBy("seed-admin").
		SetReason("seed exemption").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed exemption: %v", err)
	}
	override, err := client.RateLimitUserOverride.Create().
		SetID("rate-user-forbidden").
		SetMaxPendingParents(2).
		SetMaxPendingChildren(3).
		SetCooldownSeconds(4).
		SetReason("seed override").
		SetUpdatedBy("seed-admin").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}

	createContext, createResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"rate-user-forbidden","reason":"unauthorized update"}`,
		"unauthorized-user",
		[]string{"cluster:read"},
	)
	srv.CreateRateLimitExemption(createContext)
	assertRateLimitForbiddenResponse(t, createResponse.Code, createResponse.Body.Bytes())

	deleteContext, deleteResponse := newAuthedGinContext(
		t,
		http.MethodDelete,
		"/admin/rate-limits/exemptions/rate-user-forbidden",
		"",
		"unauthorized-user",
		[]string{"cluster:read"},
	)
	srv.DeleteRateLimitExemption(deleteContext, "rate-user-forbidden")
	assertRateLimitForbiddenResponse(t, deleteResponse.Code, deleteResponse.Body.Bytes())

	overrideContext, overrideResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-forbidden",
		`{"max_pending_parents":9,"reason":"unauthorized update"}`,
		"unauthorized-user",
		[]string{"cluster:read"},
	)
	srv.UpdateRateLimitUserOverrides(overrideContext, "rate-user-forbidden")
	assertRateLimitForbiddenResponse(t, overrideResponse.Code, overrideResponse.Body.Bytes())

	persistedExemption, err := client.RateLimitExemption.Get(t.Context(), exemption.ID)
	if err != nil {
		t.Fatalf("query exemption after forbidden requests: %v", err)
	}
	if persistedExemption.Reason != "seed exemption" || persistedExemption.ExemptedBy != "seed-admin" {
		t.Fatalf("exemption changed after forbidden requests: reason/exempted_by = (%q, %q)", persistedExemption.Reason, persistedExemption.ExemptedBy)
	}
	wantExemptionUpdatedAt := exemption.UpdatedAt.Truncate(time.Microsecond)
	if !persistedExemption.UpdatedAt.Equal(wantExemptionUpdatedAt) {
		t.Fatalf("exemption updated_at changed after forbidden requests: got %s, want %s", persistedExemption.UpdatedAt, wantExemptionUpdatedAt)
	}
	persistedOverride, err := client.RateLimitUserOverride.Get(t.Context(), override.ID)
	if err != nil {
		t.Fatalf("query override after forbidden request: %v", err)
	}
	assertRateLimitOverrideValues(t, persistedOverride, 2, 3, 4, "seed override", "seed-admin")
	wantOverrideUpdatedAt := override.UpdatedAt.Truncate(time.Microsecond)
	if !persistedOverride.UpdatedAt.Equal(wantOverrideUpdatedAt) {
		t.Fatalf("override updated_at changed after forbidden request: got %s, want %s", persistedOverride.UpdatedAt, wantOverrideUpdatedAt)
	}
}

func TestUpdateRateLimitUserOverrides_WhitespaceReasonOnlyClearsReasonAndPreservesLimits(t *testing.T) {
	t.Parallel()

	srv, client := newRateLimitMutationTestServer(t, "override_blank_reason")
	mustCreateRateLimitUser(t, client, "rate-user-blank-reason")
	_, err := client.RateLimitUserOverride.Create().
		SetID("rate-user-blank-reason").
		SetMaxPendingParents(2).
		SetMaxPendingChildren(3).
		SetCooldownSeconds(4).
		SetReason("temporary override").
		SetUpdatedBy("seed-admin").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed override: %v", err)
	}
	requestContext, response := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-blank-reason",
		`{"reason":"   "}`,
		"rate-admin",
		[]string{"rate_limit:manage"},
	)
	srv.UpdateRateLimitUserOverrides(requestContext, "rate-user-blank-reason")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	persisted, err := client.RateLimitUserOverride.Get(t.Context(), "rate-user-blank-reason")
	if err != nil {
		t.Fatalf("query override after reason clear: %v", err)
	}
	assertRateLimitOverrideValues(t, persisted, 2, 3, 4, "", "rate-admin")
}

func TestRateLimitMutationHandlers_CanceledRequestsHaveNoSideEffects(t *testing.T) {
	t.Parallel()

	t.Run("create exemption", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "cancel_exemption_create")
		mustCreateRateLimitUser(t, client, "rate-user-cancel-create")
		requestContext, _ := newAuthedGinContext(
			t,
			http.MethodPost,
			"/admin/rate-limits/exemptions",
			`{"user_id":"rate-user-cancel-create","reason":"support"}`,
			"rate-admin",
			[]string{"rate_limit:manage"},
		)
		cancelContext, cancel := context.WithCancel(requestContext.Request.Context())
		cancel()
		requestContext.Request = requestContext.Request.WithContext(cancelContext)

		srv.CreateRateLimitExemption(requestContext)
		if _, err := client.RateLimitExemption.Get(t.Context(), "rate-user-cancel-create"); !ent.IsNotFound(err) {
			t.Fatalf("exemption query error = %v, want not found after cancellation", err)
		}
	})

	t.Run("delete exemption", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "cancel_exemption_delete")
		mustCreateRateLimitUser(t, client, "rate-user-cancel-delete")
		_, err := client.RateLimitExemption.Create().
			SetID("rate-user-cancel-delete").
			SetExemptedBy("seed-admin").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed exemption: %v", err)
		}
		requestContext, _ := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/admin/rate-limits/exemptions/rate-user-cancel-delete",
			"",
			"rate-admin",
			[]string{"rate_limit:manage"},
		)
		cancelContext, cancel := context.WithCancel(requestContext.Request.Context())
		cancel()
		requestContext.Request = requestContext.Request.WithContext(cancelContext)

		srv.DeleteRateLimitExemption(requestContext, "rate-user-cancel-delete")
		if _, err := client.RateLimitExemption.Get(t.Context(), "rate-user-cancel-delete"); err != nil {
			t.Fatalf("exemption should remain after cancellation: %v", err)
		}
	})

	t.Run("update override", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "cancel_override")
		mustCreateRateLimitUser(t, client, "rate-user-cancel-override")
		requestContext, _ := newAuthedGinContext(
			t,
			http.MethodPut,
			"/admin/rate-limits/users/rate-user-cancel-override",
			`{"max_pending_parents":2}`,
			"rate-admin",
			[]string{"rate_limit:manage"},
		)
		cancelContext, cancel := context.WithCancel(requestContext.Request.Context())
		cancel()
		requestContext.Request = requestContext.Request.WithContext(cancelContext)

		srv.UpdateRateLimitUserOverrides(requestContext, "rate-user-cancel-override")
		if _, err := client.RateLimitUserOverride.Get(t.Context(), "rate-user-cancel-override"); !ent.IsNotFound(err) {
			t.Fatalf("override query error = %v, want not found after cancellation", err)
		}
	})
}

func TestRateLimitMutationHandlers_DatabaseMutationErrorsPreserveState(t *testing.T) {
	t.Parallel()

	forcedError := errors.New("forced rate-limit mutation failure")
	t.Run("create exemption", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "db_error_exemption_create")
		mustCreateRateLimitUser(t, client, "rate-user-db-create")
		client.RateLimitExemption.Use(failRateLimitMutation(forcedError))
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPost,
			"/admin/rate-limits/exemptions",
			`{"user_id":"rate-user-db-create","reason":"support"}`,
			"rate-admin",
			[]string{"rate_limit:manage"},
		)

		srv.CreateRateLimitExemption(requestContext)
		assertRateLimitInternalErrorResponse(t, response.Code, response.Body.Bytes())
		if _, err := client.RateLimitExemption.Get(t.Context(), "rate-user-db-create"); !ent.IsNotFound(err) {
			t.Fatalf("exemption query error = %v, want not found after failed save", err)
		}
	})

	t.Run("delete exemption", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "db_error_exemption_delete")
		mustCreateRateLimitUser(t, client, "rate-user-db-delete")
		_, err := client.RateLimitExemption.Create().
			SetID("rate-user-db-delete").
			SetExemptedBy("seed-admin").
			Save(t.Context())
		if err != nil {
			t.Fatalf("seed exemption: %v", err)
		}
		client.RateLimitExemption.Use(failRateLimitMutation(forcedError))
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodDelete,
			"/admin/rate-limits/exemptions/rate-user-db-delete",
			"",
			"rate-admin",
			[]string{"rate_limit:manage"},
		)

		srv.DeleteRateLimitExemption(requestContext, "rate-user-db-delete")
		assertRateLimitInternalErrorResponse(t, response.Code, response.Body.Bytes())
		persisted, err := client.RateLimitExemption.Get(t.Context(), "rate-user-db-delete")
		if err != nil {
			t.Fatalf("exemption should remain after failed delete: %v", err)
		}
		if persisted.ExemptedBy != "seed-admin" {
			t.Fatalf("persisted exempted_by = %q, want seed-admin", persisted.ExemptedBy)
		}
	})

	t.Run("create override", func(t *testing.T) {
		srv, client := newRateLimitMutationTestServer(t, "db_error_override")
		mustCreateRateLimitUser(t, client, "rate-user-db-override")
		client.RateLimitUserOverride.Use(failRateLimitMutation(forcedError))
		requestContext, response := newAuthedGinContext(
			t,
			http.MethodPut,
			"/admin/rate-limits/users/rate-user-db-override",
			`{"max_pending_children":3}`,
			"rate-admin",
			[]string{"rate_limit:manage"},
		)

		srv.UpdateRateLimitUserOverrides(requestContext, "rate-user-db-override")
		assertRateLimitInternalErrorResponse(t, response.Code, response.Body.Bytes())
		if _, err := client.RateLimitUserOverride.Get(t.Context(), "rate-user-db-override"); !ent.IsNotFound(err) {
			t.Fatalf("override query error = %v, want not found after failed save", err)
		}
	})
}

func TestCreateRateLimitExemption_ConcurrentUpsertsAllSucceedWithOneFinalRow(t *testing.T) {
	t.Parallel()

	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "exemption_concurrent_upsert")
	mustCreateRateLimitUser(t, client, "rate-user-concurrent")

	const requestCount = 2
	releaseGuard, blockerPID := holdUserMutationGuard(t, srv.pool, "rate-user-concurrent")

	firstContext, firstResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"rate-user-concurrent","reason":"first request"}`,
		"rate-admin-first",
		[]string{"rate_limit:manage"},
	)
	secondContext, secondResponse := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/rate-limits/exemptions",
		`{"user_id":"rate-user-concurrent","reason":"second request"}`,
		"rate-admin-second",
		[]string{"rate_limit:manage"},
	)

	firstDone := runHandlerAsync(func() { srv.CreateRateLimitExemption(firstContext) })
	secondDone := runHandlerAsync(func() { srv.CreateRateLimitExemption(secondContext) })

	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, requestCount)
	releaseGuard()
	waitForHandlerCompletion(t, firstDone, "first concurrent rate-limit exemption upsert")
	waitForHandlerCompletion(t, secondDone, "second concurrent rate-limit exemption upsert")

	rows, err := client.RateLimitExemption.Query().All(t.Context())
	if err != nil {
		t.Fatalf("query final exemptions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("final exemption count = %d, want 1", len(rows))
	}
	if rows[0].ID != "rate-user-concurrent" {
		t.Fatalf("final exemption ID = %q, want rate-user-concurrent", rows[0].ID)
	}
	firstWon := rows[0].Reason == "first request" && rows[0].ExemptedBy == "rate-admin-first"
	secondWon := rows[0].Reason == "second request" && rows[0].ExemptedBy == "rate-admin-second"
	if !firstWon && !secondWon {
		t.Fatalf("final exemption mixes concurrent requests: %+v", rows[0])
	}
	if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
		t.Fatalf(
			"concurrent upsert statuses = (%d, %d), want (%d, %d); bodies=(%s, %s)",
			firstResponse.Code,
			secondResponse.Code,
			http.StatusOK,
			http.StatusOK,
			firstResponse.Body.String(),
			secondResponse.Body.String(),
		)
	}
}

func TestUpdateRateLimitUserOverrides_ConcurrentFirstUpsertsAllSucceedWithOneFinalRow(t *testing.T) {
	t.Parallel()

	srv, client, _ := newAdminIdentityTestServerWithAuthSessions(t, "override_concurrent_upsert")
	mustCreateRateLimitUser(t, client, "rate-user-override-concurrent")

	const requestCount = 2
	releaseGuard, blockerPID := holdUserMutationGuard(t, srv.pool, "rate-user-override-concurrent")

	firstContext, firstResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-override-concurrent",
		`{"max_pending_parents":2,"max_pending_children":3,"cooldown_seconds":4,"reason":"first request"}`,
		"rate-admin-first",
		[]string{"rate_limit:manage"},
	)
	secondContext, secondResponse := newAuthedGinContext(
		t,
		http.MethodPut,
		"/admin/rate-limits/users/rate-user-override-concurrent",
		`{"max_pending_parents":5,"max_pending_children":6,"cooldown_seconds":7,"reason":"second request"}`,
		"rate-admin-second",
		[]string{"rate_limit:manage"},
	)

	firstDone := runHandlerAsync(func() {
		srv.UpdateRateLimitUserOverrides(firstContext, "rate-user-override-concurrent")
	})
	secondDone := runHandlerAsync(func() {
		srv.UpdateRateLimitUserOverrides(secondContext, "rate-user-override-concurrent")
	})

	waitForBlockedAdvisoryCalls(t, srv.pool, blockerPID, requestCount)
	releaseGuard()
	waitForHandlerCompletion(t, firstDone, "first concurrent rate-limit override upsert")
	waitForHandlerCompletion(t, secondDone, "second concurrent rate-limit override upsert")

	rows, err := client.RateLimitUserOverride.Query().All(t.Context())
	if err != nil {
		t.Fatalf("query final overrides: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("final override count = %d, want 1", len(rows))
	}
	row := rows[0]
	firstWon := rateLimitOverrideMatches(row, 2, 3, 4, "first request", "rate-admin-first")
	secondWon := rateLimitOverrideMatches(row, 5, 6, 7, "second request", "rate-admin-second")
	if !firstWon && !secondWon {
		t.Fatalf("final override mixes concurrent requests: %+v", row)
	}
	if firstResponse.Code != http.StatusOK || secondResponse.Code != http.StatusOK {
		t.Fatalf(
			"concurrent override upsert statuses = (%d, %d), want (%d, %d); bodies=(%s, %s)",
			firstResponse.Code,
			secondResponse.Code,
			http.StatusOK,
			http.StatusOK,
			firstResponse.Body.String(),
			secondResponse.Body.String(),
		)
	}
}

func newRateLimitMutationTestServer(t *testing.T, prefix string) (*Server, *ent.Client) {
	t.Helper()
	client := testutil.OpenEntPostgres(t, "admin_rate_limit_"+prefix)
	return NewServer(ServerDeps{EntClient: client}), client
}

func rateLimitOverrideMatches(override *ent.RateLimitUserOverride, parents, children, cooldown int, reason, actor string) bool {
	return override.MaxPendingParents != nil && *override.MaxPendingParents == parents &&
		override.MaxPendingChildren != nil && *override.MaxPendingChildren == children &&
		override.CooldownSeconds != nil && *override.CooldownSeconds == cooldown &&
		override.Reason == reason && override.UpdatedBy == actor
}

func mustCreateRateLimitUser(t *testing.T, client *ent.Client, id string) *ent.User {
	t.Helper()
	user, err := client.User.Create().
		SetID(id).
		SetUsername(id).
		SetDisplayName("Rate User").
		SetEmail(id + "@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create rate-limit user %q: %v", id, err)
	}
	return user
}

func assertRateLimitOverrideValues(
	t *testing.T,
	override *ent.RateLimitUserOverride,
	wantParents int,
	wantChildren int,
	wantCooldown int,
	wantReason string,
	wantActor string,
) {
	t.Helper()
	if override.MaxPendingParents == nil || *override.MaxPendingParents != wantParents {
		t.Fatalf("max_pending_parents = %v, want %d", override.MaxPendingParents, wantParents)
	}
	if override.MaxPendingChildren == nil || *override.MaxPendingChildren != wantChildren {
		t.Fatalf("max_pending_children = %v, want %d", override.MaxPendingChildren, wantChildren)
	}
	if override.CooldownSeconds == nil || *override.CooldownSeconds != wantCooldown {
		t.Fatalf("cooldown_seconds = %v, want %d", override.CooldownSeconds, wantCooldown)
	}
	if override.Reason != wantReason || override.UpdatedBy != wantActor {
		t.Fatalf("reason/updated_by = (%q, %q), want (%q, %q)", override.Reason, override.UpdatedBy, wantReason, wantActor)
	}
}

func assertRateLimitForbiddenResponse(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want %d body=%s", status, http.StatusForbidden, string(body))
	}
	assertErrorCode(t, body, "FORBIDDEN")
}

func assertRateLimitInternalErrorResponse(t *testing.T, status int, body []byte) {
	t.Helper()
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", status, http.StatusInternalServerError, string(body))
	}
	assertErrorCode(t, body, "INTERNAL_ERROR")
}

func failRateLimitMutation(err error) ent.Hook {
	return func(ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(context.Context, ent.Mutation) (ent.Value, error) {
			return nil, err
		})
	}
}
