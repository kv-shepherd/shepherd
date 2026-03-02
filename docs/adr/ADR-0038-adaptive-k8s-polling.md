---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-03-02
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0038: Adaptive K8s VM Status Polling with State-Machine-Driven Backoff

> **Review Period**: Until 2026-03-04 (48-hour minimum)<br>
> **Discussion**: [Issue #TBD](https://github.com/kv-shepherd/shepherd/issues/)<br>
> **Related**: `ADR-0006-unified-async-model.md`, `ADR-0008-postgresql-stability.md`, `ADR-0014-capability-detection.md`

---

## Context and Problem Statement

KubeVirt Shepherd manages VM lifecycle by polling K8s API Server to synchronize VM status into its PostgreSQL database. The current polling strategy is **uniform-frequency**: all VMs, regardless of their lifecycle state, are polled at the same fixed interval.

As the platform scales to thousands of VMs, uniform polling creates a linear pressure curve on the K8s API Server. A VM that has been `Running` stably for weeks requires the same polling frequency as one that is actively `Creating`. This is inefficient and limits the platform's horizontal scale ceiling.

Additionally, K8s List requests that do not carry a `resourceVersion` bypass the watch cache and penetrate directly to etcd, multiplying the load impact further.

**The core question**: How should the polling frequency for K8s VM status be governed, such that the system remains responsive to state transitions while minimizing unnecessary API pressure?

## Decision Drivers

* K8s API Server stability must be preserved as VM count grows toward 10,000+ instances.
* VMs in transitional states (Creating, Deleting, Updating) require low-latency status detection to provide good UX.
* VMs in stable states (Running, Stopped, Failed) require only drift detection, not real-time monitoring.
* The solution must not introduce new infrastructure dependencies (no external message broker, no Informer).
* Must align with ADR-0006 (River Queue as the async backbone) and ADR-0008 (PostgreSQL stability constraints).
* Must be implementable as a River Worker strategy change, not a full architectural overhaul.

## Considered Options

* **Option 1**: Uniform fixed-frequency polling (status quo)
* **Option 2**: State-machine-driven adaptive polling with ResourceVersion caching
* **Option 3**: Kubernetes Informer Watch (long-lived streaming connection)

## Decision Outcome

**Chosen option**: **"Option 2: State-machine-driven adaptive polling with ResourceVersion caching"**, because it directly aligns with the VM lifecycle semantics already modeled in the system, delivers meaningful K8s API pressure reduction without new infrastructure, and preserves the existing River Queue + PostgreSQL architecture.

### Normative Decisions

#### 1. Polling frequency tiers (state-machine driven)

| VM Lifecycle State | Tier | Poll Interval | Rationale |
|--------------------|------|---------------|-----------|
| `Creating`, `Deleting`, `Updating` (transitional) | **High-frequency** | ≤ 15 seconds | Must detect terminal state fast for UX responsiveness |
| `Running`, `Stopped`, `Failed` (stable) | **Low-frequency** | ≥ 30 minutes | Drift detection only; state is expected to be stable |
| Transitional, stuck > 30 minutes | **Auto-downgrade** | Downgrade to low-frequency | Prevent zombie high-frequency loops |

> **Enforcement**: The polling tier is determined by the VM's `status` field at the time the polling job is scheduled, not at the time of last observation.

#### 2. ResourceVersion mandatory requirement

All K8s VM List/Get requests issued by the polling subsystem **MUST** carry the `resourceVersion` value from the previous API response.

```go
listOpts := metav1.ListOptions{
    ResourceVersion: vm.LastK8sResourceVersion, // stored in DB
    // This routes the request through the K8s watch cache,
    // NOT through etcd directly.
}
```

- `ResourceVersion` **MUST** be persisted in the VM record after every successful poll.
- On first poll (no prior ResourceVersion), use `resourceVersion: ""` (etcd read) to establish the baseline.
- If a `410 Gone` response is received (ResourceVersion expired), reset to `""` and re-establish baseline.

#### 3. Storage of polling state

Polling tier and ResourceVersion are stored in the VM's own database record as indexed fields:

```
vm.polling_tier       ENUM('high', 'low')   NOT NULL DEFAULT 'high'
vm.poll_interval_sec  INTEGER               NOT NULL DEFAULT 15
vm.last_k8s_rv        TEXT                  NULLABLE  -- K8s resourceVersion
vm.last_polled_at     TIMESTAMPTZ           NULLABLE
```

> Do **NOT** create a separate polling-state table. Colocate with the VM record to avoid join overhead on hot paths.

#### 4. Scheduling via River

Polling is implemented as a **River scheduled job** (`river.JobArgs` with `ScheduledAt`):

```go
// After a successful poll of a transitional-state VM:
river.Insert(ctx, PollingJobArgs{VMID: vm.ID}, &river.InsertOpts{
    ScheduledAt: time.Now().Add(15 * time.Second),
})

// After a successful poll of a stable-state VM:
river.Insert(ctx, PollingJobArgs{VMID: vm.ID}, &river.InsertOpts{
    ScheduledAt: time.Now().Add(30 * time.Minute),
})
```

This leverages River's built-in `scheduled` state and `ScheduledAt` semantics, with no custom timer infrastructure.

#### 5. Tier transition rules

```
Current: high-frequency (transitional state observed)
  → VM moves to stable state (Running/Stopped/Failed)
  → Next scheduled job uses low-frequency interval

Current: high-frequency (transitional state observed)  
  → 30 minutes elapsed without terminal state reached
  → Auto-downgrade: next job uses low-frequency interval
  → Platform marks VM with error hint (potential K8s-side stuck state)

Current: low-frequency
  → Platform receives user request to change VM state
  → Immediately upgrade to high-frequency (next interval ≤ 15s)
  → Upgrade is triggered by the River Job that initiates the state change
```

### Consequences

* ✅ Good, because polling load on K8s API Server grows sub-linearly with VM count (stable VMs contribute minimal load).
* ✅ Good, because ResourceVersion caching eliminates etcd penetration for routine status checks.
* ✅ Good, because no new infrastructure (no Informer, no Kafka, no Redis) is required.
* ✅ Good, because River's `ScheduledAt` semantics are a natural fit, leveraging existing ADR-0006 machinery.
* ✅ Good, because state is colocated in the VM record, preserving single-source-of-truth semantics.
* 🟡 Neutral, because the auto-downgrade rule for stuck transitional VMs may delay detection of certain edge case failures (mitigated by the error hint marking).
* ❌ Bad, because polling latency for stable VMs increases to 30 minutes (acceptable: stable VMs do not need real-time tracking; any platform-initiated change triggers an immediate upgrade).

### Confirmation

* Architecture review confirms polling tier transitions are correctly driven by VM `status` field semantics.
* Code review confirms all K8s List/Get calls in the polling subsystem carry `ResourceVersion`.
* Code review confirms `last_k8s_rv` is persisted after every successful poll (including on 304 Not Modified responses).
* DB schema review confirms `polling_tier`, `poll_interval_sec`, `last_k8s_rv`, `last_polled_at` fields are added via Atlas migration.
* Load test: with 1,000 stable VMs at 30-minute interval → ~0.5 req/s to K8s; with 100 transitional VMs at 15s interval → ~6.7 req/s. Total within acceptable bounds for governance platform scale.
* CI gate: `check_k8s_polling_rv.go` verifies that all K8s List calls in `internal/provider/` pass `ResourceVersion` from DB field (not empty string except on explicit baseline reset).

---

## Pros and Cons of the Options

### Option 1: Uniform fixed-frequency polling (status quo)

All VMs polled at the same interval regardless of state.

* ✅ Good, because implementation is trivially simple.
* ❌ Bad, because polling load is strictly O(n) with VM count, hitting K8s API Server limits at scale.
* ❌ Bad, because stable VMs consume the same polling budget as actively transitioning ones.
* ❌ Bad, because List without ResourceVersion penetrates etcd on every call.

### Option 2: State-machine-driven adaptive polling with ResourceVersion caching

Tier-based intervals driven by VM state machine; ResourceVersion cached per VM.

* ✅ Good, because polling load growth is sub-linear; stable VMs contribute negligible load.
* ✅ Good, because directly maps to existing ADR-0006 River Job scheduling primitives.
* ✅ Good, because ResourceVersion cache reduces etcd load significantly for large-scale deployments.
* ✅ Good, because no new infrastructure required.
* 🟡 Neutral, because requires DB schema migration to add polling state fields (low complexity).
* ❌ Bad, because introduces tier-transition logic that must be carefully tested to avoid stuck-in-high-frequency scenarios.

### Option 3: Kubernetes Informer Watch (streaming)

Use K8s watch API to receive real-time VM state change events, eliminating polling entirely.

* ✅ Good, because provides instant state change notification without polling overhead.
* ❌ Bad, because requires persistent long-lived connections per cluster, violating the project's "no persistent K8s connection" philosophy for governance platforms.
* ❌ Bad, because Informer reconnect logic, resync behavior, and event deduplication add significant complexity.
* ❌ Bad, because Informer watch connections are not easily governed by the existing River + PG stability model (ADR-0006, ADR-0008).
* ❌ Bad, because this is effectively an architectural reversal of the deliberate decision to use polling for decoupling and operational simplicity.

---

## More Information

### Related Decisions

* `ADR-0006-unified-async-model.md` — River Queue scheduling primitives; `ScheduledAt` semantics.
* `ADR-0008-postgresql-stability.md` — Worker concurrency limits (MaxWorkers ≤ 10) and PollInterval baseline.
* `ADR-0014-capability-detection.md` — Established precedent of "not overusing K8s API"; health check minimalism.
* `RFC-0015-per-cluster-concurrency.md` — Per-cluster concurrency limits complement this ADR.

### References

* [Kubernetes API: ResourceVersion semantics](https://kubernetes.io/docs/reference/using-api/api-concepts/#resource-versions)
* [River Queue: Scheduled Jobs](https://riverqueue.com/docs/scheduled-jobs)
* [K8s List with resourceVersion to avoid etcd penetration](https://kubernetes.io/docs/reference/using-api/api-concepts/#the-resourceversion-parameter)

### Implementation Notes

* **Phase 1**: Add DB schema fields (`polling_tier`, `poll_interval_sec`, `last_k8s_rv`, `last_polled_at`) via Atlas migration.
* **Phase 2**: Refactor the existing polling River Worker to read `polling_tier` and schedule next job with computed `ScheduledAt`.
* **Phase 3**: Add CI gate `check_k8s_polling_rv.go` to enforce ResourceVersion requirement.
* **Revisit trigger**: If K8s API Server p99 latency from Shepherd exceeds 50ms under normal load, revisit tier intervals.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-02 | @jindyzhao | Initial draft |
