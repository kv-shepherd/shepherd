---
status: "accepted"
date: 2026-05-28
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0054: Minimal Prometheus Observability Baseline

> **Extends**: [ADR-0008](./ADR-0008-postgresql-stability.md), [ADR-0030](./ADR-0030-design-documentation-layering-and-fullstack-governance.md), [ADR-0037](./ADR-0037-openapi-validation-architecture-and-enforcement-policy.md), [RFC-0010](../rfc/RFC-0010-observability.md)

---

## Context and Problem Statement

Shepherd needs production-ready runtime metrics before expanding deployment
coverage. The baseline must make HTTP traffic, runtime validation failures,
database maintenance risk, and async queue health visible without leaking tenant
or workload identifiers and without binding operators to a specific monitoring
stack.

## Decision Drivers

* Keep the first observability layer small enough to operate and validate in CI.
* Use low-cardinality Prometheus labels only.
* Avoid raw paths, query strings, user identifiers, tickets, VM names,
  namespaces, clusters, payloads, and job arguments.
* Expose database and River health through scrape-time collectors, not schema
  changes.
* Keep business SLO metrics and deep tracing as RFC follow-up scope.

## Considered Options

* **Option 1**: Keep observability deferred to downstream deployments.
* **Option 2**: Add a minimal Prometheus runtime metrics baseline.
* **Option 3**: Add a full metrics, tracing, logs, alerts, and dashboard stack.

## Decision Outcome

**Chosen option**: "Option 2", because it gives operators useful production
signals now while keeping cardinality, privacy, and deployment drift under
control.

### Normative Decisions

* Shepherd exposes `/metrics` as an operational endpoint outside `/api/v1`.
* Metrics use an isolated Prometheus registry with Go runtime, process, build
  info, HTTP request, OpenAPI validation, PostgreSQL table, and River queue
  collectors.
* HTTP request metrics use normalized route labels from the router contract,
  not raw URL paths.
* OpenAPI validation failure metrics use only fixed `phase`, `code`, `method`,
  and normalized `route` labels.
* PostgreSQL table metrics scrape `pg_stat_user_tables` for a fixed table allow
  list covering River, audit, and domain-event operational tables.
* River queue metrics expose aggregate queue health only: queue name, job state,
  job kind, backlog, oldest ready age, terminal job counts, and scrape success.
* Metrics must not include users, tickets, VMs, namespaces, clusters, raw job
  args, payloads, raw SQL, raw errors, URL query strings, or resource names.
* Custom business metrics, per-tenant metrics, provider-level metrics, and
  advanced SLO policy remain deferred to [RFC-0010](../rfc/RFC-0010-observability.md).

### Consequences

* Operators get a stable scrape endpoint and enough runtime signals to diagnose
  HTTP health, validation drift, database bloat risk, and async backlog.
* CI and code review can enforce a small label vocabulary.
* Detailed business analytics remain out of scope for this baseline.
* Adding future metrics requires reviewing cardinality and privacy impact before
  extending the registry.

### Confirmation

This ADR is implemented when:

* `/metrics` serves the isolated registry and standard collectors.
* Unit tests cover HTTP, OpenAPI validation, PostgreSQL, and River queue metric
  collection.
* Design governance documents the accepted metric names, labels, and deferred
  scope.
* `make ci-governance`, `make ci-go-lint`, and `make pr` pass.

## References

* [RFC-0010: Observability](../rfc/RFC-0010-observability.md)
* [Metrics baseline design](../design/observability/metrics-baseline.md)
