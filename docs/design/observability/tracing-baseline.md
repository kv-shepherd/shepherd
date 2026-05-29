# OpenTelemetry HTTP Tracing Baseline

> **Decision**: [ADR-0057](../../adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md)

## Scope

The tracing baseline provides default-off OpenTelemetry HTTP ingress tracing for
Shepherd. It establishes a tracer provider, W3C TraceContext/Baggage
propagation, and Gin middleware without adding service-layer, database, River,
KubeVirt, provider, frontend, or business spans.

## Configuration

```yaml
observability:
  tracing_enabled: false
  tracing_service_name: "shepherd"
  tracing_exporter: "otlp_http"
  tracing_sample_ratio: 0.10
  tracing_shutdown_timeout: "5s"
```

Environment overrides:

| Variable | Meaning |
|----------|---------|
| `OBSERVABILITY_TRACING_ENABLED` | Enable or disable OpenTelemetry tracing |
| `OBSERVABILITY_TRACING_SERVICE_NAME` | OpenTelemetry service name |
| `OBSERVABILITY_TRACING_EXPORTER` | `otlp_http` or `stdout` |
| `OBSERVABILITY_TRACING_SAMPLE_RATIO` | Parent-based TraceID ratio sampler value from `0.0` to `1.0` |
| `OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT` | Tracer provider shutdown timeout |

For `otlp_http`, endpoint and authentication are configured with standard
OpenTelemetry environment variables such as `OTEL_EXPORTER_OTLP_ENDPOINT`,
`OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, and
`OTEL_EXPORTER_OTLP_TRACES_HEADERS`. Docker Compose deployments pass these
through explicitly because Compose services do not inherit arbitrary host
environment variables unless they are part of the service environment.

## Instrumentation Contract

Accepted baseline:

| Area | Contract |
|------|----------|
| Propagation | W3C TraceContext + Baggage |
| Root span | Gin HTTP ingress span |
| Service name | `observability.tracing_service_name`, default `shepherd` |
| Sampling | Parent-based TraceID ratio sampling |
| Shutdown | Application shutdown calls tracer provider shutdown with timeout |
| Request correlation | ADR-0057 logs request completion fields and exposes `X-Shepherd-Trace-ID` when span context is valid |

Deferred:

* Service/use-case spans.
* Database, River, KubeVirt, and provider instrumentation.
* Frontend tracing.
* Deep service/worker log correlation beyond ADR-0057 request completion logs.
* Trace-aware exemplars.
* Business span names and domain attributes.

## Attribute Policy

The baseline must not add span attributes from:

* raw URL paths
* query strings or headers
* users, roles, sessions, tickets
* VM, namespace, cluster, system, service, or provider names
* request or response bodies

Route-level identity comes from the repository tracing middleware reading
Gin's normalized `FullPath()` route pattern.

## Verification

Runtime tests verify that:

* tracing is not wired when disabled
* enabled tracing adds HTTP middleware
* invalid tracing configuration fails validation
* application shutdown shuts down tracing without panics

## Deferred Work

Deeper tracing must be accepted by later ADRs before implementation. In
particular, service-layer spans and provider/KubeVirt attributes need a
low-cardinality attribute contract before they are safe to expose.
