# OpenTelemetry Tracing Baseline

> **Decision**: [ADR-0057](../../adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md)

## Scope

The tracing baseline provides built-in OpenTelemetry tracing for Shepherd. It
establishes a tracer provider, W3C TraceContext/Baggage propagation, Gin
middleware, low-cardinality business spans, pgx database spans, River worker
spans, and KubeVirt/provider client spans.

The Compose monitoring stack uses OpenTelemetry Collector as the only
application trace ingest endpoint and Tempo as the default trace backend. The
Shepherd administrator UI queries Tempo through protected backend APIs; Grafana
is not started or embedded by default. Jaeger remains a compatible alternative
for deployments that replace Tempo behind the Collector, but it is not the
built-in default.

## Configuration

```yaml
observability:
  tracing_enabled: true
  tracing_service_name: "shepherd"
  tracing_exporter: "otlp_http"
  tracing_sample_ratio: 0.10
  tracing_shutdown_timeout: "5s"
  trace_query_enabled: true
  trace_query_url: "http://tempo:3200"
  trace_query_timeout: "3s"
  trace_query_limit: 100
  trace_query_lookback: "1h"
```

Environment overrides:

| Variable | Meaning |
|----------|---------|
| `OBSERVABILITY_TRACING_ENABLED` | Enable or disable OpenTelemetry tracing |
| `OBSERVABILITY_TRACING_SERVICE_NAME` | OpenTelemetry service name |
| `OBSERVABILITY_TRACING_EXPORTER` | `otlp_http` or `stdout` |
| `OBSERVABILITY_TRACING_SAMPLE_RATIO` | Parent-based TraceID ratio sampler value from `0.0` to `1.0` |
| `OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT` | Tracer provider shutdown timeout |
| `OBSERVABILITY_TRACE_QUERY_ENABLED` | Enable the Shepherd administrator trace summary API |
| `OBSERVABILITY_TRACE_QUERY_URL` | Tempo HTTP API base URL used by Shepherd, default `http://tempo:3200` |
| `OBSERVABILITY_TRACE_QUERY_TIMEOUT` | Timeout for the administrator trace summary query |
| `OBSERVABILITY_TRACE_QUERY_LIMIT` | Maximum recent traces inspected per query |
| `OBSERVABILITY_TRACE_QUERY_LOOKBACK` | Default administrator trace lookback window |

For `otlp_http`, Compose defaults point to:

| Variable | Default |
|----------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://otel-collector:4318` |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `http://otel-collector:4318/v1/traces` |

Authentication or alternate endpoints can be configured with standard
OpenTelemetry environment variables such as `OTEL_EXPORTER_OTLP_HEADERS` and
`OTEL_EXPORTER_OTLP_TRACES_HEADERS`.

The Shepherd administrator UI queries `/api/v1/admin/observability/traces`,
which reads Tempo through `OBSERVABILITY_TRACE_QUERY_URL` and returns route,
dependency span, and slow-trace aggregates. The UI does not expose raw Tempo
payloads or require administrators to open Grafana Explore for the default
interface-health view.

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
| Business spans | Approval, batch power, and VM request use-case boundaries |
| Database spans | pgx query spans for Ent, River, and sqlc through the shared pool |
| River spans | Worker consumer spans with trace context propagated through River metadata |
| KubeVirt/provider spans | Kubernetes/KubeVirt client spans and SSA apply/dry-run spans |

Deferred:

* Frontend tracing.
* Trace-aware frontend/backend session stitching.
* Trace-aware exemplars.
* Jaeger as an additional built-in query UI.

## Attribute Policy

The baseline must not add span attributes from:

* raw URL paths
* query strings or headers
* users, roles, sessions, tickets
* VM names or IDs
* ticket IDs, event IDs, request IDs, or requester/approver IDs
* request or response bodies

Route-level identity comes from the repository tracing middleware reading
Gin's normalized `FullPath()` route pattern. Database spans record only SQL
operation and collection/table name; raw SQL and query arguments are not
exported. KubeVirt/provider spans record operation/resource metadata and bounded
result counts; individual resource names are not exported.

## Verification

Runtime tests verify that:

* tracing is not wired when disabled
* enabled tracing adds HTTP middleware
* pgx tracing summarizes SQL without raw queries or args
* River middleware creates worker spans and propagates trace metadata
* invalid tracing configuration fails validation
* application shutdown shuts down tracing without panics
