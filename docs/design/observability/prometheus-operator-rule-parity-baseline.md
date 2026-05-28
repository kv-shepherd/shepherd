# Prometheus Operator Rule Parity Baseline

> **Decision**: [ADR-0056](../../adr/ADR-0056-observability-deployment-packaging-baseline.md)

## Scope

The Prometheus Operator rule parity baseline protects the optional
`PrometheusRule` packaging accepted by ADR-0056. It verifies that Operator users
receive the same recording and alert rules validated by native Prometheus rule
checks and rule-unit-test fixtures.

Operator manifest:

```text
deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml
```

Native rule sources:

```text
deploy/monitoring/prometheus/shepherd-recording-rules.yml
deploy/monitoring/prometheus/shepherd-alerts.yml
```

## Parity Contract

The native rule files remain authoritative. The Operator manifest must package
those same groups under:

```yaml
spec:
  groups:
    - name: shepherd.recording
    - name: shepherd.baseline
```

In prose, `PrometheusRule.spec.groups` is the only Operator rule payload this
baseline accepts for parity with native rules.

The parity gate compares the extracted `spec.groups` content against a combined
native rule file built from:

1. `shepherd-recording-rules.yml`
2. `shepherd-alerts.yml`

The comparison covers rule names, expressions, intervals, `for` durations,
labels, annotations, and runbook URLs. A same-name rule with different content
is not acceptable packaging evidence.

## Validation

The governance check is:

```bash
bash docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh
```

The script:

1. builds a temporary combined native rule file;
2. extracts `spec.groups` from the Operator `PrometheusRule` into a temporary
   native Prometheus rule file;
3. compares the two files;
4. runs `promtool check rules` on the extracted Operator groups when `promtool`
   is available.

With `PROMTOOL_REQUIRED=1`, missing `promtool` is a hard failure. Without
`promtool`, the parity comparison still runs, but the syntax check is a weaker
local fallback.

This check does not require Kubernetes, Prometheus Operator CRDs, or a live
Prometheus instance.

## Deferred Work

This baseline does not introduce manifest generation, Helm chart values,
Prometheus `ruleSelector` policy, namespace policy, or Alertmanager routing.
Those remain deployment-owned concerns.
