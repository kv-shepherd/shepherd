package jobs

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type workerDirectorySyncAdapter struct {
	typeKey   string
	listUsers []provider.DirectoryUserRecord
}

func (a *workerDirectorySyncAdapter) Type() string { return a.typeKey }

func (a *workerDirectorySyncAdapter) ValidateConfig(map[string]interface{}) error { return nil }

func (a *workerDirectorySyncAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}

func (a *workerDirectorySyncAdapter) SampleFields(context.Context, map[string]interface{}) ([]provider.AuthProviderSampleField, error) {
	return nil, nil
}

func (a *workerDirectorySyncAdapter) DescribeDirectorySync() provider.DirectorySyncDescriptor {
	return provider.DirectorySyncDescriptor{
		DisplayName:     "Worker Directory Sync",
		SupportsPreview: true,
	}
}

func (a *workerDirectorySyncAdapter) PreviewDirectorySync(context.Context, map[string]interface{}, map[string]interface{}) (*provider.DirectorySyncPreview, error) {
	items := make([]provider.DirectoryPreviewItem, 0, len(a.listUsers))
	for _, record := range a.listUsers {
		items = append(items, provider.DirectoryPreviewItem{Record: record})
	}
	return &provider.DirectorySyncPreview{
		TotalCount: len(items),
		Items:      items,
	}, nil
}

func (a *workerDirectorySyncAdapter) ListDirectoryUsers(context.Context, map[string]interface{}, map[string]interface{}) ([]provider.DirectoryUserRecord, error) {
	return a.listUsers, nil
}

func TestDirectorySyncWorker_ClassifiesConflictsAndUpdatesCounters(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	client := testutil.OpenEntPostgres(t, "directory_sync_worker")

	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
		listUsers: []provider.DirectoryUserRecord{
			{
				ExternalID:  "ext-imported-1",
				Username:    "updated-imported",
				DisplayName: "Updated Imported",
				Email:       "updated-imported@example.com",
				Attributes: map[string]interface{}{
					"source": "directory-update",
				},
			},
			{
				ExternalID:  "ext-username-conflict",
				Username:    "taken-user",
				DisplayName: "Taken Username",
				Email:       "new-username-conflict@example.com",
			},
			{
				ExternalID:  "ext-email-conflict",
				Username:    "unique-email-conflict",
				DisplayName: "Taken Email",
				Email:       "duplicate@example.com",
			},
			{
				ExternalID:  "ext-new-user",
				Username:    "new-directory-user",
				DisplayName: "New Directory User",
				Email:       "new-directory-user@example.com",
				Attributes: map[string]interface{}{
					"department": "ops",
				},
			},
		},
	})

	providerID := "auth-provider-directory-worker"
	if _, createErr := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Directory Worker Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create auth provider: %v", createErr)
	}
	if _, createErr := client.User.Create().
		SetID("existing-imported-user").
		SetUsername("imported-user").
		SetDisplayName("Imported User").
		SetEmail("imported@example.com").
		SetAuthProviderID(providerID).
		SetExternalID("ext-imported-1").
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create existing imported user: %v", createErr)
	}
	if _, createErr := client.User.Create().
		SetID("existing-username-user").
		SetUsername("taken-user").
		SetDisplayName("Taken Username").
		SetEmail("taken-user@example.com").
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create username conflict user: %v", createErr)
	}
	if _, createErr := client.User.Create().
		SetID("existing-email-user").
		SetUsername("existing-email-user").
		SetDisplayName("Taken Email").
		SetEmail("duplicate@example.com").
		SetEnabled(true).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create email conflict user: %v", createErr)
	}
	if _, createErr := client.DirectorySyncJob.Create().
		SetID("directory-sync-job-1").
		SetAuthProviderID(providerID).
		SetRequestSnapshot(map[string]interface{}{"opaque": "payload"}).
		SetConflictResolution(service.DirectoryConflictResolutionSkip).
		SetTriggeredBy("admin-1").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create directory sync job: %v", createErr)
	}

	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))
	if err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{
			AuthProviderID: providerID,
			JobID:          "directory-sync-job-1",
		},
	}); err != nil {
		t.Fatalf("worker work: %v", err)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), "directory-sync-job-1")
	if err != nil {
		t.Fatalf("get directory sync job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusCompleted {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusCompleted)
	}
	if jobRow.TotalEntries != 4 || jobRow.CreateCount != 1 || jobRow.UpdateCount != 1 || jobRow.BlockedCount != 2 || jobRow.ErrorCount != 0 {
		t.Fatalf(
			"job counters = total:%d create:%d update:%d blocked:%d errors:%d, want total:4 create:1 update:1 blocked:2 errors:0",
			jobRow.TotalEntries,
			jobRow.CreateCount,
			jobRow.UpdateCount,
			jobRow.BlockedCount,
			jobRow.ErrorCount,
		)
	}

	importedUser, err := client.User.Query().
		Where(
			user.AuthProviderIDEQ(providerID),
			user.ExternalIDEQ("ext-imported-1"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query updated imported user: %v", err)
	}
	if importedUser.Username != "updated-imported" || importedUser.Email != "updated-imported@example.com" {
		t.Fatalf("updated imported user = %#v", importedUser)
	}

	newUser, err := client.User.Query().
		Where(
			user.AuthProviderIDEQ(providerID),
			user.ExternalIDEQ("ext-new-user"),
		).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query new synced user: %v", err)
	}
	if newUser.Username != "new-directory-user" {
		t.Fatalf("new synced username = %q, want %q", newUser.Username, "new-directory-user")
	}

	newProfile, err := client.UserDirectoryProfile.Query().
		Where(userdirectoryprofile.UserIDEQ(newUser.ID)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query new user directory profile: %v", err)
	}
	if got := newProfile.Attributes["department"]; got != "ops" {
		t.Fatalf("new profile department = %#v, want %q", got, "ops")
	}

	usernameConflictCount, err := client.User.Query().
		Where(user.UsernameEQ("taken-user")).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count taken-user: %v", err)
	}
	if usernameConflictCount != 1 {
		t.Fatalf("taken-user count = %d, want 1", usernameConflictCount)
	}
}

