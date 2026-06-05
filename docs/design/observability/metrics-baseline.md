# Prometheus Metrics Baseline

> **Decision**: [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md)

## Scope

The baseline provides production-safe metrics that can be enabled without
changing product API contracts:

| Metric family | Type | Labels | Purpose |
|---------------|------|--------|---------|
| `shepherd_http_requests_total` | Counter | `method`, `route`, `status` | HTTP request volume by normalized route |
| `shepherd_http_request_duration_seconds` | Histogram | `method`, `route`, `status` | HTTP latency distribution |
| `shepherd_openapi_validation_failures_total` | Counter | `phase`, `code`, `method`, `route` | Runtime OpenAPI validator setup, request, and response failures |
| `shepherd_postgres_table_live_tuples` | Gauge | `table` | PostgreSQL estimated live tuples for monitored operational tables |
| `shepherd_postgres_table_dead_tuples` | Gauge | `table` | PostgreSQL estimated dead tuples for monitored operational tables |
| `shepherd_postgres_table_dead_tuple_ratio` | Gauge | `table` | Dead tuple ratio for monitored operational tables |
| `shepherd_postgres_table_last_autovacuum_timestamp_seconds` | Gauge | `table` | Last autovacuum timestamp as Unix seconds; `0` means never observed |
| `shepherd_postgres_table_stats_scrape_success` | Gauge | none | `1` when PostgreSQL stats collection succeeds, otherwise `0` |
| `shepherd_river_dead_tuple_ratio` | Gauge | none | Alias for the `river_job` dead tuple ratio used by ADR-0008/RFC-0001 |
| `shepherd_river_jobs_by_state` | Gauge | `queue`, `state`, `kind` | Current River jobs grouped by bounded queue state |
| `shepherd_river_ready_jobs` | Gauge | `queue`, `kind` | Due River jobs that should become work |
| `shepherd_river_oldest_ready_job_age_seconds` | Gauge | `queue` | Age of the oldest due job in each River queue |
| `shepherd_river_recent_terminal_jobs` | Gauge | `queue`, `state`, `kind` | Recent `cancelled` and `discarded` River jobs |
| `shepherd_river_queue_stats_scrape_success` | Gauge | none | `1` when River queue stats collection succeeds, otherwise `0` |
| `shepherd_business_metrics_scrape_success` | Gauge | none | `1` when business metrics collection succeeds, otherwise `0` |
| `shepherd_business_approval_tickets` | Gauge | `status`, `operation_type` | Current approval tickets by workflow status and operation type |
| `shepherd_business_approval_pending_oldest_age_seconds` | Gauge | `operation_type` | Oldest pending approval ticket age by operation type |
| `shepherd_business_batch_approval_tickets` | Gauge | `status`, `batch_type` | Current batch approval tickets by workflow status and batch type |
| `shepherd_business_batch_approval_pending_oldest_age_seconds` | Gauge | `batch_type` | Oldest pending batch approval age by batch type |
| `shepherd_business_batch_approval_failed_children` | Gauge | `batch_type` | Failed child count on batch approvals by batch type |
| `shepherd_business_approval_audit_actions_recent` | Gauge | `action` | Recent approval-related audit actions from `audit_logs` |
| `shepherd_business_approval_failure_audit_actions_recent` | Gauge | `action` | Recent failure-related approval audit actions from `audit_logs` |
| Go runtime metrics | Collector | upstream-defined | goroutines, GC, memory, scheduler |
| Process metrics | Collector | upstream-defined | CPU, memory, file descriptors, process start |
| Build info | Collector | upstream-defined | Go module build metadata |

## Configuration

```yaml
observability:
  metrics_enabled: true
  metrics_path: "/metrics"
  database_metrics_enabled: true
  database_metrics_timeout: "2s"
  river_metrics_enabled: true
  river_metrics_timeout: "2s"
  business_metrics_enabled: true
  business_metrics_timeout: "2s"
```

Environment overrides:

| Variable | Meaning |
|----------|---------|
| `OBSERVABILITY_METRICS_ENABLED` | Enable or disable the scrape endpoint |
| `OBSERVABILITY_METRICS_PATH` | Absolute HTTP path for Prometheus scraping |
| `OBSERVABILITY_DATABASE_METRICS_ENABLED` | Enable or disable PostgreSQL table statistics |
| `OBSERVABILITY_DATABASE_METRICS_TIMEOUT` | Per-scrape PostgreSQL stats query timeout |
| `OBSERVABILITY_RIVER_METRICS_ENABLED` | Enable or disable River queue health metrics |
| `OBSERVABILITY_RIVER_METRICS_TIMEOUT` | Per-scrape River queue stats query timeout |
| `OBSERVABILITY_BUSINESS_METRICS_ENABLED` | Enable or disable approval/audit business metrics |
| `OBSERVABILITY_BUSINESS_METRICS_TIMEOUT` | Per-scrape business metrics query timeout |

## Routing Contract

The metrics endpoint is registered before JWT auth and OpenAPI validation. This
keeps Prometheus text exposition out of `api/openapi.yaml` and prevents browser
session policy from becoming a monitoring dependency.

The endpoint still passes through panic recovery, request ID injection, and
HTTP metrics middleware.

## Label Policy

Allowed HTTP labels:

| Label | Source |
|-------|--------|
| `method` | `Request.Method` |
| `route` | `gin.Context.FullPath()` with fallback to `unmatched` |
| `status` | `gin.ResponseWriter.Status()` |

Allowed database labels:

| Label | Source |
|-------|--------|
| `table` | Fixed monitored set: `river_job`, `audit_logs`, `domain_events` |

Allowed River queue labels:

| Label | Source |
|-------|--------|
| `queue` | River queue name from the bounded worker configuration |
| `state` | River job state |
| `kind` | River job kind registered by Shepherd workers |

Allowed OpenAPI validation labels:

| Label | Source |
|-------|--------|
| `phase` | Fixed values: `setup`, `request`, `response` |
| `code` | Fixed validator response codes from ADR-0054 |
| `method` | `Request.Method` |
| `route` | `gin.Context.FullPath()` with fallback to `unmatched` |

Allowed business labels:

| Label | Source |
|-------|--------|
| `status` | Ticket and batch-ticket enum status |
| `operation_type` | Ticket enum operation type |
| `batch_type` | Batch-ticket enum batch type |
| `action` | Fixed audit action name such as `approval.validation_failed` |

Forbidden HTTP labels:

| Label source | Reason |
|--------------|--------|
| Raw URL path | High cardinality, may include identifiers |
| User, role, session, ticket, VM, namespace, cluster names | Sensitive or high-cardinality |
| Query strings and headers | Sensitive, unstable, and high-cardinality |
| Validation reason text, schema locations, request/response bodies | Sensitive or high-cardinality |
| River job args, job IDs, errors, tags, metadata, ticket IDs, VM names, namespaces, clusters | Sensitive or high-cardinality |
| Business requester, approver, actor, ticket ID, resource ID, VM name, namespace, cluster | Sensitive or high-cardinality |

## Deferred Work

The following remain outside the baseline:

* Frontend OpenTelemetry instrumentation.
* VM, provider, notification, and richer approval SLO metrics beyond the
  approval/audit business baseline.
* Advanced Grafana dashboard suites and alert routing.
* Business burn-rate SLO alert rules beyond the starter approval/audit alerts.
