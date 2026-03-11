# RFC-0020: Optional K8s Watch Acceleration for VM Status Freshness

> **Status**: Deferred
> **Priority**: P3
> **Source**: ADR-0038 follow-up boundary clarification
> **Trigger**: Near-realtime VM drift visibility or watch-cache-backed read acceleration becomes a product requirement

---

## Problem

The current accepted baseline for VM status convergence is
[ADR-0038](../adr/ADR-0038-adaptive-k8s-polling.md): adaptive polling with
`ResourceVersion` caching.

This is a good fit for the platform's current governance-oriented scope, but it
does not provide near-instant detection of:

- out-of-band VM status changes
- operator-side interventions outside Shepherd write paths
- large dashboard/read scenarios that benefit from warm local state

The team needs a documented future path in case lower-latency status freshness
becomes necessary.

---

## Current State

- `internal/jobs/vm_status_sync.go` is the **only authoritative** status
  convergence path.
- No `internal/provider/watcher.go` implementation exists.
- Phase 2 docs previously treated `ResourceWatcher` as a V1 deliverable; that
  expectation has now been corrected to match ADR-0038.

---

## Scope Boundary

This RFC does **not** reopen ADR-0038.

If implemented, the watch path must satisfy all of the following:

- It is an **optional acceleration layer**, not a second authoritative pipeline.
- Polling/reconcile remains the canonical fallback for correctness.
- Watch health must not gate core write-path correctness.
- Loss of watch events must degrade to slower freshness, not incorrect state.

---

## Proposed Solution

### High-Level Model

Use Kubernetes `LIST -> WATCH` per cluster to improve freshness, but keep DB
truth and periodic polling as the final consistency mechanism.

```text
K8s LIST/WATCH
    -> local watch event handler
    -> enqueue immediate sync hint / warm cache
    -> canonical vm_status_sync polling still persists final state
```

### Allowed Responsibilities

The watch layer may:

- enqueue an earlier `vm_status_sync` job for affected VMs
- maintain non-authoritative in-memory/cache snapshots for fast reads
- emit freshness hints for observability or UI acceleration

The watch layer must **not**:

- become the only path that updates VM state in PostgreSQL
- replace periodic reconcile/poll fallback
- introduce a dual-authority state model

### Failure Handling

If introduced later, the watch implementation must handle:

- `410 Gone` / expired `resourceVersion` by re-listing
- reconnect with jitter/backoff
- watch bookmarks when available
- multi-replica ownership / leader-election or equivalent single-writer safety

---

## Why Not Needed Now

For the current governance platform scope:

- transitional VMs already use high-frequency polling
- stable VMs only need drift detection, not realtime control-loop behavior
- River + PostgreSQL gives a simpler and more governable operational model than
  long-lived watcher infrastructure

Therefore this RFC is explicitly **deferred** until the trigger conditions are
met.

---

## Trigger Conditions

Promote this RFC only when one or more of the following becomes true:

- product requires near-realtime external drift visibility (seconds, not minutes)
- operators need large read-heavy dashboards backed by warm local state
- polling latency for stable VMs becomes user-visible and unacceptable
- scale tests show polling alone is no longer the best latency/load trade-off

---

## References

- [ADR-0038: Adaptive K8s Polling](../adr/ADR-0038-adaptive-k8s-polling.md)
- [ADR-0006: Unified Async Model](../adr/ADR-0006-unified-async-model.md)
- [ADR-0008: PostgreSQL Stability](../adr/ADR-0008-postgresql-stability.md)
- [ADR-0033: Realtime Notification Acceleration](../adr/ADR-0033-realtime-notification-acceleration.md)
