package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	// DefaultDomainEventArchiveRetention is the V1 baseline retention window for
	// hot DomainEvent rows before they are marked archived.
	DefaultDomainEventArchiveRetention = 30 * 24 * time.Hour
)

// DomainEventArchiveArgs is a periodic maintenance job that marks cold,
// terminal DomainEvent rows as archived.
type DomainEventArchiveArgs struct{}

// Kind returns the job kind identifier for periodic domain-event archiving.
func (DomainEventArchiveArgs) Kind() string { return "event_archive" }

// InsertOpts ensures at most one archive job is enqueued within the same day.
func (DomainEventArchiveArgs) InsertOpts() river.InsertOpts {
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

// DomainEventArchiveWorker marks old terminal events as archived.
type DomainEventArchiveWorker struct {
	river.WorkerDefaults[DomainEventArchiveArgs]
	entClient *ent.Client
	retention time.Duration
}

// NewDomainEventArchiveWorker creates an archive worker. Non-positive retention
// falls back to the 30-day default.
func NewDomainEventArchiveWorker(entClient *ent.Client, retention time.Duration) *DomainEventArchiveWorker {
	if retention <= 0 {
		retention = DefaultDomainEventArchiveRetention
	}
	return &DomainEventArchiveWorker{
		entClient: entClient,
		retention: retention,
	}
}

// Work marks eligible events with archived_at while preserving the immutable
// event payload/history.
func (w *DomainEventArchiveWorker) Work(ctx context.Context, _ *river.Job[DomainEventArchiveArgs]) error {
	if w == nil || w.entClient == nil {
		return fmt.Errorf("domain event archive worker is not initialized")
	}

	now := time.Now().UTC()
	cutoff := now.Add(-w.retention)
	updated, err := w.entClient.DomainEvent.Update().
		Where(
			domainevent.StatusIn(
				domainevent.StatusCOMPLETED,
				domainevent.StatusFAILED,
				domainevent.StatusCANCELLED,
			),
			domainevent.CreatedAtLT(cutoff),
			domainevent.ArchivedAtIsNil(),
		).
		SetArchivedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("archive domain events before %s: %w", cutoff.Format(time.RFC3339), err)
	}

	logger.Info("domain event archive completed",
		zap.Int("archived_rows", updated),
		zap.String("cutoff", cutoff.Format(time.RFC3339)),
		zap.Duration("retention", w.retention),
	)
	return nil
}
