package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/ent/enttest"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	"kv-shepherd.io/shepherd/ent/rolebinding"
	"kv-shepherd.io/shepherd/ent/user"
	"kv-shepherd.io/shepherd/ent/userdirectoryprofile"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type workerDirectorySyncAdapter struct {
	typeKey   string
	listUsers []provider.DirectoryUserRecord
	listErr   error
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
	if a.listErr != nil {
		return nil, a.listErr
	}
	return a.listUsers, nil
}

type workerUnsupportedDirectorySyncAdapter struct {
	typeKey string
}

func (a workerUnsupportedDirectorySyncAdapter) Type() string { return a.typeKey }

func (a workerUnsupportedDirectorySyncAdapter) ValidateConfig(map[string]interface{}) error {
	return nil
}

func (a workerUnsupportedDirectorySyncAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}

func (a workerUnsupportedDirectorySyncAdapter) SampleFields(context.Context, map[string]interface{}) ([]provider.AuthProviderSampleField, error) {
	return nil, nil
}

func createDirectorySyncWorkerAuthProvider(t *testing.T, client *ent.Client, providerID, authType string) {
	t.Helper()
	if _, err := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Directory Worker Failure Provider " + uuid.NewString()[:8]).
		SetAuthType(authType).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
}

func createDirectorySyncWorkerJob(t *testing.T, client *ent.Client, jobID, providerID string) {
	t.Helper()
	if _, err := client.DirectorySyncJob.Create().
		SetID(jobID).
		SetAuthProviderID(providerID).
		SetRequestSnapshot(map[string]interface{}{"opaque": "payload"}).
		SetConflictResolution(service.DirectoryConflictResolutionSkip).
		SetTriggeredBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create directory sync job: %v", err)
	}
}

