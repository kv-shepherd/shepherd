# Observability Design

> **Authority**: Implementation design layer under ADR-0030. Accepted ADRs
> remain the architecture source of truth.

This directory documents Shepherd's production observability contracts.

## Current Baseline

| Area | Status | Reference |
|------|--------|-----------|
| Prometheus scrape endpoint | Accepted minimal baseline | [metrics-baseline.md](./metrics-baseline.md) |
| HTTP request count and latency | Accepted minimal baseline | [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md) |
| Go runtime, process, build info | Accepted minimal baseline | [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md) |
| PostgreSQL/River dead tuple metrics | Accepted database baseline | [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md) |
| OpenAPI validation failure metrics | Accepted contract baseline | [ADR-0054](../../adr/ADR-0054-minimal-prometheus-observability-baseline.md) |
| River queue health metrics | Accepted async execution baseline | [river-queue-metrics-baseline.md](./river-queue-metrics-baseline.md) |
| HTTP request correlation logs | Accepted ingress correlation baseline | [request-correlation-logging-baseline.md](./request-correlation-logging-baseline.md) |
| River worker correlation logs | Accepted async execution log baseline | [river-worker-correlation-logging-baseline.md](./river-worker-correlation-logging-baseline.md) |
| Prometheus alert rule pack | Accepted alert baseline | [alerts-baseline.md](./alerts-baseline.md) |
| Prometheus alert runbook links | Accepted alert runbook baseline | [alert-runbook-link-baseline.md](./alert-runbook-link-baseline.md) |
| Grafana starter dashboard | Accepted dashboard baseline | [dashboards-baseline.md](./dashboards-baseline.md) |
| Grafana dashboard PromQL validation | Accepted dashboard query baseline | [grafana-dashboard-promql-baseline.md](./grafana-dashboard-promql-baseline.md) |
| OpenTelemetry HTTP tracing | Accepted tracing baseline | [tracing-baseline.md](./tracing-baseline.md) |
| Prometheus recording rules | Accepted SLI baseline | [recording-rules-baseline.md](./recording-rules-baseline.md) |
| Prometheus rule tests | Accepted monitoring test baseline | [rule-tests-baseline.md](./rule-tests-baseline.md) |
| Prometheus config validation | Accepted monitoring config baseline | [prometheus-config-validation-baseline.md](./prometheus-config-validation-baseline.md) |
| Prometheus Operator packaging | Accepted optional deployment baseline | [prometheus-operator-baseline.md](./prometheus-operator-baseline.md) |
| Prometheus Operator rule parity | Accepted packaging parity baseline | [prometheus-operator-rule-parity-baseline.md](./prometheus-operator-rule-parity-baseline.md) |
| Docker Compose monitoring packaging | Accepted optional deployment baseline | [compose-monitoring-baseline.md](./compose-monitoring-baseline.md) |
| VM/business metrics | Deferred | [RFC-0010](../../rfc/RFC-0010-observability.md) |

## Boundaries

Operational endpoints are not product REST APIs. `/metrics` is intentionally
outside `/api/v1`, OpenAPI runtime validation, generated frontend clients, and
browser-auth middleware. Deployment ingress, service mesh, or network policy is
responsible for scrape access control.

Metric labels must stay low-cardinality. Route labels use Gin route patterns
such as `/api/v1/vms/:id`, not raw request paths.

OpenAPI validation metrics record only fixed phase/code values, method, and
normalized route. Validation reasons, payloads, schema paths, and resource
identifiers stay out of metric labels.

River queue metrics record only aggregate `queue`, `state`, and `kind` labels.
Job args, job IDs, errors, metadata, tags, ticket IDs, VM names, namespaces,
clusters, users, and payload values stay out of metrics.

HTTP request correlation logs record bounded ingress fields only: request ID,
trace ID, span ID, method, normalized route, status, and duration. They do not
record raw paths, query strings, headers, bodies, users, tickets, VMs,
namespaces, clusters, or provider identifiers.

River worker correlation logs record bounded async execution fields only: job
ID, job kind, queue, attempt, max attempts, result, duration, and trace/span IDs
copied into Shepherd-owned River metadata when a job is inserted under a valid
OpenTelemetry span context. They do not record raw args, encoded args, full
metadata, tags, payloads, arbitrary error text, users, tickets, VMs,
namespaces, clusters, or provider identifiers.

The baseline alert rule pack and starter Grafana dashboard are optional
deployment inputs. Baseline recording rules centralize shared HTTP, OpenAPI, and
River queue queries for those assets. Alert runbook link validation proves each
baseline alert points to an existing local Markdown runbook section. They are
validated by repository governance, but advanced Alertmanager routing, business
SLO dashboards, and tracing views remain deployment-specific RFC-0010 follow-ups.
Dashboard PromQL validation wraps panel queries as temporary recording rules and
runs `promtool check rules`, so syntax drift is caught without requiring a live
Grafana or Prometheus server.

Prometheus rule tests provide executable sample-series coverage for the baseline
recording and alert rules when `promtool` is available. Prometheus config
validation renders the Compose config's container-mounted rule paths to local
repository paths before running `promtool check config`, so rule loading and
scrape-job drift are caught without changing deployment paths.

Prometheus Operator manifests are optional deployment packaging for
`ServiceMonitor` and `PrometheusRule` users. They do not make Prometheus
Operator a Shepherd runtime dependency. Operator rule parity validation ensures
`PrometheusRule.spec.groups` remains aligned with the native recording and alert
rule files.

Docker Compose monitoring manifests are optional deployment packaging for
Prometheus and Grafana users. They do not make either service a mandatory
runtime dependency.

OpenTelemetry tracing is accepted for default-off HTTP ingress spans, W3C
context propagation, and the ADR-0057 HTTP/River correlation-log baseline.
Service, database, KubeVirt, provider, frontend, and business spans remain
deferred.
