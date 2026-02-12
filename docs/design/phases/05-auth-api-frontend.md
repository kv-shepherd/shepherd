# Phase 5: Authentication, API Completion & Frontend

> **Status**: In Progress (~95%)
> **Started**: 2026-02-09
> **Dependencies**: Phase 0-4 completed

## Deliverables

> **Last Updated**: 2026-02-11

| Deliverable | File Path | Status | Notes |
|-------------|-----------|--------|-------|
| JWT Auth Middleware | `internal/middleware/jwt.go` | ✅ | HS256 signing, Bearer extraction, claims injection |
| Auth Handlers | `internal/api/handlers/auth.go` | ✅ | Login/Me/ChangePassword |
| RBAC Middleware | `internal/middleware/rbac.go` | ✅ | RequirePermission + RequireResourceAccess |
| Member Handler | `internal/api/handlers/member.go` | ✅ | ResourceRoleBinding CRUD + audit |
| oapi-codegen config | `api/oapi-codegen.yaml` | ✅ | v2 format, gin-server + models |
| Generated Server | `internal/api/generated/server.gen.go` | ✅ | 38 endpoints (ADR-0028 omitzero value types), all model types |
| openapi-typescript | `web/src/types/api.gen.ts` | ✅ | Auto-generated from OpenAPI spec |
| Seed Command | `cmd/seed/main.go` | ✅ | 6 roles + default admin |
| Bootstrap | `internal/app/bootstrap.go` | ✅ | 65 lines ≤ 100 limit (ADR-0022) |
| Frontend: Login | `web/src/app/(auth)/login/page.tsx` | ✅ | Force password change flow |
| Frontend: Dashboard | `web/src/app/dashboard/page.tsx` | ✅ | System overview + health stats |
| Frontend: Systems | `web/src/app/systems/page.tsx` | ✅ | CRUD + DELETE with RFC 1035 validation |
| Frontend: Services | `web/src/app/services/page.tsx` | ✅ | CRUD + DELETE with cascade constraint |
| Frontend: VMs | `web/src/app/vms/page.tsx` | ✅ | Request wizard + power ops + delete confirm |
| Frontend: Approvals | `web/src/app/admin/approvals/page.tsx` | ✅ | Approve/Reject + DELETE target VM + priority highlighting |
| Frontend: Audit Logs | `web/src/app/admin/audit/page.tsx` | ✅ | Filtering + pagination |
| Frontend: Clusters | `web/src/app/admin/clusters/page.tsx` | ✅ | GET/POST with kubeconfig |
| Frontend: Namespaces | `web/src/app/admin/namespaces/page.tsx` | ✅ | CRUD + confirm_name delete (ADR-0015 §13) |
| Frontend: Templates | `web/src/app/admin/templates/page.tsx` | ✅ | Read-only list with column filters + deferred search |
| Frontend: Instance Sizes | `web/src/app/admin/instance-sizes/page.tsx` | ✅ | Read-only list with capability filters + sort |
| Frontend: Users | `web/src/app/admin/users/page.tsx` | ✅ | User management |
| Namespace Handlers | `internal/api/handlers/server_namespace.go` | ✅ | CRUD with environment filter + confirm_name delete gate |
| Notification Handlers | `internal/api/handlers/server_notification.go` | ✅ | List/UnreadCount/MarkRead/MarkAllRead + InboxSender + Triggers + Frontend Bell |
| Admin Handlers | `internal/api/handlers/server_admin.go` | ✅ | Clusters/Templates/InstanceSizes + UpdateClusterEnvironment |
| i18n Locales | `web/src/i18n/locales/{en,zh-CN}/` | ✅ | 5 namespaces (common, vm, approval, admin, auth) |

---

## Overview

Phase 5 bridges the backend to a usable product by implementing:
1. **Authentication & Authorization** — JWT-based auth with RBAC middleware
2. **API Completion** — Contract-first code generation, approval flow enhancements, audit API
3. **Frontend** — React SPA generated from OpenAPI contract, consuming backend APIs

---

## 5.1 Authentication System

### Local Authentication (Stage 1.5 of master-flow.md)

- **Login**: POST `/api/v1/auth/login` with username/password (bcrypt verification)
- **Password Hashing**: bcrypt cost fixed to 12 for seed + password change paths
- **JWT Signing**: HS256 with configurable secret, 24h default expiry
- **Force Password Change**: First login with default password requires immediate change
- **Current User**: GET `/api/v1/auth/me` returns user info + roles + permissions
- **Credential Failure Logging**: login failure logs keep generic messages (no username/password/token leakage)

