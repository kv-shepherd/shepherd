# Phase 4 Checklist: Governance Capabilities

> **Detailed Document**: [phases/04-governance.md](../phases/04-governance.md)

---

## Database Migration

- [ ] Database migration tool configured (Atlas)
- [ ] `atlas.hcl` configuration complete
- [ ] `vms` table migration complete
- [ ] `vm_revisions` table migration complete
- [ ] `audit_logs` table migration complete
- [ ] `approval_tickets` table migration complete (Governance Core)
- [ ] `approval_policies` table migration complete (Governance Core)
- [ ] **Migration Rollback Test** (CI must include)

---

## Environment Isolation (ADR-0015 §1, §15)

- [ ] **Schema Fields**:
  - [ ] `Cluster.environment` - Cluster environment type (test/prod)
  - [ ] `ent/schema/namespace_registry.go` - Namespace registry with explicit environment
    - [ ] Contains `name` field
    - [ ] Contains `environment` field (test/prod) - **explicitly set by admin**
    - [ ] Does NOT contain `cluster_id` field (ADR-0017)
  - [ ] ❌ **No `System.environment`** - System is decoupled from environment (ADR-0015 §1)
- [ ] **Platform RBAC**:
  - [ ] `RoleBinding.allowed_environments` field
  - [ ] Environment-based query filtering
- [ ] **Visibility Filtering** - users see only namespaces matching their allowed_environments
- [ ] **Scheduling Constraints** - namespace environment must match cluster environment

---

## RevisionService

- [ ] Version number auto-increment
- [ ] Supports diff calculation
- [ ] YAML compressed storage

---

## TemplateService

- [ ] `ent/schema/template.go` Schema definition complete
- [ ] **TemplateService Implementation**:
  - [ ] `GetActiveTemplate(name)` implemented
  - [ ] `GetLatestTemplate(name)` implemented
  - [ ] `CreateTemplate(name, content)` implemented
  - [ ] `ListTemplates()` implemented
  - [ ] `ExportTemplate(name)` implemented
  - [ ] **Lifecycle Management** (Publish, Deprecate, Archive)
  - [ ] **Save Validation** (3-step: syntax, mock render, dry run)
- [ ] **Initial Import** from `deploy/seed/` to PostgreSQL (ADR-0018: templates stored in DB, not files)

---

## River Queue Task System (ADR-0006)

- [ ] River database migration complete
- [ ] River Client initialization configured
- [ ] Job type definitions in `internal/jobs/`
- [ ] Worker registration mechanism
- [ ] **Handler Unified 202 Return** implemented
- [ ] **Task Query API** implemented
- [ ] River retry mechanism configured
- [ ] River dead letter queue handling
- [ ] **PostgreSQL Stability Measures** (ADR-0008) applied

---

## Domain Event Pattern (ADR-0009)

- [ ] **DomainEvent Schema** complete
- [ ] **Key Constraint 1: Payload Immutability** enforced
- [ ] **Key Constraint 2: Atomic Transaction Pattern (ADR-0012)** implemented
- [ ] **Key Constraint 3: Worker Fault Tolerance** implemented
- [ ] **EventDispatcher** implemented
- [ ] **Event Handlers** registered
- [ ] **Idempotency Guarantee** implemented
- [ ] **Soft Archiving** configured

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
  - [ ] Field visibility (`quick_fields`, `advanced_fields` for UI)
  - [ ] ❌ No Go Template variables (removed per ADR-0018)
  - [ ] ❌ No RequiredFeatures/Hardware (moved to InstanceSize per ADR-0018)
- [ ] **Template Lifecycle Management** complete (draft → active → deprecated → archived)
- [ ] **Template Save Validation** (cloud-init YAML syntax + K8s Dry-Run)
- [ ] **SSA Resource Submission (ADR-0011)** implemented

---

## Approval Flow (Governance Core)

- [ ] **Directory Structure** created
- [ ] **ApprovalGateway** implemented
- [ ] **Admin Parameter Modification** supported
- [ ] **Full Replacement Safety Protection** implemented
- [ ] **Request Type Enum** defined
- [ ] **State Flow** implemented
- [ ] **User View - My Requests** API
- [ ] **Admin View - Approval Workbench** API
  - [ ] Default sort by `days_pending` (oldest first within priority tier)
  - [ ] `priority_tier` field in response (normal/warning/urgent)
  - [ ] Color coding: 0-3d normal, 4-7d yellow, 7+d red (ADR-0015 §11)
- [ ] **User Self-Cancellation** API (`POST /api/v1/approvals/{id}/cancel`)
- [ ] **AuditLogger** implemented
- [ ] **Approval API** endpoints complete
- [ ] Policy matching logic implemented
- [ ] **Extensible Approval Handler Architecture** designed
- [ ] **Notification Service (Reserved Interface)** defined
- [ ] **External State Management** (no pre-approval job insertion)

