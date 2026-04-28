# Design Note: ADR-0038 — Adaptive K8s VM Status Polling

> **Status**: Implementation Complete (Phases 1-3). ADR accepted on 2026-03-05.  
> **ADR**: [ADR-0038](../../adr/ADR-0038-adaptive-k8s-polling.md)  
> **Author**: @jindyzhao  
> **Created**: 2026-03-02  
> **Last Updated**: 2026-03-05 (accepted + ResourceVersion 410 handling completed)

This note captures concrete implementation details and impact analysis for ADR-0038.

---

## Implementation Progress

| Phase | Description | Status | Date |
|-------|-------------|--------|------|
| Phase 1 | DB Schema (Ent + Atlas migration) | ✅ Complete | 2026-03-03 |
| Phase 2 | River Worker (VMStatusSyncWorker) | ✅ Complete | 2026-03-03 |
| Phase 3 | CI Gate (k8spollingrv Analyzer) | ✅ Complete | 2026-03-03 |

---

## Phase 1: Database Schema

**Files changed**:
- `ent/schema/vm.go` — 5 new fields + 1 index
- `migrations/atlas/20260303000100_adr0038_vm_polling_fields.sql` — Atlas migration
- `migrations/atlas/20260304000100_adr0038_high_tier_since.sql` — follow-up migration for precise high-tier timing

New fields on the `vms` table:

```sql
ALTER TABLE vms
  ADD COLUMN polling_tier       VARCHAR(10)  NOT NULL DEFAULT 'high',
  ADD COLUMN poll_interval_sec  INTEGER      NOT NULL DEFAULT 15,
  ADD COLUMN last_k8s_rv        TEXT         NULL,
  ADD COLUMN last_polled_at     TIMESTAMPTZ  NULL,
  ADD COLUMN high_tier_since    TIMESTAMPTZ  NULL;

CREATE INDEX idx_vms_polling_tier ON vms (polling_tier);
```

**Ent schema** (`ent/schema/vm.go`):
- `polling_tier` — ENUM("high", "low"), default "high"
- `poll_interval_sec` — INT, default 15
- `last_k8s_rv` — STRING, Optional/Nillable
- `last_polled_at` — TIME, Optional/Nillable
- `high_tier_since` — TIME, Optional/Nillable (high-tier entry timestamp)
- Index: `polling_tier` (bulk tier queries by River Worker)

---

## Phase 2: River Worker

**Files created/changed**:
- `internal/jobs/vm_status_sync.go` — VMStatusSyncWorker
- `internal/jobs/vm_status_sync_test.go` — Tests (tier mapping, interval, status conversion)
- `internal/app/modules/vm.go` — Worker registration
- `internal/domain/vm.go` — Added `ResourceVersion` field to domain.VM
- `internal/provider/mapper.go` — Populates `ResourceVersion` from K8s VM metadata
- `internal/provider/interface.go` — Added `ResourceVersion` to `ListOptions`
- `internal/provider/kubevirt.go` — Passes `ResourceVersion` in `ListVMs`

### Worker Design

**Self-rescheduling River job per VM** with dynamic `ScheduledAt`:

```go
type VMStatusSyncArgs struct {
    EventID string `json:"event_id"`
}

// Job kind: "vm_status_sync"
// Queue: "vm_status_sync"
// MaxAttempts: 3
// UniqueOpts: ByArgs + ByQueue (prevents duplicate scheduling)
```

**Execution flow**:
1. Resolve VM row from DB via EventID (EventID -> ApprovalTicket -> VM)
2. Call K8s ListVMs via VMService (`metadata.name` fieldSelector + ResourceVersion from DB)
3. Map K8s status → DB status; determine new tier
4. Auto-downgrade check: transitional VMs stuck >30min → low tier
5. Persist: status, last_k8s_rv, last_polled_at, polling_tier, poll_interval_sec
6. Schedule next poll: `ScheduledAt = now + poll_interval_sec`

### Tier Constants (ADR-0038 §Polling frequency tiers)

| Tier | Interval | VM States |
|------|----------|-----------|
| high | 15 seconds | CREATING, DELETING, STOPPING, MIGRATING, PENDING |
| low | 1800 seconds (30min) | RUNNING, STOPPED, FAILED, PAUSED, UNKNOWN |

### Auto-downgrade

A VM stuck in high-frequency polling for >30 minutes is automatically downgraded to
low-frequency. This prevents zombie high-frequency loops from consuming K8s API budget.

### Graceful Degradation

- **VM deleted from DB**: poll chain cancelled (`river.JobCancel`)
- **VM has no cluster_id**: skip poll, reschedule at low frequency
- **K8s unreachable**: log warning, retry at same interval (transient)
- **River insert error**: return worker error so River retry path retries scheduling

---

## Phase 3: CI Gate

**Files created/changed**:
- `tools/shepherd-linter/analyzer/k8spollingrv/analyzer.go` — Analyzer
- `tools/shepherd-linter/analyzer/k8spollingrv/analyzer_test.go` — Tests
- `tools/shepherd-linter/analyzer/k8spollingrv/testdata/` — Test fixtures
- `tools/shepherd-linter/plugin.go` — Registered as Batch 3 analyzer

### Detection Rules

The `k8spollingrv` analyzer flags `metav1.ListOptions{}` and `metav1.GetOptions{}` struct
literals that do NOT set the `ResourceVersion` field, but **only** in polling-related files
(filename contains: `status_sync`, `polling`, `poll_`, `_poll`, `sync_status`).

This conservative scope prevents false positives on non-polling code (e.g., API handlers,
idempotency checks) while ensuring the critical polling path always uses cached ResourceVersion.

---

## Open Questions (from ADR-0038 review)

1. **Deduplication**: River UniqueOpts (ByArgs + ByQueue) prevents duplicate scheduling for the
   same VM. No need to store Job ID in DB.

2. **Cluster-level rate limiting**: RFC-0015 per-cluster concurrency applies. Polling jobs use
   the `vm_status_sync` queue, separate from `vm_operations`, so they don't compete with
   user-initiated operations.

3. **ResourceVersion 410 Gone handling**: Implemented in `VMStatusSyncWorker`.
   On `IsResourceExpired/IsGone`, worker clears `last_k8s_rv` and reschedules the next poll
   with baseline `resourceVersion=""` to re-establish cache state.
