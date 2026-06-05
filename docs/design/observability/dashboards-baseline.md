# Grafana Dashboard Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)
> **Related**: [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md)

## Scope

The baseline dashboard is a starter Grafana overview for the metrics accepted by
ADR-0054. HTTP, OpenAPI, and River queue computed panels use the recording
series accepted by ADR-0055. It complements the ADR-0055 alert rule pack without
taking ownership of advanced dashboard suites, broad business SLO panels,
Alertmanager routing, trace-derived alerting, or log-based monitoring views.

Dashboard assets:

```text
deploy/monitoring/grafana/dashboards/shepherd-overview.json
deploy/monitoring/grafana/provisioning/dashboards/shepherd.yml
```

## Panels

| Panel | Query source | Purpose |
|-------|--------------|---------|
| Metrics target availability | `up{job="shepherd"}` | Show whether Prometheus can scrape Shepherd |
| HTTP request rate | `shepherd:http_requests:rate5m` | Show global request throughput |
| HTTP 5xx ratio | `shepherd:http_5xx:ratio5m` | Show sustained server-side failure ratio |
| HTTP p95 latency | `shepherd:http_request_duration_seconds:p95_5m` | Show broad latency regression |
| OpenAPI validation failures | `shepherd:openapi_validation_failures:rate5m` | Show contract drift or invalid client behavior |
| PostgreSQL stats scrape success | `shepherd_postgres_table_stats_scrape_success` | Show whether database stats are available |
| PostgreSQL table dead tuple ratio | `shepherd_postgres_table_dead_tuple_ratio` | Show bloat by monitored table |
| River dead tuple ratio | `shepherd_river_dead_tuple_ratio` | Show the RFC-0001 River partitioning trigger metric |
| River queue stats scrape success | `shepherd_river_queue_stats_scrape_success` | Show whether queue-health metrics are available |
| River ready jobs | `shepherd:river_ready_jobs:sum` | Show async backlog by queue |
| River oldest ready job age | `shepherd_river_oldest_ready_job_age_seconds` | Show oldest due job wait time by queue |
| River recent terminal jobs | `shepherd_river_recent_terminal_jobs` | Show recent cancelled/discarded jobs by queue and kind |
| Firing alerts | `sum(ALERTS{alertstate="firing", service="shepherd"}) or vector(0)` | Show current Shepherd alert count from Prometheus rule evaluation |
| Firing alert details | `ALERTS{alertstate="firing", service="shepherd"}` | Show firing alert names and severities |
| Business metrics scrape success | `shepherd_business_metrics_scrape_success` | Show whether approval/audit business metrics are available |
| Pending approvals | `shepherd:business_approval_pending:sum` | Show pending approval backlog by operation type |
| Oldest pending approval age | `shepherd_business_approval_pending_oldest_age_seconds` | Show long-running unhandled approvals |
| Failed approvals | `shepherd:business_approval_failed:sum` | Show failed approval tickets by operation type |
| Pending batch approvals | `shepherd:business_batch_approval_pending:sum` | Show pending batch approval backlog |
| Batch approval failures | `shepherd:business_batch_approval_failed:sum` | Show failed batch approvals and failed child counts by batch type |
| Approval failure audit actions | `shepherd:business_approval_failure_audit_actions:sum` | Show recent failure signals derived from approval audit actions |

Single-value zero-state panels use `or vector(0)` where needed. Label-split
time series avoid that fallback so Grafana does not render an extra unlabeled
zero series when real labeled data exists.

## Provisioning

The provisioning YAML uses Grafana file provisioning with:

| Field | Value |
|-------|-------|
| provider name | `shepherd` |
| provider type | `file` |
| folder | `Shepherd` |
| update interval | `30` seconds |
| UI updates | disabled |
| dashboard path | `/var/lib/grafana/dashboards/shepherd` |

Deployments may copy the JSON file to that path or adapt the path in their own
Grafana provisioning layer.

## Dashboard Variables

The dashboard defines only one variable:

| Variable | Type | Purpose |
|----------|------|---------|
| `datasource` | Grafana datasource variable | Select Prometheus-compatible datasource |

No variable may represent users, tickets, VMs, namespaces, clusters, raw paths,
queries, headers, or payload-derived values.

## Validation

The governance checks validate both dashboard structure and PromQL syntax.
`docs/design/ci/scripts/check_grafana_dashboards.sh` validates the dashboard and
provisioning files:

* JSON parses successfully.
* UID, title, tags, refresh, time range, and datasource variable are present.
* Expected panel titles exist exactly once.
* Panel targets reference only accepted metric families.
* Provisioning YAML points at the expected dashboard path.

`docs/design/ci/scripts/check_grafana_dashboard_promql.sh` extracts every panel
target expression and wraps it as a temporary Prometheus recording rule, then
runs `promtool check rules` as accepted by ADR-0055. This catches dashboard
query syntax regressions without requiring a running Grafana or Prometheus
server.

## Deferred Work

The following remain outside this baseline:

* Broad business SLO dashboards for VM, provider, and notification workflows.
* Alertmanager receiver routing and escalation policy.
* Trace-specific Grafana dashboards, trace-aware exemplars, and trace/log
  correlation views beyond the Shepherd administrator trace summary and the
  embedded Tempo datasource.
* Log backend installation, log-derived metrics, and log-based alert rules.
* Grafana folder taxonomy beyond the starter `Shepherd` folder.
