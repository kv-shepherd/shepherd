---
status: "accepted"
date: 2026-05-28
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0057: OpenTelemetry and Correlation Logging Baseline

> **Extends**: [ADR-0054](./ADR-0054-minimal-prometheus-observability-baseline.md), [RFC-0010](../rfc/RFC-0010-observability.md)

---

## Context and Problem Statement

Metrics show that something is wrong, but operators also need bounded request
and worker lifecycle correlation. The baseline must provide OpenTelemetry
propagation and completion logs without exposing raw paths, payloads, job args,
resource identifiers, or arbitrary error text.

## Decision Drivers

* Keep tracing default-off and safe for deployments without a collector.
* Use W3C TraceContext and Baggage propagation.
* Use normalized route names instead of raw paths or query strings.
* Make HTTP and River lifecycle logs correlate with trace IDs while preserving
  a bounded field vocabulary.
* Avoid deep service/provider spans until the first baseline is proven in
  production.

## Considered Options

* **Option 1**: Keep tracing and correlation logging fully deferred.
* **Option 2**: Add default-off HTTP ingress tracing plus bounded HTTP and River
  worker lifecycle logs.
* **Option 3**: Add full distributed tracing and detailed structured logs across
  all service/provider boundaries.

## Decision Outcome

**Chosen option**: "Option 2", because it gives production operators enough
correlation to debug requests and async work without introducing high-cardinality
or sensitive telemetry.

### Normative Decisions

* OpenTelemetry tracing is disabled by default and enabled only through explicit
  configuration.
* The HTTP server extracts and injects W3C TraceContext and Baggage.
* HTTP spans use a repository-owned Gin middleware that names spans and
  attributes from normalized routes only.
* HTTP spans must not emit raw URL paths, query strings, users, tickets, VMs,
  namespaces, clusters, or payload data.
* Tracer provider shutdown participates in application shutdown.
* HTTP request completion logs include bounded fields such as request ID,
  trace ID, method, normalized route, status, duration, and error type.
* River worker completion logs include bounded lifecycle fields such as job kind,
  queue, attempt, max attempts, result, duration, trace ID, span ID, and error
  type.
* Shepherd-owned River trace metadata may be stored in River job metadata, but
  logs must not emit raw metadata blobs, encoded args, payloads, resource IDs,
  or arbitrary error strings.
* Deep spans, provider spans, SQL spans, and broad service log correlation remain
  deferred to RFC-0010.

### Consequences

* Operators can correlate ingress requests and async work through request IDs and
  trace IDs.
* The implementation avoids dependency behavior that would record raw URL paths.
* The baseline is intentionally not a full distributed tracing program.
* Future instrumentation must prove it preserves the same cardinality and
  sensitive-data constraints.

### Confirmation

This ADR is implemented when:

* Tracing setup, Gin middleware, and graceful shutdown are covered by unit tests.
* HTTP log tests reject raw paths, queries, payload identifiers, and arbitrary
  error text.
* River worker log tests reject raw args, metadata, resource identifiers, and
  arbitrary error text.
* `go test ./internal/observability -count=1`, `make ci-go-lint`, and
  `make pr` pass.

## References

* [Tracing baseline design](../design/observability/tracing-baseline.md)
* [Request correlation logging baseline](../design/observability/request-correlation-logging-baseline.md)
* [River worker correlation logging baseline](../design/observability/river-worker-correlation-logging-baseline.md)