func TestDirectorySyncWorker_ClassifiesConflictsAndUpdatesCounters(t *testing.T) {
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
			JobID: "directory-sync-job-1",
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

func TestDirectorySyncWorker_FailurePathsPersistJobFailure(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, client *ent.Client) (jobID string, wantError string)
		verify func(t *testing.T, client *ent.Client)
	}{
		{
			name: "missing_auth_provider",
			setup: func(t *testing.T, client *ent.Client) (string, string) {
				jobID := "directory-sync-missing-provider-" + uuid.NewString()
				createDirectorySyncWorkerJob(t, client, jobID, "auth-provider-missing-"+uuid.NewString())
				return jobID, "load auth provider"
			},
		},
		{
			name: "disabled_auth_provider",
			setup: func(t *testing.T, client *ent.Client) (string, string) {
				adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
					listUsers: []provider.DirectoryUserRecord{
						{
							ExternalID:  "ext-disabled-provider-user",
							Username:    "disabled-provider-user",
							DisplayName: "Disabled Provider User",
							Email:       "disabled-provider-user@example.com",
						},
					},
				})
				providerID := "auth-provider-disabled-" + uuid.NewString()
				if _, err := client.AuthProvider.Create().
					SetID(providerID).
					SetName("Directory Worker Disabled Provider " + uuid.NewString()[:8]).
					SetAuthType(adapter.typeKey).
					SetConfig(map[string]interface{}{"tenant": "acme"}).
					SetEnabled(false).
					SetCreatedBy("admin-1").
					Save(t.Context()); err != nil {
					t.Fatalf("create disabled auth provider: %v", err)
				}
				jobID := "directory-sync-disabled-provider-" + uuid.NewString()
				createDirectorySyncWorkerJob(t, client, jobID, providerID)
				return jobID, "is disabled"
			},
			verify: func(t *testing.T, client *ent.Client) {
				t.Helper()
				count, err := client.User.Query().
					Where(user.UsernameEQ("disabled-provider-user")).
					Count(t.Context())
				if err != nil {
					t.Fatalf("count disabled-provider-user: %v", err)
				}
				if count != 0 {
					t.Fatalf("disabled provider created %d users, want 0", count)
				}
			},
		},
		{
			name: "unregistered_adapter",
			setup: func(t *testing.T, client *ent.Client) (string, string) {
				providerID := "auth-provider-unregistered-" + uuid.NewString()
				createDirectorySyncWorkerAuthProvider(t, client, providerID, "missing-adapter-"+uuid.NewString())
				jobID := "directory-sync-unregistered-" + uuid.NewString()
				createDirectorySyncWorkerJob(t, client, jobID, providerID)
				return jobID, "no auth provider adapter registered"
			},
		},
		{
			name: "unsupported_directory_sync",
			setup: func(t *testing.T, client *ent.Client) (string, string) {
				adapter := workerUnsupportedDirectorySyncAdapter{typeKey: "test-worker-unsupported-directory-sync-" + uuid.NewString()}
				if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
					t.Fatalf("register unsupported adapter: %v", err)
				}
				providerID := "auth-provider-unsupported-" + uuid.NewString()
				createDirectorySyncWorkerAuthProvider(t, client, providerID, adapter.typeKey)
				jobID := "directory-sync-unsupported-" + uuid.NewString()
				createDirectorySyncWorkerJob(t, client, jobID, providerID)
				return jobID, "does not support directory sync"
			},
		},
		{
			name: "list_directory_users_failure",
			setup: func(t *testing.T, client *ent.Client) (string, string) {
				adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
					listErr: errors.New("directory source unavailable"),
				})
				providerID := "auth-provider-list-failure-" + uuid.NewString()
				createDirectorySyncWorkerAuthProvider(t, client, providerID, adapter.typeKey)
				jobID := "directory-sync-list-failure-" + uuid.NewString()
				createDirectorySyncWorkerJob(t, client, jobID, providerID)
				return jobID, "list directory users"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.OpenEntPostgres(t, "directory_sync_failure_"+tc.name)
			jobID, wantError := tc.setup(t, client)
			worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))

			if err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
				Args: DirectorySyncArgs{JobID: jobID},
			}); err != nil {
				t.Fatalf("Work() error = %v, want nil after persisted job failure", err)
			}

			jobRow, err := client.DirectorySyncJob.Get(t.Context(), jobID)
			if err != nil {
				t.Fatalf("get failed directory sync job: %v", err)
			}
			if jobRow.Status != directorysyncjob.StatusFailed {
				t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusFailed)
			}
			if jobRow.StartedAt == nil {
				t.Fatal("StartedAt = nil, want running marker before failure")
			}
			if jobRow.CompletedAt == nil {
				t.Fatal("CompletedAt = nil, want failure completion timestamp")
			}
			if jobRow.ErrorCount != 1 {
				t.Fatalf("ErrorCount = %d, want 1", jobRow.ErrorCount)
			}
			if len(jobRow.Errors) != 1 || !strings.Contains(jobRow.Errors[0], wantError) {
				t.Fatalf("Errors = %#v, want one error containing %q", jobRow.Errors, wantError)
			}
			if tc.verify != nil {
				tc.verify(t, client)
			}
		})
	}
}

func TestDirectorySyncWorker_ContextCancellationDoesNotPersistJobFailure(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "directory_sync_context_canceled")
	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
		listErr: errors.Join(errors.New("directory source interrupted"), context.Canceled),
	})
	providerID := "auth-provider-context-canceled-" + uuid.NewString()
	createDirectorySyncWorkerAuthProvider(t, client, providerID, adapter.typeKey)
	jobID := "directory-sync-context-canceled-" + uuid.NewString()
	createDirectorySyncWorkerJob(t, client, jobID, providerID)
	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))

	err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{JobID: jobID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("get directory sync job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusRunning {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusRunning)
	}
	if jobRow.StartedAt == nil {
		t.Fatal("StartedAt = nil, want running marker")
	}
	if jobRow.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", *jobRow.CompletedAt)
	}
	if jobRow.ErrorCount != 0 {
		t.Fatalf("ErrorCount = %d, want 0", jobRow.ErrorCount)
	}
	if len(jobRow.Errors) != 0 {
		t.Fatalf("Errors = %#v, want empty", jobRow.Errors)
	}
}

