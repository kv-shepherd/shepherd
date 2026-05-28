# HTTP Request Correlation Logging Baseline

> **Decision**: [ADR-0057](../../adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md)

## Scope

This baseline adds one structured HTTP request completion log at the ingress
boundary. It complements Prometheus metrics and default-off HTTP tracing without
adding service-layer, database, River, KubeVirt, provider, frontend, or business
spans.

## Runtime Contract

Each logged product request includes only:

| Field | Source |
|-------|--------|
| `request_id` | `X-Request-ID` request/response correlation ID |
| `trace_id` | Current OpenTelemetry span context when valid |
| `span_id` | Current OpenTelemetry span context when valid |
| `method` | HTTP method |
| `route` | Normalized Gin route pattern, or `unmatched` |
| `status` | HTTP response status |
| `duration_ms` | Request duration in milliseconds |

The middleware sets `X-Shepherd-Trace-ID` when a valid span context exists.
This is an operational response header, not an OpenAPI product response field.

## Noise Policy

The middleware skips successful operational self-observation requests:

* `/metrics` below HTTP 500
* `/api/v1/health/*` below HTTP 500

HTTP 500 and above are always logged, including operational endpoints.

## Data Policy

The baseline must not log:

* raw URL paths or query strings
* request or response headers other than the bounded correlation IDs
* request or response bodies
* users, roles, sessions
* tickets, VM IDs, VM names
* namespaces, clusters, systems, services, providers
* arbitrary validation or payload error text

Route identity must come from Gin's normalized route pattern.

## Validation

Runtime tests must prove:

* request completion logs use normalized route labels;
* request logs include `request_id`;
* valid span contexts add `trace_id`, `span_id`, and `X-Shepherd-Trace-ID`;
* metrics and health requests below 500 are skipped;
* metrics and health requests at 500 or above are logged.

Design governance freezes the ADR, this design doc, runtime middleware, router
wiring, and tests.

## Deferred Work

The following remain deferred until accepted by later ADRs:

* context-aware logger migration inside individual services and workers beyond
  the River lifecycle log accepted by ADR-0057
* database, River, KubeVirt, provider, and business spans
* frontend tracing
* log shipping, index templates, retention, and redaction policy
* trace-aware metric exemplars
