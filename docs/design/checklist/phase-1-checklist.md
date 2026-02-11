# Phase 1 Checklist: Core Contract Definitions

> **Detailed Document**: [phases/01-contracts.md](../phases/01-contracts.md)
>
> **Implementation Status**: 🔄 Partial (~80%) — Ent schemas complete, CI enforcement and frontend type generation gaps

---

## Core Types (Ent Schema)

> **Governance Model Hierarchy** (ADR-0015): `System → Service → VM Instance`
>
> System is a logical grouping decoupled from namespace. Namespace specified at VM creation.

- [x] `ent/schema/` directory created
- [x] **Governance Model Core Schema**:
  - [x] `ent/schema/system.go` - System/Project (e.g., demo, shop)
    - [x] Contains `id` field (immutable)
    - [x] Contains `description` field (optional)
    - [x] Contains `created_by` field
    - [x] Contains `tenant_id` field (default: "default", reserved for multi-tenancy)
    - [x] ❌ **No `namespace` field** (ADR-0015 §1)
    - [x] ❌ **No `environment` field** (ADR-0015 §1)
    - [x] ❌ **No `maintainers` field** - use RoleBinding table (ADR-0015 §22)
    - [x] Globally unique name constraint
    - [x] **User self-service creation, no approval required**
  - [x] `ent/schema/service.go` - Service (e.g., redis, mysql)
    - [x] Contains `id` field (immutable)
    - [x] Contains `name` field (**immutable after creation**, ADR-0015 §2)
    - [x] Contains `description` field (optional)
    - [x] ❌ **No `created_by` field** - inherited from System (ADR-0015 §2)
    - [x] Contains `next_instance_index` field (**permanently incrementing, no reset**)
    - [x] Unique name constraint **within parent System** (per [master-flow.md Stage 4.B](../interaction-flows/master-flow.md#stage-4-b))
    - [x] **User self-service creation, no approval required**
- [x] `ent/schema/vm.go` - VM Schema definition
  - [x] Associates `service_id` **only** (ADR-0015 §3)
  - [x] ❌ **No `system_id` field** - obtain via service edge (ADR-0015 §3)
  - [x] `instance` field stores instance number (e.g., "01")
- [x] `ent/schema/vm_revision.go` - VM version history
- [x] `ent/schema/audit_log.go` - Audit log Schema
- [x] `ent/schema/approval_ticket.go` - Approval ticket (Governance Core)
- [x] `ent/schema/approval_policy.go` - Approval policy (Governance Core)
- [x] `ent/schema/cluster.go` - Multi-cluster credential management
- [x] `ent/schema/template.go` - Template definition
- [x] `ent/schema/instance_size.go` - Instance size (ADR-0018, replaces resource_spec)
- [x] `ent/schema/pending_adoption.go` - Pending adoption resources
- [x] `ent/schema/domain_event.go` - Domain event (ADR-0009)
- [ ] `ent/schema/infra_worker_pod.go` - Worker Pod registry (deferred to Phase 4+)

---

## ResourceSpec Overcommit Design

- [x] `cpu_request` defaults to `cpu_limit` (no overcommit)
- [x] `memory_request_mb` defaults to `memory_limit_mb`
- [x] Admin can set `request < limit` for overcommit
- [x] User-facing API only returns limit fields

---

## Instance Number Design (Permanently Incrementing)

- [x] `Service.next_instance_index` only increases
- [x] VM creation auto-increments
- [x] ❌ No reset API provided

---

## Multi-cluster Credential Management

- [x] **Cluster Schema Fields** complete
- [ ] **Encryption Service** (`internal/pkg/crypto/cluster_crypto.go`) implemented (deferred to Phase 4+)
- [x] **CredentialProvider Interface** (Strategy Pattern) defined
- [ ] **ClusterRepository** methods implemented (deferred to Phase 2+)
- [ ] **Admin API** for dynamic cluster management (deferred to Phase 3+)
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

- [x] `InfrastructureProvider` base interface definition
- [x] `KubeVirtProvider` specialized interface definition
- [x] `ResourceSpec` type definition
- [x] `ResourceStatus` type definition
- [x] `ValidationResult` type definition
- [x] KubeVirt-specific types defined

---

## API Contract-First Artifacts (ADR-0021, ADR-0029)

> **Details**: See [CI README §API Contract-First](../ci/README.md#api-contract-first-enforcement-adr-0021-adr-0029) for full implementation guidance.

- [x] `api/openapi.yaml` exists and is OpenAPI 3.1 canonical spec (953 lines, full P1 coverage)
- [ ] `api/.vacuum.yaml` exists and `make api-lint` passes (ADR-0029: vacuum replaces spectral)
- [x] `api/oapi-codegen.yaml` exists and targets `internal/api/generated/` — *Phase 5: v2 format with gin-server + models*
- [x] `make api-generate` produces:
  - [x] `internal/api/generated/` Go server types — *Phase 5: 1393 lines, ServerInterface with 28 endpoints*
  - [ ] `web/src/types/api.gen.ts` TypeScript types — *Phase 5: pending*
- [ ] `make api-check` passes with no uncommitted generated changes
- [ ] If 3.1-only features are used:
  - [ ] `api/openapi.compat.yaml` is generated (3.0-compatible)
  - [ ] CI runs `make api-compat-generate` before `make api-compat`
- [ ] CI blocks merges unless `make api-check` passes
- [ ] ADR-0029 Compliance: libopenapi-validator with StrictMode, version-pinned CI actions

---

## Optional Field Strategy (ADR-0028)

> **Purpose**: Ensure generated Go types use `omitzero` tag to eliminate pointer hell.    
> **Status**: ADR-0028 **Accepted** ✅. See [ADR-0028](../../adr/ADR-0028-oapi-codegen-optional-field-strategy.md).

- [x] `go.mod` requires Go 1.25+ (enables `omitzero` support) — *Go 1.25.7*
- [x] `api/oapi-codegen.yaml` contains:
  - [x] `output-options.prefer-skip-optional-pointer-with-omitzero: true`
- [ ] **Generated types verification** (Code Review enforcement):
  - [ ] Optional-only fields use value types with `json:",omitzero"` tag
  - [ ] `nullable: true` fields use pointer types with `json:",omitempty"` tag
  - [ ] No unnecessary `*string`, `*int` for non-nullable optional fields
- [ ] Business logic does not contain excessive `if ptr != nil` checks for optional fields

---

## Frontend Testing Configuration (ADR-0020)

> **Implementation Guide**: [ADR-0020 Testing Toolchain](../notes/ADR-0020-frontend-testing-toolchain.md)

- [ ] `web/vitest.config.ts` exists with coverage thresholds configured:
  - [ ] `coverage.thresholds.lines: 80`
  - [ ] `coverage.thresholds.functions: 80`
  - [ ] `coverage.thresholds.branches: 75`
  - [ ] `coverage.thresholds.statements: 80`
- [ ] `web/tests/setup.ts` exists with MSW initialization
- [ ] `web/tests/mocks/handlers.ts` exists for API mocking
- [ ] `web/playwright.config.ts` exists for E2E testing
- [ ] `.github/workflows/frontend-tests.yml` exists with:
  - [ ] Unit test job with coverage reporting
  - [ ] E2E test job with Playwright
  - [ ] Coverage threshold enforcement (block PR on failure)
- [ ] Package.json contains required test scripts:
  - [ ] `test`, `test:run`, `test:coverage`, `test:e2e`
- [ ] Frontend design docs layering (ADR-0030):
  - [ ] `docs/design/frontend/README.md` exists and links architecture/features/contracts/testing
  - [ ] Batch queue UX spec exists at `docs/design/frontend/features/batch-operations-queue.md`

---

## Extension Interfaces

- [x] **AuthProvider Interface** defined (`internal/provider/auth.go`)
- [x] **JWT Implementation** completed — *Phase 5: `middleware/jwt.go` HS256 + JWTClaims + JWTAuth middleware*
- [x] **ApprovalProvider Interface** defined (`internal/provider/auth.go`)
- [x] **NotificationProvider Interface** defined (`internal/provider/auth.go`)

---

## Platform RBAC Schema (ADR-0015 §22, ADR-0019)

- [ ] `ent/schema/permission.go` - Atomic permission definitions (deferred — permissions stored as JSON in role)
- [x] `ent/schema/role.go` - Role = bundle of permissions
- [x] `ent/schema/role_binding.go` - User-role assignments with scope
- [x] `ent/schema/resource_role_binding.go` - Resource-level member management (owner/admin/member/viewer)
- [x] Built-in roles seeded (per master-flow.md Stage 2.A) — *Phase 5: `cmd/seed/main.go`*:
  - [x] **Bootstrap** - Initial setup only (`platform:admin`), ⚠️ MUST be disabled after initialization
  - [x] **PlatformAdmin** - Super admin (`platform:admin`, explicit permission per ADR-0019)
  - [x] **SystemAdmin** - Resource management (explicit permissions)
  - [x] **Approver** - Can approve requests (`approval:approve`, `approval:view`)
  - [x] **Operator** - Power operations (`vm:operate`, `vm:read`)
  - [x] **Viewer** - Read-only access (explicit: `system:read`, `service:read`, `vm:read`)
- [ ] Environment-based permission control (`allowed_environments` field)

---

## Provider Configuration Type Safety

- [ ] `ProviderConfig` uses interface type (not `map[string]interface{}`)
- [ ] `ParseProviderConfig()` implements Discriminated Union logic
- [ ] Validation using `go-playground/validator`

---

## Error System

- [x] `AppError` struct definition
- [x] `ErrorCode` constants definition
- [x] Errors only contain `code` + `params`, no hardcoded messages

---

## Context

- [x] `AppContext` struct definition — *Phase 5: context helpers in `middleware/request_id.go`*
- [x] Context passing uses `context.Context` — *SetUserContext/GetUserID/GetUsername/GetRoles*
- [x] Request ID middleware — *Phase 5: X-Request-ID with UUID v7*

## Go Module Configuration (ADR-0016)

- [ ] `go.mod` uses vanity import path: `kv-shepherd.io/shepherd`
- [ ] All internal imports use vanity path: `kv-shepherd.io/shepherd/internal/...`
- [ ] Vanity import server configured (for production deployment)

---

## Pre-Phase 2 Verification

- [x] `go generate ./ent` generates code without errors
- [ ] Ent Schema unit tests 100% pass (unit tests deferred — requires testcontainers)
- [x] Provider interface definitions compile without errors
- [x] `go vet ./...` passes
- [x] `go build ./...` passes
- [x] `go test -race ./...` passes
- [x] `stdlib.OpenDBFromPool` integrated in `database.go`
