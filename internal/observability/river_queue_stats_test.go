package observability

import (
	"context"
	"errors"
	"testing"
)

type fakeRiverQueueStatsProvider struct {
	stats RiverQueueStats
	err   error
}

func (p fakeRiverQueueStatsProvider) RiverQueueStats(context.Context) (RiverQueueStats, error) {
	if p.err != nil {
		return RiverQueueStats{}, p.err
	}
	return p.stats, nil
}

func TestRiverQueueStatsCollectorEmitsQueueHealthMetrics(t *testing.T) {
	metrics := NewMetrics(WithRiverQueueStatsProvider(fakeRiverQueueStatsProvider{
		stats: RiverQueueStats{
			JobsByState: []RiverJobStateCount{
				{Queue: "vm_operations", State: "available", Kind: "vm_create", Count: 3},
				{Queue: "vm_operations", State: "running", Kind: "vm_delete", Count: 1},
			},
			ReadyJobs: []RiverReadyJobCount{
				{Queue: "vm_operations", Kind: "vm_create", Count: 3},
			},
			OldestReadyJobAges: []RiverOldestReadyJobAge{
				{Queue: "vm_operations", AgeSeconds: 420},
			},
			RecentTerminalJobs: []RiverRecentTerminalJobCount{
				{Queue: "vm_operations", State: "discarded", Kind: "vm_create", Count: 1},
			},
		},
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_river_queue_stats_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 1 {
		t.Fatalf("scrape success = %v, want 1", got)
	}

	available := findMetric(t, families, "shepherd_river_jobs_by_state", map[string]string{
		"queue": "vm_operations",
		"state": "available",
		"kind":  "vm_create",
	})
	if got := available.GetGauge().GetValue(); got != 3 {
		t.Fatalf("jobs by state = %v, want 3", got)
	}

	ready := findMetric(t, families, "shepherd_river_ready_jobs", map[string]string{
		"queue": "vm_operations",
		"kind":  "vm_create",
	})
	if got := ready.GetGauge().GetValue(); got != 3 {
		t.Fatalf("ready jobs = %v, want 3", got)
	}

	age := findMetric(t, families, "shepherd_river_oldest_ready_job_age_seconds", map[string]string{
		"queue": "vm_operations",
	})
	if got := age.GetGauge().GetValue(); got != 420 {
		t.Fatalf("oldest ready age = %v, want 420", got)
	}

	discarded := findMetric(t, families, "shepherd_river_recent_terminal_jobs", map[string]string{
		"queue": "vm_operations",
		"state": "discarded",
		"kind":  "vm_create",
	})
	if got := discarded.GetGauge().GetValue(); got != 1 {
		t.Fatalf("recent terminal jobs = %v, want 1", got)
	}
}

func TestRiverQueueStatsCollectorReportsScrapeFailure(t *testing.T) {
	metrics := NewMetrics(WithRiverQueueStatsProvider(fakeRiverQueueStatsProvider{
		err: errors.New("database unavailable"),
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_river_queue_stats_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 0 {
		t.Fatalf("scrape success = %v, want 0", got)
	}
}