func TestDirectorySyncWorker_MarkRunningCancellationReturnsContextError(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "directory_sync_mark_running_canceled")
	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{})
	providerID := "auth-provider-mark-running-canceled-" + uuid.NewString()
	createDirectorySyncWorkerAuthProvider(t, client, providerID, adapter.typeKey)
	jobID := "directory-sync-mark-running-canceled-" + uuid.NewString()
	createDirectorySyncWorkerJob(t, client, jobID, providerID)
	client.DirectorySyncJob.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("mark running interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))

	err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{JobID: jobID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("get directory sync job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusPending {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusPending)
	}
	if jobRow.StartedAt != nil {
		t.Fatalf("StartedAt = %v, want nil", *jobRow.StartedAt)
	}
	if jobRow.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", *jobRow.CompletedAt)
	}
}

func TestDirectorySyncWorker_CompleteCancellationReturnsContextError(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "directory_sync_complete_canceled")
	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{})
	providerID := "auth-provider-complete-canceled-" + uuid.NewString()
	createDirectorySyncWorkerAuthProvider(t, client, providerID, adapter.typeKey)
	jobID := "directory-sync-complete-canceled-" + uuid.NewString()
	createDirectorySyncWorkerJob(t, client, jobID, providerID)
	updateCount := 0
	client.DirectorySyncJob.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.Join(errors.New("complete interrupted"), context.Canceled)
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdateOne))
	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))

	err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{JobID: jobID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("get directory sync job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusRunning {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusRunning)
	}
	if jobRow.StartedAt == nil {
		t.Fatal("StartedAt = nil, want running marker")
	}
	if jobRow.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", *jobRow.CompletedAt)
	}
	if jobRow.TotalEntries != 0 || jobRow.CreateCount != 0 || jobRow.UpdateCount != 0 || jobRow.BlockedCount != 0 || jobRow.ErrorCount != 0 {
		t.Fatalf("job counters changed unexpectedly: total=%d create=%d update=%d blocked=%d errors=%d", jobRow.TotalEntries, jobRow.CreateCount, jobRow.UpdateCount, jobRow.BlockedCount, jobRow.ErrorCount)
	}
}

func TestDirectorySyncWorker_FailurePersistCancellationReturnsContextError(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "directory_sync_failure_persist_canceled")
	jobID := "directory-sync-failure-persist-canceled-" + uuid.NewString()
	createDirectorySyncWorkerJob(t, client, jobID, "auth-provider-missing-"+uuid.NewString())
	updateCount := 0
	client.DirectorySyncJob.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			updateCount++
			if updateCount == 2 {
				return nil, errors.Join(errors.New("failure persist interrupted"), context.Canceled)
			}
			return next.Mutate(ctx, mutation)
		})
	}, ent.OpUpdateOne))
	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"))

	err := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{JobID: jobID},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), jobID)
	if err != nil {
		t.Fatalf("get directory sync job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusRunning {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusRunning)
	}
	if jobRow.StartedAt == nil {
		t.Fatal("StartedAt = nil, want running marker")
	}
	if jobRow.CompletedAt != nil {
		t.Fatalf("CompletedAt = %v, want nil", *jobRow.CompletedAt)
	}
	if jobRow.ErrorCount != 0 {
		t.Fatalf("ErrorCount = %d, want 0", jobRow.ErrorCount)
	}
	if len(jobRow.Errors) != 0 {
		t.Fatalf("Errors = %#v, want empty", jobRow.Errors)
	}
}

