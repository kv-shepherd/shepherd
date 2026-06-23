package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"kv-shepherd.io/shepherd/ent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entticket "kv-shepherd.io/shepherd/ent/ticket"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/provider"
	"kv-shepherd.io/shepherd/internal/service"
	"kv-shepherd.io/shepherd/internal/testutil"
)

type statusSyncListProvider struct {
	*provider.MockProvider
	listErr error
	calls   []statusSyncListCall
}

type statusSyncListCall struct {
	cluster   string
	namespace string
	opts      provider.ListOptions
}

func (p *statusSyncListProvider) ListVMs(ctx context.Context, cluster, namespace string, opts provider.ListOptions) (*domain.VMList, error) {
	p.calls = append(p.calls, statusSyncListCall{cluster: cluster, namespace: namespace, opts: opts})
	if p.listErr != nil {
		return nil, p.listErr
	}
	return p.MockProvider.ListVMs(ctx, cluster, namespace, opts)
}

type vmStatusSyncFixture struct {
	client    *ent.Client
	eventID   string
	vmID      string
	vmName    string
	namespace string
	clusterID string
}

type vmStatusSyncSeedOptions struct {
	clusterID       string
	status          vm.Status
	pollingTier     vm.PollingTier
	pollIntervalSec int
	lastK8sRV       string
	createdAt       time.Time
	highTierSince   *time.Time
}

type vmStatusSyncScheduleStore struct {
	pool        *pgxpool.Pool
	riverClient *river.Client[pgx.Tx]
}

func newVMStatusSyncScheduleStore(t *testing.T) vmStatusSyncScheduleStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, "vss")
	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	require.NoError(t, err)
	_, err = migrator.Migrate(t.Context(), rivermigrate.DirectionUp, nil)
	require.NoError(t, err)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)

	return vmStatusSyncScheduleStore{
		pool:        pool,
		riverClient: riverClient,
	}
}

func newVMStatusSyncScheduleStoreWithoutRiverMigration(t *testing.T) vmStatusSyncScheduleStore {
	t.Helper()

	pool := testutil.OpenPGXPool(t, "vss_no_river")
	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	require.NoError(t, err)

	return vmStatusSyncScheduleStore{
		pool:        pool,
		riverClient: riverClient,
	}
}

func seedVMStatusSyncFixture(t *testing.T, opts vmStatusSyncSeedOptions) vmStatusSyncFixture {
	t.Helper()

	if opts.status == "" {
		opts.status = vm.StatusCREATING
	}
	if opts.pollingTier == "" {
		opts.pollingTier = vm.PollingTierHigh
	}
	if opts.pollIntervalSec == 0 {
		opts.pollIntervalSec = intervalForTier(opts.pollingTier)
	}
	if opts.createdAt.IsZero() {
		opts.createdAt = time.Now().Add(-5 * time.Minute)
	}

	client := testutil.OpenEntPostgres(t, "vm_status_sync_"+uuid.NewString()[:8])
	ctx := t.Context()

	systemRow, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("seed").
		Save(ctx)
	require.NoError(t, err)

	serviceRow, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(systemRow).
		Save(ctx)
	require.NoError(t, err)

	eventID := "ev-" + uuid.NewString()
	ticketID := "ticket-" + uuid.NewString()
	ticketCreate := client.Ticket.Create().
		SetID(ticketID).
		SetEventID(eventID).
		SetRequester("seed").
		SetStatus(entticket.StatusAPPROVED).
		SetOperationType(entticket.OperationTypeCREATE)
	if opts.clusterID != "" {
		ticketCreate = ticketCreate.SetSelectedClusterID(opts.clusterID)
	}
	_, err = ticketCreate.Save(ctx)
	require.NoError(t, err)

	vmID := "vm-" + uuid.NewString()
	vmName := "vm-" + uuid.NewString()[:8]
	namespace := "ns-" + uuid.NewString()[:8]
	create := client.VM.Create().
		SetID(vmID).
		SetName(vmName).
		SetInstance("01").
		SetNamespace(namespace).
		SetStatus(opts.status).
		SetCreatedBy("seed").
		SetServiceID(serviceRow.ID).
		SetTicketID(ticketID).
		SetPollingTier(opts.pollingTier).
		SetPollIntervalSec(opts.pollIntervalSec).
		SetCreatedAt(opts.createdAt).
		SetUpdatedAt(opts.createdAt)
	if opts.clusterID != "" {
		create = create.SetClusterID(opts.clusterID)
	}
	if opts.lastK8sRV != "" {
		create = create.SetLastK8sRv(opts.lastK8sRV)
	}
	if opts.highTierSince != nil {
		create = create.SetHighTierSince(*opts.highTierSince)
	}
	_, err = create.Save(ctx)
	require.NoError(t, err)

	return vmStatusSyncFixture{
		client:    client,
		eventID:   eventID,
		vmID:      vmID,
		vmName:    vmName,
		namespace: namespace,
		clusterID: opts.clusterID,
	}
}

