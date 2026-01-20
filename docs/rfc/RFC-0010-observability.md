# RFC-0010: Observability Stack

> **Status**: Deferred  
> **Priority**: P2  
> **Trigger**: Metrics and tracing required for production monitoring

---

## Problem

V1.0 includes basic health checks and logging. Production deployments may require:
- Prometheus metrics export
- Distributed tracing (OpenTelemetry)
- Custom business metrics

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
// Span propagation across services
func (s *VMService) CreateVM(ctx context.Context, ...) error {
    ctx, span := tracer.Start(ctx, "VMService.CreateVM")
    defer span.End()
    
    span.SetAttributes(
        attribute.String("vm.name", name),
        attribute.String("cluster", clusterName),
    )
    // ...
}
```

### Metrics Endpoint

```
GET /metrics  # Prometheus scrape endpoint
```

---

## Trigger Conditions

- Production deployment requires SLO monitoring
- Distributed tracing needed for debugging
- Integration with existing monitoring stack (Prometheus/Grafana)

---

## References

- [Prometheus Go Client](https://github.com/prometheus/client_golang)
- [OpenTelemetry Go](https://opentelemetry.io/docs/instrumentation/go/)
