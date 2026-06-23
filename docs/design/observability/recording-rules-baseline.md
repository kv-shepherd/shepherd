# Prometheus Recording Rule Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)

## Scope

The recording-rule baseline centralizes reusable PromQL calculations for the
accepted Shepherd metrics. It is a monitoring quality-control layer, not a new
runtime feature.

Rule file:

```text
deploy/monitoring/prometheus/shepherd-recording-rules.yml
```

Prometheus should load this file before alert rules that reference the recorded
series.

## Recording Series

| Record | Expression source | Purpose |
|--------|-------------------|---------|
| `shepherd:http_requests:rate5m` | `shepherd_http_requests_total` | Global Shepherd request throughput |
| `shepherd:http_requests_5xx:rate5m` | `shepherd_http_requests_total{status=~"5.."}` | Global server-side failure throughput |
| `shepherd:http_5xx:ratio5m` | `shepherd_http_requests_total` | Global 5xx ratio for alerting and dashboarding |
| `shepherd:http_request_duration_seconds:p95_5m` | `shepherd_http_request_duration_seconds_bucket` | Global HTTP p95 latency |
| `shepherd:openapi_validation_failures:rate5m` | `shepherd_openapi_validation_failures_total` | Runtime OpenAPI validation failure rate by `phase` and `code` |
| `shepherd:river_ready_jobs:sum` | `shepherd_river_ready_jobs` | Ready River jobs by `queue` |
| `shepherd:river_recent_discarded_jobs:sum` | `shepherd_river_recent_terminal_jobs{state="discarded"}` | Recently discarded River jobs by `queue` |
| `shepherd:business_approval_pending:sum` | `shepherd_business_approval_tickets{status="PENDING"}` | Pending approval tickets by `operation_type` |
| `shepherd:business_approval_failed:sum` | `shepherd_business_approval_tickets{status="FAILED"}` | Failed approval tickets by `operation_type` |
| `shepherd:business_batch_approval_pending:sum` | `shepherd_business_batch_approval_tickets{status="PENDING_APPROVAL"}` | Pending batch approvals by `batch_type` |
| `shepherd:business_batch_approval_failed:sum` | `shepherd_business_batch_approval_tickets{status="FAILED"}` and `shepherd_business_batch_approval_failed_children` | Failed batch approvals and failed child work by `batch_type` |
| `shepherd:business_approval_failure_audit_actions:sum` | `shepherd_business_approval_failure_audit_actions_recent` | Recent failure audit actions by fixed allowlisted `action`; unknown or future actions are aggregated as `approval.other` |

## Label Policy

Recording rules preserve the baseline low-cardinality policy:

* HTTP request rate, HTTP 5xx ratio, and HTTP p95 latency are global.
* OpenAPI validation failure rate keeps only fixed `phase` and `code` labels.
* River queue recordings keep only the fixed `queue` label.
* Approval/audit business recordings keep only fixed `operation_type`,
  `batch_type`, and allowlisted `action` labels; unknown action values must be
  aggregated as `approval.other`.
* Rules must not introduce user, role, session, ticket, VM, namespace, cluster,
  raw path, query, header, or payload-derived labels.

## Consumer Policy

The ADR-0055 alert pack and dashboard should use the recording series
for computed HTTP and OpenAPI SLIs. Direct queries remain acceptable for:

* Prometheus scrape availability via `up{job="shepherd"}`.
* PostgreSQL stats scrape success.
* PostgreSQL table dead tuple ratio.
* River dead tuple ratio.
* River queue stats scrape success.
* River oldest ready job age.
* River recent terminal jobs when the dashboard needs `state` and `kind`.
* Business metrics scrape success.
* Approval and batch pending oldest age.
* Batch approval failed child counts.

## Validation

The governance check
`docs/design/ci/scripts/check_prometheus_recording_rules.sh` validates the rule
file. If `promtool` is installed, the check also runs:

```bash
promtool check rules deploy/monitoring/prometheus/shepherd-recording-rules.yml
```

The fallback structural check remains mandatory when `promtool` is unavailable.

Prometheus rule-unit-test coverage for these records is defined in
[rule-tests-baseline.md](./rule-tests-baseline.md).
Use `make ci-prometheus-rules` to run recording-rule, alert-rule, and rule-test
validation together.

## Deferred Work

The following remain outside this baseline:

* Business workflow recording rules for VM, provider, or notification flows.
* Multi-window burn-rate SLO alerting.
* Alertmanager routing or receiver policy.
* Trace-aware exemplars and trace-specific dashboard suites.
