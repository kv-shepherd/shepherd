package provider

import "testing"

func TestGenericAuthProviderSupportsDirectorySyncCapability(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("generic")
	if adapter == nil {
		t.Fatal("generic auth provider adapter is nil")
	}

	directoryCapability, ok := adapter.(DirectorySyncCapability)
	if !ok {
		t.Fatal("generic auth provider adapter does not implement DirectorySyncCapability")
	}

	descriptor := directoryCapability.DescribeDirectorySync()
	if !descriptor.SupportsPreview {
		t.Fatal("generic directory sync descriptor should support preview")
	}
	if len(descriptor.RequestSchema) == 0 {
		t.Fatal("generic directory sync descriptor request_schema is empty")
	}
	if _, ok := adapter.(ScheduledDirectoryEnrichmentCapability); !ok {
		t.Fatal("generic auth provider adapter does not implement ScheduledDirectoryEnrichmentCapability")
	}
}

func TestGenericAuthProviderDirectorySyncFiltersSampleUsers(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("generic")
	directoryCapability, ok := adapter.(DirectorySyncCapability)
	if !ok {
		t.Fatal("generic auth provider adapter does not implement DirectorySyncCapability")
	}

	records, err := directoryCapability.ListDirectoryUsers(t.Context(), map[string]interface{}{
		"sample_users": []interface{}{
			map[string]interface{}{
				"external_id":  "ext-1",
				"username":     "alice",
				"display_name": "Alice",
				"email":        "alice@example.com",
				"groups":       []interface{}{"dev", "ops"},
			},
			map[string]interface{}{
				"external_id":  "ext-2",
				"username":     "bob",
				"display_name": "Bob",
				"email":        "bob@example.com",
			},
		},
	}, map[string]interface{}{
		"selected_usernames": []interface{}{"alice"},
		"limit":              float64(1),
	})
	if err != nil {
		t.Fatalf("list directory users: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records len = %d, want 1", len(records))
	}
	if records[0].Username != "alice" || records[0].ExternalID != "ext-1" {
		t.Fatalf("record = %#v", records[0])
	}
	if len(records[0].Cohorts) != 2 {
		t.Fatalf("record.Cohorts = %#v, want 2 cohorts", records[0].Cohorts)
	}
	if records[0].Cohorts[0].Kind != "group" || records[0].Cohorts[0].Key != "dev" {
		t.Fatalf("first cohort = %#v, want group:dev", records[0].Cohorts[0])
	}
}

func TestGenericAuthProviderDirectorySyncRejectsInvalidRequestShape(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("generic")
	directoryCapability, ok := adapter.(DirectorySyncCapability)
	if !ok {
		t.Fatal("generic auth provider adapter does not implement DirectorySyncCapability")
	}

	if _, err := directoryCapability.ListDirectoryUsers(t.Context(), nil, map[string]interface{}{
		"limit": "invalid",
	}); err == nil {
		t.Fatal("expected invalid request error, got nil")
	}
}

func TestGenericAuthProviderBuildsScheduledDirectoryEnrichmentPlan(t *testing.T) {
	adapter := ResolveAuthProviderAdminAdapter("generic")
	scheduledCapability, ok := adapter.(ScheduledDirectoryEnrichmentCapability)
	if !ok {
		t.Fatal("generic auth provider adapter does not implement ScheduledDirectoryEnrichmentCapability")
	}

	plan, err := scheduledCapability.BuildScheduledDirectoryEnrichmentPlan(t.Context(), map[string]interface{}{
		"enrichment_enabled": true,
		"schedule_cron":      "15 * * * *",
		"schedule_timezone":  "Asia/Shanghai",
		"join_key_type":      string(DirectoryJoinKeyUsername),
		"scheduled_provider_request": map[string]interface{}{
			"selected_usernames": []interface{}{"alice"},
			"limit":              float64(1),
		},
	})
	if err != nil {
		t.Fatalf("build scheduled enrichment plan: %v", err)
	}
	if !plan.Enabled {
		t.Fatal("plan.Enabled = false, want true")
	}
	if plan.Mode != DirectoryEnrichmentModeEnrichExistingOnly {
		t.Fatalf("plan.Mode = %q, want %q", plan.Mode, DirectoryEnrichmentModeEnrichExistingOnly)
	}
	if plan.JoinKeyType != DirectoryJoinKeyUsername {
		t.Fatalf("plan.JoinKeyType = %q, want %q", plan.JoinKeyType, DirectoryJoinKeyUsername)
	}
	if plan.ScheduleCron != "15 * * * *" {
		t.Fatalf("plan.ScheduleCron = %q, want %q", plan.ScheduleCron, "15 * * * *")
	}
	if plan.ScheduleTimezone != "Asia/Shanghai" {
		t.Fatalf("plan.ScheduleTimezone = %q, want %q", plan.ScheduleTimezone, "Asia/Shanghai")
	}
	if _, ok := plan.ProviderRequest["selected_usernames"]; !ok {
		t.Fatalf("plan.ProviderRequest = %#v, want selected_usernames", plan.ProviderRequest)
	}
}
