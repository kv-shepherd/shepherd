package handlers

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"kv-shepherd.io/shepherd/internal/api/generated"
	"kv-shepherd.io/shepherd/internal/provider"
)

type testDirectorySyncAdapter struct {
	typeKey        string
	lastRequest    map[string]interface{}
	preview        *provider.DirectorySyncPreview
	previewErr     error
	listUsers      []provider.DirectoryUserRecord
	listUsersErr   error
	requestSchema  map[string]interface{}
	supportPreview bool
}

func (a *testDirectorySyncAdapter) Type() string { return a.typeKey }

func (a *testDirectorySyncAdapter) ValidateConfig(map[string]interface{}) error { return nil }

func (a *testDirectorySyncAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}

func (a *testDirectorySyncAdapter) SampleFields(context.Context, map[string]interface{}) ([]provider.AuthProviderSampleField, error) {
	return nil, nil
}

func (a *testDirectorySyncAdapter) DescribeDirectorySync() provider.DirectorySyncDescriptor {
	return provider.DirectorySyncDescriptor{
		DisplayName:     "Test Directory Sync",
		Description:     "test capability",
		RequestSchema:   a.requestSchema,
		SupportsPreview: a.supportPreview,
	}
}

func (a *testDirectorySyncAdapter) PreviewDirectorySync(
	_ context.Context,
	_ map[string]interface{},
	providerRequest map[string]interface{},
) (*provider.DirectorySyncPreview, error) {
	a.lastRequest = providerRequest
	if a.previewErr != nil {
		return nil, a.previewErr
	}
	return a.preview, nil
}

func (a *testDirectorySyncAdapter) ListDirectoryUsers(context.Context, map[string]interface{}, map[string]interface{}) ([]provider.DirectoryUserRecord, error) {
	return a.listUsers, a.listUsersErr
}

type testScheduledDirectorySyncAdapter struct {
	*testDirectorySyncAdapter
	plan    *provider.ScheduledDirectoryEnrichmentPlan
	planErr error
}

func (a *testScheduledDirectorySyncAdapter) BuildScheduledDirectoryEnrichmentPlan(context.Context, map[string]interface{}) (*provider.ScheduledDirectoryEnrichmentPlan, error) {
	if a.planErr != nil {
		return nil, a.planErr
	}
	return a.plan, nil
}

