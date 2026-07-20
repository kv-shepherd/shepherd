# Phase 4 Checklist: Governance Capabilities

> **Detailed Document**: [phases/04-governance.md](../phases/04-governance.md)
>
> **Implementation Status**: 🔄 Partial (~98%) — Approval flow/ADR-0012 atomic commit/Audit log/Delete handlers/ApprovalValidator/Confirm params/Notification system/Namespace CRUD/Batch Operations (Stage 5.E)/VNC token hardening (AES-256-GCM + shared replay marker)/Catalog Scope/Cluster Policy/VM Status Sync (ADR-0038)/Template+InstanceSize validators/User My Requests API all completed; full resource reconciler, rich revision diff/compression service, and template lifecycle states deferred
>
> **Last Audited**: 2026-05-08 (Session: Phase 4 V1 scope alignment)
>
> **Gate Checklist**: [../ci/GATE_HARDENING_CHECKLIST.md](../ci/GATE_HARDENING_CHECKLIST.md)

---

## Master-Flow Alignment Status (VM Lifecycle, audited 2026-02-10)

> **Source**: [master-flow.md Part 3: VM Lifecycle Flow](../interaction-flows/master-flow.md#part-3-vm-lifecycle-flow)
>
> **Audit scope**: All backend code in `internal/` compared against master-flow.md Stages 5.A–5.F and Stage 6.

| Stage | Description | Alignment | Key Gaps | Priority |
|-------|-------------|-----------|----------|----------|
| 5.A | VM Request Submission | ✅ 90% | Request path creates pending Ticket/DomainEvent records without a VM row; VM `PENDING` remains K8s scheduler wait only | P2-done |
| 5.B | Admin Approval | ✅ 95% | Prod overcommit informational warning already surfaced in approval UI; template lifecycle follow-ups remain deferred | P3-done |
| 5.C | VM Creation Execution | ✅ 97% | Provider-side hard idempotency now guards create-style SSA with a same-name ownership check before apply | P3-done |
| 5.D | Delete Operations | ✅ 96% | VM hard-delete plus periodic `DELETING` tombstone retry cleanup implemented | P2-done |
| 5.E | Batch Operations | ✅ 97% | Canonical API baseline + parent-child linkage + submit throttling (pending parent + global/min + pending child + cooldown) + parent approval dispatch to independent child workers + retry/cancel + parent projection table persisted counters + admin override APIs + `/vms/batch/power` compatibility execution + frontend queue UX (`status_url` polling / `429 Retry-After` countdown / affected-child feedback / `aria-live`) and JSON result export implemented | P2-done |
| 5.F | Notification System | ✅ 95% | V1 inbox notification flow implemented end-to-end (API + triggers + InboxSender + NotificationBell + 90-day retention cleanup) | P3-done |
| 6 | VNC Console Access | ⚠️ 96% | Stage 6 V1 baseline + shared PG replay marker + AES-256-GCM encrypted token envelope implemented; noVNC proxy internals + VNC active revocation remain V2+ scope | V2+ |
| Part 4 | State Machines | ✅ 90% | ~~`FAILED`, `DELETING`, `STOPPING` states~~ added; ~~`PENDING` clarified~~ as K8s-only | P2-done |

### Blocking Issues (must fix before further feature work)

1. **P0 — VM Status Enum Mismatch**: ✅ **RESOLVED** — `domain/vm.go` now uses `FAILED` (not `ERROR`), `STOPPING` and `DELETING` transitional states added. `PENDING` clarified as K8s scheduler wait state (not approval-level).
2. **P2 — ApprovalValidator partial**: ✅ **RESOLVED** — `dedicated_cpu` + overcommit blocking, `spec_overrides`, GPU/Hugepages/SR-IOV capability matching are implemented.
3. **P1 — Delete governance**: ✅ **RESOLVED** — DeleteVM approval ticket flow, state guard, confirm params, OpenAPI endpoints, DeleteService/GetService handlers are all implemented.
4. **P2 — InstanceSize schema partial**: ✅ **RESOLVED** — `requires_gpu`, `requires_sriov`, `requires_hugepages`, `hugepages_size`, `spec_overrides` fields added.
5. **P1 — Ticket.operation_type**: ✅ **RESOLVED** — enum (`CREATE`, `MODIFY`, `DELETE`, `POWER`, `VNC_ACCESS`) with `CREATE` default for backward compatibility.
6. **P1 — ADR-0012 atomic approval transaction**: ✅ **RESOLVED** — `ApprovalAtomicWriter` uses `sqlc + pgx.Tx + river.InsertTx` to eliminate post-commit enqueue gap.

### Previously Fixed (this session chain)

| Fix | Component | Session |
|-----|-----------|---------|
| ✅ VM Status Enum | `domain/vm.go` | Added `STOPPING`, `DELETING`, `FAILED`; clarified `PENDING` |
| ✅ DeleteVM handler | `server_vm.go` | State guard, VMDeletePayload, DomainEvent, River job, audit |
| ✅ Power operation handlers | `server_vm.go` | StartVM, StopVM, RestartVM with shared `enqueueVMPowerOp` |
| ✅ Worker error handling | `vm_create.go`, `vm_delete.go`, `vm_power.go` | Audit logging, FAILED status persistence, critical alert for K8s+DB divergence |
| ✅ ApprovalGateway | `gateway.go` | Approve/Reject/Cancel/ListPending + VM creation in approval |
| ✅ Audit logger | `audit/logger.go` | `LogVMOperation` method added |
| ✅ Delete payload | `domain/event.go` | `VMDeletePayload` and `VMPowerPayload` structs |
| ✅ ADR-0015 §13 addendum | `ADR-0015-governance-model-v2.md` | confirm_name → query param per RFC 9110/OpenAPI 3.0 |
| ✅ OpenAPI DeleteService | `api/openapi.yaml` | Added GetService + DeleteService + confirm params for all deletes |
| ✅ InstanceSize.dedicated_cpu | `ent/schema/instance_size.go` | Added `dedicated_cpu` bool field + ent codegen |
| ✅ ApprovalValidator enhanced | `approval_validator.go` | 3-rule validation: dedicated_cpu+overcommit block, cpu limit, memory limit |
| ✅ DeleteVM confirm params | `server_vm.go` | Tiered confirm gate: confirm=true (test) or confirm_name (prod) |
| ✅ DeleteSystem confirm params | `server_system.go` | confirm_name via generated params struct, not raw c.Query() |
| ✅ GetService handler | `server_system.go` | GET /systems/{system_id}/services/{service_id} |
| ✅ DeleteService handler | `server_system.go` | Cascade check (zero VMs), confirm=true gate, hard delete, audit |
| ✅ Ticket.operation_type | `ent/schema/ticket.go` | Enum field (`CREATE`, `MODIFY`, `DELETE`, `POWER`, `VNC_ACCESS`) with `CREATE` default |
| ✅ ADR-0012 atomic approval | `usecase/approval_atomic.go` + `repository/sqlc/` | Approval writes + River enqueue in single transaction |
| ✅ VM create idempotency guard | `jobs/vm_create.go` | Event label + pre-create lookup + safe retry on DB write failure |
| ✅ ListApprovals DELETE target VM | `server_approval.go` | Batch-fetch DomainEvent payload for DELETE tickets, populate target_vm_id/name |
| ✅ Approvals priority highlighting | `web/src/app/(protected)/admin/approvals/page.tsx` | ADR-0015 §11 visual priority tier (🟡 4-7d, 🔴 7+d) |
| ✅ Service delete frontend | `web/src/app/services/page.tsx` | Popconfirm + DELETE API + confirm=true |
| ✅ i18n: target_vm + delete modal | `web/src/i18n/locales/{en,zh-CN}/approval.json` | target_vm, delete_target_vm keys |
| ✅ NotificationSender interface | `internal/notification/sender.go` | Sender interface + InboxSender (sync DB write) |
| ✅ Notification triggers | `internal/notification/triggers.go` | OnTicketSubmitted/Approved/Rejected + OnVMStatusChanged |
| ✅ Gateway notification integration | `internal/governance/ticketing/service.go` | SetNotifier + trigger calls on approve/reject |
| ✅ Handler notification calls | `internal/api/handlers/server_vm.go` | OnTicketSubmitted on CreateVM/DeleteVM |
| ✅ DI wiring | `internal/app/modules/approval.go` | InboxSender → Triggers → Gateway.SetNotifier |
| ✅ Frontend NotificationBell | `web/src/components/ui/NotificationBell.tsx` | Badge + Popover + mark-read + 30s polling |
| ✅ i18n: notification keys | `web/src/i18n/locales/{en,zh-CN}/common.json` | notification.title/empty/markAllRead/type.* keys |
| ✅ Namespace Admin Page | `web/src/app/(protected)/admin/namespaces/page.tsx` | CRUD + confirm_name delete gate (ADR-0015 §13) |
| ✅ Templates Admin Page | `web/src/app/(protected)/admin/templates/page.tsx` | CRUD list/forms + column filters + useDeferredValue search |
| ✅ InstanceSizes Admin Page | `web/src/app/(protected)/admin/instance-sizes/page.tsx` | CRUD list/forms + capability filters + numeric sorters |
| ✅ i18n: admin page keys | `web/src/i18n/locales/{en,zh-CN}/admin.json` | 44+ keys for namespaces/templates/instanceSizes |
| ✅ Navigation updated | `web/src/components/layouts/AppLayout.tsx` | 3 new admin menu entries with icons |

### Code References (for quick lookup)

| Component | File | Lines | Status |
|-----------|------|-------|--------|
| VM Status Enum | `internal/domain/vm.go` | 35-49 | ✅ Fixed |
| CreateVM UseCase | `internal/usecase/create_vm.go` | 78-145 | ✅ Aligned |
| ApprovalGateway | `internal/governance/approval/provider_router.go`, `internal/governance/ticketing/service.go` | full | ✅ Router seam + ticket execution service with ADR-0012 atomic writer integration |
| ApprovalValidator | `internal/service/approval_validator.go` | 27-220 | ✅ Dedicated CPU + capability matching complete |
| ApprovalAtomicWriter | `internal/usecase/approval_atomic.go` | full | ✅ `sqlc + InsertTx` atomic commit |
| VM Instance Allocation | `internal/repository/sqlc/queries/ticket.sql`, `internal/usecase/approval_atomic.go` | full | ✅ Transaction-local `UPDATE ... RETURNING`; legacy naming helper removed |
| VMCreateWorker | `internal/jobs/vm_create.go` | full | ✅ Retry-safe idempotency guard added |
| VMDeleteWorker | `internal/jobs/vm_delete.go` | full | ✅ Aligned |
| VMPowerWorker | `internal/jobs/vm_power.go` | full | ✅ START/STOP retry-safe; RESTART uses a durable at-most-once dispatch fence |
| DeleteVM Handler | `internal/api/handlers/server_vm.go` | 133-232 | ✅ Fixed — tiered confirm gate |
| DeleteSystem Handler | `internal/api/handlers/server_system.go` | 165-223 | ✅ Fixed — confirm_name via generated params |
| GetService Handler | `internal/api/handlers/server_system.go` | 336-365 | ✅ New |
| DeleteService Handler | `internal/api/handlers/server_system.go` | 367-446 | ✅ New — cascade + confirm + audit |
| Ticket Schema | `ent/schema/ticket.go` | full | ✅ Fixed — complete operation_type enum added |
| AuditLogger | `internal/governance/audit/logger.go` | full | ✅ Aligned |
| ListApprovals DELETE enrichment | `internal/api/handlers/server_approval.go` | 17-100 | ✅ Batch-fetch DomainEvent for DELETE ticket target VM |
| Approvals Frontend + Priority | `web/src/app/(protected)/admin/approvals/page.tsx` | full | ✅ target_vm column + ADR-0015 §11 priority highlighting |
| Service Delete Frontend | `web/src/app/services/page.tsx` | full | ✅ Popconfirm + DELETE with confirm=true |
| NotificationSender | `internal/notification/sender.go` | full | ✅ Sender interface + InboxSender |
| NotificationTriggers | `internal/notification/triggers.go` | full | ✅ 4 event triggers + approver lookup |
| NotificationBell (Frontend) | `web/src/components/ui/NotificationBell.tsx` | full | ✅ Badge + Popover + mark-read |
| Namespace Admin Page | `web/src/app/(protected)/admin/namespaces/page.tsx` | full | ✅ CRUD + confirm_name delete |
| Templates Admin Page | `web/src/app/(protected)/admin/templates/page.tsx` | full | ✅ Column filters + deferred search |
| InstanceSizes Admin Page | `web/src/app/(protected)/admin/instance-sizes/page.tsx` | full | ✅ Capability filters + sort |

RESTART is intentionally fail-closed because the provider contract has no
idempotency key or durable dispatch receipt. Only the worker that atomically
claims `PENDING` → `PROCESSING` may call the provider. A redelivery that sees a
`PROCESSING` restart preserves the Event/Ticket state and cancels without a
second provider call; direct, approval, batch, and retry submissions treat that
state as a hard per-VM fence even if the River job or Ticket is absent or
terminal. An orphaned fence therefore requires explicit operator verification
and reconciliation. START and STOP retain automatic retry behavior because
their already-running/already-stopped outcomes are idempotently repairable.

---

## Database Migration

- [x] Database migration tool configured (Atlas) — *Phase 5: `migrations/atlas/atlas.hcl`*
- [x] `atlas.hcl` configuration complete — *Phase 5: ent://ent/schema → PostgreSQL 18*
- [x] Ent schema coverage exists for `vms`, `vm_revisions`, `audit_logs`, `tickets`, and `approval_policies`
- [ ] Live migration apply/rollback verification (environment verification)

---

## Environment Isolation (ADR-0015 §1, §15)

- [x] **Schema Fields**:
  - [x] `Cluster.environment` - Cluster environment type (test/prod)
  - [x] `ent/schema/namespace_registry.go` - Namespace registry with explicit environment
    - [x] Contains `name` field
    - [x] Contains `environment` field (test/prod) - **explicitly set by admin**
    - [x] Does NOT contain `cluster_id` field (ADR-0017)
  - [x] ❌ **No `System.environment`** - System is decoupled from environment (ADR-0015 §1)
- [x] **Platform RBAC**:
  - [x] `RoleBinding.allowed_environments` field
  - [x] Environment-based query filtering (`ListNamespaces`, `ListVMs`)
- [x] **Visibility Filtering** - users see only namespaces matching their allowed_environments (includes VM read/request path guard)
- [x] **Scheduling Constraints** - namespace environment must match cluster environment (`ApprovalValidator` + `VMCreateWorker` runtime guard)

---

## RevisionService

- [x] `ent/schema/vm_revision.go` persistence model exists
- [ ] Version number auto-increment (RFC-backed future scope; not V1 governance baseline)
- [ ] Supports diff calculation (RFC-backed future scope; not V1 governance baseline)
- [ ] YAML compressed storage (RFC-backed future scope; not V1 governance baseline)

> Rich revision diff/compression service behavior remains future scope; VM
> lifecycle auditability does not depend on this service for the V1 governance
> baseline.

---

## TemplateService

- [x] `ent/schema/template.go` Schema definition complete
- [x] **TemplateService Implementation** (`internal/service/template_service.go`):
  - [x] `GetActiveTemplate(name)` implemented
  - [x] `GetLatestTemplate(name)` implemented
  - [x] `CreateTemplate(...)` implemented with boot-source fields
  - [x] `ListTemplates()` implemented
  - [x] Source-type normalization and boot transport validation implemented
  - [x] Template/InstanceSize boundary enforcement represented by the Template schema and validator helper (`template_validator.go`)
  - [ ] Export/import automation (deferred)
  - [ ] **Lifecycle Management** (Publish, Deprecate, Archive) (deferred)
  - [ ] Template-save live K8s dry-run (deferred; approval-time dry-run is the V1 gate)

---

## River Queue Task System (ADR-0006)

- [ ] River database migration complete (deferred — requires running DB)
- [x] River Client initialization configured — *Phase 5: `database.go` InitRiverClient + bootstrap wiring*
- [x] Job type definitions in `internal/jobs/` (VMCreateArgs, VMDeleteArgs, VMPowerArgs)
- [x] Worker registration mechanism (VMCreateWorker, VMPowerWorker)
- [x] **Handler Unified 202 Return** implemented (VMHandler returns 202)
- [ ] **Task Query API** implemented (deferred)
- [x] River retry mechanism configured (MaxAttempts: 3)
- [ ] River dead letter queue handling (deferred)
- [ ] **PostgreSQL Stability Measures** (ADR-0008) applied (deferred)

---

## Domain Event Pattern (ADR-0009)

- [x] **DomainEvent Schema** complete (Ent schema + domain model)
- [x] **Key Constraint 1: Payload Immutability** enforced (immutable Ent fields)
- [x] **Key Constraint 2: Atomic Transaction Pattern (ADR-0012)** implemented (CreateVMUseCase)
- [x] **Key Constraint 3: Worker Fault Tolerance** implemented (retry-safe status handling + `JobCancel` for non-retryable payload errors)
- [x] **EventDispatcher** implemented (`internal/domain/dispatcher.go`)
- [x] **V1 event integration path** implemented (approval services, River workers, notification triggers); generic dispatcher handler registration is not the current execution path
- [x] **Idempotency Guarantee** implemented (VM create event-label guard + unique River enqueue by args/queue)
- [x] **Soft Archiving** configured (`internal/jobs/event_archive.go` + periodic bootstrap registration; marks 30d-old terminal events with `archived_at`)

---

## Reconciler

- [x] **V1 status convergence baseline** implemented via ADR-0038 adaptive polling (`internal/jobs/vm_status_sync.go`)
  - [x] Polls managed VMs with ResourceVersion caching
  - [x] Persists status, polling tier, last poll time, and ResourceVersion
  - [x] Handles expired ResourceVersion by clearing the cache and rescheduling
  - [x] Stops the poll chain for deleted DB bindings and `DELETING` tombstones
- [x] **Resource adoption compensation workflow** exists (`pending_adoptions` schema + discovery job + platform-admin API/UI adoption workflow)
- [ ] Full ghost/orphan reconciler with dry-run/mark/delete modes (deferred to [DEFERRED_FOLLOWUPS.md](../DEFERRED_FOLLOWUPS.md))
- [ ] Full reconciler circuit-breaker UX and reports (deferred)

---

## Template Engine (ADR-0007, ADR-0011, ADR-0018)

> **Updated per ADR-0018/ADR-0046**: Templates define boot source and
> cloud-init only. SchemaMask owns UI field visibility. Go Template variables
> and hardware capability requirements are not part of Template persistence.

- [x] **Template Scope** (after ADR-0018):
  - [x] OS image source (`containerdisk`, `cdi_image_import`, `cdi_pvc_clone`)
  - [x] Cloud-init YAML
  - [x] SchemaMask field visibility (`quick_fields`, `advanced_fields`, `professional_fields`) is exposed outside Template persistence
  - [x] ❌ No Go Template variables (removed per ADR-0018)
  - [x] ❌ No RequiredFeatures/Hardware (moved to InstanceSize per ADR-0018)
- [x] **Template Save Validation V1 baseline** (source-type/dependent-field validation + Template schema boundary enforcement)
- [x] **SSA Resource Submission (ADR-0011)** implemented via dynamic-client SSA
- [ ] **Template Lifecycle Management** complete (draft → active → deprecated → archived) (deferred)
- [ ] Template-save live K8s dry-run (deferred; approval-time dry-run remains the V1 gate)

---

## Approval Flow (Governance Core)

- [x] **Directory Structure** created (`internal/governance/approval/`)
- [x] **ApprovalGateway** implemented (`gateway.go` — Approve, Reject, Cancel, ListPending)
- [x] **Admin Parameter Modification** supported (selected_cluster_id, selected_storage_class)
- [x] **Approval Snapshot Persistence** complete (`template_snapshot`, `instance_size_snapshot`, `modified_spec` persisted in atomic approval write)
- [ ] **Full Replacement Safety Protection** implemented (deferred)
- [x] **Request Type Enum** defined (domain event types)
- [x] **State Flow** implemented (PENDING → APPROVED/REJECTED/CANCELLED)
- [x] **Post-Execution Ticket Status**: Worker updates ticket `APPROVED → EXECUTING → SUCCESS/FAILED`
- [x] **VM Create Status Progression**: `vm_create` worker persists `CREATING -> RUNNING|FAILED` on execution result
- [x] **Duplicate Pending Guard Scope**: same-resource + same-operation check with `existing_ticket_id` in error params
- [x] **User View - My Requests** API implemented via `GET /api/v1/tickets?mine=true`
  - [x] Requires authentication but not `ticket:view`
  - [x] Filters server-side by `ticket.requester == actor`
  - [x] Invalid ticket list enum filters return `400 INVALID_REQUEST`
- [x] **Admin View - Approval Workbench** API (ListPending sorted oldest first)
  - [x] Default sort by `days_pending` (oldest first within priority tier)
  - [x] `priority_tier` field in response (normal/warning/urgent) — `PriorityTier()` function
  - [x] Color coding: 0-3d normal, 4-7d yellow, 7+d red (ADR-0015 §11)
- [x] **User Self-Cancellation (HTTP API)** implemented
  - OpenAPI route + handler + `Gateway.Cancel` error mapping are now wired
- [x] `POST /api/v1/tickets/{id}/cancel` implemented and contract-defined
- [x] **AuditLogger** implemented (`internal/governance/audit/logger.go`)
- [x] **Approval/Ticket API** endpoints complete (list/approve/reject + ticket cancel)
- [x] Policy matching logic implemented (`ApprovalRequirementService`: operation + environment + priority matching with default matrix fallback)
- [ ] **Extensible Approval Handler Architecture** designed (deferred)
- [x] **Notification Service (Reserved Interface)** defined (`internal/provider/notificationcontract/contract.go`, thin re-export in `internal/provider/notification.go`)
- [x] **Notification Integration** implemented — Gateway calls `OnTicketApproved`/`OnTicketRejected`, handlers call `OnTicketSubmitted`
- [x] **External State Management** (no pre-approval job insertion — River jobs only after approval)

### ⚠️ Approval Validation Gaps (master-flow Stage 5.B)

- [x] **InstanceSize schema enhancement**: `dedicated_cpu`, `requires_gpu`, `requires_sriov`, `requires_hugepages`, `hugepages_size`, `spec_overrides` added
- [x] **Resource Capability Matching**: Requirements are extracted from InstanceSize flags/spec_overrides and matched to cluster capabilities
- [x] **Dedicated CPU + Overcommit Mutual Exclusion**: `dedicatedCpuPlacement` enforces blocking error when `cpu_request != cpu_limit`
- [x] **Prod Overcommit Warning**: `request ≠ limit` in prod environment → yellow informational warning

---

## External Approval Provider Boundary

- [x] `ApprovalProvider` contract defined (`SubmitForApproval`, `ProcessApproval`) — in `internal/provider/approvalcontract/contract.go` (thin re-export in `internal/provider/approval.go`)
- [x] `external_approval_systems` schema + migration present for adapter registry
- [x] V1 runtime keeps built-in approval fallback while supporting outbound webhook dispatch
- [x] Admin API/UI manages external approval webhook registry entries
- [x] Signed external approval callback maps provider decisions to the canonical approval path

---

## Delete Confirmation Mechanism (ADR-0015 §13.1)

### ✅ OpenAPI Contract Gaps — RESOLVED

- [x] **DeleteVM**: OpenAPI spec has `confirm` + `confirm_name` query params ✅
- [x] **DeleteService**: OpenAPI spec has `DELETE /systems/{system_id}/services/{service_id}` + `confirm` ✅
- [x] **DeleteSystem**: OpenAPI spec has `confirm_name` query param ✅

### Implementation Status

- [x] **DeleteVM handler** — state guard + DomainEvent + River job + audit + **tiered confirm gate**
- [x] **DeleteVM confirm mechanism** — accepts `confirm=true` (test env) or `confirm_name` matching VM name (prod env)
- [x] **DeleteSystem handler** — cascade check (child Service count) + confirm_name via generated params + hard delete + audit
- [x] **DeleteService handler** — cascade check (child VM count == 0) + confirm=true gate + hard delete + audit
- [x] **GetService handler** — verifies service belongs to system + returns service detail

### Delete Flow Gaps (master-flow Stage 5.D)

- [x] **VM Delete Approval**: VM deletion creates approval ticket (`operation_type=DELETE`) per entity rule matrix
- [x] **Ticket.operation_type**: ✅ Enum field added (`CREATE`, `MODIFY`, `DELETE`, `POWER`, `VNC_ACCESS`) with `CREATE` default
- [x] **DELETING Transient State**: ✅ VM status `DELETING` added; worker correctly sets it before K8s cleanup

---

## VNC Console Permissions (ADR-0015 §18, §18.1 Addendum)

- [x] **Environment-Based Access**:
  - [x] test environment - no approval required
  - [x] prod environment - requires approval ticket
- [x] **VNC Token Security**:
  - [x] Single-use token
  - [x] Time-bounded (max 2 hours)
  - [x] User-bound (`sub` binds token to requester user ID)
  - [x] AES-256-GCM encryption (encrypted JWT envelope, `ENCRYPTION_KEY`)
- [x] Shared replay marker store (`jti` + `used_at`) works across replicas (no Redis dependency)
- [x] V1 has **no active token revocation API** (documented limitation, see ADR-0015 §18.1 addendum); revocation capability is tracked as V2+ enhancement
- [x] **VNC Session Audit** logging

---

## Batch Operations (ADR-0015 §19)

> **Design**: [04-governance.md §5.6](../phases/04-governance.md#56-batch-operations-adr-0015-19)

- [x] **Parent-Child Ticket Schema**
  - [x] `batch_tickets` parent API-projection table implemented
  - [x] `tickets.parent_ticket_id` child linkage implemented
  - [x] Parent aggregate counters (`success/failed/pending`) are persisted
- [x] **Atomic Submission + Independent Execution**
  - [x] Parent + child ticket creation is atomic in one DB transaction
  - [x] Child jobs execute independently via River (parent approval dispatches child create/modify/delete jobs)
  - [x] Parent status aggregation supports persisted `PARTIAL_SUCCESS` projection state
- [x] **Two-Layer Rate Limiting**
  - [x] Global pending parent limit
  - [x] Global API request rate
  - [x] User pending parent limit
  - [x] User pending child count + cooldown
  - [x] Admin exemption and override APIs implemented (`POST/DELETE /admin/rate-limits/exemptions`, `PUT /admin/rate-limits/users/{user_id}`, `GET /admin/rate-limits/status`)
- [x] **Batch APIs**
  - [x] `POST /api/v1/vms/batch` submit
  - [x] `POST /api/v1/vms/batch/power` compatibility submit
  - [x] `GET /api/v1/vms/batch/{id}` status query
  - [x] `POST /api/v1/vms/batch/{id}/retry` retry failed children
  - [x] `POST /api/v1/vms/batch/{id}/cancel` terminate pending children
  - [x] Batch submit/query/retry/cancel and `/vms/batch/power` are normalized into the same parent-child + execution pipeline
- [x] **Ambiguous Restart Fence**
  - [x] Power conflicts expose `operator_action_required` and a read-only operator runbook identifier
  - [x] No API may clear or redispatch an ambiguous restart fence
  - [x] River terminal state is not treated as proof that the provider request cannot complete late
  - [ ] Safe release requires provider receipt/idempotency or provable cancellation
- [x] **Frontend Batch Queue UX**
  - [x] Parent row + child detail panel implemented
  - [x] Status polling uses backend `status_url` until terminal state
  - [x] Retry/terminate actions show affected child items explicitly
  - [x] `429` with `Retry-After` is handled with countdown and disabled actions
  - [x] Accessibility: live status updates announced (`aria-live`)

---

## Notification System (ADR-0015 §20)

> **Design**: [04-governance.md §6.3](../phases/04-governance.md#63-notification-system-adr-0015-20)
> **Example**: [examples/notification/sender.go](../examples/notification/sender.go)

- [x] `ent/schema/notification.go` - Internal inbox
- [x] **NotificationSender Interface** (`internal/notification/sender.go`) — decoupled `Sender` interface
- [x] **V1 Implementation**: `InboxSender` (database-backed, synchronous write per ADR-0015 §20)
- [x] **API Endpoints** (`internal/api/handlers/server_notification.go`):
  - [x] `GET /api/v1/notifications` - List user's notifications (paginated, unread_only filter)
  - [x] `GET /api/v1/notifications/unread-count` - Unread count for badge
  - [x] `PATCH /api/v1/notifications/{notification_id}/read` - Mark as read
  - [x] `POST /api/v1/notifications/mark-all-read` - Mark all as read
- [x] **Notification Triggers** (`internal/notification/triggers.go`):
  - [x] `APPROVAL_PENDING` → approvers (users with `builtin_approval:approve` permission)
  - [x] `APPROVAL_COMPLETED`/`APPROVAL_REJECTED` → requester
  - [x] `VM_STATUS_CHANGE` → VM owner
- [x] **Integration Points**:
  - [x] `ApprovalGateway.SetNotifier()` — triggers on approve/reject
  - [x] `CreateVMRequest` / `DeleteVM` handlers — trigger `OnTicketSubmitted`
  - [x] DI wiring in `ApprovalModule` (InboxSender → Triggers → Gateway)
- [x] **Frontend NotificationBell** (`web/src/components/ui/NotificationBell.tsx`):
  - [x] Badge with unread count (30s polling)
  - [x] Popover dropdown with notification list
  - [x] Type-colored icons and tags (APPROVAL_PENDING/COMPLETED/REJECTED, VM_STATUS_CHANGE)
  - [x] Click to mark-read + navigate to resource
  - [x] Mark-all-read action
  - [x] Integrated into `AppLayout.tsx` header via `actionsRender`
- [x] **i18n**: notification keys in en + zh-CN (`common.json`)
- [x] **Retention cleanup** (90 days, via River periodic job) — *Implemented via `internal/jobs/notification_cleanup.go`, worker registration, and periodic schedule in bootstrap*
