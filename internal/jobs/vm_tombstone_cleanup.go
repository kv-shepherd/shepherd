package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	// DefaultVMTombstoneRetention is the grace period before retrying hard
	// deletion of VM rows left in DELETING after K8s deletion succeeded.
	DefaultVMTombstoneRetention = 24 * time.Hour
	vmTombstoneCleanupBatchSize = 500
)

// VMTombstoneCleanupArgs is a periodic maintenance job that retries hard-delete
// cleanup for stale VM tombstones.
type VMTombstoneCleanupArgs struct{}

// Kind returns the job kind identifier for VM tombstone cleanup.
func (VMTombstoneCleanupArgs) Kind() string { return "vm_tombstone_cleanup" }

// InsertOpts ensures at most one tombstone cleanup job is enqueued per day.
func (VMTombstoneCleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       river.QueueDefault,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByPeriod: 24 * time.Hour,
			ByQueue:  true,
			ByArgs:   true,
		},
	}
}

// VMTombstoneCleanupWorker removes stale DELETING VM rows after the delete
// execution path has already removed the K8s resource.
type VMTombstoneCleanupWorker struct {
	river.WorkerDefaults[VMTombstoneCleanupArgs]
	entClient *ent.Client
	retention time.Duration
}

// NewVMTombstoneCleanupWorker creates a VM tombstone cleanup worker.
// Non-positive retention falls back to the 24-hour default.
func NewVMTombstoneCleanupWorker(entClient *ent.Client, retention time.Duration) *VMTombstoneCleanupWorker {
	if retention <= 0 {
		retention = DefaultVMTombstoneRetention
	}
	return &VMTombstoneCleanupWorker{
		entClient: entClient,
		retention: retention,
	}
}

// Work retries hard deletion for stale DELETING rows. Per-row delete failures
// are logged and do not fail the maintenance job so one blocked tombstone cannot
// starve cleanup for later rows.
func (w *VMTombstoneCleanupWorker) Work(ctx context.Context, _ *river.Job[VMTombstoneCleanupArgs]) error {
	if w == nil || w.entClient == nil {
		return fmt.Errorf("vm tombstone cleanup worker is not initialized")
	}

	cutoff := time.Now().UTC().Add(-w.retention)
	candidates, err := w.entClient.VM.Query().
		Where(
			vm.StatusEQ(vm.StatusDELETING),
			vm.UpdatedAtLT(cutoff),
		).
		Order(ent.Asc(vm.FieldUpdatedAt)).
		Limit(vmTombstoneCleanupBatchSize).
		All(ctx)
	if err != nil {
		return fmt.Errorf("list stale VM tombstones before %s: %w", cutoff.Format(time.RFC3339), err)
	}

	deleted := 0
	skippedActive := 0
	failed := 0
	for _, candidate := range candidates {
		active, err := w.hasActiveDeleteEvent(ctx, candidate.ID)
		if err != nil {
			return fmt.Errorf("check active delete event for VM tombstone %s: %w", candidate.ID, err)
		}
		if active {
			skippedActive++
			continue
		}
		if err := w.entClient.VM.DeleteOneID(candidate.ID).Exec(ctx); err != nil {
			failed++
			logger.Warn("failed to hard-delete stale VM tombstone",
				zap.String("vm_id", candidate.ID),
				zap.String("vm_name", candidate.Name),
				zap.Error(err),
			)
			continue
		}
		deleted++
	}

	logger.Info("vm tombstone cleanup completed",
		zap.Int("candidates", len(candidates)),
		zap.Int("deleted_rows", deleted),
		zap.Int("skipped_active_rows", skippedActive),
		zap.Int("failed_rows", failed),
		zap.String("cutoff", cutoff.Format(time.RFC3339)),
		zap.Duration("retention", w.retention),
	)
	return nil
}

func (w *VMTombstoneCleanupWorker) hasActiveDeleteEvent(ctx context.Context, vmID string) (bool, error) {
	return w.entClient.DomainEvent.Query().
		Where(
			domainevent.AggregateTypeEQ("vm"),
			domainevent.AggregateIDEQ(vmID),
			domainevent.EventTypeEQ(string(domain.EventVMDeletionRequested)),
			domainevent.StatusIn(domainevent.StatusPENDING, domainevent.StatusPROCESSING),
		).
		Exist(ctx)
}