func TestDirectorySyncWorker_ScheduledEnrichmentUpdatesExistingUsersOnly(t *testing.T) {
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

	client := testutil.OpenEntPostgres(t, "directory_enrichment_worker")

	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
		listUsers: []provider.DirectoryUserRecord{
			{
				ExternalID:  "ext-existing-user",
				Username:    "existing-user",
				DisplayName: "Existing User",
				Attributes: map[string]interface{}{
					"department": "ops",
					"phone":      "13800000000",
				},
				Cohorts: []provider.ExternalCohort{
					{Kind: "group", Key: "ops", DisplayName: "ops"},
				},
			},
			{
				ExternalID:  "ext-missing-user",
				Username:    "missing-user",
				DisplayName: "Missing User",
				Attributes: map[string]interface{}{
					"department": "finance",
				},
				Cohorts: []provider.ExternalCohort{
					{Kind: "group", Key: "finance", DisplayName: "finance"},
				},
			},
		},
	})

	providerID := "auth-provider-directory-enrichment-worker"
	if _, createErr := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Directory Enrichment Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create auth provider: %v", createErr)
	}
	existingUser, err := client.User.Create().
		SetID("existing-user-id").
		SetUsername("existing-user").
		SetDisplayName("Existing User").
		SetEmail("existing-user@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing canonical user: %v", err)
	}
	if _, createErr := client.DirectorySyncJob.Create().
		SetID("directory-enrichment-job-1").
		SetAuthProviderID(providerID).
		SetRequestSnapshot(map[string]interface{}{"opaque": "payload"}).
		SetConflictResolution(service.DirectoryConflictResolutionSkip).
		SetSyncMode(service.DirectoryExecutionModeScheduledEnrichment).
		SetJoinKeyType(string(provider.DirectoryJoinKeyUsername)).
		SetTriggeredBy("system:directory-enrichment-scheduler").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create directory enrichment job: %v", createErr)
	}

	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))
	if workerErr := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{
			AuthProviderID: providerID,
			JobID:          "directory-enrichment-job-1",
		},
	}); workerErr != nil {
		t.Fatalf("worker work: %v", workerErr)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), "directory-enrichment-job-1")
	if err != nil {
		t.Fatalf("get directory enrichment job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusCompleted {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusCompleted)
	}
	if jobRow.TotalEntries != 2 || jobRow.CreateCount != 0 || jobRow.UpdateCount != 1 || jobRow.BlockedCount != 1 || jobRow.ErrorCount != 0 {
		t.Fatalf(
			"job counters = total:%d create:%d update:%d blocked:%d errors:%d, want total:2 create:0 update:1 blocked:1 errors:0",
			jobRow.TotalEntries,
			jobRow.CreateCount,
			jobRow.UpdateCount,
			jobRow.BlockedCount,
			jobRow.ErrorCount,
		)
	}

	profile, err := client.UserDirectoryProfile.Query().
		Where(userdirectoryprofile.UserIDEQ(existingUser.ID)).
		Only(t.Context())
	if err != nil {
		t.Fatalf("query existing user directory profile: %v", err)
	}
	if got := profile.Attributes["department"]; got != "ops" {
		t.Fatalf("profile department = %#v, want %q", got, "ops")
	}

	missingCount, err := client.User.Query().
		Where(user.UsernameEQ("missing-user")).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count missing-user: %v", err)
	}
	if missingCount != 0 {
		t.Fatalf("missing-user count = %d, want 0", missingCount)
	}
}

func registerWorkerDirectorySyncAdapter(t *testing.T, adapter *workerDirectorySyncAdapter) *workerDirectorySyncAdapter {
	t.Helper()
	if adapter == nil {
		t.Fatal("adapter is nil")
	}
	if adapter.typeKey == "" {
		adapter.typeKey = "test-worker-directory-sync-" + uuid.NewString()
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register worker directory sync adapter: %v", err)
	}
	return adapter
}
