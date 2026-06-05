---
status: "accepted"
date: 2026-05-28
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0056: Observability Deployment Packaging Baseline

> **Extends**: [ADR-0054](./ADR-0054-minimal-prometheus-observability-baseline.md), [ADR-0055](./ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md), [RFC-0010](../rfc/RFC-0010-observability.md)

---

## Context and Problem Statement

Public deployments need a repeatable way to run the accepted observability
baseline in local Compose and Kubernetes monitoring environments. Packaging must
stay optional and must not make Prometheus, Tempo, OpenTelemetry Collector,
Grafana, or Prometheus Operator a
mandatory runtime dependency for the Shepherd API.

## Decision Drivers

* Keep public deployment assets reusable across Compose and Kubernetes modes.
* Avoid drift between native Prometheus rule files and Prometheus Operator
  packaging.
* Validate scrape config, rule loading, dashboard provisioning, and Compose
  structure in CI.
* Keep environment-specific secrets and image mirroring outside public defaults.

## Considered Options

* **Option 1**: Document monitoring setup only.
* **Option 2**: Ship optional Compose and Prometheus Operator packaging for the
  accepted baseline.
* **Option 3**: Require a bundled monitoring stack in every deployment mode.

## Decision Outcome

**Chosen option**: "Option 2", because it gives operators working starter
packaging without forcing a single monitoring stack into all installations.

### Normative Decisions

* Docker Compose monitoring is an optional overlay that adds Prometheus, Tempo,
  and OpenTelemetry Collector around the Shepherd API service.
* Grafana dashboard and datasource assets remain repository-owned import
  examples, but Grafana is not part of the default Compose overlay.
* Compose packaging uses explicit image references, explicit ports, explicit
  volumes, and documented environment variables.
* Prometheus Operator packaging is optional and limited to a `ServiceMonitor`
  plus `PrometheusRule` starter manifests.
* Operator rule groups must remain content-equivalent to the native Prometheus
  recording and alert rule files.
* Prometheus config validation must prove the scrape config and referenced rule
  files load successfully.
* Public deployment assets must use non-secret placeholders and must not embed
  private registry, kubeconfig, cluster, namespace, or credential values.
* Helm packaging and private-registry mirroring are follow-up deployment work,
  not part of this public baseline.

### Consequences

* Compose users can exercise Prometheus metrics and Tempo tracing without hand
  wiring all assets.
* Kubernetes users with Prometheus Operator can reuse the same rules through
  native CRDs.
* CI catches rule parity and packaging drift before merge.
* Operators still own production hardening such as persistence, TLS, auth,
  resource sizing, and private image registry policy.

### Confirmation

This ADR is implemented when:

* `deploy/prod/docker-compose.monitoring.yml` renders with the production
  Compose files.
* Prometheus config, rules, optional Grafana import assets, Tempo/Collector
  config, and Operator manifests are present under `deploy/monitoring/`.
* CI validates Compose assets, Prometheus config loading, Operator asset shape,
  and Operator rule parity.
* `PROMTOOL_REQUIRED=1 make ci-governance` and `PROMTOOL_REQUIRED=1 make pr`
  pass.

## References

* [Compose monitoring baseline](../design/observability/compose-monitoring-baseline.md)
* [Prometheus Operator baseline](../design/observability/prometheus-operator-baseline.md)
* [Prometheus config validation baseline](../design/observability/prometheus-config-validation-baseline.md)
