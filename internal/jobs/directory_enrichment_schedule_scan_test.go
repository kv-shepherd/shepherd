package jobs

import (
	"os"
	"strings"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
	"kv-shepherd.io/shepherd/internal/testutil"
)

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
	if strings.TrimSpace(os.Getenv("TEST_DATABASE_URL")) == "" && strings.TrimSpace(os.Getenv("DATABASE_URL")) == "" {
		t.Skip("PostgreSQL test DSN is required: set TEST_DATABASE_URL or DATABASE_URL")
	}

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
