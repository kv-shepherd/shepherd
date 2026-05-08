# Phase 5: Authentication, API Completion & Frontend

> **Status**: Complete for current scope
> **Started**: 2026-02-09
> **Dependencies**: Phase 0-4 completed
> **Last Audited**: 2026-05-08 (code-vs-doc alignment audit)

## Deliverables

> **Last Updated**: 2026-05-08

| Deliverable | File Path | Status | Notes |
|-------------|-----------|--------|-------|
| JWT Auth Middleware | `internal/api/middleware/jwt.go` | ✅ | HS256 signing, Bearer extraction, claims injection |
| Auth Handlers | `internal/api/handlers/server_auth.go` | ✅ | Login/Me/ChangePassword |
| RBAC Middleware | `internal/api/middleware/rbac.go` | ✅ | RequirePermission + RequireResourceAccess |
| Member Handler | `internal/api/handlers/member.go` | ✅ | ResourceRoleBinding CRUD + audit |
| oapi-codegen config | `api/oapi-codegen.yaml` | ✅ | v2 format, gin-server + models |
| Generated Server | `internal/api/generated/server.gen.go` | ✅ | 135 endpoints (ADR-0028 omitzero value types), all model types |
| openapi-typescript | `web/src/types/api.gen.ts` | ✅ | Auto-generated from OpenAPI spec |
| Seed Command | `cmd/seed/main.go` | ✅ | 6 roles + default admin |
| Bootstrap | `internal/app/bootstrap.go` | ✅ | 127 file lines; `Bootstrap()` function is 57 lines and remains orchestration-only (see ADR-0043 design note) |
| Frontend: Login | `web/src/app/(auth)/login/page.tsx` | ✅ | Force password change flow + login-to-dashboard smoke E2E |
| Frontend: Dashboard | `web/src/app/(protected)/dashboard/page.tsx` | ✅ | System overview + health stats |
| Frontend: Systems | `web/src/app/(protected)/systems/page.tsx` | ✅ | CRUD + DELETE with RFC 1035 validation |
| Frontend: Services | `web/src/app/(protected)/services/page.tsx` | ✅ | CRUD + DELETE with cascade constraint |
| Frontend: VMs | `web/src/app/(protected)/vms/page.tsx` | ✅ | Request wizard + power ops + delete confirm |
| Frontend: Approvals | `web/src/app/(protected)/admin/approvals/page.tsx` | ✅ | Approve/Reject + DELETE target VM + priority highlighting |
| Frontend: Audit Logs | `web/src/app/(protected)/admin/audit/page.tsx` and `web/src/app/(protected)/admin/audit-logs/page.tsx` | ✅ | Filtering + pagination plus compatibility route |
| Frontend: Clusters | `web/src/app/(protected)/admin/clusters/page.tsx` | ✅ | GET/POST with kubeconfig |
| Frontend: Namespaces | `web/src/app/(protected)/admin/namespaces/page.tsx` and `[id]/page.tsx` | ✅ | CRUD + confirm_name delete (ADR-0015 §13) |
| Frontend: Templates | `web/src/app/(protected)/admin/templates/page.tsx` | ✅ | CRUD + column filters + deferred search + preset-driven image source/cloud-init editor |
| Frontend: Instance Sizes | `web/src/app/(protected)/admin/instance-sizes/page.tsx` | ✅ | CRUD + capability filters + sort + JSON spec_overrides editor |
| Frontend: Users | `web/src/app/(protected)/admin/users/page.tsx` | ✅ | User directory + system member management |
| Frontend: Auth Providers | `web/src/app/(protected)/admin/auth-providers/page.tsx` | ✅ | Schema-driven provider CRUD + test connection + sample fields + directory preview/sync + external cohort mappings |
| Frontend: Rate Limits | `web/src/app/(protected)/admin/rate-limits/page.tsx` | ✅ | Exemptions/overrides admin management |
| Frontend: Permissions | `web/src/app/(protected)/admin/permissions/page.tsx` | ✅ | Permission browser |
| Frontend: RBAC | `web/src/app/(protected)/admin/rbac/page.tsx` | ✅ | Role + role binding management |
| Frontend: Batch Overview | `web/src/app/(protected)/vms/batch/page.tsx` | ✅ | Batch operations list view |
| Frontend: Batch Detail | `web/src/app/(protected)/vms/batch/[id]/page.tsx` | ✅ | Parent-child status + retry/cancel |
| Frontend: VM Detail | `web/src/app/(protected)/vms/[id]/page.tsx` | ✅ | VM detail view |
| Frontend: Notifications | `web/src/app/(protected)/notifications/page.tsx` | ✅ | Full notification inbox |
| Frontend: Profile | `web/src/app/(protected)/profile/page.tsx` | ✅ | User profile view |
| Frontend: Change Password | `web/src/app/(auth)/auth/change-password/page.tsx` | ✅ | Standalone password change |
| Frontend: User Approvals | `web/src/app/(protected)/approvals/page.tsx` | ✅ | User-facing approvals view |
| Frontend: Tickets | `web/src/app/(protected)/tickets/page.tsx` | ✅ | User-facing ticket/request history |
| Frontend: Approval Tasks | `web/src/app/(protected)/admin/approval-tasks/page.tsx` | ✅ | Admin approval-task compatibility/workbench route |
| Namespace Handlers | `internal/api/handlers/server_namespace.go` | ✅ | CRUD with environment filter + confirm_name delete gate |
| Notification Handlers | `internal/api/handlers/server_notification.go` | ✅ | List/UnreadCount/MarkRead/MarkAllRead + InboxSender + Triggers + Frontend Bell |
| Admin Handlers | `internal/api/handlers/server_admin.go` | ✅ | Clusters/Templates/InstanceSizes + UpdateClusterEnvironment |
| i18n Locales | `web/src/i18n/locales/{en,zh-CN}/` | ✅ | 6 namespaces (common, errors, vm, approval, admin, schema) |

