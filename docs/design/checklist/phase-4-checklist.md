# Phase 4 Checklist: Governance Capabilities

> **Detailed Document**: [phases/04-governance.md](../phases/04-governance.md)
>
> **Implementation Status**: 🔄 Partial (~96%) — Approval flow/ADR-0012 atomic commit/Audit log/Delete handlers/ApprovalValidator/Confirm params/Notification system/Namespace CRUD/Batch Operations (Stage 5.E)/VNC token hardening (AES-256-GCM + shared replay marker)/Catalog Scope (ADR-0040)/Cluster Policy/VM Status Sync (ADR-0038)/Template+InstanceSize validators all completed; Reconciler + Template lifecycle management deferred
>
> **Last Audited**: 2026-03-10T21:38 (Session: VNC token hardening + audit backfill)
>
> **Gate Checklist**: [../ci/GATE_HARDENING_CHECKLIST.md](../ci/GATE_HARDENING_CHECKLIST.md)

---

## Master-Flow Alignment Status (VM Lifecycle, audited 2026-02-10)

> **Source**: [master-flow.md Part 3: VM Lifecycle Flow](../interaction-flows/master-flow.md#part-3-vm-lifecycle-flow)
>
> **Audit scope**: All backend code in `internal/` compared against master-flow.md Stages 5.A–5.F and Stage 6.

| Stage | Description | Alignment | Key Gaps | Priority |
|-------|-------------|-----------|----------|----------|
| 5.A | VM Request Submission | ✅ 90% | Domain `PENDING` status is redundant (VM row not created until approval) | P2 |
| 5.B | Admin Approval | ✅ 95% | Prod overcommit informational warning already surfaced in approval UI; template lifecycle follow-ups remain deferred | P3 |
| 5.C | VM Creation Execution | ✅ 95% | Provider-side hard idempotency (AlreadyExists/object ownership check) can be further strengthened | P3 |
| 5.D | Delete Operations | ✅ 90% | VM tombstone cleanup policy after successful K8s deletion still pending | P2 |
| 5.E | Batch Operations | ✅ 96% | Canonical API baseline + parent-child linkage + submit throttling (pending parent + global/min + pending child + cooldown) + parent approval dispatch to independent child workers + retry/cancel + parent projection table persisted counters + admin override APIs + `/vms/batch/power` compatibility execution + frontend queue UX (`status_url` polling / `429 Retry-After` countdown / affected-child feedback / `aria-live`) implemented; export-result UX pending | P2 |
| 5.F | Notification System | ✅ 95% | V1 inbox notification flow implemented end-to-end (API + triggers + InboxSender + NotificationBell + 90-day retention cleanup) | P3 |
| 6 | VNC Console Access | ⚠️ 96% | Stage 6 baseline + shared PG replay marker + AES-256-GCM encrypted token envelope implemented; proxy internals + active revocation remain deferred | P2 |
| Part 4 | State Machines | ✅ 90% | ~~`FAILED`, `DELETING`, `STOPPING` states~~ added; ~~`PENDING` clarified~~ as K8s-only | P2-done |

### Blocking Issues (must fix before further feature work)

1. **P0 — VM Status Enum Mismatch**: ✅ **RESOLVED** — `domain/vm.go` now uses `FAILED` (not `ERROR`), `STOPPING` and `DELETING` transitional states added. `PENDING` clarified as K8s scheduler wait state (not approval-level).
2. **P2 — ApprovalValidator partial**: ✅ **RESOLVED** — `dedicated_cpu` + overcommit blocking, `spec_overrides`, GPU/Hugepages/SR-IOV capability matching are implemented.
3. **P1 — Delete governance**: ✅ **RESOLVED** — DeleteVM approval ticket flow, state guard, confirm params, OpenAPI endpoints, DeleteService/GetService handlers are all implemented.
4. **P2 — InstanceSize schema partial**: ✅ **RESOLVED** — `requires_gpu`, `requires_sriov`, `requires_hugepages`, `hugepages_size`, `spec_overrides` fields added.
5. **P1 — ApprovalTicket.operation_type**: ✅ **RESOLVED** — enum (`CREATE`, `DELETE`) with `CREATE` default for backward compatibility.
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
| ✅ ApprovalTicket.operation_type | `ent/schema/approval_ticket.go` | Enum field (CREATE/DELETE) with CREATE default |
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
| VM Naming | `internal/service/vm_naming.go` | 29-50 | ⚠️ Legacy helper (gateway no longer depends on it) |
| VMCreateWorker | `internal/jobs/vm_create.go` | full | ✅ Retry-safe idempotency guard added |
| VMDeleteWorker | `internal/jobs/vm_delete.go` | full | ✅ Aligned |
| VMPowerWorker | `internal/jobs/vm_power.go` | full | ✅ Aligned |
| DeleteVM Handler | `internal/api/handlers/server_vm.go` | 133-232 | ✅ Fixed — tiered confirm gate |
| DeleteSystem Handler | `internal/api/handlers/server_system.go` | 165-223 | ✅ Fixed — confirm_name via generated params |
| GetService Handler | `internal/api/handlers/server_system.go` | 336-365 | ✅ New |
| DeleteService Handler | `internal/api/handlers/server_system.go` | 367-446 | ✅ New — cascade + confirm + audit |
| ApprovalTicket Schema | `ent/schema/approval_ticket.go` | full | ✅ Fixed — operation_type enum added |
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

---

## Database Migration

- [x] Database migration tool configured (Atlas) — *Phase 5: `migrations/atlas/atlas.hcl`*
- [x] `atlas.hcl` configuration complete — *Phase 5: ent://ent/schema → PostgreSQL 18*
- [ ] `vms` table migration complete
- [ ] `vm_revisions` table migration complete
- [ ] `audit_logs` table migration complete
- [ ] `approval_tickets` table migration complete (Governance Core)
- [ ] `approval_policies` table migration complete (Governance Core)
- [ ] **Migration Rollback Test** (CI must include)

---

## Environment Isolation (ADR-0015 §1, §15)

- [x] **Schema Fields**:
  - [x] `Cluster.environment` - Cluster environment type (test/prod)
  - [x] `ent/schema/namespace_registry.go` - Namespace registry with explicit environment
    - [x] Contains `name` field
    - [x] Contains `environment` field (test/prod) - **explicitly set by admin**
    - [x] Does NOT contain `cluster_id` field (ADR-0017)
  - [x] ❌ **No `System.environment`** - System is decoupled from environment (ADR-0015 §1)
- [ ] **Platform RBAC**:
  - [x] `RoleBinding.allowed_environments` field
  - [x] Environment-based query filtering (`ListNamespaces`, `ListVMs`)
- [x] **Visibility Filtering** - users see only namespaces matching their allowed_environments (includes VM read/request path guard)
- [x] **Scheduling Constraints** - namespace environment must match cluster environment (`ApprovalValidator` + `VMCreateWorker` runtime guard)

---

## RevisionService

- [ ] Version number auto-increment
- [ ] Supports diff calculation
- [ ] YAML compressed storage

---

## TemplateService

- [x] `ent/schema/template.go` Schema definition complete
- [x] **TemplateService Implementation** (`internal/service/template_service.go`):
  - [x] `GetActiveTemplate(name)` implemented
  - [x] `GetLatestTemplate(name)` implemented
  - [x] `CreateTemplate(name, content)` implemented
  - [x] `ListTemplates()` implemented
  - [ ] `ExportTemplate(name)` implemented (deferred)
  - [ ] **Lifecycle Management** (Publish, Deprecate, Archive) (deferred)
  - [ ] **Save Validation** (3-step: syntax, mock render, dry run) (deferred)
- [ ] **Initial Import** from `deploy/seed/` to PostgreSQL (ADR-0018: templates stored in DB, not files)

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
- [ ] **Event Handlers** registered (deferred — wired at composition root)
- [x] **Idempotency Guarantee** implemented (VM create event-label guard + unique River enqueue by args/queue)
- [x] **Soft Archiving** configured (`internal/jobs/event_archive.go` + periodic bootstrap registration; marks 30d-old terminal events with `archived_at`)

---

## Reconciler

- [ ] Supports dry-run mode
- [ ] Only marks, doesn't delete
- [ ] Circuit breaker (50% threshold)
- [ ] Report ghost and orphan resources separately

---

## Template Engine (ADR-0007, ADR-0011, ADR-0018)

> **Updated per ADR-0018**: Templates define OS image source and cloud-init only. Go Template variables removed.

- [ ] **Template Scope** (after ADR-0018):
  - [ ] OS image source (DataVolume, ContainerDisk, PVC reference)
  - [ ] Cloud-init YAML (SSH keys, one-time password, network config)
  - [ ] Field visibility (`quick_fields`, `advanced_fields`, `professional_fields` for UI)
  - [ ] ❌ No Go Template variables (removed per ADR-0018)
  - [ ] ❌ No RequiredFeatures/Hardware (moved to InstanceSize per ADR-0018)
- [ ] **Template Lifecycle Management** complete (draft → active → deprecated → archived)
- [ ] **Template Save Validation** (cloud-init YAML syntax + K8s Dry-Run)
- [ ] **SSA Resource Submission (ADR-0011)** implemented

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
- [ ] **User View - My Requests** API (deferred)
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

## External Approval Provider Boundary (V1 Interface Only)

- [x] `ApprovalProvider` contract defined (`SubmitForApproval`, `ProcessApproval`) — in `internal/provider/approvalcontract/contract.go` (thin re-export in `internal/provider/approval.go`)
- [x] `external_approval_systems` schema + migration present for adapter registry
- [x] V1 runtime keeps built-in approval as required go-live path
- [x] External approval adapters are explicitly treated as V2+ plugin roadmap capability

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
- [x] **ApprovalTicket.operation_type**: ✅ Enum field added (`CREATE`/`DELETE`) with `CREATE` default
- [x] **DELETING Transient State**: ✅ VM status `DELETING` added; worker correctly sets it before K8s cleanup

---

## VNC Console Permissions (ADR-0015 §18, §18.1 Addendum)

- [x] **Environment-Based Access**:
  - [x] test environment - no approval required
  - [x] prod environment - requires approval ticket
- [ ] **VNC Token Security**:
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
  - [x] `batch_approval_tickets` parent table implemented
  - [x] `approval_tickets.parent_ticket_id` child linkage implemented
  - [x] Parent aggregate counters (`success/failed/pending`) are persisted
- [x] **Atomic Submission + Independent Execution**
  - [x] Parent + child ticket creation is atomic in one DB transaction
  - [x] Child jobs execute independently via River (parent approval dispatches child create/delete jobs)
  - [x] Parent status aggregation supports `PARTIAL_SUCCESS` (runtime-computed response model)
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
