package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	riverQueueStatsRecentTerminalWindow = 15 * time.Minute

	riverJobsByStateQuery = `
SELECT queue, state::text, kind, count(*)::float8
FROM river_job
GROUP BY queue, state, kind
ORDER BY queue, state, kind
`

	riverReadyJobsQuery = `
SELECT queue, kind, count(*)::float8
FROM river_job
WHERE state IN ('available', 'retryable', 'scheduled')
  AND scheduled_at <= now()
GROUP BY queue, kind
ORDER BY queue, kind
`

	riverOldestReadyJobAgeQuery = `
SELECT queue, GREATEST(EXTRACT(EPOCH FROM now() - MIN(scheduled_at)), 0)::float8
FROM river_job
WHERE state IN ('available', 'retryable', 'scheduled')
  AND scheduled_at <= now()
GROUP BY queue
ORDER BY queue
`

	riverRecentTerminalJobsQuery = `
SELECT queue, state::text, kind, count(*)::float8
FROM river_job
WHERE state IN ('cancelled', 'discarded')
  AND finalized_at IS NOT NULL
  AND finalized_at >= now() - ($1::int * interval '1 second')
GROUP BY queue, state, kind
ORDER BY queue, state, kind
`
)

// RiverJobStateCount is one aggregate River job count by queue, state, and kind.
type RiverJobStateCount struct {
	Queue string
	State string
	Kind  string
	Count float64
}

// RiverReadyJobCount is one aggregate ready-job count by queue and kind.
type RiverReadyJobCount struct {
	Queue string
	Kind  string
	Count float64
}

// RiverOldestReadyJobAge is the oldest ready-job age for one River queue.
type RiverOldestReadyJobAge struct {
	Queue      string
	AgeSeconds float64
}

// RiverRecentTerminalJobCount is one recent cancelled/discarded aggregate.
type RiverRecentTerminalJobCount struct {
	Queue string
	State string
	Kind  string
	Count float64
}

// RiverQueueStats is the scrape-time River queue health snapshot.
type RiverQueueStats struct {
	JobsByState        []RiverJobStateCount
	ReadyJobs          []RiverReadyJobCount
	OldestReadyJobAges []RiverOldestReadyJobAge
	RecentTerminalJobs []RiverRecentTerminalJobCount
}

// RiverQueueStatsProvider provides aggregate River queue statistics.
type RiverQueueStatsProvider interface {
	RiverQueueStats(context.Context) (RiverQueueStats, error)
}

type pgxRiverQueueStatsProvider struct {
	queryer              pgxQueryer
	timeout              time.Duration
	recentTerminalWindow time.Duration
}

// NewPGXRiverQueueStatsProvider reads River queue statistics through pgx.
func NewPGXRiverQueueStatsProvider(pool *pgxpool.Pool, timeout time.Duration) RiverQueueStatsProvider {
	if pool == nil {
		return &pgxRiverQueueStatsProvider{
			timeout:              timeout,
			recentTerminalWindow: riverQueueStatsRecentTerminalWindow,
		}
	}
	return &pgxRiverQueueStatsProvider{
		queryer:              pool,
		timeout:              timeout,
		recentTerminalWindow: riverQueueStatsRecentTerminalWindow,
	}
}

func (p *pgxRiverQueueStatsProvider) RiverQueueStats(ctx context.Context) (RiverQueueStats, error) {
	if p == nil || p.queryer == nil {
		return RiverQueueStats{}, fmt.Errorf("river queue stats queryer is nil")
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	jobsByState, err := queryRiverJobStateCounts(ctx, p.queryer, riverJobsByStateQuery)
	if err != nil {
		return RiverQueueStats{}, err
	}
	readyJobs, err := queryRiverReadyJobCounts(ctx, p.queryer, riverReadyJobsQuery)
	if err != nil {
		return RiverQueueStats{}, err
	}
	oldestReadyJobAges, err := queryRiverOldestReadyJobAges(ctx, p.queryer, riverOldestReadyJobAgeQuery)
	if err != nil {
		return RiverQueueStats{}, err
	}
	recentTerminalJobs, err := queryRiverRecentTerminalJobCounts(ctx, p.queryer, riverRecentTerminalJobsQuery, int64(p.recentTerminalWindow.Seconds()))
	if err != nil {
		return RiverQueueStats{}, err
	}

	return RiverQueueStats{
		JobsByState:        jobsByState,
		ReadyJobs:          readyJobs,
		OldestReadyJobAges: oldestReadyJobAges,
		RecentTerminalJobs: recentTerminalJobs,
	}, nil
}

func queryRiverJobStateCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]RiverJobStateCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query river jobs by state: %w", err)
	}
	defer rows.Close()

	var counts []RiverJobStateCount
	for rows.Next() {
		var count RiverJobStateCount
		if err := rows.Scan(&count.Queue, &count.State, &count.Kind, &count.Count); err != nil {
			return nil, fmt.Errorf("scan river jobs by state: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read river jobs by state: %w", err)
	}
	return counts, nil
}

func queryRiverReadyJobCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]RiverReadyJobCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query river ready jobs: %w", err)
	}
	defer rows.Close()

	var counts []RiverReadyJobCount
	for rows.Next() {
		var count RiverReadyJobCount
		if err := rows.Scan(&count.Queue, &count.Kind, &count.Count); err != nil {
			return nil, fmt.Errorf("scan river ready jobs: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read river ready jobs: %w", err)
	}
	return counts, nil
}

