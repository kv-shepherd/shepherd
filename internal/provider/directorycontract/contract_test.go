package directorycontract

import (
	"encoding/json"
	"testing"
)

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

func TestDirectoryPreviewItemSupportsCanonicalIdentitySignals(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(DirectoryPreviewItem{
		Match: DirectoryPreviewMatch{
			Action:    DirectoryActionUpdate,
			MatchedBy: DirectoryPreviewMatchByCanonicalIdentity,
		},
		Conflicts: []DirectoryConflict{
			{Code: DirectoryConflictSameCanonicalIdentity},
		},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded DirectoryPreviewItem
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.Match.MatchedBy != DirectoryPreviewMatchByCanonicalIdentity {
		t.Fatalf("decoded.Match.MatchedBy = %q, want %q", decoded.Match.MatchedBy, DirectoryPreviewMatchByCanonicalIdentity)
	}
	if len(decoded.Conflicts) != 1 || decoded.Conflicts[0].Code != DirectoryConflictSameCanonicalIdentity {
		t.Fatalf("decoded.Conflicts = %#v, want same_canonical_identity", decoded.Conflicts)
	}
}
