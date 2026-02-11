---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-10
deciders: []
consulted: []
informed: []
---

# ADR-0031: Concurrency Safety and Worker Pool Standard

> **Review Period**: Until 2026-02-10 (48-hour minimum)  
> **Discussion**: [Issue #151](https://github.com/kv-shepherd/shepherd/issues/151)  
> **Related**: [ADR-0006](./ADR-0006-unified-async-model.md), [ADR-0012](./ADR-0012-hybrid-transaction.md), [ADR-0008](./ADR-0008-postgresql-stability.md)  
> **CI Enforcement**: `docs/design/ci/scripts/check_naked_goroutine.go`, `docs/design/ci/scripts/check_semaphore_usage.go`

---

## Context and Problem Statement

Shepherd is a long-running control-plane service that interacts with Kubernetes/KubeVirt, PostgreSQL, and background workers. Unbounded or ad-hoc concurrency (for example, scattered `go func()` usage) introduces repeated failure modes:

* unbounded goroutine growth and resource exhaustion under load
* inconsistent panic recovery and shutdown semantics
* hard-to-review concurrency patterns with duplicated error handling
* missing observability (no consistent queueing/metrics surface)
* semaphore leaks (Acquire without Release) leading to deadlocks

We need a single, enforceable concurrency standard that keeps runtime behavior predictable and reviewable across the codebase.

## Decision Drivers

* Provide bounded, observable concurrency for in-process work.
* Centralize panic recovery and lifecycle management (startup/shutdown).
* Make concurrency patterns easy to review and hard to misuse.
* Avoid deadlocks by standardizing semaphore usage.
* Keep River worker execution model separate and deterministic.

## Considered Options

* **Option 1**: Allow direct `go` statements with "best effort" code review guidance.
* **Option 2**: Require structured concurrency primitives (`errgroup`, semaphores), but allow `go` statements freely.
* **Option 3**: Enforce a Worker Pool standard for in-process concurrency and forbid naked `go` statements in application code.

## Decision Outcome

**Chosen option**: "Option 3", because it provides an enforceable baseline that reduces concurrency-related footguns and keeps runtime behavior consistent.

### Normative Rules

1. **No naked `go` statements in application code**
   - For any non-test code under `internal/`, direct `go` statements are forbidden.
   - All in-process concurrency must be submitted via a Worker Pool API.
   - Exceptions are limited to concurrency infrastructure itself (for example, the Worker Pool implementation package) and River internals. The CI exemption list is the source of truth.

2. **Worker Pool API must support context propagation**
   - The submission method (e.g., `Submit(ctx, func(ctx))`) MUST accept `context.Context`.
   - The implementation MUST respect context cancellation/timeout for graceful shutdown and task lifecycle management.
   - **Request-scoped tasks** MUST pass the upstream request context; task functions SHOULD check `ctx.Done()` at blocking points.
   - **Detached background tasks** MUST use a service-lifecycle context and explicitly declare "detached" semantics (e.g., `SubmitDetached(task)`).

3. **Do not nest Worker Pool inside River workers**
   - River already provides worker concurrency controls and backpressure.
   - River job handlers must execute synchronously; do not offload job work into a pool.

4. **Semaphore usage must be leak-safe**
   - Any semaphore `Acquire(...)` must be paired with a `defer Release(...)` in the same function.
   - Avoid complex control flow that makes release non-obvious.

### Consequences

* ✅ Good, because concurrency becomes bounded and centrally configurable.
* ✅ Good, because panic recovery and metrics can be implemented once and reused everywhere.
* ✅ Good, because CI can reliably detect forbidden patterns (`go` statements and unsafe semaphore usage).
* 🟡 Neutral, because some concurrency helpers that spawn goroutines indirectly still require code review discipline.
* ❌ Bad, because it is stricter than typical Go style and requires developers to follow a project-specific pattern (mitigated by examples and CI guidance).

### Confirmation

* CI blocks `go` statements in `internal/` (non-test) via `docs/design/ci/scripts/check_naked_goroutine.go`.
* CI blocks unsafe semaphore usage via `docs/design/ci/scripts/check_semaphore_usage.go`.
* Code review verifies any new exemption is justified and added to the exemption list.

---

## Pros and Cons of the Options

### Option 1: Allow direct `go` statements + code review guidance

* ✅ Good, because it is simple and idiomatic.
* ✅ Good, because it avoids adding infrastructure.
* ❌ Bad, because failures repeat (unbounded concurrency, missing recovery, inconsistent shutdown).
* ❌ Bad, because enforcement relies on reviewers catching subtle issues.

### Option 2: Structured concurrency but allow `go` statements freely

* ✅ Good, because it encourages better patterns than ad-hoc goroutines.
* 🟡 Neutral, because enforcement remains difficult without a single submission surface.
* ❌ Bad, because it still allows bypassing constraints and observability.

### Option 3: Worker Pool standard + forbid naked `go` statements

* ✅ Good, because it is enforceable and consistent.
* ✅ Good, because it provides a single point for metrics, rate limiting, and panic recovery.
* ❌ Bad, because it can be overused and must not replace River semantics for async writes.

---

## More Information

### Related Decisions

* [ADR-0006](./ADR-0006-unified-async-model.md) - Unified async model (River) for state-changing operations that coordinate with external systems.
* [ADR-0012](./ADR-0012-hybrid-transaction.md) - Transaction boundaries and "no external calls in DB transactions" rule.

### Implementation Notes

* The reference Worker Pool example is maintained in `docs/design/examples/worker/pool.go`.
* The reference CI gates live under `docs/design/ci/` and are required before coding-phase transition.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-08 | @jindyzhao | Initial draft |
| 2026-02-08 | @jindyzhao | Added Rule 2: context propagation requirement (review feedback) |
