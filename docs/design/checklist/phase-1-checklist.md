# Phase 1 Checklist: Core Contract Definitions

> **Detailed Document**: [phases/01-contracts.md](../phases/01-contracts.md)

---

## Core Types (Ent Schema)

> **Governance Model Hierarchy**: `Namespace(K8s) → System → Service → VM Instance`

- [ ] `ent/schema/` directory created
- [ ] **Governance Model Core Schema**:
  - [ ] `ent/schema/system.go` - System/Project (e.g., demo, shop)
    - [ ] Contains `documentation` field (Markdown)
    - [ ] Contains `created_by` field
    - [ ] Contains `namespace` field (K8s Namespace isolation)
    - [ ] **User self-service creation, no approval required**
  - [ ] `ent/schema/service.go` - Service (e.g., redis, mysql)
    - [ ] Contains `documentation` field (Markdown)
    - [ ] Contains `created_by` field
    - [ ] Contains `next_instance_index` field (**permanently incrementing, no reset**)
    - [ ] **User self-service creation, no approval required**
- [ ] `ent/schema/vm.go` - VM Schema definition
  - [ ] Associates `system_id`, `service_id`
  - [ ] `instance` field stores instance number (e.g., "06")
- [ ] `ent/schema/vm_revision.go` - VM version history
- [ ] `ent/schema/audit_log.go` - Audit log Schema
- [ ] `ent/schema/approval_ticket.go` - Approval ticket (Governance Core)
- [ ] `ent/schema/approval_policy.go` - Approval policy (Governance Core)
- [ ] `ent/schema/cluster.go` - Multi-cluster credential management
- [ ] `ent/schema/template.go` - Template definition
- [ ] `ent/schema/resource_spec.go` - Resource spec template
- [ ] `ent/schema/pending_adoption.go` - Pending adoption resources
- [ ] `ent/schema/domain_event.go` - Domain event (ADR-0009)
- [ ] `ent/schema/infra_worker_pod.go` - Worker Pod registry

---

## ResourceSpec Overcommit Design

- [ ] `cpu_request` defaults to `cpu_limit` (no overcommit)
- [ ] `memory_request_mb` defaults to `memory_limit_mb`
- [ ] Admin can set `request < limit` for overcommit
- [ ] User-facing API only returns limit fields

---

## Instance Number Design (Permanently Incrementing)

- [ ] `Service.next_instance_index` only increases
- [ ] VM creation auto-increments
- [ ] ❌ No reset API provided

---

## Multi-cluster Credential Management

- [ ] **Cluster Schema Fields** complete
- [ ] **Encryption Service** (`internal/pkg/crypto/cluster_crypto.go`) implemented
- [ ] **CredentialProvider Interface** (Strategy Pattern) defined
- [ ] **ClusterRepository** methods implemented
- [ ] **Admin API** for dynamic cluster management
- [ ] **File-based Approach Forbidden** (CI detection)

---

## Ent Usage Standards (CI Enforcement)

- [ ] **Schema Definition Standards** followed
- [ ] **Code Generation Sync** (CI detection)
- [ ] **Dynamic Queries Must Be Type-Safe**
- [ ] **Transaction Management** per ADR-0012
- [ ] **Test Infrastructure** (PostgreSQL via testcontainers-go)
- [ ] **Test Coverage** (CI enforcement)

---

## Contract Interfaces

- [ ] `InfrastructureProvider` base interface definition
- [ ] `KubeVirtProvider` specialized interface definition
- [ ] `ResourceSpec` type definition
- [ ] `ResourceStatus` type definition
- [ ] `ValidationResult` type definition
- [ ] KubeVirt-specific types defined

---

## Extension Interfaces

- [ ] **AuthProvider Interface** defined
- [ ] **JWT Implementation** completed
- [ ] **ApprovalProvider Interface** defined
- [ ] **NotificationProvider Interface** defined

---

## Provider Configuration Type Safety

- [ ] `ProviderConfig` uses interface type (not `map[string]interface{}`)
- [ ] `ParseProviderConfig()` implements Discriminated Union logic
- [ ] Validation using `go-playground/validator`

---

## Error System

- [ ] `AppError` struct definition
- [ ] `ErrorCode` constants definition
- [ ] Errors only contain `code` + `params`, no hardcoded messages

---

## Context

- [ ] `AppContext` struct definition
- [ ] Context passing uses `context.Context`
- [ ] Request ID middleware

---

## Pre-Phase 2 Verification

- [ ] `go generate ./ent` generates code without errors
- [ ] Ent Schema unit tests 100% pass
- [ ] Provider interface definitions compile without errors