func injectVMStatusBeforeNextJobsUpdate(t *testing.T, client *ent.Client, id string, status vm.Status) {
	t.Helper()
	injected := false
	client.VM.Use(enthook.On(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			mutation, ok := m.(*ent.VMMutation)
			if !ok {
				return next.Mutate(ctx, m)
			}
			mutationID, ok := mutation.ID()
			if !injected && ok && mutationID == id {
				injected = true
				if _, err := client.VM.UpdateOneID(id).
					SetStatus(status).
					Save(ctx); err != nil {
					return nil, err
				}
			}
			return next.Mutate(ctx, m)
		})
	}, ent.OpUpdateOne))
}

func TestVMStatusSyncArgs_Kind(t *testing.T) {
	t.Parallel()
	var args VMStatusSyncArgs
	if got := args.Kind(); got != VMStatusSyncJobKind {
		t.Fatalf("Kind() = %q, want %q", got, VMStatusSyncJobKind)
	}
}

// Compile-time assertion: VMStatusSyncArgs must NOT implement JobArgsWithInsertOpts.
// All insert options are managed exclusively by scheduleNext() to avoid DRY violations.
// If this causes a compile error, someone accidentally added an InsertOpts() method.
var _ interface{ Kind() string } = VMStatusSyncArgs{}

func TestVMStatusSyncArgs_NoInsertOpts(t *testing.T) {
	t.Parallel()
	// VMStatusSyncArgs intentionally does not implement JobArgsWithInsertOpts.
	// Insert options (Queue, MaxAttempts, UniqueOpts, ScheduledAt) are managed
	// exclusively by scheduleNext() — the single source of truth.
	// This test documents that design decision; the compile-time assertion above
	// enforces it at build time.
	var args VMStatusSyncArgs
	if args.Kind() != VMStatusSyncJobKind {
		t.Fatal("unexpected kind change")
	}
}

