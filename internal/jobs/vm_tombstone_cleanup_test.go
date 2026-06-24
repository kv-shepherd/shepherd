package jobs

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	enthook "kv-shepherd.io/shepherd/ent/hook"
	entvm "kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestVMTombstoneCleanupArgsKind(t *testing.T) {
	t.Parallel()

	if got := (VMTombstoneCleanupArgs{}).Kind(); got != "vm_tombstone_cleanup" {
		t.Fatalf("Kind() = %q, want %q", got, "vm_tombstone_cleanup")
	}
}

func TestVMTombstoneCleanupArgsInsertOpts(t *testing.T) {
	t.Parallel()

	opts := (VMTombstoneCleanupArgs{}).InsertOpts()
	if opts.Queue != river.QueueDefault {
		t.Fatalf("Queue = %q, want %q", opts.Queue, river.QueueDefault)
	}
	if opts.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want 1", opts.MaxAttempts)
	}
	if opts.UniqueOpts.ByPeriod != 24*time.Hour {
		t.Fatalf("UniqueOpts.ByPeriod = %s, want %s", opts.UniqueOpts.ByPeriod, 24*time.Hour)
	}
	if !opts.UniqueOpts.ByQueue {
		t.Fatal("UniqueOpts.ByQueue = false, want true")
	}
	if !opts.UniqueOpts.ByArgs {
		t.Fatal("UniqueOpts.ByArgs = false, want true")
	}
}

func TestNewVMTombstoneCleanupWorkerRetention(t *testing.T) {
	t.Parallel()

	t.Run("defaults to one day when non-positive", func(t *testing.T) {
		w := NewVMTombstoneCleanupWorker(nil, 0)
		if w.retention != DefaultVMTombstoneRetention {
			t.Fatalf("retention = %s, want %s", w.retention, DefaultVMTombstoneRetention)
		}
	})

	t.Run("uses explicit retention when provided", func(t *testing.T) {
		want := 2 * time.Hour
		w := NewVMTombstoneCleanupWorker(nil, want)
		if w.retention != want {
			t.Fatalf("retention = %s, want %s", w.retention, want)
		}
	})
}

func TestVMTombstoneCleanupWorkerWork_Uninitialized(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver", func(t *testing.T) {
		var w *VMTombstoneCleanupWorker
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})

	t.Run("nil ent client", func(t *testing.T) {
		w := &VMTombstoneCleanupWorker{}
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})
}

func TestVMTombstoneCleanupWorkerWork_DeletesOnlyStaleInactiveTombstones(t *testing.T) {
	t.Parallel()
	requirePostgresForVMTombstoneCleanup(t)

	client := testutil.OpenEntPostgres(t, "vm_tombstone_cleanup_basic")
	ctx := t.Context()
	now := time.Now().UTC()
	serviceID := createVMTombstoneCleanupService(ctx, t, client)

	staleID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "stale", now.Add(-48*time.Hour))
	freshID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "fresh", now)

	worker := NewVMTombstoneCleanupWorker(client, 24*time.Hour)
	if err := worker.Work(ctx, nil); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	if _, err := client.VM.Get(ctx, staleID); !ent.IsNotFound(err) {
		t.Fatalf("stale tombstone get err = %v, want ent.IsNotFound", err)
	}
	fresh, err := client.VM.Get(ctx, freshID)
	if err != nil {
		t.Fatalf("fresh tombstone was removed: %v", err)
	}
	if fresh.Status != entvm.StatusDELETING {
		t.Fatalf("fresh tombstone status = %s, want %s", fresh.Status, entvm.StatusDELETING)
	}
}

func TestVMTombstoneCleanupWorkerWork_SkipsActiveDeleteEvent(t *testing.T) {
	t.Parallel()
	requirePostgresForVMTombstoneCleanup(t)

	client := testutil.OpenEntPostgres(t, "vm_tombstone_cleanup_active")
	ctx := t.Context()
	serviceID := createVMTombstoneCleanupService(ctx, t, client)
	vmID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "active", time.Now().UTC().Add(-48*time.Hour))
	createDeleteEventForTombstoneCleanupTest(ctx, t, client, vmID, domainevent.StatusPENDING)

	worker := NewVMTombstoneCleanupWorker(client, 24*time.Hour)
	if err := worker.Work(ctx, nil); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	stored, err := client.VM.Get(ctx, vmID)
	if err != nil {
		t.Fatalf("active tombstone was removed: %v", err)
	}
	if stored.Status != entvm.StatusDELETING {
		t.Fatalf("active tombstone status = %s, want %s", stored.Status, entvm.StatusDELETING)
	}
}