---

## External Approval Provider Boundary (V1 Interface Only)

- [ ] `ExternalApprovalProvider` contract defined (`SubmitForApproval`, `CheckStatus`, `CancelRequest`)
- [ ] `external_approval_systems` schema + migration present for adapter registry
- [ ] V1 runtime keeps built-in approval as required go-live path
- [ ] External approval adapters are explicitly treated as V2+ plugin roadmap capability

---

## Delete Confirmation Mechanism (ADR-0015 §13.1)

- [ ] **Tiered Confirmation**:
  - [ ] Simple `confirm=true` parameter for test VMs and Services
  - [ ] Name typing confirmation for prod VMs and Systems
- [ ] **Reject without confirmation** returns `400 BAD_REQUEST`
- [ ] **Error code**: `CONFIRMATION_REQUIRED` with clear params

---

## VNC Console Permissions (ADR-0015 §18, §18.1 Addendum)

- [ ] **Environment-Based Access**:
  - [ ] test environment - no approval required
  - [ ] prod environment - requires approval ticket
- [ ] **VNC Token Security**:
  - [ ] Single-use token
  - [ ] Time-bounded (max 2 hours)
  - [ ] User-bound (`sub` binds token to requester user ID)
  - [ ] AES-256-GCM encryption
- [ ] Shared replay marker store (`jti` + `used_at`) works across replicas (no Redis dependency)
- [ ] V1 has **no active token revocation API** (documented limitation, see ADR-0015 §18.1 addendum); revocation capability is tracked as V2+ enhancement
- [ ] **VNC Session Audit** logging

---

## Batch Operations (ADR-0015 §19)

> **Design**: [04-governance.md §5.6](../phases/04-governance.md#56-batch-operations-adr-0015-19)

- [ ] **Parent-Child Ticket Schema**
  - [ ] `batch_approval_tickets` parent table implemented
  - [ ] `approval_tickets.parent_ticket_id` child linkage implemented
  - [ ] Parent aggregate counters (`success/failed/pending`) are persisted
- [ ] **Atomic Submission + Independent Execution**
  - [ ] Parent + child ticket creation is atomic in one DB transaction
  - [ ] Child jobs execute independently via River
  - [ ] Parent status aggregation supports `PARTIAL_SUCCESS`
- [ ] **Two-Layer Rate Limiting**
  - [ ] Global limits: pending parent tickets + API request rate
  - [ ] User limits: pending parent count, pending child count, cooldown
  - [ ] Admin exemption and override APIs implemented
- [ ] **Batch APIs**
  - [ ] `POST /api/v1/vms/batch` submit
  - [ ] `GET /api/v1/vms/batch/{id}` status query
  - [ ] `POST /api/v1/vms/batch/{id}/retry` retry failed children
  - [ ] `POST /api/v1/vms/batch/{id}/cancel` terminate pending children
  - [ ] Compatibility endpoints (`/api/v1/approvals/batch`, `/api/v1/vms/batch/power`) normalized into same parent-child pipeline
- [ ] **Frontend Batch Queue UX**
  - [ ] Parent row + child detail panel implemented
  - [ ] Status polling uses backend `status_url` until terminal state
  - [ ] Retry/terminate actions show affected child items explicitly
  - [ ] `429` with `Retry-After` is handled with countdown and disabled actions
  - [ ] Accessibility: live status updates announced (`aria-live`)

---

## Notification System (ADR-0015 §20)

> **Design**: [04-governance.md §6.3](../phases/04-governance.md#63-notification-system-adr-0015-20)
> **Example**: [examples/notification/sender.go](../examples/notification/sender.go)

- [ ] `ent/schema/notification.go` - Internal inbox
- [ ] **NotificationSender Interface** (decoupled)
- [ ] **V1 Implementation**: InboxSender (database-backed)
- [ ] **API Endpoints**:
  - [ ] `GET /api/v1/notifications` - List user's notifications (paginated)
  - [ ] `GET /api/v1/notifications/unread-count` - Unread count for badge
  - [ ] `PATCH /api/v1/notifications/{id}/read` - Mark as read
  - [ ] `POST /api/v1/notifications/mark-all-read` - Mark all as read
- [ ] Notification triggers:
  - [ ] `APPROVAL_PENDING` → all admins
  - [ ] `APPROVAL_COMPLETED`/`APPROVAL_REJECTED` → requester
  - [ ] `VM_STATUS_CHANGE` → VM owner
- [ ] **Retention cleanup** (90 days, via River periodic job)