func TestPreviewAuthProviderDirectoryUsesOpaqueProviderRequest(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		requestSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"vendor_filter": map[string]interface{}{"type": "object"},
			},
		},
		supportPreview: true,
		preview: &provider.DirectorySyncPreview{
			TotalCount: 1,
			Items: []provider.DirectoryPreviewItem{
				{
					Record: provider.DirectoryUserRecord{
						ExternalID:  "ext-opaque-1",
						Username:    "opaque-user",
						DisplayName: "Opaque User",
						Email:       "existing@example.com",
						Cohorts: []provider.ExternalCohort{
							{Kind: "department", Key: "dept-a", DisplayName: "Engineering"},
						},
						Attributes: map[string]interface{}{
							"vendor_field": "kept-for-audit",
						},
					},
					Warnings: []string{"provider warning preserved"},
					Conflicts: []provider.DirectoryConflict{
						{
							Code: provider.DirectoryConflictUsernameConflict,
						},
					},
				},
			},
		},
	})

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-preview").
		SetName("Directory Preview Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	if _, err := client.User.Create().
		SetID("existing-email-user").
		SetUsername("existing-email-user").
		SetEmail("existing@example.com").
		SetDisplayName("Existing Email").
		SetAuthProviderID("other-provider").
		SetExternalID("other-external-id").
		SetEnabled(true).
		Save(t.Context()); err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	previewCtx, previewW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/directory/preview",
		`{"provider_request":{"vendor_filter":{"department_ids":["dept-a"],"cursor":"cursor-1"}}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.PreviewAuthProviderDirectory(previewCtx, providerRow.ID)
	if previewW.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d, body=%s", previewW.Code, http.StatusOK, previewW.Body.String())
	}

	wantRequest := map[string]interface{}{
		"vendor_filter": map[string]interface{}{
			"department_ids": []interface{}{"dept-a"},
			"cursor":         "cursor-1",
		},
	}
	if !deepEqualJSONMap(adapter.lastRequest, wantRequest) {
		t.Fatalf("provider_request = %#v, want %#v", adapter.lastRequest, wantRequest)
	}

	var previewResp generated.DirectorySyncPreview
	mustDecodeJSON(t, previewW.Body.Bytes(), &previewResp)
	if previewResp.TotalCount != 1 {
		t.Fatalf("total_count = %d, want 1", previewResp.TotalCount)
	}
	if len(previewResp.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(previewResp.Items))
	}
	if got := previewResp.Items[0].Warnings; len(got) != 1 || got[0] != "provider warning preserved" {
		t.Fatalf("warnings = %#v, want preserved provider warning", got)
	}
	if len(previewResp.Items[0].Conflicts) != 1 {
		t.Fatalf("conflicts len = %d, want 1", len(previewResp.Items[0].Conflicts))
	}
	if previewResp.Items[0].Match.Action != generated.Blocked {
		t.Fatalf("match.action = %q, want %q", previewResp.Items[0].Match.Action, generated.Blocked)
	}
	if len(previewResp.Items[0].Record.Cohorts) != 1 {
		t.Fatalf("record cohorts len = %d, want 1", len(previewResp.Items[0].Record.Cohorts))
	}
	if previewResp.Items[0].Record.Cohorts[0].Kind != "department" || previewResp.Items[0].Record.Cohorts[0].Key != "dept-a" {
		t.Fatalf("record cohort = %#v, want department:dept-a", previewResp.Items[0].Record.Cohorts[0])
	}
	if previewResp.Items[0].Conflicts[0].Code != generated.EmailConflict {
		t.Fatalf("conflict code = %q, want %q", previewResp.Items[0].Conflicts[0].Code, generated.EmailConflict)
	}
}

func TestPreviewAuthProviderDirectory_MatchActionCreateForNewUser(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
		preview: &provider.DirectorySyncPreview{
			TotalCount: 1,
			Items: []provider.DirectoryPreviewItem{
				{
					Record: provider.DirectoryUserRecord{
						ExternalID:  "ext-create-1",
						Username:    "fresh-user",
						DisplayName: "Fresh User",
						Email:       "fresh-user@example.com",
					},
				},
			},
		},
	})

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-create").
		SetName("Directory Create Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	previewCtx, previewW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/directory/preview",
		`{"provider_request":{"vendor_filter":{"tenant":"acme"}}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.PreviewAuthProviderDirectory(previewCtx, providerRow.ID)
	if previewW.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d, body=%s", previewW.Code, http.StatusOK, previewW.Body.String())
	}

	var previewResp generated.DirectorySyncPreview
	mustDecodeJSON(t, previewW.Body.Bytes(), &previewResp)
	if len(previewResp.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(previewResp.Items))
	}
	if previewResp.Items[0].Match.Action != generated.Create {
		t.Fatalf("match.action = %q, want %q", previewResp.Items[0].Match.Action, generated.Create)
	}
	if previewResp.Items[0].Match.ExistingUserId != "" {
		t.Fatalf("existing_user_id = %q, want empty", previewResp.Items[0].Match.ExistingUserId)
	}
}

