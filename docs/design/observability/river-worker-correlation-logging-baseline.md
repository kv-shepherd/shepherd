# River Worker Correlation Logging Baseline

> **Decision**: [ADR-0057](../../adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md)

## Scope

This baseline adds centralized River worker completion logs and trace metadata
propagation for jobs inserted under an active OpenTelemetry span context. It
complements River queue health metrics and HTTP request correlation logs without
adding business metrics, deep tracing, or per-worker custom log schemas.

## Insert Contract

The River insert middleware may write only this Shepherd-owned metadata object:

```json
{
  "shepherd_trace": {
    "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
    "span_id": "00f067aa0ba902b7"
  }
}
```

The middleware must preserve existing metadata keys and must not inspect or log
job args, encoded args, tags, payloads, request paths, users, tickets, VMs,
namespaces, clusters, providers, or KubeVirt object names.

## Work Contract

Each worked job emits one completion log event:

```text
river_job_completed
```

Allowed fields:

| Field | Source |
|-------|--------|
| `job_id` | River job row ID |
| `job_kind` | River job kind |
| `queue` | River queue name |
| `attempt` | River attempt number |
| `max_attempts` | River max attempts |
| `duration_ms` | Middleware-measured work duration |
| `result` | `succeeded`, `failed`, `snoozed`, or `cancelled` |
| `trace_id` | Shepherd-owned River metadata when present |
| `span_id` | Shepherd-owned River metadata when present |
| `error_type` | Go error type for non-success results only |

Success logs are emitted at info level. Non-success logs are emitted at warn
level. The middleware must return the worker error unchanged.

## Data Policy

The baseline must not log:

* raw job args or encoded args
* full River metadata or tags
* arbitrary error strings
* users, roles, sessions
* tickets, VM IDs, VM names
* namespaces, clusters, systems, services, providers
* KubeVirt object names or manifest fragments
* request paths, query strings, headers, or bodies

`error_type` is allowed because it is code-owned and stable enough for routing;
error messages are not allowed because provider/runtime errors can contain
operator-specific resource names.

## Validation

Runtime tests must prove:

* valid OpenTelemetry span contexts are copied into `shepherd_trace` metadata;
* pre-existing metadata keys are preserved;
* completion logs include only the allowed fields;
* `succeeded`, `failed`, `snoozed`, and `cancelled` results are classified;
* raw error messages and job args do not appear in structured log fields;
* River client initialization wires the middleware globally.

Design governance freezes the ADR, this design doc, runtime middleware, River
client wiring, and tests.

## Deferred Work

The following remain deferred until accepted by later ADRs:

* service-layer and provider spans
* context-aware logger migration inside individual workers and services
* River job duration metrics or trace-aware exemplars
* frontend tracing
* log shipping, index templates, retention, and redaction policy