---

## Overview

Phase 5 bridges the backend to a usable product by implementing:
1. **Authentication & Authorization** — JWT-based auth with RBAC middleware
2. **API Completion** — Contract-first code generation, approval flow enhancements, audit API
3. **Frontend** — React SPA generated from OpenAPI contract, consuming backend APIs

---

## Current Runtime Alignment

- `api/openapi.yaml` currently exposes 135 `operationId`s.
- `web/src/app` currently contains 29 `page.tsx` files, including the root route and compatibility/alias pages.
- Auth middleware lives under `internal/api/middleware/`; there is no standalone `internal/middleware` package.
- Route registration is OpenAPI-generated and centralized through `internal/app/router.go`.

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
- **Form Validation**: Zod 4.x with Ant Design rule adapters for localized field validation
- **Internationalization**: react-i18next 16.x
- **API Client**: Generated from OpenAPI via `openapi-typescript` + `openapi-fetch`

### Contract-First Frontend Development

1. `openapi-typescript` generates TypeScript types from `api/openapi.yaml`
2. `openapi-fetch` creates type-safe API client (no manual typing)
3. All API calls are fully typed end-to-end (OpenAPI → Go server → TS client)

### Pages (MVP — 29 App Router page files)

- Login page with force password change
- Change password page (standalone)
- Dashboard with system overview
- System/Service CRUD management
- VM lifecycle management (list, detail, request wizard, power operations)
- Approval workbench (admin + user-facing views)
- Audit log viewer (admin)
- Clusters management (admin)
- Namespaces management (admin: CRUD + confirm_name delete)
- Templates management (admin: CRUD, column filters, deferred search)
- Instance Sizes management (admin: CRUD, capability filters, sort)
- Users management (admin)
- Auth Providers management (admin: CRUD + test connection + sample fields + directory preview/sync + external cohort mappings)
- External Approval Systems management (admin: webhook registry CRUD)
- Rate Limits management (admin: exemptions/overrides)
- Permissions browser (admin)
- RBAC management (admin: roles + role bindings)
- Batch operations (list + detail with parent-child status)
- Notifications inbox
- Profile page

---

## Architecture Constraints

- `Bootstrap()` remains orchestration-only and concise; file-level line count is not used as a mechanical quality gate (see ADR-0043 design note)
- Manual DI only (ADR-0013)
- OpenAPI spec is single source of truth (ADR-0021)
- No hardcoded API types in frontend — all generated from contract
