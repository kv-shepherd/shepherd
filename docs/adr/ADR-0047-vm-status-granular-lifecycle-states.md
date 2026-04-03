---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-03-18
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0047: Granular VM Lifecycle States (STARTING / NOT_FOUND)

> **Accepted**: 2026-03-20 (48-hour review period completed)<br>
> **Discussion**: [Issue #389](https://github.com/kv-shepherd/shepherd/issues/389)<br>
> **Amends**: `ADR-0038-adaptive-k8s-polling.md#§1-polling-frequency-tiers` *(adds two new states to tier classification)*
>
> 📝 **Design Note**: The accepted decision is implemented in `vm_status_sync.go`, the VM status contract, and related frontend status rendering.

---

## Context and Problem Statement

The existing VM status enum (`CREATING`, `RUNNING`, `STOPPING`, `STOPPED`, `FAILED`, `DELETING`, `PENDING`, `MIGRATING`, `PAUSED`, `UNKNOWN`) conflates two semantically distinct situations into a single status:

1. **`CREATING` vs `STARTING`**: A VM being created for the first time from a template/request looks identical to an existing VM being powered on from a stopped state. These have different operational semantics — `CREATING` may need bootstrap hold protection (ADR-0038 §2-minute grace window), while `STARTING` is a routine power-on operation from a known-good baseline.

2. **`UNKNOWN` vs `NOT_FOUND`**: A VM whose status cannot be determined because the K8s API server is unreachable (`UNKNOWN`) looks identical to a VM whose K8s resource genuinely no longer exists on a responsive cluster (`NOT_FOUND`). These require fundamentally different operator actions — `UNKNOWN` requires investigation, while `NOT_FOUND` should allow direct database cleanup (deletion).

Without this distinction, the platform cannot provide accurate guidance to users about what happened to their VMs, and operators cannot safely clean up orphaned database records.

## Decision Drivers

* KubeVirt's `PrintableStatus` already provides a `VirtualMachineStatusStarting` value that is distinct from `Creating`.
* Users must be able to distinguish between "cluster unreachable" and "VM resource deleted out-of-band" to take appropriate action.
* The delete workflow (ADR-0015 §13) must guard against deleting VMs that may still be running — `NOT_FOUND` provides an explicit safe-to-delete signal.
* ADR-0038's adaptive polling requires accurate tier classification for new states.
* Frontend must render accurate, actionable status badges instead of ambiguous `UNKNOWN`.

## Considered Options

* **Option 1**: Keep existing enum, use error metadata to distinguish sub-states
* **Option 2**: Add `STARTING` and `NOT_FOUND` as first-class enum values

## Decision Outcome

**Chosen option**: **"Option 2: Add `STARTING` and `NOT_FOUND` as first-class enum values"**, because status enum values are the single source of truth consumed by API, frontend, workers, and polling tiers — metadata-based sub-states would require every consumer to implement custom parsing logic, violating the state-machine-driven architecture established in ADR-0038.

### Normative Decisions

#### 1. New status definitions

| Status | Category | Semantic | Source Signal |
|--------|----------|----------|---------------|
| `STARTING` | Transitional | Existing VM is booting from stopped/paused state (not first-time creation) | KubeVirt `PrintableStatus == "Starting"` |
| `NOT_FOUND` | Terminal-stable | K8s API server is responsive, but the VM resource does not exist in the target namespace | K8s GET returns `404 NotFound` on a responding cluster |

#### 2. State machine integration

```
                                    ┌──────────┐
     ┌─────────────┐                │ STOPPED  │
     │  CREATING   │───▶ RUNNING ◀──│          │
     │ (first time)│                └────┬─────┘
     └─────────────┘                     │
                                         ▼
                                  ┌──────────────┐
                                  │  STARTING    │───▶ RUNNING
                                  │(power-on boot)│
                                  └──────────────┘

     K8s resource missing (cluster responsive):

     Any state ──── K8s 404 ────▶ NOT_FOUND
                                      │
                                      ▼
                               (allows deletion)
```

Transitions:
- `STOPPED` → `STARTING` → `RUNNING`: VM power-on lifecycle
- `CREATING` → `RUNNING`: First-time VM creation lifecycle (unchanged)
- Any state → `NOT_FOUND`: When polling detects K8s resource absence on a responsive cluster
- `NOT_FOUND` → (DB row deleted): Delete worker hard-deletes without K8s API call

#### 3. ADR-0038 polling tier classification (Amendment)

| Status | Tier | Rationale |
|--------|------|-----------|
| `STARTING` | **High-frequency** (≤ 15s) | Transitional state requiring fast terminal-state detection |
| `NOT_FOUND` | **Low-frequency** (≥ 30min) | Terminal-stable; resource is absent; drift detection only |

#### 4. Delete workflow guard integration

VMs in `NOT_FOUND` state are explicitly allowed for deletion:

```go
// vmDeleteExecutableStatus returns true for states that allow deletion execution
func vmDeleteExecutableStatus(status vm.Status) bool {
    switch status {
    case vm.StatusSTOPPED, vm.StatusFAILED, vm.StatusNOT_FOUND, vm.StatusUNKNOWN:
        return true
    default:
        return false
    }
}
```

When deleting a `NOT_FOUND` VM, the delete worker skips the K8s deletion step (resource already absent) and proceeds directly to database hard-delete.

#### 5. Provider ACL mapping

The `KubeVirtMapper` translates KubeVirt signals to domain status:

| KubeVirt Signal | Domain Status |
|-----------------|---------------|
| `PrintableStatus == "Starting"` | `STARTING` |
| K8s GET → `404 NotFound` (cluster healthy) | `NOT_FOUND` |
| K8s GET → network error / timeout | `UNKNOWN` |

#### 6. Bootstrap hold protection

The 2-minute bootstrap grace window (ADR-0038) applies to `CREATING` **and** `STARTING` states, preventing spurious status downgrades during the initial boot phase:

```go
func shouldHoldCreateBootstrapStatus(vmRow *ent.Vm) bool {
    if vmRow.Status != vm.StatusCREATING &&
       vmRow.Status != vm.StatusSTARTING &&
       vmRow.Status != vm.StatusRUNNING {
        return false
    }
    return time.Since(vmRow.CreatedAt) < 2*time.Minute
}
```

### Consequences

* ✅ Good, because users can now distinguish between "VM is starting up" and "VM is being created for the first time", enabling accurate UX messaging.
* ✅ Good, because operators can safely delete `NOT_FOUND` VMs without risk of deleting a VM that is actually running on an unreachable cluster.
* ✅ Good, because ADR-0038 polling tiers correctly classify both new states without requiring workarounds.
* ✅ Good, because KubeVirt's existing `PrintableStatus` semantics are preserved without lossy mapping.
* 🟡 Neutral, because this adds two new enum values requiring a DB migration (additive-only, backward compatible).
* ❌ Bad, because all API consumers (frontend, CLI, external integrations) must handle two additional status values (mitigated: defaults to existing badge rendering for unknown values).

### Confirmation

* Code review confirms `STARTING` maps to `vm.PollingTierHigh` in `tierForStatus()`.
* Code review confirms `NOT_FOUND` maps to `vm.PollingTierLow` in `tierForStatus()`.
* Unit tests verify `mapDomainStatusToEntVM()` correctly translates `VMStatusStarting` ↔ `StatusSTARTING`.
* Unit tests verify `tierForStatus()` returns `High` for `STARTING` and `Low` for `NOT_FOUND`.
* Delete worker test confirms `NOT_FOUND` VMs are accepted for deletion execution.
* Provider mapper test confirms `PrintableStatus("Starting")` → `domain.VMStatusStarting`.
* Atlas migration `20260317_add_vm_starting_status.sql` adds both enum values.

---

## Pros and Cons of the Options

### Option 1: Keep existing enum, use error metadata to distinguish sub-states

Store sub-state information (e.g., `{"sub_status": "starting"}`) in a separate metadata/JSON field.

* ✅ Good, because no schema migration required.
* ❌ Bad, because every consumer (API, frontend, polling tier logic, delete guard) must parse metadata to determine true state.
* ❌ Bad, because ADR-0038 tier classification operates on the `status` enum directly; metadata-based sub-states break this contract.
* ❌ Bad, because "check the metadata" is fragile and easily skipped in new code paths.

### Option 2: Add STARTING and NOT_FOUND as first-class enum values

Add two new values to the VM `status` enum at domain, Ent schema, and database levels.

* ✅ Good, because status is the single source of truth — no secondary lookups needed.
* ✅ Good, because all existing status-driven logic (polling tiers, delete guards, frontend badges) works with standard switch/case patterns.
* ✅ Good, because KubeVirt already provides the underlying `Starting` status, making the mapping natural.
* 🟡 Neutral, because additive enum migration is low-risk and backward compatible.

---

## More Information

### Related Decisions

* `ADR-0038-adaptive-k8s-polling.md` — Defines polling tier classification; this ADR adds `STARTING` to high-tier and `NOT_FOUND` to low-tier.
* `ADR-0015-governance-model-v2.md` — §13 Deletion Cascade Constraints; this ADR clarifies `NOT_FOUND` as a deletable state.
* `ADR-0006-unified-async-model.md` — River workers consume status enum for scheduling decisions.

### References

* [KubeVirt VirtualMachine PrintableStatus](https://github.com/kubevirt/kubevirt/blob/main/staging/src/kubevirt.io/api/core/v1/types.go) — `VirtualMachineStatusStarting` constant
* [Kubernetes API: 404 NotFound semantics](https://kubernetes.io/docs/reference/using-api/api-concepts/#404-not-found)

### Implementation Notes

* **Migration**: `migrations/atlas/20260317_add_vm_starting_status.sql` documents the additive enum change.
* **Revisit trigger**: If KubeVirt introduces additional `PrintableStatus` values (e.g., `Snapshotting`, `Restoring`), evaluate whether to add corresponding domain states.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-18 | @jindyzhao | Initial draft — published for 48-hour public review |
| 2026-03-20 | @jindyzhao | Marked accepted after the review window closed; implementation subsequently landed in VM status sync and API/UI contracts |
