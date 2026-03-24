package directoryview

import (
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
)

func TestDirectorySyncJobListToAPI_ComputesPaginationAndActions(t *testing.T) {
	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	rows := []*ent.DirectorySyncJob{
		{
			ID:                 "job-1",
			AuthProviderID:     "provider-1",
			Status:             directorysyncjob.StatusCompleted,
			ConflictResolution: "skip",
			CreateCount:        2,
			UpdateCount:        1,
			BlockedCount:       3,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	got := DirectorySyncJobListToAPI(rows, 2, 10, 21)
	if got.Pagination.Page != 2 || got.Pagination.PerPage != 10 || got.Pagination.Total != 21 || got.Pagination.TotalPages != 3 {
		t.Fatalf("DirectorySyncJobListToAPI() pagination = %#v", got.Pagination)
	}
	if len(got.Items) != 1 {
		t.Fatalf("DirectorySyncJobListToAPI() items len = %d, want 1", len(got.Items))
	}
	if got.Items[0].ResultSummary.CreateCount != 2 || got.Items[0].ResultSummary.UpdateCount != 1 || got.Items[0].ResultSummary.BlockedCount != 3 {
		t.Fatalf("DirectorySyncJobListToAPI() result summary = %#v", got.Items[0].ResultSummary)
	}
}

func TestUnsupportedDirectoryScheduleStatus_ReturnsDisabledUnsupported(t *testing.T) {
	got := UnsupportedDirectoryScheduleStatus()
	if got.Supported {
		t.Fatal("UnsupportedDirectoryScheduleStatus() supported = true, want false")
	}
	if got.Enabled {
		t.Fatal("UnsupportedDirectoryScheduleStatus() enabled = true, want false")
	}
}
