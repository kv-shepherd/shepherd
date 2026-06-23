package observability

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeTableStatsProvider struct {
	stats   []PostgresTableStats
	err     error
	inspect func(context.Context)
}

func (p fakeTableStatsProvider) TableStats(ctx context.Context) ([]PostgresTableStats, error) {
	if p.inspect != nil {
		p.inspect(ctx)
	}
	if p.err != nil {
		return nil, p.err
	}
	return p.stats, nil
}

func TestPostgresTableStatsCollectorEmitsTableAndRiverMetrics(t *testing.T) {
	metrics := NewMetrics(WithPostgresTableStatsProvider(fakeTableStatsProvider{
		stats: []PostgresTableStats{
			{
				Table:                          "river_job",
				LiveTuples:                     90,
				DeadTuples:                     10,
				DeadTupleRatio:                 0.1,
				LastAutovacuumTimestampSeconds: 1710000000,
			},
			{
				Table:                          "audit_logs",
				LiveTuples:                     80,
				DeadTuples:                     20,
				DeadTupleRatio:                 0.2,
				LastAutovacuumTimestampSeconds: 0,
			},
		},
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_postgres_table_stats_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 1 {
		t.Fatalf("scrape success = %v, want 1", got)
	}

	ratio := findMetric(t, families, "shepherd_postgres_table_dead_tuple_ratio", map[string]string{"table": "river_job"})
	if got := ratio.GetGauge().GetValue(); got != 0.1 {
		t.Fatalf("river_job ratio = %v, want 0.1", got)
	}

	riverAlias := findMetric(t, families, "shepherd_river_dead_tuple_ratio", nil)
	if got := riverAlias.GetGauge().GetValue(); got != 0.1 {
		t.Fatalf("river alias ratio = %v, want 0.1", got)
	}

	autovacuum := findMetric(t, families, "shepherd_postgres_table_last_autovacuum_timestamp_seconds", map[string]string{"table": "audit_logs"})
	if got := autovacuum.GetGauge().GetValue(); got != 0 {
		t.Fatalf("audit_logs last autovacuum = %v, want 0", got)
	}
}

func TestPostgresTableStatsCollectorReportsScrapeFailure(t *testing.T) {
	metrics := NewMetrics(WithPostgresTableStatsProvider(fakeTableStatsProvider{
		err: errors.New("database unavailable"),
	}))

	families, err := metrics.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}

	success := findMetric(t, families, "shepherd_postgres_table_stats_scrape_success", nil)
	if got := success.GetGauge().GetValue(); got != 0 {
		t.Fatalf("scrape success = %v, want 0", got)
	}
}

func TestPostgresTableStatsCollectorPassesBoundedScrapeContext(t *testing.T) {
	seenDeadline := false
	metrics := NewMetrics(WithPostgresTableStatsProvider(fakeTableStatsProvider{
		inspect: func(ctx context.Context) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("TableStats context missing deadline")
			}
			remaining := time.Until(deadline)
			if remaining <= 0 || remaining > defaultCollectorScrapeTimeout {
				t.Fatalf("TableStats context remaining deadline = %s, want within %s", remaining, defaultCollectorScrapeTimeout)
			}
			seenDeadline = true
		},
	}))

	if _, err := metrics.Gather(); err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	if !seenDeadline {
		t.Fatal("TableStats provider was not called")
	}
}
