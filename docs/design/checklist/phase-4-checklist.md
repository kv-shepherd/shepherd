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

## Environment Isolation (test/prod)

- [ ] **Schema Fields**:
  - [ ] `Cluster.environment` - Cluster environment type (test/prod)
  - [ ] `System.environment` - System environment type (test/prod)
  - [ ] Joint index for scheduling matching
- [ ] **Visibility Filtering** implemented
- [ ] **Scheduling Constraints** enforced
- [ ] **Permission Control** configured

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
