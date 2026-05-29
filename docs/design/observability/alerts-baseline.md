# Prometheus Alert Rule Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)

## Scope

The baseline alert pack is a deployment-ready starter for the metrics accepted
by ADR-0054. HTTP, OpenAPI, and River queue computed SLIs use the recording
series accepted by ADR-0055. It is
intentionally smaller than the full RFC-0010 observability stack: no
Alertmanager routing, receiver configuration, tracing, or business SLO alerts
are included.

Rule file:

```text
deploy/monitoring/prometheus/shepherd-alerts.yml
```

Load the ADR-0055 recording-rule file before this alert file when using the
baseline derived SLI alerts.

## Alerts

| Alert | Severity | Trigger | Rationale |
|-------|----------|---------|-----------|
| `ShepherdMetricsTargetDown` | critical | Prometheus `up{job="shepherd"} == 0` for 2 minutes | The monitoring surface itself is unavailable |
| `ShepherdHighHTTP5xxRate` | warning | `shepherd:http_5xx:ratio5m` above 5% for 10 minutes | Detect sustained server-side failure without route-cardinality expansion |
| `ShepherdHighHTTPP95Latency` | warning | `shepherd:http_request_duration_seconds:p95_5m` above 2 seconds for 10 minutes | Detect broad latency regression using the accepted HTTP histogram |
| `ShepherdOpenAPIValidationFailures` | warning | `shepherd:openapi_validation_failures:rate5m` by `phase` and `code` above 0.01/s for 10 minutes | Detect contract drift or invalid client behavior |
| `ShepherdPostgresStatsScrapeFailed` | warning | Database stats scrape success is `0` for 10 minutes | Detect broken PostgreSQL stats collection before bloat alerts go silent |
| `ShepherdRiverDeadTupleRatioHigh` | critical | `shepherd_river_dead_tuple_ratio > 0.30` for 30 minutes | Implements the RFC-0001 partitioning-evaluation trigger as an alert |
| `ShepherdRiverQueueStatsScrapeFailed` | warning | River queue stats scrape success is `0` for 10 minutes | Detect broken queue-health collection before async alerts go silent |
| `ShepherdRiverQueueBacklogAgeHigh` | warning | `shepherd_river_oldest_ready_job_age_seconds > 300` for 15 minutes | Detect ready async work waiting too long |
| `ShepherdRiverJobsDiscarded` | critical | `shepherd:river_recent_discarded_jobs:sum > 0` for 5 minutes | Detect jobs that exhausted retries and reached River's discarded terminal state |

Each alert must include a `runbook_url` annotation pointing to its local runbook
section in this document. That link contract is protected by
[alert-runbook-link-baseline.md](./alert-runbook-link-baseline.md).

## Threshold Policy

The baseline thresholds are conservative starter values:

* HTTP failure and latency rules are global, not per route, to keep the first
  alert pack low-cardinality.
* OpenAPI validation alerting groups only by fixed `phase` and `code` labels.
* River dead tuple ratio uses RFC-0001's `30%` evaluation trigger.
* River queue backlog age uses a 5 minute starter threshold. Deployments with
  intentionally slow queues may tune the threshold in their deployment copy.
* River discarded jobs alert on any recent discarded job because discarded jobs
  represent exhausted retry policy and usually require operator review.
* The scrape availability rule assumes the Prometheus job label is `shepherd`.
  Deployments using a different scrape job name must adjust only that selector.

## Label Policy

Rule labels added by the baseline are limited to:

| Label | Values |
|-------|--------|
| `severity` | `warning`, `critical` |
| `service` | `shepherd` |

Rule expressions must not add or group by user, ticket, VM, namespace, cluster,
raw path, query, header, or payload-derived labels.

## Validation

The governance check
`docs/design/ci/scripts/check_prometheus_alert_rules.sh` validates the rule
file. If `promtool` is installed, the check also runs:

```bash
promtool check rules deploy/monitoring/prometheus/shepherd-alerts.yml
```

The fallback structural check remains mandatory even when `promtool` is not
available.

Prometheus rule-unit-test coverage for these alerts is defined in
[rule-tests-baseline.md](./rule-tests-baseline.md).
Use `make ci-prometheus-rules` to run recording-rule, alert-rule, and rule-test
validation together.

Runbook URL validation is defined by
[alert-runbook-link-baseline.md](./alert-runbook-link-baseline.md). It verifies
that every `runbook_url` annotation points to an existing local Markdown anchor.

## Runbooks

### ShepherdMetricsTargetDown

Check whether the Shepherd process is running, whether `/metrics` is enabled,
and whether Prometheus uses the expected scrape job label. If the deployment
does not use `job="shepherd"`, adjust only that selector in the rule file.

### ShepherdHighHTTP5xxRate

Inspect recent application logs by request ID and route. Correlate with deploy
events, database connectivity, provider availability, and panic recovery logs.
Do not split this alert by raw path; use HTTP metrics and logs together.

### ShepherdHighHTTPP95Latency

Check database latency, KubeVirt API latency, worker queue pressure, and recent
deployments. If the traffic profile is intentionally long-running, tune the
threshold in the deployment copy instead of adding high-cardinality labels.

### ShepherdOpenAPIValidationFailures

Use the fixed `phase` and `code` labels to identify request, response, or setup
failures. Request failures usually indicate invalid client payloads or contract
drift. Response failures indicate backend response/schema drift and should block
release promotion when observed in non-production validation.

### ShepherdPostgresStatsScrapeFailed

Verify `observability.database_metrics_enabled`, database connectivity, and
permissions for reading `pg_stat_user_tables`. This alert can hide bloat alerts,
so treat sustained failures as a monitoring issue even if the product API is
healthy.

### ShepherdRiverDeadTupleRatioHigh

Run the PostgreSQL operations checks for River bloat and autovacuum health. If
the ratio remains above `30%`, RFC-0001's partitioning evaluation trigger is
met.

### ShepherdRiverQueueStatsScrapeFailed

Verify `observability.river_metrics_enabled`, database connectivity, River
schema presence, and permissions for reading `river_job`. This alert can hide
queue backlog and discarded-job alerts, so treat sustained failures as a
monitoring issue even when HTTP health checks pass.

### ShepherdRiverQueueBacklogAgeHigh

Check the `queue` label, River worker process health, `RIVER_MAX_WORKERS`, DB
connectivity, KubeVirt API availability, and recent deployment changes. Use the
River ready-job dashboard panel to distinguish one stuck queue from broad worker
starvation.

### ShepherdRiverJobsDiscarded

Inspect the discarded job kind and application logs around the same time
window. Discarded jobs exhausted retries; reconcile the affected ticket/event
state before retrying or manually compensating. Do not add job args, IDs,
payload fields, VM names, namespaces, or clusters as metric labels.

## Deferred Work

The following remain outside this baseline:

* Alertmanager receiver routing and escalation policy.
* Advanced Grafana dashboard suites beyond the ADR-0055 starter dashboard.
* Business SLO alerts for VM, approval, batch, provider, or notification flows.
* Per-job execution histograms from River event subscriptions.
* Deep OpenTelemetry instrumentation and trace-aware exemplars.