func queryRiverOldestReadyJobAges(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]RiverOldestReadyJobAge, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query river oldest ready job age: %w", err)
	}
	defer rows.Close()

	var ages []RiverOldestReadyJobAge
	for rows.Next() {
		var age RiverOldestReadyJobAge
		if err := rows.Scan(&age.Queue, &age.AgeSeconds); err != nil {
			return nil, fmt.Errorf("scan river oldest ready job age: %w", err)
		}
		ages = append(ages, age)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read river oldest ready job age: %w", err)
	}
	return ages, nil
}

func queryRiverRecentTerminalJobCounts(ctx context.Context, queryer pgxQueryer, query string, args ...any) ([]RiverRecentTerminalJobCount, error) {
	rows, err := queryer.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query river recent terminal jobs: %w", err)
	}
	defer rows.Close()

	var counts []RiverRecentTerminalJobCount
	for rows.Next() {
		var count RiverRecentTerminalJobCount
		if err := rows.Scan(&count.Queue, &count.State, &count.Kind, &count.Count); err != nil {
			return nil, fmt.Errorf("scan river recent terminal jobs: %w", err)
		}
		counts = append(counts, count)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read river recent terminal jobs: %w", err)
	}
	return counts, nil
}

type riverQueueStatsCollector struct {
	provider                RiverQueueStatsProvider
	jobsByStateDesc         *prometheus.Desc
	readyJobsDesc           *prometheus.Desc
	oldestReadyJobAgeDesc   *prometheus.Desc
	recentTerminalJobsDesc  *prometheus.Desc
	queueStatsScrapeSuccess *prometheus.Desc
}

// NewRiverQueueStatsCollector creates a scrape-time collector for River queue health.
func NewRiverQueueStatsCollector(provider RiverQueueStatsProvider) prometheus.Collector {
	return &riverQueueStatsCollector{
		provider: provider,
		jobsByStateDesc: prometheus.NewDesc(
			"shepherd_river_jobs_by_state",
			"Current River jobs grouped by queue, state, and kind.",
			[]string{"queue", "state", "kind"},
			nil,
		),
		readyJobsDesc: prometheus.NewDesc(
			"shepherd_river_ready_jobs",
			"River jobs whose scheduled_at is due and should become work, grouped by queue and kind.",
			[]string{"queue", "kind"},
			nil,
		),
		oldestReadyJobAgeDesc: prometheus.NewDesc(
			"shepherd_river_oldest_ready_job_age_seconds",
			"Age in seconds of the oldest due River job grouped by queue.",
			[]string{"queue"},
			nil,
		),
		recentTerminalJobsDesc: prometheus.NewDesc(
			"shepherd_river_recent_terminal_jobs",
			"River cancelled and discarded jobs finalized within the recent observation window, grouped by queue, state, and kind.",
			[]string{"queue", "state", "kind"},
			nil,
		),
		queueStatsScrapeSuccess: prometheus.NewDesc(
			"shepherd_river_queue_stats_scrape_success",
			"Whether River queue statistics collection succeeded for this scrape.",
			nil,
			nil,
		),
	}
}

// WithRiverQueueStats registers River queue health metrics backed by pgx.
func WithRiverQueueStats(pool *pgxpool.Pool, timeout time.Duration) Option {
	return WithRiverQueueStatsProvider(NewPGXRiverQueueStatsProvider(pool, timeout))
}

// WithRiverQueueStatsProvider registers River queue health metrics with a custom provider.
func WithRiverQueueStatsProvider(provider RiverQueueStatsProvider) Option {
	return WithCollector(NewRiverQueueStatsCollector(provider))
}

func (c *riverQueueStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.jobsByStateDesc
	ch <- c.readyJobsDesc
	ch <- c.oldestReadyJobAgeDesc
	ch <- c.recentTerminalJobsDesc
	ch <- c.queueStatsScrapeSuccess
}

func (c *riverQueueStatsCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil {
		return
	}
	if c.provider == nil {
		ch <- prometheus.MustNewConstMetric(c.queueStatsScrapeSuccess, prometheus.GaugeValue, 0)
		return
	}

	stats, err := c.provider.RiverQueueStats(context.Background())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.queueStatsScrapeSuccess, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.queueStatsScrapeSuccess, prometheus.GaugeValue, 1)
	for _, count := range stats.JobsByState {
		ch <- prometheus.MustNewConstMetric(c.jobsByStateDesc, prometheus.GaugeValue, count.Count, count.Queue, count.State, count.Kind)
	}
	for _, count := range stats.ReadyJobs {
		ch <- prometheus.MustNewConstMetric(c.readyJobsDesc, prometheus.GaugeValue, count.Count, count.Queue, count.Kind)
	}
	for _, age := range stats.OldestReadyJobAges {
		ch <- prometheus.MustNewConstMetric(c.oldestReadyJobAgeDesc, prometheus.GaugeValue, age.AgeSeconds, age.Queue)
	}
	for _, count := range stats.RecentTerminalJobs {
		ch <- prometheus.MustNewConstMetric(c.recentTerminalJobsDesc, prometheus.GaugeValue, count.Count, count.Queue, count.State, count.Kind)
	}
}
