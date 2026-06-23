package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	"kv-shepherd.io/shepherd/ent/enttest"
	"kv-shepherd.io/shepherd/internal/provider"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type scheduledDirectoryEnrichmentTestAdapter struct {
	typeKey string
	plan    *provider.ScheduledDirectoryEnrichmentPlan
	planErr error
}

type directoryEnrichmentScheduleScanStore struct {
	client      *ent.Client
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
}

func (a *scheduledDirectoryEnrichmentTestAdapter) Type() string { return a.typeKey }

func (a *scheduledDirectoryEnrichmentTestAdapter) ValidateConfig(map[string]interface{}) error {
	return nil
}

func (a *scheduledDirectoryEnrichmentTestAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}

func (a *scheduledDirectoryEnrichmentTestAdapter) SampleFields(context.Context, map[string]interface{}) ([]provider.AuthProviderSampleField, error) {
	return nil, nil
}

func (a *scheduledDirectoryEnrichmentTestAdapter) Describe() provider.AuthProviderTypeDescriptor {
	return provider.AuthProviderTypeDescriptor{Type: a.typeKey, DisplayName: "Scheduled Directory Enrichment Test"}
}

func (a *scheduledDirectoryEnrichmentTestAdapter) DescribeDirectorySync() provider.DirectorySyncDescriptor {
	return provider.DirectorySyncDescriptor{DisplayName: "Scheduled Directory Enrichment Test", SupportsPreview: true}
}

func (a *scheduledDirectoryEnrichmentTestAdapter) PreviewDirectorySync(context.Context, map[string]interface{}, map[string]interface{}) (*provider.DirectorySyncPreview, error) {
	return &provider.DirectorySyncPreview{}, nil
}

func (a *scheduledDirectoryEnrichmentTestAdapter) ListDirectoryUsers(context.Context, map[string]interface{}, map[string]interface{}) ([]provider.DirectoryUserRecord, error) {
	return nil, nil
}

func (a *scheduledDirectoryEnrichmentTestAdapter) BuildScheduledDirectoryEnrichmentPlan(context.Context, map[string]interface{}) (*provider.ScheduledDirectoryEnrichmentPlan, error) {
	if a.planErr != nil {
		return nil, a.planErr
	}
	return a.plan, nil
}

func registerScheduledDirectoryEnrichmentTestAdapter(t *testing.T, plan *provider.ScheduledDirectoryEnrichmentPlan) *scheduledDirectoryEnrichmentTestAdapter {
	t.Helper()

	adapter := &scheduledDirectoryEnrichmentTestAdapter{
		typeKey: "test-scheduled-directory-enrichment-" + uuid.NewString(),
		plan:    plan,
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register scheduled directory enrichment adapter: %v", err)
	}
	return adapter
}

func newDirectoryEnrichmentScheduleScanStore(t *testing.T) directoryEnrichmentScheduleScanStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, "des")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("create river migrator: %v", err)
	}
	if _, migrateErr := migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil); migrateErr != nil {
		t.Fatalf("migrate river schema: %v", migrateErr)
	}
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client: %v", err)
	}
	return directoryEnrichmentScheduleScanStore{
		client:      client,
		pool:        pool,
		riverClient: riverClient,
	}
}

func newDirectoryEnrichmentScheduleScanStoreWithoutRiverMigration(t *testing.T) directoryEnrichmentScheduleScanStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, "des_no_river")
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })

	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.Postgres, db))))
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("create river client without migration: %v", err)
	}
	return directoryEnrichmentScheduleScanStore{
		client:      client,
		pool:        pool,
		riverClient: riverClient,
	}
}

