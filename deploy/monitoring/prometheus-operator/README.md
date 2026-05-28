# Shepherd Prometheus Operator Monitoring

This directory contains optional Prometheus Operator starter manifests for the
accepted Shepherd monitoring baseline.

| File | Purpose |
|------|---------|
| `shepherd-service-monitor.yml` | `ServiceMonitor` for scraping Shepherd `/metrics` |
| `shepherd-prometheus-rule.yml` | `PrometheusRule` containing baseline recording and alert rules, including ADR-0054 River queue health rules |

The manifests assume:

* the Shepherd Kubernetes `Service` has label `app.kubernetes.io/name: shepherd`
* the service exposes a named `http` port
* the Prometheus instance selects `PrometheusRule` objects with
  `app.kubernetes.io/name: shepherd` and `role: alert-rules`, or deployments
  adjust the labels to their local selector policy

These manifests are deployment inputs, not runtime dependencies.
