package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	entnotification "kv-shepherd.io/shepherd/ent/notification"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func createNotificationCleanupTestUser(t *testing.T, client *ent.Client, userID string) {
	t.Helper()

	if _, err := client.User.Create().
		SetID(userID).
		SetUsername(userID).
		Save(t.Context()); err != nil {
		t.Fatalf("create notification cleanup user %s: %v", userID, err)
	}
}

func createNotificationCleanupTestNotification(t *testing.T, client *ent.Client, id, userID string, createdAt time.Time) {
	t.Helper()

	if _, err := client.Notification.Create().
		SetID(id).
		SetType(entnotification.TypeAPPROVAL_PENDING).
		SetTitle("title-" + id).
		SetMessage("message-" + id).
		SetUserID(userID).
		SetCreatedAt(createdAt.UTC()).
		Save(t.Context()); err != nil {
		t.Fatalf("create notification %s: %v", id, err)
	}
}

func assertNotificationExists(t *testing.T, client *ent.Client, notificationID string) {
	t.Helper()

	if _, err := client.Notification.Get(t.Context(), notificationID); err != nil {
		t.Fatalf("notification %s should exist: %v", notificationID, err)
	}
}

func assertNotificationDeleted(t *testing.T, client *ent.Client, notificationID string) {
	t.Helper()

	_, err := client.Notification.Get(t.Context(), notificationID)
	if !ent.IsNotFound(err) {
		t.Fatalf("notification %s get error = %v, want not found", notificationID, err)
	}
}

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

func TestNotificationCleanupWorkerWork_DeletesOnlyExpiredNotifications(t *testing.T) {
	client := testutil.OpenEntPostgres(t, "notification_cleanup")
	now := time.Now().UTC()
	userAID := "notification-cleanup-user-a"
	userBID := "notification-cleanup-user-b"
	createNotificationCleanupTestUser(t, client, userAID)
	createNotificationCleanupTestUser(t, client, userBID)

	createNotificationCleanupTestNotification(t, client, "notif-expired-a", userAID, now.Add(-8*24*time.Hour))
	createNotificationCleanupTestNotification(t, client, "notif-expired-b", userBID, now.Add(-10*24*time.Hour))
	createNotificationCleanupTestNotification(t, client, "notif-retained-recent", userAID, now.Add(-2*24*time.Hour))
	createNotificationCleanupTestNotification(t, client, "notif-retained-inside-window", userBID, now.Add(-6*24*time.Hour))

	worker := NewNotificationCleanupWorker(client, 7*24*time.Hour)
	if err := worker.Work(t.Context(), &river.Job[NotificationCleanupArgs]{}); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	assertNotificationDeleted(t, client, "notif-expired-a")
	assertNotificationDeleted(t, client, "notif-expired-b")
	assertNotificationExists(t, client, "notif-retained-recent")
	assertNotificationExists(t, client, "notif-retained-inside-window")
}
