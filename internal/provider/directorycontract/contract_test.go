package directorycontract

import "testing"

func TestDirectoryActionSummaryAdd(t *testing.T) {
	var summary DirectoryActionSummary
	summary.Add(DirectoryActionCreate)
	summary.Add(DirectoryActionUpdate)
	summary.Add(DirectoryActionBlocked)

	if summary.CreateCount != 1 || summary.UpdateCount != 1 || summary.BlockedCount != 1 {
		t.Fatalf("summary = %#v, want 1/1/1", summary)
	}
}

func TestNormalizeScheduledDirectoryEnrichmentPlanDefaults(t *testing.T) {
	plan, location, err := NormalizeScheduledDirectoryEnrichmentPlan(&ScheduledDirectoryEnrichmentPlan{
		Enabled:      true,
		ScheduleCron: "0 * * * *",
	})
	if err != nil {
		t.Fatalf("NormalizeScheduledDirectoryEnrichmentPlan() error = %v", err)
	}
	if plan.JoinKeyType != DirectoryJoinKeyUsername {
		t.Fatalf("plan.JoinKeyType = %q, want %q", plan.JoinKeyType, DirectoryJoinKeyUsername)
	}
	if plan.Mode != DirectoryEnrichmentModeEnrichExistingOnly {
		t.Fatalf("plan.Mode = %q, want %q", plan.Mode, DirectoryEnrichmentModeEnrichExistingOnly)
	}
	if got := location.String(); got != "UTC" {
		t.Fatalf("location = %q, want %q", got, "UTC")
	}
}
