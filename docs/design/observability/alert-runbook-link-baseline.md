# Prometheus Alert Runbook Link Baseline

> **Decision**: [ADR-0055](../../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md)

## Scope

The alert runbook link baseline protects the operational usefulness of the
starter Prometheus alert pack, including the River queue health alerts accepted
by ADR-0054. It validates that every baseline alert has a `runbook_url`
annotation and that the annotation points to a real local Markdown anchor.

Rule file:

```text
deploy/monitoring/prometheus/shepherd-alerts.yml
```

Default runbook:

```text
docs/design/observability/alerts-baseline.md
```

## Contract

Each baseline alert must include:

```yaml
annotations:
  summary: ...
  description: ...
  runbook_url: docs/design/observability/alerts-baseline.md#<anchor>
```

The runbook URL must be repository-local and point under
`docs/design/observability/`. External URLs, absolute paths, missing fragments,
and anchors that do not exist in the target Markdown file are not accepted for
the baseline pack.

Operator packaging inherits these annotations through the ADR-0056 parity gate.

## Validation

The governance check is:

```bash
bash docs/design/ci/scripts/check_prometheus_alert_runbooks.sh
```

The script validates:

1. every expected baseline alert appears exactly once;
2. every alert has a `runbook_url` annotation;
3. the URL is a local Markdown path with an anchor fragment;
4. the target file exists;
5. the target Markdown heading generates that anchor.

`make ci-prometheus-rules` and `make ci-monitoring-assets` include this check.

## Deferred Work

This baseline does not define Alertmanager notification templates, external
runbook hosting, paging escalation, or receiver routing. Those remain
deployment-owned policy.
