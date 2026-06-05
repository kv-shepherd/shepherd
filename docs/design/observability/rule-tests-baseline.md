# Prometheus Rule Test Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)

## Scope

The rule-test baseline provides deterministic sample-series tests for the
accepted Prometheus recording and alert rules. It validates monitoring assets,
not Shepherd runtime code.

Test fixture:

```text
deploy/monitoring/prometheus/shepherd-rules.test.yml
```

## Coverage

The fixture covers every accepted baseline recording series:

| Record | Expected behavior |
|--------|-------------------|
| `shepherd:http_requests:rate5m` | Calculates global request rate |
| `shepherd:http_requests_5xx:rate5m` | Calculates global 5xx request rate |
| `shepherd:http_5xx:ratio5m` | Calculates global 5xx ratio |
| `shepherd:http_request_duration_seconds:p95_5m` | Calculates global HTTP p95 latency |
| `shepherd:openapi_validation_failures:rate5m` | Preserves only fixed `phase` and `code` labels |
| `shepherd:river_ready_jobs:sum` | Calculates ready River jobs by `queue` |
| `shepherd:river_recent_discarded_jobs:sum` | Calculates recently discarded River jobs by `queue` |
| `shepherd:business_approval_pending:sum` | Calculates pending approval backlog by `operation_type` |
| `shepherd:business_approval_failed:sum` | Calculates failed approval tickets by `operation_type` |
| `shepherd:business_batch_approval_pending:sum` | Calculates pending batch approvals by `batch_type` |
| `shepherd:business_batch_approval_failed:sum` | Calculates failed batch approvals by `batch_type` |
| `shepherd:business_approval_failure_audit_actions:sum` | Calculates recent failure audit actions by fixed `action` |

The fixture also covers every accepted baseline alert:

* `ShepherdMetricsTargetDown`
* `ShepherdHighHTTP5xxRate`
* `ShepherdHighHTTPP95Latency`
* `ShepherdOpenAPIValidationFailures`
* `ShepherdPostgresStatsScrapeFailed`
* `ShepherdRiverDeadTupleRatioHigh`
* `ShepherdRiverQueueStatsScrapeFailed`
* `ShepherdRiverQueueBacklogAgeHigh`
* `ShepherdRiverJobsDiscarded`
* `ShepherdBusinessMetricsScrapeFailed`
* `ShepherdApprovalPendingTooLong`
* `ShepherdApprovalFailuresPresent`
* `ShepherdBatchApprovalPendingTooLong`
* `ShepherdBatchApprovalFailuresPresent`
* `ShepherdApprovalFailureAuditActionsRecent`

## Evaluation Order

Rule tests load the recording rules before the alert rules and declare:

```yaml
group_eval_order:
  - shepherd.recording
  - shepherd.baseline
```

This keeps alerts that reference recording series deterministic.

The fixture intentionally omits newer optional test-file fields such as
`fuzzy_compare`. GitHub Actions installs the Ubuntu 24.04 `promtool` package,
and that packaged baseline must be able to run the rule tests directly.

## Validation

The governance check
`docs/design/ci/scripts/check_prometheus_rule_tests.sh` validates the test
fixture. If `promtool` is installed, the check also runs:

```bash
promtool test rules deploy/monitoring/prometheus/shepherd-rules.test.yml
```

When `promtool` is unavailable, the fallback structural check verifies rule-file
references, group evaluation order, required recording expression tests, and
required alert tests.

GitHub Actions must not use that fallback as release evidence. The CI governance
job installs `promtool` and runs with:

```bash
PROMTOOL_REQUIRED=1 make ci-governance
```

With `PROMTOOL_REQUIRED=1`, missing `promtool` is a hard failure for recording
rules, alert rules, Prometheus config validation, Operator rule parity, and
rule unit tests.

## Tool Discovery

Prometheus rule validation scripts use this tool discovery order:

1. `PROMTOOL=/absolute/path/to/promtool`
2. `promtool` found on `PATH`
3. structural fallback checks

The unified local entry point is:

```bash
make ci-prometheus-rules
```

Use `PROMTOOL=/path/to/promtool make ci-prometheus-rules` when `promtool` is
installed outside `PATH`.

Use `PROMTOOL_REQUIRED=1` only in CI or release-evidence environments where a
structural fallback would be too weak.

## Deferred Work

The baseline does not define broad VM/provider/notification business SLO tests,
burn-rate alert tests, or deployment-specific alert routing tests. Those remain
RFC-0010 follow-ups.
