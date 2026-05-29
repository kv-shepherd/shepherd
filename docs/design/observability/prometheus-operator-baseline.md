# Prometheus Operator Packaging Baseline

> **Decision**: [ADR-0056](../../adr/ADR-0056-observability-deployment-packaging-baseline.md)

## Scope

The Prometheus Operator packaging baseline provides optional Kubernetes-native
starter manifests for clusters that use `ServiceMonitor` and `PrometheusRule`
resources.

Manifest directory:

```text
deploy/monitoring/prometheus-operator/
```

## Assets

| File | Purpose |
|------|---------|
| `shepherd-service-monitor.yml` | Scrape Shepherd `/metrics` through a named `http` service port |
| `shepherd-prometheus-rule.yml` | Package the accepted recording and alert rules as a `PrometheusRule` |

## Selector Contract

The starter `ServiceMonitor` selects services with:

```yaml
app.kubernetes.io/name: shepherd
```

The selected Kubernetes `Service` must expose a named port:

```yaml
name: http
```

The scrape endpoint is:

```yaml
path: /metrics
interval: 30s
scheme: http
```

Deployments may adjust labels, namespace placement, and selectors to match their
Prometheus Operator installation. Those choices are deployment policy, not
Shepherd runtime behavior.

## Rule Packaging

The `PrometheusRule` manifest includes:

* `shepherd.recording` from ADR-0055.
* `shepherd.baseline` from ADR-0055.

Native rule files under `deploy/monitoring/prometheus/` remain the inputs for
`promtool check rules` and `promtool test rules`. The operator manifest is a
deployment packaging layer and must stay aligned through governance checks.
Rule-content parity is governed by
[prometheus-operator-rule-parity-baseline.md](./prometheus-operator-rule-parity-baseline.md),
which extracts `PrometheusRule.spec.groups`, compares it with the combined
native recording and alert rule groups, and validates the extracted rules with
`promtool check rules` when available.

## Validation

The governance check
`docs/design/ci/scripts/check_prometheus_operator_assets.sh` validates:

* required manifest files exist
* `ServiceMonitor` API version, kind, selector, endpoint port, path, scheme, and
  interval
* `PrometheusRule` API version, kind, labels, group names, recording rule names,
  and alert names
* no forbidden high-cardinality labels are introduced

`docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh` additionally
validates rule-content parity against the native Prometheus rule files.

## Deferred Work

The baseline does not provide:

* Prometheus or Alertmanager custom resources.
* Alertmanager routing, receivers, inhibition, or silence policy.
* Helm chart integration.
* Namespace, network policy, ingress, or service mesh policy.
* Grafana deployment resources.