func TestVMTombstoneCleanupWorkerWork_ToleratesPerRowDeleteFailures(t *testing.T) {
	t.Parallel()
	requirePostgresForVMTombstoneCleanup(t)

	client := testutil.OpenEntPostgres(t, "vm_tombstone_cleanup_failure")
	ctx := t.Context()
	serviceID := createVMTombstoneCleanupService(ctx, t, client)
	old := time.Now().UTC().Add(-48 * time.Hour)
	blockedID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "blocked", old)
	freeID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "free", old)
	_, err := client.VMRevision.Create().
		SetID("rev-" + uuid.NewString()).
		SetRevision(1).
		SetSpec(map[string]interface{}{"seed": true}).
		SetChangedBy("tester").
		SetVMID(blockedID).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocking VM revision: %v", err)
	}

	worker := NewVMTombstoneCleanupWorker(client, 24*time.Hour)
	if err := worker.Work(ctx, nil); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	if _, err := client.VM.Get(ctx, blockedID); err != nil {
		t.Fatalf("blocked tombstone get err = %v, want row preserved after delete failure", err)
	}
	if _, err := client.VM.Get(ctx, freeID); !ent.IsNotFound(err) {
		t.Fatalf("free tombstone get err = %v, want ent.IsNotFound", err)
	}
}

func TestVMTombstoneCleanupWorkerWork_ReturnsContextCancellationFromDelete(t *testing.T) {
	t.Parallel()
	requirePostgresForVMTombstoneCleanup(t)

	client := testutil.OpenEntPostgres(t, "vm_tombstone_cleanup_cancel")
	ctx := t.Context()
	serviceID := createVMTombstoneCleanupService(ctx, t, client)
	vmID := createVMTombstoneForCleanupTest(ctx, t, client, serviceID, "cancel", time.Now().UTC().Add(-48*time.Hour))
	client.VM.Use(enthook.On(
		enthook.FixedError(errors.Join(errors.New("delete interrupted"), context.Canceled)),
		ent.OpDeleteOne,
	))

	worker := NewVMTombstoneCleanupWorker(client, 24*time.Hour)
	err := worker.Work(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Work() error = %v, want context.Canceled", err)
	}

	stored, err := client.VM.Get(ctx, vmID)
	if err != nil {
		t.Fatalf("tombstone get err = %v, want row preserved after cancellation", err)
	}
	if stored.Status != entvm.StatusDELETING {
		t.Fatalf("tombstone status = %s, want %s", stored.Status, entvm.StatusDELETING)
	}
}

func requirePostgresForVMTombstoneCleanup(t *testing.T) {
	t.Helper()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}
}

func createVMTombstoneCleanupService(ctx context.Context, t *testing.T, client *ent.Client) string {
	t.Helper()
	system, err := client.System.Create().
		SetID("sys-" + uuid.NewString()).
		SetName("sys" + uuid.NewString()[:8]).
		SetCreatedBy("tester").
		Save(ctx)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	service, err := client.Service.Create().
		SetID("svc-" + uuid.NewString()).
		SetName("svc" + uuid.NewString()[:8]).
		SetSystem(system).
		Save(ctx)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return service.ID
}

func createVMTombstoneForCleanupTest(
	ctx context.Context,
	t *testing.T,
	client *ent.Client,
	serviceID string,
	suffix string,
	updatedAt time.Time,
) string {
	t.Helper()
	vmID := "vm-" + suffix + "-" + uuid.NewString()
	createdAt := updatedAt.Add(-time.Hour)
	_, err := client.VM.Create().
		SetID(vmID).
		SetName("vm-" + suffix + "-" + uuid.NewString()[:8]).
		SetInstance("01").
		SetNamespace("prod-ns").
		SetClusterID("cluster-a").
		SetStatus(entvm.StatusDELETING).
		SetCreatedBy("tester").
		SetServiceID(serviceID).
		SetCreatedAt(createdAt.UTC()).
		SetUpdatedAt(updatedAt.UTC()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create VM tombstone %s: %v", suffix, err)
	}
	return vmID
}

func createDeleteEventForTombstoneCleanupTest(
	ctx context.Context,
	t *testing.T,
	client *ent.Client,
	vmID string,
	status domainevent.Status,
) {
	t.Helper()
	payloadBytes, err := domain.VMDeletePayload{
		VMID:      vmID,
		VMName:    "vm-active",
		ClusterID: "cluster-a",
		Namespace: "prod-ns",
		Actor:     "tester",
	}.ToJSON()
	if err != nil {
		t.Fatalf("marshal delete payload: %v", err)
	}
	_, err = client.DomainEvent.Create().
		SetID("ev-" + uuid.NewString()).
		SetEventType(string(domain.EventVMDeletionRequested)).
		SetAggregateType("vm").
		SetAggregateID(vmID).
		SetPayload(payloadBytes).
		SetStatus(status).
		SetCreatedBy("tester").
		Save(ctx)
	if err != nil {
		t.Fatalf("create delete event: %v", err)
	}
}
