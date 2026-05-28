# River Queue Metrics Baseline

> **Decision**: [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md)

## Scope

This baseline exposes aggregate River queue health through Shepherd's existing
Prometheus `/metrics` endpoint. It complements ADR-0054's `river_job` bloat
metrics by showing async execution health:

* current jobs by `queue`, `state`, and `kind`
* ready job depth by `queue` and `kind`
* oldest ready job age by `queue`
* recent terminal `cancelled` and `discarded` jobs
* scrape success for River queue statistics

The collector reads only aggregate values from `river_job`. It never exposes job
args, job IDs, errors, metadata, tags, VM identifiers, tickets, namespaces,
clusters, users, or payload values.

## Metric Families

| Metric family | Type | Labels | Purpose |
|---------------|------|--------|---------|
| `shepherd_river_jobs_by_state` | Gauge | `queue`, `state`, `kind` | Current River jobs grouped by bounded queue state |
| `shepherd_river_ready_jobs` | Gauge | `queue`, `kind` | Jobs whose `scheduled_at` is due and should become work |
| `shepherd_river_oldest_ready_job_age_seconds` | Gauge | `queue` | Age of the oldest due job in a queue |
| `shepherd_river_recent_terminal_jobs` | Gauge | `queue`, `state`, `kind` | `cancelled` and `discarded` jobs finalized in the observation window |
| `shepherd_river_queue_stats_scrape_success` | Gauge | none | `1` when River queue stats collection succeeds, otherwise `0` |

Ready jobs are River jobs in `available`, `retryable`, or `scheduled` state with
`scheduled_at <= now()`. This catches normal available backlog and scheduler
lag for retryable/scheduled jobs that are already due.

The recent terminal observation window is fixed at 15 minutes for the baseline.
Longer-window policy belongs to deployment-specific dashboards and alerting.

## Configuration

```yaml
observability:
  metrics_enabled: true
  river_metrics_enabled: true
  river_metrics_timeout: "2s"
```

Environment overrides:

| Variable | Meaning |
|----------|---------|
| `OBSERVABILITY_RIVER_METRICS_ENABLED` | Enable or disable River queue metrics |
| `OBSERVABILITY_RIVER_METRICS_TIMEOUT` | Per-scrape River queue stats query timeout |

River queue metrics require the shared PostgreSQL pool and the River schema to
exist. When the query fails, `shepherd_river_queue_stats_scrape_success` is set
to `0`; the collector does not panic or fail the `/metrics` response.

## Alerting Contract

The baseline Prometheus rules include:

| Alert | Trigger |
|-------|---------|
| `ShepherdRiverQueueStatsScrapeFailed` | `shepherd_river_queue_stats_scrape_success == 0` for 10 minutes |
| `ShepherdRiverQueueBacklogAgeHigh` | `shepherd_river_oldest_ready_job_age_seconds > 300` for 15 minutes |
| `ShepherdRiverJobsDiscarded` | `shepherd:river_recent_discarded_jobs:sum > 0` for 5 minutes |

`cancelled` terminal jobs are measured but not alerted by default because
operator-initiated cancellation can be expected behavior.

## Dashboard Contract

The starter Grafana dashboard includes River queue health panels for:

* River queue stats scrape success
* River ready jobs by queue
* River oldest ready job age
* River recent terminal jobs

Dashboard queries must pass ADR-0055 PromQL parser validation.

## Boundaries

This baseline does not add per-job execution histograms, River event
subscription counters, provider-specific KubeVirt metrics, or business SLOs.
Those remain follow-up work until the project has production evidence for the
right labels and thresholds.