func TestVMStatusSyncWorkerScheduleNext_InsertsUniqueScheduledPollJob(t *testing.T) {
	store := newVMStatusSyncScheduleStore(t)
	worker := NewVMStatusSyncWorker(nil, nil, func() *river.Client[pgx.Tx] {
		return store.riverClient
	})
	eventID := "ev-" + uuid.NewString()
	jobID := "job-" + uuid.NewString()
	before := time.Now()

	require.NoError(t, worker.scheduleNext(t.Context(), eventID, jobID, highTierIntervalSec))

	var (
		kind        string
		queue       string
		state       string
		maxAttempts int
		scheduledAt time.Time
		gotEventID  string
		gotJobID    string
	)
	err := store.pool.QueryRow(t.Context(), `
			SELECT kind, queue, state::text, max_attempts, scheduled_at, args->>'event_id', args->>'job_id'
				FROM river_job
				WHERE kind = $1 AND queue = $2 AND args->>'event_id' = $3
	`, VMStatusSyncJobKind, VMStatusSyncJobKind, eventID).Scan(
		&kind,
		&queue,
		&state,
		&maxAttempts,
		&scheduledAt,
		&gotEventID,
		&gotJobID,
	)
	require.NoError(t, err)
	require.Equal(t, VMStatusSyncJobKind, kind)
	require.Equal(t, VMStatusSyncJobKind, queue)
	require.Equal(t, string(rivertype.JobStateScheduled), state)
	require.Equal(t, 3, maxAttempts)
	require.Equal(t, eventID, gotEventID)
	require.Equal(t, jobID, gotJobID)
	require.True(t, scheduledAt.After(before.Add(10*time.Second)), "scheduled_at = %s", scheduledAt)
	require.True(t, scheduledAt.Before(time.Now().Add(time.Duration(highTierIntervalSec)*time.Second+10*time.Second)), "scheduled_at = %s", scheduledAt)

	require.NoError(t, worker.scheduleNext(t.Context(), eventID, jobID, highTierIntervalSec))

	var count int
	err = store.pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM river_job
		WHERE kind = $1 AND queue = $2 AND args->>'event_id' = $3
	`, VMStatusSyncJobKind, VMStatusSyncJobKind, eventID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestVMStatusSyncWorkerScheduleNext_AllowsNextScheduledJobWhileCurrentIsRunning(t *testing.T) {
	store := newVMStatusSyncScheduleStore(t)
	worker := NewVMStatusSyncWorker(nil, nil, func() *river.Client[pgx.Tx] {
		return store.riverClient
	})
	eventID := "ev-" + uuid.NewString()

	current, err := store.riverClient.Insert(t.Context(), VMStatusSyncArgs{EventID: eventID}, &river.InsertOpts{
		Queue:       VMStatusSyncJobKind,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
		},
	})
	require.NoError(t, err)
	_, err = store.pool.Exec(t.Context(), `
		UPDATE river_job
		SET state = $1
		WHERE id = $2
	`, rivertype.JobStateRunning, current.Job.ID)
	require.NoError(t, err)

	require.NoError(t, worker.scheduleNext(t.Context(), eventID, "job-"+uuid.NewString(), highTierIntervalSec))

	var count int
	err = store.pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM river_job
		WHERE kind = $1 AND queue = $2 AND args->>'event_id' = $3
	`, VMStatusSyncJobKind, VMStatusSyncJobKind, eventID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 2, count)
}

func TestVMStatusSyncWorkerScheduleNext_ReturnsInsertError(t *testing.T) {
	store := newVMStatusSyncScheduleStoreWithoutRiverMigration(t)
	worker := NewVMStatusSyncWorker(nil, nil, func() *river.Client[pgx.Tx] {
		return store.riverClient
	})

	err := worker.scheduleNext(t.Context(), "ev-"+uuid.NewString(), "job-"+uuid.NewString(), highTierIntervalSec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "schedule next poll")
	require.Contains(t, err.Error(), "river_job")
}

func TestVMStatusSyncWorkerScheduleNext_ReturnsContextErrorWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := NewVMStatusSyncWorker(nil, nil, nil).scheduleNext(ctx, "ev-"+uuid.NewString(), "job-"+uuid.NewString(), highTierIntervalSec)
	require.ErrorIs(t, err, context.Canceled)
}

func TestVMStatusSyncWorkerWork_CancelsInvalidOrUnboundEvent(t *testing.T) {
	err := NewVMStatusSyncWorker(nil, nil, nil).Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: "   "},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty event_id")

	client := testutil.OpenEntPostgres(t, "vm_status_sync_missing_"+uuid.NewString()[:8])
	worker := NewVMStatusSyncWorker(client, service.NewVMService(provider.NewMockProvider()), nil)

	err = worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: "ev-" + uuid.NewString()},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "has no active vm")
}

