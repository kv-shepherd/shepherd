package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/notification"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
)

const (
	// DefaultNotificationRetention is the V1 retention baseline for inbox
	// notifications (master-flow Stage 5.F / phase-4 checklist).
	DefaultNotificationRetention = 90 * 24 * time.Hour
)

// NotificationCleanupArgs is a periodic maintenance job that removes expired
// notifications from the platform inbox.
type NotificationCleanupArgs struct{}

// Kind returns the job kind identifier for periodic notification cleanup.
func (NotificationCleanupArgs) Kind() string { return "notification_cleanup" }

// InsertOpts ensures at most one cleanup job is enqueued within the same day.
func (NotificationCleanupArgs) InsertOpts() river.InsertOpts {
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

// NotificationCleanupWorker deletes notifications older than the configured
// retention duration.
type NotificationCleanupWorker struct {
	river.WorkerDefaults[NotificationCleanupArgs]
	entClient *ent.Client
	retention time.Duration
}

// NewNotificationCleanupWorker creates a cleanup worker. Non-positive retention
// falls back to the 90-day default.
func NewNotificationCleanupWorker(entClient *ent.Client, retention time.Duration) *NotificationCleanupWorker {
	if retention <= 0 {
		retention = DefaultNotificationRetention
	}
	return &NotificationCleanupWorker{
		entClient: entClient,
		retention: retention,
	}
}

// Work removes expired notification rows.
func (w *NotificationCleanupWorker) Work(ctx context.Context, _ *river.Job[NotificationCleanupArgs]) error {
	if w == nil || w.entClient == nil {
		return fmt.Errorf("notification cleanup worker is not initialized")
	}

	cutoff := time.Now().UTC().Add(-w.retention)
	deleted, err := w.entClient.Notification.Delete().
		Where(notification.CreatedAtLT(cutoff)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete expired notifications before %s: %w", cutoff.Format(time.RFC3339), err)
	}

	logger.Info("notification cleanup completed",
		zap.Int("deleted_rows", deleted),
		zap.String("cutoff", cutoff.Format(time.RFC3339)),
		zap.Duration("retention", w.retention),
	)
	return nil
}