func TestPreviewAuthProviderDirectory_MatchActionUpdateForSameExternalIdentity(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
		preview: &provider.DirectorySyncPreview{
			TotalCount: 1,
			Items: []provider.DirectoryPreviewItem{
				{
					Record: provider.DirectoryUserRecord{
						ExternalID:  "ext-update-1",
						Username:    "managed-user",
						DisplayName: "Managed User",
						Email:       "managed-user@example.com",
					},
				},
			},
		},
	})

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-update").
		SetName("Directory Update Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	existingUser, err := client.User.Create().
		SetID("existing-managed-user").
		SetUsername("managed-user").
		SetEmail("managed-user@example.com").
		SetDisplayName("Managed User").
		SetAuthProviderID(providerRow.ID).
		SetExternalID("ext-update-1").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	previewCtx, previewW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/directory/preview",
		`{"provider_request":{"vendor_filter":{"tenant":"acme"}}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.PreviewAuthProviderDirectory(previewCtx, providerRow.ID)
	if previewW.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want %d, body=%s", previewW.Code, http.StatusOK, previewW.Body.String())
	}

	var previewResp generated.DirectorySyncPreview
	mustDecodeJSON(t, previewW.Body.Bytes(), &previewResp)
	if len(previewResp.Items) != 1 {
		t.Fatalf("preview items len = %d, want 1", len(previewResp.Items))
	}
	if previewResp.Items[0].Match.Action != generated.Update {
		t.Fatalf("match.action = %q, want %q", previewResp.Items[0].Match.Action, generated.Update)
	}
	if previewResp.Items[0].Match.ExistingUserId != existingUser.ID {
		t.Fatalf("existing_user_id = %q, want %q", previewResp.Items[0].Match.ExistingUserId, existingUser.ID)
	}
	if previewResp.Items[0].Match.MatchedBy != generated.ExternalId {
		t.Fatalf("matched_by = %q, want %q", previewResp.Items[0].Match.MatchedBy, generated.ExternalId)
	}
}

func TestGetAuthProviderDirectoryDescriptor_Returns501WhenCapabilityMissing(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-unsupported").
		SetName("Unsupported Directory Provider").
		SetAuthType("oidc").
		SetConfig(map[string]interface{}{"issuer_url": "https://issuer.example.com"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	descriptorCtx, descriptorW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/directory/descriptor",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderDirectoryDescriptor(descriptorCtx, providerRow.ID)
	if descriptorW.Code != http.StatusNotImplemented {
		t.Fatalf("descriptor status = %d, want %d, body=%s", descriptorW.Code, http.StatusNotImplemented, descriptorW.Body.String())
	}
}

func TestPreviewAuthProviderDirectory_MapsProviderRequestValidationErrorToBadRequest(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
		previewErr:     provider.NewDirectorySyncRequestError("missing vendor selector"),
	})
	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-invalid-request").
		SetName("Invalid Directory Request Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	previewCtx, previewW := newAuthedGinContext(
		t,
		http.MethodPost,
		"/admin/auth-providers/"+providerRow.ID+"/directory/preview",
		`{"provider_request":{"vendor_filter":{}}}`,
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.PreviewAuthProviderDirectory(previewCtx, providerRow.ID)
	if previewW.Code != http.StatusBadRequest {
		t.Fatalf("preview status = %d, want %d, body=%s", previewW.Code, http.StatusBadRequest, previewW.Body.String())
	}
}

func TestGetAuthProviderDirectorySchedule_ReturnsUnsupportedWhenCapabilityMissing(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
	})
	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-schedule-unsupported").
		SetName("Directory Schedule Unsupported Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/directory/schedule",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderDirectorySchedule(reqCtx, providerRow.ID)
	if reqW.Code != http.StatusOK {
		t.Fatalf("schedule status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.DirectoryEnrichmentScheduleStatus
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if resp.Supported {
		t.Fatal("supported = true, want false")
	}
	if resp.Enabled {
		t.Fatal("enabled = true, want false")
	}
}

func TestGetAuthProviderDirectorySchedule_ComputesNextAndLastRun(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testScheduledDirectorySyncAdapter{
		testDirectorySyncAdapter: &testDirectorySyncAdapter{
			supportPreview: true,
		},
		plan: &provider.ScheduledDirectoryEnrichmentPlan{
			Enabled:          true,
			Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
			JoinKeyType:      provider.DirectoryJoinKeyUsername,
			ScheduleCron:     "0 * * * *",
			ScheduleTimezone: "UTC",
			ProviderRequest: map[string]interface{}{
				"department_names": []string{"Engineering", "Finance"},
				"include_nested":   true,
			},
		},
	})
	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-schedule-supported").
		SetName("Directory Schedule Supported Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	completedAt := time.Date(2026, 3, 21, 9, 5, 0, 0, time.UTC)
	createdAt := time.Date(2026, 3, 21, 9, 0, 0, 0, time.UTC)
	if _, err := client.DirectorySyncJob.Create().
		SetID("directory-schedule-job-1").
		SetAuthProviderID(providerRow.ID).
		SetRequestSnapshot(map[string]interface{}{}).
		SetConflictResolution("skip").
		SetSyncMode("scheduled_enrichment").
		SetJoinKeyType("username").
		SetTriggeredBy("system:directory-enrichment-scheduler").
		SetStatus("completed").
		SetCreatedAt(createdAt).
		SetCompletedAt(completedAt).
		Save(t.Context()); err != nil {
		t.Fatalf("create schedule job: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/directory/schedule",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderDirectorySchedule(reqCtx, providerRow.ID)
	if reqW.Code != http.StatusOK {
		t.Fatalf("schedule status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.DirectoryEnrichmentScheduleStatus
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if !resp.Supported || !resp.Enabled {
		t.Fatalf("supported/enabled = %#v, want both true", resp)
	}
	if resp.Mode != generated.EnrichExistingOnly {
		t.Fatalf("mode = %q, want %q", resp.Mode, generated.EnrichExistingOnly)
	}
	if resp.JoinKeyType != generated.Username {
		t.Fatalf("join_key_type = %q, want %q", resp.JoinKeyType, generated.Username)
	}
	if got, ok := resp.ProviderRequest["include_nested"].(bool); !ok || !got {
		t.Fatalf("provider_request include_nested = %#v, want true", resp.ProviderRequest["include_nested"])
	}
	if resp.LastJobId != "directory-schedule-job-1" {
		t.Fatalf("last_job_id = %q, want %q", resp.LastJobId, "directory-schedule-job-1")
	}
	if resp.LastJobStatus != generated.DirectoryEnrichmentScheduleStatusLastJobStatusCompleted {
		t.Fatalf("last_job_status = %q, want %q", resp.LastJobStatus, generated.DirectoryEnrichmentScheduleStatusLastJobStatusCompleted)
	}
	if !resp.LastJobCreatedAt.Equal(createdAt) {
		t.Fatalf("last_job_created_at = %s, want %s", resp.LastJobCreatedAt, createdAt)
	}
	if !resp.LastJobCompletedAt.Equal(completedAt) {
		t.Fatalf("last_job_completed_at = %s, want %s", resp.LastJobCompletedAt, completedAt)
	}
	wantNext := time.Date(2026, 3, 21, 10, 0, 0, 0, time.UTC)
	if !resp.NextRunAt.Equal(wantNext) {
		t.Fatalf("next_run_at = %s, want %s", resp.NextRunAt, wantNext)
	}
}

func TestListAuthProviderDirectorySyncJobs_UsesCanonicalResultSummary(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
	})
	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-job-summary").
		SetName("Directory Job Summary Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	if _, err := client.DirectorySyncJob.Create().
		SetID("directory-sync-job-summary-1").
		SetAuthProviderID(providerRow.ID).
		SetRequestSnapshot(map[string]interface{}{}).
		SetConflictResolution("skip").
		SetSyncMode("scheduled_enrichment").
		SetJoinKeyType("username").
		SetTriggeredBy("system:directory-enrichment-scheduler").
		SetStatus("completed").
		SetTotalEntries(5).
		SetCreateCount(1).
		SetUpdateCount(3).
		SetBlockedCount(1).
		SetErrorCount(0).
		Save(t.Context()); err != nil {
		t.Fatalf("create directory sync job: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/directory/sync-jobs",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.ListAuthProviderDirectorySyncJobs(reqCtx, providerRow.ID, generated.ListAuthProviderDirectorySyncJobsParams{})
	if reqW.Code != http.StatusOK {
		t.Fatalf("list sync jobs status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.DirectorySyncJobList
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if len(resp.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(resp.Items))
	}
	if resp.Items[0].ResultSummary.CreateCount != 1 || resp.Items[0].ResultSummary.UpdateCount != 3 || resp.Items[0].ResultSummary.BlockedCount != 1 {
		t.Fatalf("result_summary = %#v, want create:1 update:3 blocked:1", resp.Items[0].ResultSummary)
	}
}

func TestGetAuthProviderDirectorySyncJob_ReturnsRequestSnapshotAndCanonicalSummary(t *testing.T) {
	srv, client := newAdminIdentityTestServer(t)

	adapter := registerDirectorySyncTestAdapter(t, &testDirectorySyncAdapter{
		supportPreview: true,
	})
	providerRow, err := client.AuthProvider.Create().
		SetID("auth-provider-directory-job-detail").
		SetName("Directory Job Detail Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	if _, err := client.DirectorySyncJob.Create().
		SetID("directory-sync-job-detail-1").
		SetAuthProviderID(providerRow.ID).
		SetRequestSnapshot(map[string]interface{}{
			"department_ids": []string{"dept-a"},
			"include_nested": true,
		}).
		SetConflictResolution("skip").
		SetSyncMode("manual_import").
		SetTriggeredBy("admin-1").
		SetStatus("completed").
		SetTotalEntries(3).
		SetCreateCount(1).
		SetUpdateCount(1).
		SetBlockedCount(1).
		SetErrorCount(0).
		Save(t.Context()); err != nil {
		t.Fatalf("create directory sync job: %v", err)
	}

	reqCtx, reqW := newAuthedGinContext(
		t,
		http.MethodGet,
		"/admin/auth-providers/"+providerRow.ID+"/directory/sync-jobs/directory-sync-job-detail-1",
		"",
		"admin-1",
		[]string{"platform:admin"},
	)
	srv.GetAuthProviderDirectorySyncJob(reqCtx, providerRow.ID, "directory-sync-job-detail-1")
	if reqW.Code != http.StatusOK {
		t.Fatalf("get sync job status = %d, want %d, body=%s", reqW.Code, http.StatusOK, reqW.Body.String())
	}

	var resp generated.DirectorySyncJobDetail
	mustDecodeJSON(t, reqW.Body.Bytes(), &resp)
	if resp.ResultSummary.CreateCount != 1 || resp.ResultSummary.UpdateCount != 1 || resp.ResultSummary.BlockedCount != 1 {
		t.Fatalf("result_summary = %#v, want create:1 update:1 blocked:1", resp.ResultSummary)
	}
	if resp.Errors == nil || len(resp.Errors) != 0 {
		t.Fatalf("errors = %#v, want empty array", resp.Errors)
	}
	if got, ok := resp.RequestSnapshot["include_nested"].(bool); !ok || !got {
		t.Fatalf("request_snapshot include_nested = %#v, want true", resp.RequestSnapshot["include_nested"])
	}
}

func registerDirectorySyncTestAdapter[T provider.AuthProviderAdminAdapter](t *testing.T, adapter T) T {
	t.Helper()
	switch typed := any(adapter).(type) {
	case *testDirectorySyncAdapter:
		if typed == nil {
			t.Fatal("adapter is nil")
			return adapter
		}
		if typed.typeKey == "" {
			typed.typeKey = "test-directory-sync-" + uuid.NewString()
		}
	case *testScheduledDirectorySyncAdapter:
		if typed == nil || typed.testDirectorySyncAdapter == nil {
			t.Fatal("adapter is nil")
			return adapter
		}
		if typed.typeKey == "" {
			typed.typeKey = "test-directory-sync-" + uuid.NewString()
		}
	default:
		t.Fatalf("unsupported test adapter type %T", adapter)
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register test directory sync adapter: %v", err)
	}
	return adapter
}

func deepEqualJSONMap(left, right map[string]interface{}) bool {
	return reflect.DeepEqual(left, right)
}