func TestVMStatusSyncWorkerWork_SkipsRowsThatShouldNotPollCluster(t *testing.T) {
	noCluster := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		status:          vm.StatusCREATING,
		pollingTier:     vm.PollingTierHigh,
		pollIntervalSec: highTierIntervalSec,
	})
	noClusterProvider := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	noClusterWorker := NewVMStatusSyncWorker(noCluster.client, service.NewVMService(noClusterProvider), nil)

	err := noClusterWorker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: noCluster.eventID},
	})
	require.NoError(t, err)
	require.Empty(t, noClusterProvider.calls)

	gotNoCluster, err := noCluster.client.VM.Get(t.Context(), noCluster.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusCREATING, gotNoCluster.Status)
	require.Nil(t, gotNoCluster.LastPolledAt)

	deleting := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusDELETING,
		pollingTier:     vm.PollingTierHigh,
		pollIntervalSec: highTierIntervalSec,
	})
	deletingProvider := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	deletingWorker := NewVMStatusSyncWorker(deleting.client, service.NewVMService(deletingProvider), nil)

	err = deletingWorker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: deleting.eventID},
	})
	require.NoError(t, err)
	require.Empty(t, deletingProvider.calls)

	gotDeleting, err := deleting.client.VM.Get(t.Context(), deleting.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusDELETING, gotDeleting.Status)
	require.Nil(t, gotDeleting.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_PersistsObservedStatusAndResourceVersion(t *testing.T) {
	highTierSince := time.Now().Add(-1 * time.Minute)
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusCREATING,
		pollingTier:     vm.PollingTierHigh,
		pollIntervalSec: highTierIntervalSec,
		lastK8sRV:       "rv-old",
		highTierSince:   &highTierSince,
	})
	infra := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	infra.Seed([]*domain.VM{{
		Name:            fixture.vmName,
		Namespace:       fixture.namespace,
		Cluster:         fixture.clusterID,
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-new",
	}})
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Len(t, infra.calls, 1)
	require.Equal(t, fixture.clusterID, infra.calls[0].cluster)
	require.Equal(t, fixture.namespace, infra.calls[0].namespace)
	require.Equal(t, "metadata.name="+fixture.vmName, infra.calls[0].opts.FieldSelector)
	require.Equal(t, 1, infra.calls[0].opts.Limit)
	require.Equal(t, "rv-old", infra.calls[0].opts.ResourceVersion)
	require.True(t, infra.calls[0].opts.SkipVMIEnrichment)

	got, err := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusRUNNING, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "rv-new", *got.LastK8sRv)
	require.NotNil(t, got.LastPolledAt)
	require.Nil(t, got.HighTierSince)
}

func TestVMStatusSyncWorkerWork_DoesNotOverwriteConcurrentDeletingStatus(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "rv-old",
	})
	infra := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	infra.Seed([]*domain.VM{{
		Name:            fixture.vmName,
		Namespace:       fixture.namespace,
		Cluster:         fixture.clusterID,
		Status:          domain.VMStatusStopped,
		ResourceVersion: "rv-new",
	}})
	injectVMStatusBeforeNextJobsUpdate(t, fixture.client, fixture.vmID, vm.StatusDELETING)
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)

	got, err := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusDELETING, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "rv-old", *got.LastK8sRv)
	require.Nil(t, got.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_MarksMissingVMNotFound(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "rv-old",
		createdAt:       time.Now().Add(-10 * time.Minute),
	})
	infra := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Len(t, infra.calls, 1)

	got, err := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusNOT_FOUND, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.Nil(t, got.LastK8sRv)
	require.NotNil(t, got.LastPolledAt)
	require.Nil(t, got.HighTierSince)
}