func TestDirectorySyncWorker_ScheduledEnrichmentUpdatesExistingUsersOnly(t *testing.T) {
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
			JobID: "directory-enrichment-job-1",
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

func TestDirectorySyncWorker_RevokesSessionsWhenScheduledEnrichmentChangesRBAC(t *testing.T) {
	pool := testutil.OpenPGXPool(t, "directory_enrichment_worker_revoke")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	t.Cleanup(func() { _ = client.Close() })
	authSessions := service.NewAuthSessionManager(pool, client, 0)

	adapter := registerWorkerDirectorySyncAdapter(t, &workerDirectorySyncAdapter{
		listUsers: []provider.DirectoryUserRecord{
			{
				ExternalID:  "ext-existing-rbac-user",
				Username:    "existing-rbac-user",
				DisplayName: "Existing RBAC User",
				Cohorts: []provider.ExternalCohort{
					{Kind: "group", Key: "ops", DisplayName: "ops"},
				},
			},
		},
	})

	providerID := "auth-provider-directory-enrichment-rbac-revoke"
	if _, createErr := client.AuthProvider.Create().
		SetID(providerID).
		SetName("Directory Enrichment RBAC Revoke Provider").
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create auth provider: %v", createErr)
	}
	roleEnt, err := client.Role.Create().
		SetID("role-directory-enrichment-rbac-revoke").
		SetName("directory_enrichment_rbac_revoke").
		SetPermissions([]string{"vm:read"}).
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	if _, createErr := client.ExternalCohortMapping.Create().
		SetID("mapping-directory-enrichment-rbac-revoke").
		SetProviderID(providerID).
		SetCohortKind("group").
		SetCohortKey("ops").
		SetRoleID(roleEnt.ID).
		SetScopeType("global").
		SetCreatedBy("admin-1").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create external cohort mapping: %v", createErr)
	}
	existingUser, err := client.User.Create().
		SetID("existing-rbac-user-id").
		SetUsername("existing-rbac-user").
		SetDisplayName("Existing RBAC User").
		SetEmail("existing-rbac-user@example.com").
		SetEnabled(true).
		Save(t.Context())
	if err != nil {
		t.Fatalf("create existing canonical user: %v", err)
	}
	if _, createErr := client.DirectorySyncJob.Create().
		SetID("directory-enrichment-rbac-revoke-job-1").
		SetAuthProviderID(providerID).
		SetRequestSnapshot(map[string]interface{}{"opaque": "payload"}).
		SetConflictResolution(service.DirectoryConflictResolutionSkip).
		SetSyncMode(service.DirectoryExecutionModeScheduledEnrichment).
		SetJoinKeyType(string(provider.DirectoryJoinKeyUsername)).
		SetTriggeredBy("system:directory-enrichment-scheduler").
		Save(t.Context()); createErr != nil {
		t.Fatalf("create directory enrichment job: %v", createErr)
	}
	beforeVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("seed session version: %v", err)
	}

	worker := NewDirectorySyncWorker(client, service.NewDirectorySyncService(client), nil, []byte("0123456789abcdef0123456789abcdef"), authSessions)
	if workerErr := worker.Work(t.Context(), &river.Job[DirectorySyncArgs]{
		Args: DirectorySyncArgs{
			JobID: "directory-enrichment-rbac-revoke-job-1",
		},
	}); workerErr != nil {
		t.Fatalf("worker work: %v", workerErr)
	}

	jobRow, err := client.DirectorySyncJob.Get(t.Context(), "directory-enrichment-rbac-revoke-job-1")
	if err != nil {
		t.Fatalf("get directory enrichment job: %v", err)
	}
	if jobRow.Status != directorysyncjob.StatusCompleted {
		t.Fatalf("job status = %s, want %s", jobRow.Status, directorysyncjob.StatusCompleted)
	}
	if jobRow.ErrorCount != 0 {
		t.Fatalf("ErrorCount = %d, want 0", jobRow.ErrorCount)
	}
	bindingCount, err := client.RoleBinding.Query().
		Where(rolebinding.HasUserWith(user.IDEQ(existingUser.ID))).
		Count(t.Context())
	if err != nil {
		t.Fatalf("count role bindings: %v", err)
	}
	if bindingCount != 1 {
		t.Fatalf("role binding count = %d, want 1", bindingCount)
	}
	afterVersion, err := authSessions.CurrentSessionVersion(t.Context(), existingUser.ID)
	if err != nil {
		t.Fatalf("read session version after directory enrichment: %v", err)
	}
	if afterVersion != beforeVersion+1 {
		t.Fatalf("session version after RBAC-changing enrichment = %d, want %d", afterVersion, beforeVersion+1)
	}
}

func registerWorkerDirectorySyncAdapter(t *testing.T, adapter *workerDirectorySyncAdapter) *workerDirectorySyncAdapter {
	t.Helper()
	if adapter == nil {
		t.Fatal("adapter is nil")
		return adapter
	}
	if adapter.typeKey == "" {
		adapter.typeKey = "test-worker-directory-sync-" + uuid.NewString()
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register worker directory sync adapter: %v", err)
	}
	return adapter
}