func TestDirectoryEnrichmentScheduleScanArgs_KindAndInsertOpts(t *testing.T) {
	t.Parallel()

	var args DirectoryEnrichmentScheduleScanArgs
	if got := args.Kind(); got != DirectoryEnrichmentScheduleScanJobKind {
		t.Fatalf("Kind() = %q, want %q", got, DirectoryEnrichmentScheduleScanJobKind)
	}

	opts := args.InsertOpts()
	if opts.Queue != river.QueueDefault {
		t.Fatalf("Queue = %q, want %q", opts.Queue, river.QueueDefault)
	}
	if opts.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want 1", opts.MaxAttempts)
	}
	if opts.UniqueOpts.ByPeriod != DefaultDirectoryEnrichmentScheduleScanInterval {
		t.Fatalf("ByPeriod = %s, want %s", opts.UniqueOpts.ByPeriod, DefaultDirectoryEnrichmentScheduleScanInterval)
	}
	if !opts.UniqueOpts.ByQueue || !opts.UniqueOpts.ByArgs {
		t.Fatalf("UniqueOpts = %#v, want ByQueue and ByArgs", opts.UniqueOpts)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_WorkInitializationAndEmptyStore(t *testing.T) {
	if err := NewDirectoryEnrichmentScheduleScanWorker(nil, nil, nil, nil, nil).Work(
		t.Context(),
		&river.Job[DirectoryEnrichmentScheduleScanArgs]{},
	); err == nil {
		t.Fatal("Work() error = nil, want initialization error")
	}

	clientWithoutPool := testutil.OpenEntPostgres(t, "directory_enrichment_schedule_scan_missing_pool")
	if err := NewDirectoryEnrichmentScheduleScanWorker(clientWithoutPool, nil, func() *river.Client[pgx.Tx] {
		t.Fatal("river client provider should not be called when pool dependency is missing")
		return nil
	}, nil, nil).Work(
		t.Context(),
		&river.Job[DirectoryEnrichmentScheduleScanArgs]{},
	); err == nil {
		t.Fatal("Work() missing pool error = nil, want initialization error")
	}

	store := newDirectoryEnrichmentScheduleScanStore(t)
	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		t.Fatal("river client provider should not be called when there are no auth providers")
		return nil
	}, nil, nil)

	if err := worker.Work(t.Context(), &river.Job[DirectoryEnrichmentScheduleScanArgs]{}); err != nil {
		t.Fatalf("Work() empty store error = %v", err)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_WorkEnqueuesDueScheduledEnrichment(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStore(t)
	planRequest := map[string]interface{}{
		"directory_filter": "(uid=*)",
		"limit":            float64(25),
	}
	adapter := registerScheduledDirectoryEnrichmentTestAdapter(t, &provider.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      provider.DirectoryJoinKeyUsername,
		ScheduleCron:     "* * * * *",
		ScheduleTimezone: "UTC",
		ProviderRequest:  planRequest,
	})
	providerID := "auth-provider-scheduled-" + uuid.NewString()
	if _, err := store.client.AuthProvider.Create().
		SetID(providerID).
		SetName("Scheduled Directory Enrichment " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		return store.riverClient
	}, nil, nil)
	if err := worker.Work(t.Context(), &river.Job[DirectoryEnrichmentScheduleScanArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	jobs, err := store.client.DirectorySyncJob.Query().All(t.Context())
	if err != nil {
		t.Fatalf("query directory sync jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("directory sync job count = %d, want 1", len(jobs))
	}
	jobRow := jobs[0]
	if jobRow.AuthProviderID != providerID {
		t.Fatalf("AuthProviderID = %q, want %q", jobRow.AuthProviderID, providerID)
	}
	if jobRow.Status != directorysyncjob.StatusPending {
		t.Fatalf("Status = %q, want %q", jobRow.Status, directorysyncjob.StatusPending)
	}
	if jobRow.SyncMode != "scheduled_enrichment" || jobRow.ConflictResolution != "skip" || jobRow.JoinKeyType != "username" {
		t.Fatalf("job policy fields = mode:%q conflict:%q join:%q", jobRow.SyncMode, jobRow.ConflictResolution, jobRow.JoinKeyType)
	}
	if jobRow.TriggeredBy != directoryEnrichmentSchedulerActor {
		t.Fatalf("TriggeredBy = %q, want %q", jobRow.TriggeredBy, directoryEnrichmentSchedulerActor)
	}
	if got := jobRow.RequestSnapshot["directory_filter"]; got != "(uid=*)" {
		t.Fatalf("request_snapshot.directory_filter = %#v, want %q", got, "(uid=*)")
	}

	planRequest["directory_filter"] = "(mutated=*)"
	refetchedJob, err := store.client.DirectorySyncJob.Get(t.Context(), jobRow.ID)
	if err != nil {
		t.Fatalf("refetch directory sync job: %v", err)
	}
	if got := refetchedJob.RequestSnapshot["directory_filter"]; got != "(uid=*)" {
		t.Fatalf("request snapshot was not cloned, got %#v", got)
	}

	var riverKind, riverQueue, riverJobID string
	queryErr := store.pool.QueryRow(
		t.Context(),
		`SELECT kind, queue, args->>'job_id' FROM river_job WHERE kind = $1`,
		DirectorySyncJobKind,
	).Scan(&riverKind, &riverQueue, &riverJobID)
	if queryErr != nil {
		t.Fatalf("query river job: %v", queryErr)
	}
	if riverKind != DirectorySyncJobKind || riverQueue != river.QueueDefault || riverJobID != jobRow.ID {
		t.Fatalf("river job = kind:%q queue:%q job_id:%q, want kind:%q queue:%q job_id:%q", riverKind, riverQueue, riverJobID, DirectorySyncJobKind, river.QueueDefault, jobRow.ID)
	}

	secondErr := worker.Work(t.Context(), &river.Job[DirectoryEnrichmentScheduleScanArgs]{})
	if secondErr != nil {
		t.Fatalf("second Work() error = %v", secondErr)
	}
	count, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if count != 1 {
		t.Fatalf("directory sync job count after second scan = %d, want 1", count)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_SkipsDisabledAuthProviders(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStore(t)
	adapter := registerScheduledDirectoryEnrichmentTestAdapter(t, &provider.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      provider.DirectoryJoinKeyUsername,
		ScheduleCron:     "* * * * *",
		ScheduleTimezone: "UTC",
		ProviderRequest: map[string]interface{}{
			"directory_filter": "(uid=*)",
		},
	})
	if _, err := store.client.AuthProvider.Create().
		SetID("auth-provider-scheduled-disabled-" + uuid.NewString()).
		SetName("Disabled Scheduled Directory Enrichment " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetEnabled(false).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create disabled auth provider: %v", err)
	}

	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		t.Fatal("river client provider should not be called for disabled auth providers")
		return nil
	}, nil, nil)
	if err := worker.Work(t.Context(), &river.Job[DirectoryEnrichmentScheduleScanArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	count, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("directory sync job count for disabled provider = %d, want 0", count)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_EnqueueFailureRollsBackJobCreate(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStoreWithoutRiverMigration(t)
	adapter := registerScheduledDirectoryEnrichmentTestAdapter(t, &provider.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      provider.DirectoryJoinKeyUsername,
		ScheduleCron:     "* * * * *",
		ScheduleTimezone: "UTC",
		ProviderRequest: map[string]interface{}{
			"directory_filter": "(uid=*)",
		},
	})
	authProviderRow, err := store.client.AuthProvider.Create().
		SetID("auth-provider-failing-river-" + uuid.NewString()).
		SetName("Failing River Enrichment " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		return store.riverClient
	}, nil, nil)

	err = worker.enqueueScheduledEnrichmentIfDue(t.Context(), authProviderRow, time.Now().UTC())
	if err == nil {
		t.Fatal("enqueueScheduledEnrichmentIfDue() error = nil, want river insert error")
	}

	jobCount, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("directory sync job count after enqueue rollback = %d, want 0", jobCount)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_ContextCancellationStopsScanWithoutCreatingJob(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStore(t)
	adapter := &scheduledDirectoryEnrichmentTestAdapter{
		typeKey: "test-scheduled-directory-enrichment-canceled-" + uuid.NewString(),
		planErr: errors.Join(
			errors.New("scheduled directory enrichment interrupted"),
			context.Canceled,
		),
	}
	if err := provider.RegisterAuthProviderAdminAdapter(adapter); err != nil {
		t.Fatalf("register scheduled directory enrichment adapter: %v", err)
	}
	if _, err := store.client.AuthProvider.Create().
		SetID("auth-provider-canceled-" + uuid.NewString()).
		SetName("Canceled Scheduled Directory Enrichment " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{"tenant": "acme"}).
		SetCreatedBy("admin-1").
		Save(t.Context()); err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		return store.riverClient
	}, nil, nil)

	err := worker.Work(t.Context(), &river.Job[DirectoryEnrichmentScheduleScanArgs]{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	count, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("directory sync job count = %d, want 0", count)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_EnqueueReturnsContextErrorWhenCanceled(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStore(t)
	adapter := registerScheduledDirectoryEnrichmentTestAdapter(t, &provider.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      provider.DirectoryJoinKeyUsername,
		ScheduleCron:     "* * * * *",
		ScheduleTimezone: "UTC",
	})
	authProviderRow, err := store.client.AuthProvider.Create().
		SetID("auth-provider-canceled-enqueue-" + uuid.NewString()).
		SetName("Canceled Enqueue " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}
	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, store.pool, func() *river.Client[pgx.Tx] {
		return store.riverClient
	}, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = worker.enqueueScheduledEnrichmentIfDue(ctx, authProviderRow, time.Now().UTC())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("enqueueScheduledEnrichmentIfDue() error = %v, want context.Canceled", err)
	}

	count, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if count != 0 {
		t.Fatalf("directory sync job count = %d, want 0", count)
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_MissingPoolRollsBackJobCreate(t *testing.T) {
	store := newDirectoryEnrichmentScheduleScanStore(t)
	adapter := registerScheduledDirectoryEnrichmentTestAdapter(t, &provider.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             provider.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      provider.DirectoryJoinKeyUsername,
		ScheduleCron:     "* * * * *",
		ScheduleTimezone: "UTC",
	})
	authProviderRow, err := store.client.AuthProvider.Create().
		SetID("auth-provider-canceled-after-create-" + uuid.NewString()).
		SetName("Canceled After Create " + uuid.NewString()[:8]).
		SetAuthType(adapter.typeKey).
		SetConfig(map[string]interface{}{}).
		SetCreatedBy("admin-1").
		Save(t.Context())
	if err != nil {
		t.Fatalf("create auth provider: %v", err)
	}

	worker := NewDirectoryEnrichmentScheduleScanWorker(store.client, nil, func() *river.Client[pgx.Tx] {
		return store.riverClient
	}, nil, nil)

	err = worker.enqueueScheduledEnrichmentIfDue(t.Context(), authProviderRow, time.Now().UTC())
	if err == nil {
		t.Fatal("enqueueScheduledEnrichmentIfDue() error = nil, want dependency error")
	}

	jobCount, err := store.client.DirectorySyncJob.Query().Count(t.Context())
	if err != nil {
		t.Fatalf("count directory sync jobs: %v", err)
	}
	if jobCount != 0 {
		t.Fatalf("directory sync job count after dependency failure = %d, want 0", jobCount)
	}
}

func TestNormalizeScheduledDirectoryEnrichmentPlan_DefaultsAndValidates(t *testing.T) {
	plan, location, err := directorycontract.NormalizeScheduledDirectoryEnrichmentPlan(&directorycontract.ScheduledDirectoryEnrichmentPlan{
		Enabled:      true,
		ScheduleCron: "0 * * * *",
	})
	if err != nil {
		t.Fatalf("normalize plan: %v", err)
	}
	if plan.Mode != directorycontract.DirectoryEnrichmentModeEnrichExistingOnly {
		t.Fatalf("plan.Mode = %q, want %q", plan.Mode, directorycontract.DirectoryEnrichmentModeEnrichExistingOnly)
	}
	if plan.JoinKeyType != directorycontract.DirectoryJoinKeyUsername {
		t.Fatalf("plan.JoinKeyType = %q, want %q", plan.JoinKeyType, directorycontract.DirectoryJoinKeyUsername)
	}
	if got := location.String(); got != "UTC" {
		t.Fatalf("location = %q, want %q", got, "UTC")
	}
}

func TestDirectoryEnrichmentScheduleScanWorker_ScheduledDueEvaluation(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "directory_enrichment_due")
	worker := &DirectoryEnrichmentScheduleScanWorker{entClient: client}
	now := time.Date(2026, 3, 21, 10, 30, 0, 0, time.UTC)

	due, err := worker.scheduledDirectoryEnrichmentDue(t.Context(), "provider-no-history", "0 * * * *", time.UTC, now)
	if err != nil {
		t.Fatalf("scheduledDirectoryEnrichmentDue(no history): %v", err)
	}
	if !due {
		t.Fatal("due with no history = false, want true")
	}

	if _, createErr := client.DirectorySyncJob.Create().
		SetID("job-pending").
		SetAuthProviderID("provider-pending").
		SetRequestSnapshot(map[string]interface{}{}).
		SetConflictResolution("skip").
		SetSyncMode("scheduled_enrichment").
		SetTriggeredBy("system:directory-enrichment-scheduler").
		SetStatus(directorysyncjob.StatusPending).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create pending job: %v", createErr)
	}
	due, err = worker.scheduledDirectoryEnrichmentDue(t.Context(), "provider-pending", "0 * * * *", time.UTC, now)
	if err != nil {
		t.Fatalf("scheduledDirectoryEnrichmentDue(pending): %v", err)
	}
	if due {
		t.Fatal("due with pending job = true, want false")
	}

	if _, createErr := client.DirectorySyncJob.Create().
		SetID("job-recent").
		SetAuthProviderID("provider-recent").
		SetRequestSnapshot(map[string]interface{}{}).
		SetConflictResolution("skip").
		SetSyncMode("scheduled_enrichment").
		SetTriggeredBy("system:directory-enrichment-scheduler").
		SetStatus(directorysyncjob.StatusCompleted).
		SetCreatedAt(now.Add(-30 * time.Minute)).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create recent completed job: %v", createErr)
	}
	due, err = worker.scheduledDirectoryEnrichmentDue(t.Context(), "provider-recent", "0 * * * *", time.UTC, now)
	if err != nil {
		t.Fatalf("scheduledDirectoryEnrichmentDue(recent): %v", err)
	}
	if due {
		t.Fatal("due with recent completed job = true, want false")
	}

	if _, createErr := client.DirectorySyncJob.Create().
		SetID("job-old").
		SetAuthProviderID("provider-old").
		SetRequestSnapshot(map[string]interface{}{}).
		SetConflictResolution("skip").
		SetSyncMode("scheduled_enrichment").
		SetTriggeredBy("system:directory-enrichment-scheduler").
		SetStatus(directorysyncjob.StatusCompleted).
		SetCreatedAt(now.Add(-2 * time.Hour)).
		Save(t.Context()); createErr != nil {
		t.Fatalf("create old completed job: %v", createErr)
	}
	due, err = worker.scheduledDirectoryEnrichmentDue(t.Context(), "provider-old", "0 * * * *", time.UTC, now)
	if err != nil {
		t.Fatalf("scheduledDirectoryEnrichmentDue(old): %v", err)
	}
	if !due {
		t.Fatal("due with old completed job = false, want true")
	}
}