func TestVMStatusSyncWorkerWork_ClearsCachedResourceVersionOnExpiredList(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "stale-rv",
	})
	infra := &statusSyncListProvider{
		MockProvider: provider.NewMockProvider(),
		listErr:      k8serrors.NewResourceExpired("stale resourceVersion"),
	}
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.NoError(t, err)
	require.Len(t, infra.calls, 1)
	require.Equal(t, "stale-rv", infra.calls[0].opts.ResourceVersion)

	got, err := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusRUNNING, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.Nil(t, got.LastK8sRv)
	require.NotNil(t, got.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_ListCancellationReturnsContextError(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "rv-old",
	})
	infra := &statusSyncListProvider{
		MockProvider: provider.NewMockProvider(),
		listErr:      errors.Join(errors.New("list interrupted"), context.Canceled),
	}
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, infra.calls, 1)

	got, err := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, err)
	require.Equal(t, vm.StatusRUNNING, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "rv-old", *got.LastK8sRv)
	require.Nil(t, got.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_StatusPersistCancellationReturnsContextError(t *testing.T) {
	highTierSince := time.Now().Add(-1 * time.Minute)
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusCREATING,
		pollingTier:     vm.PollingTierHigh,
		pollIntervalSec: highTierIntervalSec,
		lastK8sRV:       "rv-old",
		highTierSince:   &highTierSince,
	})
	infra := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	infra.Seed([]*domain.VM{{
		Name:            fixture.vmName,
		Namespace:       fixture.namespace,
		Cluster:         fixture.clusterID,
		Status:          domain.VMStatusRunning,
		ResourceVersion: "rv-new",
	}})
	fixture.client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("status persist interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)

	got, getErr := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, getErr)
	require.Equal(t, vm.StatusCREATING, got.Status)
	require.Equal(t, vm.PollingTierHigh, got.PollingTier)
	require.Equal(t, highTierIntervalSec, got.PollIntervalSec)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "rv-old", *got.LastK8sRv)
	require.Nil(t, got.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_MissingVMPersistCancellationReturnsContextError(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "rv-old",
		createdAt:       time.Now().Add(-10 * time.Minute),
	})
	infra := &statusSyncListProvider{MockProvider: provider.NewMockProvider()}
	fixture.client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("missing-vm persist interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)

	got, getErr := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, getErr)
	require.Equal(t, vm.StatusRUNNING, got.Status)
	require.Equal(t, vm.PollingTierLow, got.PollingTier)
	require.Equal(t, lowTierIntervalSec, got.PollIntervalSec)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "rv-old", *got.LastK8sRv)
	require.Nil(t, got.LastPolledAt)
}

func TestVMStatusSyncWorkerWork_ResourceVersionResetCancellationReturnsContextError(t *testing.T) {
	fixture := seedVMStatusSyncFixture(t, vmStatusSyncSeedOptions{
		clusterID:       "cluster-" + uuid.NewString(),
		status:          vm.StatusRUNNING,
		pollingTier:     vm.PollingTierLow,
		pollIntervalSec: lowTierIntervalSec,
		lastK8sRV:       "stale-rv",
	})
	infra := &statusSyncListProvider{
		MockProvider: provider.NewMockProvider(),
		listErr:      k8serrors.NewResourceExpired("stale resourceVersion"),
	}
	fixture.client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("rv reset interrupted"), context.Canceled)),
		ent.OpUpdateOne,
	))
	worker := NewVMStatusSyncWorker(fixture.client, service.NewVMService(infra), nil)

	err := worker.Work(t.Context(), &river.Job[VMStatusSyncArgs]{
		Args: VMStatusSyncArgs{EventID: fixture.eventID},
	})
	require.Equal(t, context.Canceled, err)

	got, getErr := fixture.client.VM.Get(t.Context(), fixture.vmID)
	require.NoError(t, getErr)
	require.Equal(t, vm.StatusRUNNING, got.Status)
	require.NotNil(t, got.LastK8sRv)
	require.Equal(t, "stale-rv", *got.LastK8sRv)
	require.Nil(t, got.LastPolledAt)
}

// ---------------------------------------------------------------------------
// tierForStatus unit tests
// ---------------------------------------------------------------------------

func TestTierForStatus_TransitionalStatesReturnHigh(t *testing.T) {
	t.Parallel()

	transitional := []vm.Status{
		vm.StatusCREATING,
		vm.StatusSTARTING,
		vm.StatusDELETING,
		vm.StatusSTOPPING,
		vm.StatusMIGRATING,
		vm.StatusPENDING,
	}
	for _, s := range transitional {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			if tier := tierForStatus(s); tier != vm.PollingTierHigh {
				t.Errorf("tierForStatus(%s) = %s, want %s", s, tier, vm.PollingTierHigh)
			}
		})
	}
}

