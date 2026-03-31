package directoryview

import (
	"reflect"
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/directorysyncjob"
	directorycontract "kv-shepherd.io/shepherd/internal/provider/directorycontract"
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

func TestDirectorySyncJobDetailToAPI_UsesEmptyErrorsSlice(t *testing.T) {
	now := time.Date(2026, 3, 31, 1, 2, 3, 0, time.UTC)
	row := &ent.DirectorySyncJob{
		ID:                 "job-empty-errors",
		AuthProviderID:     "provider-1",
		Status:             directorysyncjob.StatusCompleted,
		ConflictResolution: "skip",
		RequestSnapshot: map[string]interface{}{
			"department_names": []string{"Engineering"},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	got := DirectorySyncJobDetailToAPI(row)
	if got.Errors == nil {
		t.Fatal("DirectorySyncJobDetailToAPI() errors = nil, want empty slice")
	}
	if len(got.Errors) != 0 {
		t.Fatalf("DirectorySyncJobDetailToAPI() errors len = %d, want 0", len(got.Errors))
	}
	if got.RequestSnapshot["department_names"] == nil {
		t.Fatal("DirectorySyncJobDetailToAPI() request_snapshot missing department_names")
	}
}

func TestDirectoryScheduleStatusFromPlan_IncludesProviderRequest(t *testing.T) {
	now := time.Date(2026, 3, 31, 1, 2, 3, 0, time.UTC)
	plan := &directorycontract.ScheduledDirectoryEnrichmentPlan{
		Enabled:          true,
		Mode:             directorycontract.DirectoryEnrichmentModeEnrichExistingOnly,
		JoinKeyType:      "username",
		ScheduleCron:     "0 * * * *",
		ScheduleTimezone: "Asia/Shanghai",
		ProviderRequest: map[string]interface{}{
			"department_names": []string{"Engineering"},
			"include_nested":   true,
		},
	}

	got, err := DirectoryScheduleStatusFromPlan(plan, nil, nil, now)
	if err != nil {
		t.Fatalf("DirectoryScheduleStatusFromPlan() error = %v", err)
	}
	if got.ProviderRequest == nil {
		t.Fatal("DirectoryScheduleStatusFromPlan() provider_request = nil, want map")
	}
	if !reflect.DeepEqual(got.ProviderRequest, plan.ProviderRequest) {
		t.Fatalf("DirectoryScheduleStatusFromPlan() provider_request = %#v, want %#v", got.ProviderRequest, plan.ProviderRequest)
	}

	got.ProviderRequest["include_nested"] = false
	if plan.ProviderRequest["include_nested"] != true {
		t.Fatal("DirectoryScheduleStatusFromPlan() provider_request was not cloned")
	}
}
