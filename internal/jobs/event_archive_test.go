package jobs

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/domainevent"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/testutil"
)

func TestDomainEventArchiveArgsKind(t *testing.T) {
	t.Parallel()

	if got := (DomainEventArchiveArgs{}).Kind(); got != "event_archive" {
		t.Fatalf("Kind() = %q, want %q", got, "event_archive")
	}
}

func TestDomainEventArchiveArgsInsertOpts(t *testing.T) {
	t.Parallel()

	opts := (DomainEventArchiveArgs{}).InsertOpts()
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

func TestNewDomainEventArchiveWorkerRetention(t *testing.T) {
	t.Parallel()

	t.Run("defaults to thirty days when non-positive", func(t *testing.T) {
		w := NewDomainEventArchiveWorker(nil, 0)
		if w.retention != DefaultDomainEventArchiveRetention {
			t.Fatalf("retention = %s, want %s", w.retention, DefaultDomainEventArchiveRetention)
		}
	})

	t.Run("uses explicit retention when provided", func(t *testing.T) {
		want := 14 * 24 * time.Hour
		w := NewDomainEventArchiveWorker(nil, want)
		if w.retention != want {
			t.Fatalf("retention = %s, want %s", w.retention, want)
		}
	})
}

func TestDomainEventArchiveWorkerWork_Uninitialized(t *testing.T) {
	t.Parallel()

	t.Run("nil receiver", func(t *testing.T) {
		var w *DomainEventArchiveWorker
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})

	t.Run("nil ent client", func(t *testing.T) {
		w := &DomainEventArchiveWorker{}
		err := w.Work(context.Background(), nil)
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Fatalf("Work() error = %v, want contains %q", err, "not initialized")
		}
	})
}

func TestDomainEventArchiveWorkerWork_ArchivesOnlyOldTerminalEvents(t *testing.T) {
	t.Parallel()
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}
	_ = logger.Init("error", "json")

	client := testutil.OpenEntPostgres(t, "domain_event_archive")
	ctx := t.Context()
	now := time.Now().UTC()

	oldCompletedID := createDomainEventForArchiveTest(ctx, t, client, "evt-old-completed", domainevent.StatusCOMPLETED, now.Add(-31*24*time.Hour), nil)
	oldFailedID := createDomainEventForArchiveTest(ctx, t, client, "evt-old-failed", domainevent.StatusFAILED, now.Add(-35*24*time.Hour), nil)
	oldCancelledID := createDomainEventForArchiveTest(ctx, t, client, "evt-old-cancelled", domainevent.StatusCANCELLED, now.Add(-40*24*time.Hour), nil)
	oldPendingID := createDomainEventForArchiveTest(ctx, t, client, "evt-old-pending", domainevent.StatusPENDING, now.Add(-45*24*time.Hour), nil)
	oldProcessingID := createDomainEventForArchiveTest(ctx, t, client, "evt-old-processing", domainevent.StatusPROCESSING, now.Add(-50*24*time.Hour), nil)
	recentCompletedID := createDomainEventForArchiveTest(ctx, t, client, "evt-recent-completed", domainevent.StatusCOMPLETED, now.Add(-7*24*time.Hour), nil)
	alreadyArchivedAt := now.Add(-2 * 24 * time.Hour)
	alreadyArchivedID := createDomainEventForArchiveTest(ctx, t, client, "evt-already-archived", domainevent.StatusCOMPLETED, now.Add(-45*24*time.Hour), &alreadyArchivedAt)

	worker := NewDomainEventArchiveWorker(client, 30*24*time.Hour)
	if err := worker.Work(ctx, nil); err != nil {
		t.Fatalf("Work() error = %v", err)
	}

	assertArchived := func(eventID string) {
		t.Helper()
		ev, err := client.DomainEvent.Get(ctx, eventID)
		if err != nil {
			t.Fatalf("get event %s: %v", eventID, err)
		}
		if ev.ArchivedAt == nil {
			t.Fatalf("event %s archived_at = nil, want non-nil", eventID)
		}
	}
	assertNotArchived := func(eventID string) {
		t.Helper()
		ev, err := client.DomainEvent.Get(ctx, eventID)
		if err != nil {
			t.Fatalf("get event %s: %v", eventID, err)
		}
		if ev.ArchivedAt != nil {
			t.Fatalf("event %s archived_at = %v, want nil", eventID, ev.ArchivedAt)
		}
	}

	assertArchived(oldCompletedID)
	assertArchived(oldFailedID)
	assertArchived(oldCancelledID)
	assertNotArchived(oldPendingID)
	assertNotArchived(oldProcessingID)
	assertNotArchived(recentCompletedID)

	alreadyArchived, err := client.DomainEvent.Get(ctx, alreadyArchivedID)
	if err != nil {
		t.Fatalf("get event %s: %v", alreadyArchivedID, err)
	}
	if alreadyArchived.ArchivedAt == nil {
		t.Fatalf("event %s archived_at = nil, want preserved value", alreadyArchivedID)
	}
	expectedArchivedAt := alreadyArchivedAt.UTC().Truncate(time.Microsecond)
	if !alreadyArchived.ArchivedAt.Equal(expectedArchivedAt) {
		t.Fatalf("event %s archived_at = %s, want %s", alreadyArchivedID, alreadyArchived.ArchivedAt.UTC(), expectedArchivedAt)
	}
}

func createDomainEventForArchiveTest(
	ctx context.Context,
	t *testing.T,
	client *ent.Client,
	eventID string,
	status domainevent.Status,
	createdAt time.Time,
	archivedAt *time.Time,
) string {
	t.Helper()

	builder := client.DomainEvent.Create().
		SetID(eventID).
		SetEventType("VM_CREATION_REQUESTED").
		SetAggregateType("vm").
		SetAggregateID("vm-" + eventID).
		SetPayload([]byte(`{"seed":true}`)).
		SetStatus(status).
		SetCreatedBy("tester").
		SetCreatedAt(createdAt.UTC())
	if archivedAt != nil {
		builder = builder.SetArchivedAt(archivedAt.UTC())
	}
	if _, err := builder.Save(ctx); err != nil {
		t.Fatalf("create domain event %s: %v", eventID, err)
	}
	return eventID
}
