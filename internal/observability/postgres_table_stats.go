package observability

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const postgresTableStatsQuery = `
SELECT
	relname,
	n_live_tup::float8,
	n_dead_tup::float8,
	COALESCE(n_dead_tup::float8 / NULLIF((n_live_tup + n_dead_tup)::float8, 0), 0)::float8 AS dead_tuple_ratio,
	COALESCE(EXTRACT(EPOCH FROM last_autovacuum), 0)::float8 AS last_autovacuum_timestamp_seconds
FROM pg_stat_user_tables
WHERE relname IN ('river_job', 'audit_logs', 'domain_events')
ORDER BY relname
`

const riverJobTableName = "river_job"

// PostgresTableStats is the fixed PostgreSQL table statistics contract exposed as metrics.
type PostgresTableStats struct {
	Table                          string
	LiveTuples                     float64
	DeadTuples                     float64
	DeadTupleRatio                 float64
	LastAutovacuumTimestampSeconds float64
}

// TableStatsProvider provides PostgreSQL table statistics for scrape-time collection.
type TableStatsProvider interface {
	TableStats(context.Context) ([]PostgresTableStats, error)
}

type pgxQueryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type pgxTableStatsProvider struct {
	queryer pgxQueryer
	timeout time.Duration
}

// NewPGXTableStatsProvider reads PostgreSQL table statistics through pgx.
func NewPGXTableStatsProvider(pool *pgxpool.Pool, timeout time.Duration) TableStatsProvider {
	if pool == nil {
		return &pgxTableStatsProvider{timeout: timeout}
	}
	return &pgxTableStatsProvider{
		queryer: pool,
		timeout: timeout,
	}
}

func (p *pgxTableStatsProvider) TableStats(ctx context.Context) ([]PostgresTableStats, error) {
	if p == nil || p.queryer == nil {
		return nil, fmt.Errorf("postgres table stats queryer is nil")
	}
	if p.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	rows, err := p.queryer.Query(ctx, postgresTableStatsQuery)
	if err != nil {
		return nil, fmt.Errorf("query postgres table stats: %w", err)
	}
	defer rows.Close()

	stats := make([]PostgresTableStats, 0, 3)
	for rows.Next() {
		var stat PostgresTableStats
		if err := rows.Scan(
			&stat.Table,
			&stat.LiveTuples,
			&stat.DeadTuples,
			&stat.DeadTupleRatio,
			&stat.LastAutovacuumTimestampSeconds,
		); err != nil {
			return nil, fmt.Errorf("scan postgres table stats: %w", err)
		}
		stats = append(stats, stat)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read postgres table stats: %w", err)
	}
	return stats, nil
}

type postgresTableStatsCollector struct {
	provider                    TableStatsProvider
	liveTuplesDesc              *prometheus.Desc
	deadTuplesDesc              *prometheus.Desc
	deadTupleRatioDesc          *prometheus.Desc
	lastAutovacuumTimestampDesc *prometheus.Desc
	scrapeSuccessDesc           *prometheus.Desc
	riverDeadTupleRatioDesc     *prometheus.Desc
}

// NewPostgresTableStatsCollector creates a scrape-time collector for PostgreSQL bloat stats.
func NewPostgresTableStatsCollector(provider TableStatsProvider) prometheus.Collector {
	labels := []string{"table"}
	return &postgresTableStatsCollector{
		provider: provider,
		liveTuplesDesc: prometheus.NewDesc(
			"shepherd_postgres_table_live_tuples",
			"Estimated live tuples for monitored PostgreSQL operational tables.",
			labels,
			nil,
		),
		deadTuplesDesc: prometheus.NewDesc(
			"shepherd_postgres_table_dead_tuples",
			"Estimated dead tuples for monitored PostgreSQL operational tables.",
			labels,
			nil,
		),
		deadTupleRatioDesc: prometheus.NewDesc(
			"shepherd_postgres_table_dead_tuple_ratio",
			"Estimated dead tuple ratio for monitored PostgreSQL operational tables.",
			labels,
			nil,
		),
		lastAutovacuumTimestampDesc: prometheus.NewDesc(
			"shepherd_postgres_table_last_autovacuum_timestamp_seconds",
			"Last autovacuum timestamp for monitored PostgreSQL operational tables as Unix seconds; 0 means never observed.",
			labels,
			nil,
		),
		scrapeSuccessDesc: prometheus.NewDesc(
			"shepherd_postgres_table_stats_scrape_success",
			"Whether PostgreSQL operational table statistics collection succeeded for this scrape.",
			nil,
			nil,
		),
		riverDeadTupleRatioDesc: prometheus.NewDesc(
			"shepherd_river_dead_tuple_ratio",
			"Estimated dead tuple ratio for the River job table.",
			nil,
			nil,
		),
	}
}

// WithPostgresTableStats registers PostgreSQL table bloat metrics backed by pgx.
func WithPostgresTableStats(pool *pgxpool.Pool, timeout time.Duration) Option {
	return WithPostgresTableStatsProvider(NewPGXTableStatsProvider(pool, timeout))
}

// WithPostgresTableStatsProvider registers PostgreSQL table bloat metrics with a custom provider.
func WithPostgresTableStatsProvider(provider TableStatsProvider) Option {
	return WithCollector(NewPostgresTableStatsCollector(provider))
}

func (c *postgresTableStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.liveTuplesDesc
	ch <- c.deadTuplesDesc
	ch <- c.deadTupleRatioDesc
	ch <- c.lastAutovacuumTimestampDesc
	ch <- c.scrapeSuccessDesc
	ch <- c.riverDeadTupleRatioDesc
}

func (c *postgresTableStatsCollector) Collect(ch chan<- prometheus.Metric) {
	if c == nil {
		return
	}
	if c.provider == nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 0)
		return
	}
	stats, err := c.provider.TableStats(context.Background())
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 0)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeSuccessDesc, prometheus.GaugeValue, 1)
	for _, stat := range stats {
		ch <- prometheus.MustNewConstMetric(c.liveTuplesDesc, prometheus.GaugeValue, stat.LiveTuples, stat.Table)
		ch <- prometheus.MustNewConstMetric(c.deadTuplesDesc, prometheus.GaugeValue, stat.DeadTuples, stat.Table)
		ch <- prometheus.MustNewConstMetric(c.deadTupleRatioDesc, prometheus.GaugeValue, stat.DeadTupleRatio, stat.Table)
		ch <- prometheus.MustNewConstMetric(c.lastAutovacuumTimestampDesc, prometheus.GaugeValue, stat.LastAutovacuumTimestampSeconds, stat.Table)
		if stat.Table == riverJobTableName {
			ch <- prometheus.MustNewConstMetric(c.riverDeadTupleRatioDesc, prometheus.GaugeValue, stat.DeadTupleRatio)
		}
	}
}