### JWT Middleware

- Bearer token extraction from `Authorization` header
- Claims injection into `context.Context` (user_id, username, roles, permissions)
- Validation hardening: method allow-list + issuer + `exp` + `nbf` + `iat` checks
- Key rotation verification path: active signing key + optional legacy verification key list
- Revocation extension point: optional JTI checker hook (V1 has no active revoke API yet)
- Integration with RequestID middleware for audit trail

### RBAC Middleware (Stage 4.A+ of master-flow.md)

- **Global Permission Check**: `RequirePermission("platform:admin")` for admin-only routes
- **Resource-Level Inheritance**: VM → Service → System walk-up chain
- **ResourceRoleBinding**: owner/admin/member/viewer roles on System resources

---

## 5.2 API Completion

### Contract-First Code Generation (ADR-0021, ADR-0029)

- `oapi-codegen` v2 generates Go types + Gin ServerInterface from `api/openapi.yaml`
- `openapi-typescript` generates TypeScript types for frontend consumption
- Makefile targets: `make api-gen`, `make ent-gen`, `make generate`

### Approval Flow Enhancements (Stage 5.B)

- **Cluster Capability Matching**: Validate cluster health before approval
- **Overcommit Validation**: CPU/Memory request ≤ limit constraint
- **VM Record Creation**: CREATING status on approval, with generated VM name
- **ADR-0012 Atomic Commit**: `sqlc + pgx.Tx + river.InsertTx` ensures approval write + enqueue are one transaction
- **River Job Enqueue**: VMCreateWorker processes creation asynchronously with retry-safe idempotency guard

### Delete Confirmation (Stage 5.D)

- **Tiered Confirmation**:
  - Test environment: `confirm=true` query parameter
  - Prod environment: `confirm_name` must match VM name exactly

### Audit Log Query API

- GET `/api/v1/audit-logs` with filtering (resource_type, resource_id, action, actor)
- Pagination support (page, per_page, total, total_pages)

### Member Management API (Stage 4.A+)

- CRUD for ResourceRoleBinding on System resources
- Roles: owner, admin, member, viewer
- Audit logging for all membership changes

---

## 5.3 Infrastructure Integration

### River Queue (ADR-0006)

- `riverpgxv5` driver sharing pgxpool connection (ADR-0012)
- Worker registration in bootstrap composition root
- VMCreateWorker with claim-check pattern (ADR-0009) and event-label idempotency check

### Atlas Migration

- `migrations/atlas/atlas.hcl` configuration (ent schema → PostgreSQL 18)
- Dev database: `docker://postgres/18/dev`

---

## 5.4 Frontend Application

> **Authoritative Reference**: [ADR-0020](../../adr/ADR-0020-frontend-technology-stack.md) (Accepted)
> **Detailed Specification**: [frontend/FRONTEND.md](../frontend/FRONTEND.md)

### Technology Stack (ADR-0020)

- **Framework**: React 19 + Next.js 16 (App Router)
- **Language**: TypeScript 5.8+ (strict mode)
- **UI Components**: Ant Design 5.x + @ant-design/pro-components 2.x
- **State Management**: Zustand 5.x + TanStack Query 5.x
- **Styling**: Tailwind CSS 4.x
- **Form Validation**: Zod 3.x
- **Internationalization**: react-i18next 15.x
- **API Client**: Generated from OpenAPI via `openapi-typescript` + `openapi-fetch`

### Contract-First Frontend Development

1. `openapi-typescript` generates TypeScript types from `api/openapi.yaml`
2. `openapi-fetch` creates type-safe API client (no manual typing)
3. All API calls are fully typed end-to-end (OpenAPI → Go server → TS client)

### Pages (MVP)

- Login page with force password change
- Dashboard with system overview
- System/Service CRUD management
- VM lifecycle management (list, create request, power operations)
- Approval workbench (admin)
- Audit log viewer (admin)
- Clusters management (admin)
- Namespaces management (admin: CRUD + confirm_name delete)
- Templates viewer (admin: read-only, column filters, deferred search)
- Instance Sizes viewer (admin: read-only, capability filters, sort)

---

## Architecture Constraints

- `bootstrap.go` ≤ 100 lines (ADR-0022) — currently 65 lines
- Manual DI only (ADR-0013)
- OpenAPI spec is single source of truth (ADR-0021)
- No hardcoded API types in frontend — all generated from contract
