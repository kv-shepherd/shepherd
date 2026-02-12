package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
)

func TestNotificationCleanupArgsKind(t *testing.T) {
	t.Parallel()

	if got := (NotificationCleanupArgs{}).Kind(); got != "notification_cleanup" {
		t.Fatalf("Kind() = %q, want %q", got, "notification_cleanup")
	}
}

func TestNotificationCleanupArgsInsertOpts(t *testing.T) {
	t.Parallel()

	opts := (NotificationCleanupArgs{}).InsertOpts()
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

func TestNewNotificationCleanupWorkerRetention(t *testing.T) {
	t.Parallel()

	t.Run("defaults to ninety days when non-positive", func(t *testing.T) {
		w := NewNotificationCleanupWorker(nil, 0)
		if w.retention != DefaultNotificationRetention {
			t.Fatalf("retention = %s, want %s", w.retention, DefaultNotificationRetention)
		}
	})

	t.Run("uses explicit retention when provided", func(t *testing.T) {
		want := 7 * 24 * time.Hour
		w := NewNotificationCleanupWorker(nil, want)
		if w.retention != want {
			t.Fatalf("retention = %s, want %s", w.retention, want)
		}
	})
}

func TestNotificationCleanupWorkerWork_Uninitialized(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver", func(t *testing.T) {
		var w *NotificationCleanupWorker
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})

	t.Run("nil ent client", func(t *testing.T) {
		w := &NotificationCleanupWorker{}
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})
}

