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
    - [ ] Contains `cluster_id` field
    - [ ] Contains `environment` field (test/prod) - **explicitly set by admin**
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
- [ ] **Initial Import** from templates directory

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

## Template Engine (ADR-0011 SSA Upgrade)

- [ ] Helm basic syntax compatible
- [ ] Supports Sprig functions
- [ ] Simulates Helm built-in objects
- [ ] Supports `_helpers.tpl` helper templates
- [ ] **Template Lifecycle Management** complete
- [ ] **Template Save Validation (Dry-Run)** working
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
- [ ] **AuditLogger** implemented
- [ ] **Approval API** endpoints complete
- [ ] Policy matching logic implemented
- [ ] **Extensible Approval Handler Architecture** designed
- [ ] **Notification Service (Reserved Interface)** defined
- [ ] **External State Management** (no pre-approval job insertion)

---

## Delete Confirmation Mechanism (ADR-0015 §13.1)

- [ ] **Tiered Confirmation**:
  - [ ] Simple `confirm=true` parameter for test VMs and Services
  - [ ] Name typing confirmation for prod VMs and Systems
- [ ] **Reject without confirmation** returns `400 BAD_REQUEST`
- [ ] **Error code**: `CONFIRMATION_REQUIRED` with clear params

---

## VNC Console Permissions (ADR-0015 §18)

- [ ] **Environment-Based Access**:
  - [ ] test environment - no approval required
  - [ ] prod environment - requires approval ticket
- [ ] **VNC Token Security**:
  - [ ] Single-use token
  - [ ] Time-bounded (max 2 hours)
  - [ ] User-bound (hashed user ID)
  - [ ] AES-256-GCM encryption
- [ ] **Token Revocation** API
- [ ] **VNC Session Audit** logging

---

## Notification System (ADR-0015 §20)

- [ ] `ent/schema/notification.go` - Internal inbox
- [ ] **NotificationSender Interface** (decoupled)
- [ ] **V1 Implementation**: InboxNotificationSender (database)
- [ ] Notification triggers:
  - [ ] New approval request → all admins
  - [ ] Request approved/rejected → creator + maintainers
  - [ ] VM created/deleted → creator + maintainers

