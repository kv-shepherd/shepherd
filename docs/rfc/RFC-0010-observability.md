# RFC-0010: Observability Stack

> **Status**: Accepted for built-in metrics/tracing baseline; advanced scope deferred
> **Priority**: P2  
> **Trigger**: Metrics and tracing required for production monitoring

---

## Problem

V1.0 includes health checks and logging. ADR-0054 accepts the runtime
Prometheus metrics baseline for HTTP traffic, process/runtime health, OpenAPI
validation failures, PostgreSQL table health, and River queue health.
ADR-0055 accepts starter Prometheus recording rules, alerts, rule tests,
runbook-link checks, and a Grafana overview dashboard for those metrics.
ADR-0056 accepts optional Compose and Prometheus Operator packaging for the
baseline monitoring assets. ADR-0057 accepts OpenTelemetry HTTP ingress tracing,
W3C propagation, and bounded HTTP/River worker correlation logs. The current
built-in monitoring path also includes approval/audit business metrics for
approval backlog, approval failures, batch approval state, recent approval
failure audit actions, and Tempo-backed tracing through OpenTelemetry Collector.
Production deployments may still require:
- Centralized log storage and log-based monitoring
- Broader VM, provider, notification, and business SLO metrics
- Advanced alert routing and dashboard contracts

---

## Proposed Components

### Prometheus Metrics

```go
// internal/observability/metrics.go

var (
    RequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: prometheus.DefBuckets,
        },
        []string{"method", "path", "status"},
    )
    
    VMOperationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "vm_operations_total",
            Help: "Total VM operations by type",
        },
        []string{"operation", "status", "cluster"},
    )
)
```

### OpenTelemetry Tracing

```go
// Span propagation across business, DB, River, and KubeVirt/provider boundaries
func (s *VMService) CreateVM(ctx context.Context, ...) error {
    ctx, span := tracer.Start(ctx, "business.vm.request_create")
    defer span.End()
    // ...
}
```

### Metrics Endpoint

```
GET /metrics  # Prometheus scrape endpoint
```

The minimal `/metrics` endpoint and low-cardinality runtime metrics are
accepted by [ADR-0054](../adr/ADR-0054-minimal-prometheus-observability-baseline.md).
Starter Prometheus rules and Grafana assets are accepted by
[ADR-0055](../adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md).
Optional monitoring deployment packaging is accepted by
[ADR-0056](../adr/ADR-0056-observability-deployment-packaging-baseline.md).
HTTP ingress tracing and bounded correlation logs are accepted by
[ADR-0057](../adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md).
OpenTelemetry Collector + Tempo trace storage/query, business spans, pgx DB
spans, River worker spans, and KubeVirt/provider spans are implemented in the
built-in Compose monitoring path. Service/provider log correlation beyond the
River lifecycle boundary, centralized log storage, generic log-derived metrics
and alerts, broader VM/provider/notification business SLOs, advanced alert
routing, and advanced dashboard contracts remain deferred until their contracts
are accepted.

---

## Trigger Conditions

- Production deployment requires SLO monitoring
- Distributed tracing needed for debugging
- Integration with existing monitoring stack (Prometheus/Grafana)

---

## References

- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)