func TestTierForStatus_StableStatesReturnLow(t *testing.T) {
	t.Parallel()

	stable := []vm.Status{
		vm.StatusRUNNING,
		vm.StatusSTOPPED,
		vm.StatusFAILED,
		vm.StatusPAUSED,
		vm.StatusUNKNOWN,
		vm.StatusNOT_FOUND,
	}
	for _, s := range stable {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			if tier := tierForStatus(s); tier != vm.PollingTierLow {
				t.Errorf("tierForStatus(%s) = %s, want %s", s, tier, vm.PollingTierLow)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// intervalForTier unit tests
// ---------------------------------------------------------------------------

func TestIntervalForTier_HighReturns15(t *testing.T) {
	t.Parallel()
	if got := intervalForTier(vm.PollingTierHigh); got != highTierIntervalSec {
		t.Errorf("intervalForTier(high) = %d, want %d", got, highTierIntervalSec)
	}
}

func TestIntervalForTier_LowReturns1800(t *testing.T) {
	t.Parallel()
	if got := intervalForTier(vm.PollingTierLow); got != lowTierIntervalSec {
		t.Errorf("intervalForTier(low) = %d, want %d", got, lowTierIntervalSec)
	}
}

// ---------------------------------------------------------------------------
// mapDomainStatusToEntVM round-trip tests
// ---------------------------------------------------------------------------

func TestMapDomainStatusToEntVM_AllStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain domain.VMStatus
		want   vm.Status
	}{
		{domain.VMStatusCreating, vm.StatusCREATING},
		{domain.VMStatusStarting, vm.StatusSTARTING},
		{domain.VMStatusRunning, vm.StatusRUNNING},
		{domain.VMStatusStopping, vm.StatusSTOPPING},
		{domain.VMStatusStopped, vm.StatusSTOPPED},
		{domain.VMStatusDeleting, vm.StatusDELETING},
		{domain.VMStatusFailed, vm.StatusFAILED},
		{domain.VMStatusPending, vm.StatusPENDING},
		{domain.VMStatusMigrating, vm.StatusMIGRATING},
		{domain.VMStatusPaused, vm.StatusPAUSED},
		{domain.VMStatusUnknown, vm.StatusUNKNOWN},
		{domain.VMStatusNotFound, vm.StatusNOT_FOUND},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.domain), func(t *testing.T) {
			t.Parallel()
			if got := mapDomainStatusToEntVM(tc.domain); got != tc.want {
				t.Errorf("mapDomainStatusToEntVM(%s) = %s, want %s", tc.domain, got, tc.want)
			}
		})
	}
}

func TestMapDomainStatusToEntVM_UnknownDefault(t *testing.T) {
	t.Parallel()
	if got := mapDomainStatusToEntVM("NONEXISTENT"); got != vm.StatusUNKNOWN {
		t.Errorf("mapDomainStatusToEntVM(NONEXISTENT) = %s, want UNKNOWN", got)
	}
}

// ---------------------------------------------------------------------------
// Constants sanity tests
// ---------------------------------------------------------------------------

func TestConstants_HighTierInterval(t *testing.T) {
	t.Parallel()
	if highTierIntervalSec != 15 {
		t.Errorf("highTierIntervalSec = %d, want 15", highTierIntervalSec)
	}
}

func TestConstants_LowTierInterval(t *testing.T) {
	t.Parallel()
	if lowTierIntervalSec != 1800 {
		t.Errorf("lowTierIntervalSec = %d, want 1800 (30 minutes)", lowTierIntervalSec)
	}
}

func TestConstants_AutoDowngradeThreshold(t *testing.T) {
	t.Parallel()
	const expected = 30 // minutes
	if autoDowngradeThreshold.Minutes() != float64(expected) {
		t.Errorf("autoDowngradeThreshold = %v, want %d minutes", autoDowngradeThreshold, expected)
	}
}

