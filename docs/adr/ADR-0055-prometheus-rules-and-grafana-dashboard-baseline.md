---
status: "accepted"
date: 2026-05-28
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0055: Prometheus Rules and Grafana Dashboard Baseline

> **Extends**: [ADR-0054](./ADR-0054-minimal-prometheus-observability-baseline.md), [RFC-0010](../rfc/RFC-0010-observability.md)

---

## Context and Problem Statement

The runtime metrics accepted by ADR-0054 need starter operational views and
alerts that are versioned with the product. The repository also needs executable
checks that prevent alert, recording-rule, runbook, and dashboard query drift.

## Decision Drivers

* Keep alerting and dashboards aligned with accepted low-cardinality metrics.
* Centralize shared SLI PromQL so alerts and dashboards do not fork equivalent
  expressions.
* Validate Prometheus rules and dashboard PromQL in CI.
* Keep routing, paging policy, full SLO policy, and advanced dashboards outside
  the baseline.

## Considered Options

* **Option 1**: Ship metrics only and leave all rules and dashboards downstream.
* **Option 2**: Ship starter recording rules, alerts, rule tests, and one
  Grafana overview dashboard.
* **Option 3**: Ship a full production alert routing and dashboard suite.

## Decision Outcome

**Chosen option**: "Option 2", because it makes the accepted metrics immediately
usable while preserving operator ownership of escalation and detailed SLO policy.

### Normative Decisions

* The repository owns a baseline Prometheus recording-rule file for shared
  HTTP, OpenAPI validation, PostgreSQL, and River queue calculations.
* The repository owns a starter alert rule file that uses only accepted metrics
  and recording series.
* Baseline alerts include local `runbook_url` annotations that resolve to
  committed Markdown anchors.
* The repository owns deterministic Prometheus rule unit-test fixtures for the
  baseline recording and alert rules.
* The repository owns one starter Grafana overview dashboard for the accepted
  metrics.
* Dashboard panel PromQL must parse through the repository's Prometheus
  validation path.
* Alertmanager routing, notification receivers, escalation policy, advanced SLOs,
  and broad dashboard suites remain deferred to RFC-0010 or deployment-specific
  policy.

### Consequences

* Operators get useful default views and alert examples without being forced
  into a complete monitoring operating model.
* PromQL drift is caught before merge.
* The baseline remains intentionally small; teams still need to define local
  paging and SLO policy before relying on these alerts for production escalation.

### Confirmation

This ADR is implemented when:

* Prometheus recording rules, alert rules, and rule-test fixtures exist under
  `deploy/monitoring/prometheus/`.
* The Grafana dashboard and provisioning example exist under
  `deploy/monitoring/grafana/`.
* CI validates rule syntax, rule tests, alert runbook links, dashboard JSON, and
  dashboard PromQL.
* `PROMTOOL_REQUIRED=1 make ci-governance` and `PROMTOOL_REQUIRED=1 make pr`
  pass.

## References

* [Alerts baseline design](../design/observability/alerts-baseline.md)
* [Recording rules baseline design](../design/observability/recording-rules-baseline.md)
* [Rule tests baseline design](../design/observability/rule-tests-baseline.md)
* [Dashboards baseline design](../design/observability/dashboards-baseline.md)
