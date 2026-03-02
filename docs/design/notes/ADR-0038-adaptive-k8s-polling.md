# Design Note: ADR-0038 — Adaptive K8s VM Status Polling

> **Status**: Pending ADR-0038 acceptance (Review period: Until 2026-03-04)  
> **ADR**: [ADR-0038](../../adr/ADR-0038-adaptive-k8s-polling.md)  
> **Author**: @jindyzhao  
> **Date**: 2026-03-02

This note captures concrete implementation details and impact analysis for ADR-0038.
It will be merged into the relevant design specs after the ADR is accepted.

---

## Impacted Components

### 1. Database Schema (Atlas Migration required)

New fields on the `vms` table:

```sql
-- Atlas migration (auto-generated from Ent schema change)
ALTER TABLE vms
  ADD COLUMN polling_tier       VARCHAR(10)  NOT NULL DEFAULT 'high',
  ADD COLUMN poll_interval_sec  INTEGER      NOT NULL DEFAULT 15,
  ADD COLUMN last_k8s_rv        TEXT         NULL,
  ADD COLUMN last_polled_at     TIMESTAMPTZ  NULL;

CREATE INDEX idx_vms_polling_tier ON vms (polling_tier);
```

**Ent schema addition** (`ent/schema/vm.go`):
```go
field.Enum("polling_tier").
    Values("high", "low").
    Default("high").
    Comment("Polling frequency tier driven by VM lifecycle state"),

field.Int("poll_interval_sec").
    Default(15).
    Comment("Seconds between K8s status polls; 15 for transitional, 1800 for stable"),

field.String("last_k8s_rv").
    Optional().
    Nillable().
    Comment("Last K8s resourceVersion observed; used to route List through watch cache"),

field.Time("last_polled_at").
    Optional().
    Nillable().
    Comment("Timestamp of last K8s status poll"),
```

### 2. River Worker Changes (`internal/worker/vm_polling_worker.go`)

**Current model** (inferred): A fixed-interval cron-like job polls all VMs at the same rate.

**New model**: Self-rescheduling River job per VM with dynamic `ScheduledAt`:

```go
type VMPollingJobArgs struct {
    VMID int `json:"vm_id"`
}

func (VMPollingJobArgs) Kind() string { return "vm_polling" }

type VMPollingWorker struct {
    db         *ent.Client
    k8sAdapter provider.KubeVirtAdapter
    river      *river.Client[pgx.Tx]
}

func (w *VMPollingWorker) Work(ctx context.Context, job *river.Job[VMPollingJobArgs]) error {
    vm, err := w.db.VM.Get(ctx, job.Args.VMID)
    if err != nil {
        return err
    }

    // K8s Get with ResourceVersion for cache routing
    listOpts := metav1.GetOptions{}
    if vm.LastK8sRv != nil {
        listOpts.ResourceVersion = *vm.LastK8sRv
    }

    k8sVM, rv, err := w.k8sAdapter.GetVMWithRV(ctx, vm.ClusterName, vm.Namespace, vm.Name, listOpts)
    if err != nil {
        // Handle 410 Gone: reset ResourceVersion and retry
        if isGoneError(err) {
            return w.resetRVAndRetry(ctx, vm)
        }
        return err
    }

    // Determine new tier based on observed K8s state
    newTier, newIntervalSec := determineTier(k8sVM.Status.Phase, vm, job)

    // Persist updated state
    _, err = w.db.VM.UpdateOne(vm).
        SetPollingTier(newTier).
        SetPollIntervalSec(newIntervalSec).
        SetLastK8sRv(rv).
        SetLastPolledAt(time.Now()).
        Save(ctx)
    if err != nil {
        return err
    }

    // Self-reschedule
    _, err = w.river.Insert(ctx, VMPollingJobArgs{VMID: vm.ID}, &river.InsertOpts{
        ScheduledAt: time.Now().Add(time.Duration(newIntervalSec) * time.Second),
    })
    return err
}

func determineTier(phase string, vm *ent.VM, job *river.Job[VMPollingJobArgs]) (string, int) {
    transitionalStates := map[string]bool{
        "Creating": true, "Deleting": true, "Updating": true, "Migrating": true,
    }

    if !transitionalStates[phase] {
        return "low", 1800 // 30 minutes for stable state
    }

    // Auto-downgrade: stuck in transitional for > 30 minutes
    if time.Since(job.CreatedAt) > 30*time.Minute {
        return "low", 1800
    }

    return "high", 15
}
```

### 3. K8s Provider Interface Change (`internal/provider/`)

New method required on `KubeVirtAdapter`:

```go
// GetVMWithRV returns the VM and its current ResourceVersion.
// The listOpts.ResourceVersion should be populated from DB if available.
GetVMWithRV(ctx context.Context, clusterName, namespace, name string, opts metav1.GetOptions) (*VMStatus, string, error)
```

### 4. Upgrade Trigger from Change Operations

When a user-initiated change (Power On, Power Off, Delete, etc.) is submitted via River:

```go
// In the River Job that initiates the state change (e.g., VMPowerOnJob):
// After submitting the K8s operation, upgrade polling if currently low-frequency.
if vm.PollingTier == "low" {
    _, _ = riverClient.Insert(ctx, VMPollingJobArgs{VMID: vm.ID}, &river.InsertOpts{
        ScheduledAt: time.Now().Add(15 * time.Second), // Immediate high-frequency
    })
    // Cancel or deprioritize existing low-frequency job if possible
}
```

---

## New CI Gate (Post-ADR-0038 Acceptance)

**File**: `docs/design/ci/scripts/check_k8s_polling_rv.go`

```go
// Verifies that all K8s List/Get calls in internal/provider/ 
// that are used for VM status polling pass ResourceVersion from a DB field,
// not a hardcoded empty string (except on explicit baseline reset).
```

---

## Pending Changes Block (to be added to master-flow.md after ADR acceptance)

```markdown
<!-- PENDING: ADR-0038 (under review until 2026-03-04) -->
> ⚠️ **Pending Change**: VM status polling will be refactored to use
> state-machine-driven adaptive intervals after ADR-0038 acceptance.
> See docs/design/notes/ADR-0038-adaptive-k8s-polling.md for details.
```

---

## Open Questions

1. **Deduplication**: If a low-frequency polling job is already scheduled for a VM and a change operation triggers an early high-frequency poll, how do we cancel the scheduled low-frequency job? (River supports cancellation by Job ID — need to store it in DB.)
2. **Cluster-level rate limiting**: RFC-0015 (`per-cluster-concurrency`) defines per-cluster concurrency limits. How does this interact with the polling tier? (Hypothesis: polling jobs count against the per-cluster concurrency budget.)
3. **ResourceVersion 410 Gone handling**: etcd compacts history periodically, causing ResourceVersion to expire. The `isGoneError` handler above resets to baseline. Need to ensure this is observable (Prometheus counter).