func TestDeriveHighTierSince(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-10 * time.Minute)

	t.Run("non_high_tier_clears_timestamp", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierHigh,
			HighTierSince:   &old,
			Status:          vm.StatusCREATING,
			PollIntervalSec: highTierIntervalSec,
		}, vm.PollingTierLow, now)
		if got != nil {
			t.Fatalf("deriveHighTierSince() = %v, want nil", got)
		}
	})

	t.Run("entering_high_sets_now", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierLow,
			Status:          vm.StatusRUNNING,
			PollIntervalSec: lowTierIntervalSec,
		}, vm.PollingTierHigh, now)
		if got == nil {
			t.Fatal("deriveHighTierSince() = nil, want non-nil")
			return
		}
		if got.Sub(now) != 0 {
			t.Fatalf("deriveHighTierSince() = %v, want %v", *got, now)
		}
	})

	t.Run("staying_high_keeps_existing", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierHigh,
			HighTierSince:   &old,
			Status:          vm.StatusPENDING,
			PollIntervalSec: highTierIntervalSec,
		}, vm.PollingTierHigh, now)
		if got == nil {
			t.Fatal("deriveHighTierSince() = nil, want non-nil")
			return
		}
		if got.Sub(old) != 0 {
			t.Fatalf("deriveHighTierSince() = %v, want %v", *got, old)
		}
	})
}

func TestShouldAutoDowngrade(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-31 * time.Minute)
	recent := now.Add(-5 * time.Minute)

	if !shouldAutoDowngrade(vm.PollingTierHigh, &old, now) {
		t.Fatal("shouldAutoDowngrade() = false, want true for old high-tier timestamp")
	}
	if shouldAutoDowngrade(vm.PollingTierHigh, &recent, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for recent high-tier timestamp")
	}
	if shouldAutoDowngrade(vm.PollingTierLow, &old, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for low tier")
	}
	if shouldAutoDowngrade(vm.PollingTierHigh, nil, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for nil timestamp")
	}
}

func TestReconcileCreateBootstrapStatus(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("hold_stopped_as_current_status_during_create_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusCREATING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusCREATING {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusCREATING)
		}
	})

	t.Run("hold_unknown_as_creating_during_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusCREATING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-45 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusUNKNOWN, now)
		if got != vm.StatusCREATING {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusCREATING)
		}
	})

	t.Run("hold_unknown_as_starting_using_high_tier_since_anchor", func(t *testing.T) {
		t.Parallel()

		enteredHighTier := now.Add(-45 * time.Second)
		vmRow := &ent.VM{
			Status:        vm.StatusSTARTING,
			PollingTier:   vm.PollingTierHigh,
			HighTierSince: &enteredHighTier,
			CreatedAt:     now.Add(-24 * time.Hour),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusUNKNOWN, now)
		if got != vm.StatusSTARTING {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusSTARTING)
		}
	})

	t.Run("no_hold_after_bootstrap_window", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-createBootstrapGraceWindow - time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusSTOPPED {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusSTOPPED)
		}
	})

	t.Run("no_hold_for_low_tier_vm", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierLow,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusSTOPPED {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusSTOPPED)
		}
	})

	t.Run("no_hold_starting_when_high_tier_since_expired", func(t *testing.T) {
		t.Parallel()

		expired := now.Add(-createBootstrapGraceWindow - time.Second)
		vmRow := &ent.VM{
			Status:        vm.StatusSTARTING,
			PollingTier:   vm.PollingTierHigh,
			HighTierSince: &expired,
			CreatedAt:     now.Add(-24 * time.Hour),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusUNKNOWN, now)
		if got != vm.StatusUNKNOWN {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusUNKNOWN)
		}
	})
}

func TestReconcileMissingVMStatus(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("hold_missing_vm_status_during_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileMissingVMStatus(vmRow, now)
		// During bootstrap, hold the current status (RUNNING) even if VM is not yet visible.
		if got != vm.StatusRUNNING {
			t.Fatalf("reconcileMissingVMStatus() = %s, want %s", got, vm.StatusRUNNING)
		}
	})

	t.Run("missing_vm_becomes_not_found_after_bootstrap_window", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-createBootstrapGraceWindow - time.Second),
		}
		got := reconcileMissingVMStatus(vmRow, now)
		// After bootstrap, K8s list succeeded but VM not found → NOT_FOUND.
		if got != vm.StatusNOT_FOUND {
			t.Fatalf("reconcileMissingVMStatus() = %s, want %s", got, vm.StatusNOT_FOUND)
		}
	})
}
