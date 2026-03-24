# Phase 5 Checklist: Authentication, API Completion & Frontend

> **Detailed Document**: [phases/05-auth-api-frontend.md](../phases/05-auth-api-frontend.md)
>
> **Implementation Status**: 🔄 In Progress (~96%, 2026-03-10) — Backend + frontend feature-complete (97 endpoints, 25 pages), E2E verification pending
>
> **Gate Checklist**: [../ci/GATE_HARDENING_CHECKLIST.md](../ci/GATE_HARDENING_CHECKLIST.md)

---

## Authentication System (Stage 1.5)

- [x] **Local Login**: POST `/api/v1/auth/login` with bcrypt password verification
- [x] **Password Hash Cost**: bcrypt cost fixed to `12` (`handlers/server_auth.go`)
- [x] **JWT Token Signing**: HS256 with configurable secret + expiry (`middleware/jwt.go`)
- [x] **Force Password Change**: Default admin must change password on first login
- [x] **Current User**: GET `/api/v1/auth/me` returns user info + roles + permissions
- [x] **Change Password**: POST `/api/v1/auth/change-password` with old/new password validation
- [x] **Credential Log Redaction**: login failure logs do not include username/password/token
- [x] **AuthHandler** (`handlers/auth.go`) — Login, GetCurrentUser, ChangePassword
- [x] **AuthModule** (`modules/auth.go`) — Public routes (login) + JWT-protected routes (me, change-password)

---

## JWT Middleware

- [x] Bearer token extraction from `Authorization` header
- [x] JWTClaims with user_id, username, roles, permissions
- [x] `GenerateToken()` and `JWTAuth()` middleware functions
- [x] Parser options hardened: valid methods allow-list + issuer/exp/nbf/iat verification
- [x] Signing key rotation support: verify with `SigningKey + jwt_verification_keys`
- [x] Revocation check hook in middleware (`RevocationChecker`); V1 still has no active revoke API
- [x] Integration with RequestID middleware (X-Request-ID with UUID v7)

---

## RBAC Middleware (Stage 4.A+)

- [x] `RequirePermission()` — Global permission check middleware
- [x] `RequireResourceAccess()` — Resource-level permission with hierarchy walk-up
- [x] `ResourceRoleChecker` — VM → Service → System inheritance chain traversal
- [x] **MemberHandler** (`internal/api/handlers/member.go`) — ResourceRoleBinding CRUD with audit logging

---

## API Contract-First Code Generation (ADR-0021)

- [x] `api/oapi-codegen.yaml` v2 format with gin-server + models generation
- [x] `internal/api/generated/server.gen.go` — 97 endpoints (omitzero value types via ADR-0028), all model types
- [x] `make api-gen` Makefile target
- [x] `make ent-gen` Makefile target
- [x] `make generate` composite target (ent-gen + api-gen)
- [x] `openapi-typescript` generates `web/src/types/api.gen.ts`

---

## Approval Flow Enhancements (Stage 5.B)

- [x] **ApprovalValidator** (`service/approval_validator.go`) — Cluster health + overcommit checks
- [x] **VM Record Creation**: CREATING status on approval with generated VM name
- [x] **VM Naming**: `{namespace}-{system}-{service}-{idx}` pattern with atomic increment
- [x] **DomainEvent Payload Parsing**: Extract service_id, namespace, requester_id
- [x] **River Job Enqueue**: VMCreateWorker via riverpgxv5 shared pgxpool

---

## Delete Confirmation (Stage 5.D)

- [x] **Tiered Confirmation** in `handlers/vm.go`:
  - [x] Test env: `confirm=true` query parameter
  - [x] Prod env: `confirm_name` must match VM name
- [x] `CONFIRMATION_REQUIRED` error code with params
- [x] Audit logging for delete requests

---

## Audit Log Query API

- [x] `handlers/audit.go` — GET `/api/v1/audit-logs`
- [x] Filtering: resource_type, resource_id, action, actor
- [x] Pagination: page, per_page, total, total_pages
- [x] Wired into GovernanceModule

---

## Infrastructure

- [x] **River Client**: `InitRiverClient()` in `database.go` with riverpgxv5
- [x] **Worker Registration**: VMCreateWorker in bootstrap composition root
- [x] **Atlas Migration Config**: `migrations/atlas/atlas.hcl`
- [x] **Bootstrap composition root**: `Bootstrap()` remains orchestration-only and concise (current function body: 55 lines; file may exceed 100 due to helpers/comments, see ADR-0043 design note)
- [x] **Seed Command**: `cmd/seed/main.go` with 6 built-in roles + default admin

---

## Frontend Application (ADR-0020)

> **Note**: Frontend rebuilt with Next.js 16 + Ant Design 5 per ADR-0020 decision.
> Scaffold operational as of 2026-02-10.

- [x] **Project Scaffold**: Next.js 16 (App Router) + React 19 + TypeScript 5.8+ (strict)
- [x] **UI Components**: Ant Design 5.x + @ant-design/pro-components 2.x
- [x] **State Management**: Zustand 5.x + TanStack Query 5.x
- [x] **Styling**: Tailwind CSS 4.x
- [ ] **Form Validation**: Zod 4.x (i18n validation messages pending)
- [x] **Internationalization**: react-i18next 16.x (en + zh-CN, 6 namespaces)
- [x] **API Client**: openapi-typescript + openapi-fetch (type-safe from contract)
- [x] **Pages feature-complete** (25/25 routes exist and are production-ready for current scope):
  - [x] Login page (with force password change flow)
  - [x] Dashboard / System overview (real API: health, stats)
  - [x] System CRUD management (GET/POST/DELETE with RFC 1035 validation)
  - [x] Service CRUD management (scoped to system, GET/POST/DELETE with cascade constraint)
  - [x] VM lifecycle (list, request wizard, start/stop/restart, delete with confirmation)
  - [x] Approval workbench (admin: approve/reject with cluster selection, DELETE ticket target VM display, ADR-0015 §11 priority tier highlighting)
  - [x] Audit log viewer (admin: filtering + pagination)
  - [x] Clusters management (admin: GET/POST with kubeconfig)
  - [x] Namespaces management (admin: CRUD + confirm_name delete gate, ADR-0015 §13)
  - [x] Templates management (admin: CRUD list/forms + column filters + useDeferredValue search)
  - [x] Instance Sizes management (admin: CRUD list/forms + capability filters + sort)
  - [x] Users management page (admin) — user directory + system member CRUD (`web/src/app/(protected)/admin/users/page.tsx`)
  - [x] Auth Providers management page (admin) — schema-driven provider CRUD + test connection + sample fields + directory preview/sync + external cohort mappings
  - [x] Rate Limits management page (admin) — exemptions/overrides admin management
  - [x] Permissions browser page (admin) — permission list
  - [x] RBAC management page (admin) — roles + role bindings
  - [x] Batch operations list page — batch overview
  - [x] Batch detail page — parent-child status + retry/cancel
  - [x] VM detail page — VM detail view with provisioning status
  - [x] Notifications inbox page — full inbox view
  - [x] Profile page — user profile
  - [x] Change password page (standalone) — separate from login flow
  - [x] User-facing approvals page — user's own approval view
- [x] **Auth Integration**: JWT token in localStorage, auto-attach via middleware, 401 redirect
- [x] **openapi-typescript** generates `web/src/types/api.gen.ts`
- [x] **Notification Bell** (`web/src/components/ui/NotificationBell.tsx`):
  - [x] Badge with unread count (30s polling via TanStack Query)
  - [x] Popover with recent notifications list
  - [x] Mark-read on click + mark-all-read
  - [x] Integrated into `AppLayout.tsx` header via `actionsRender`

---

## Pre-Phase 6 Verification

- [x] `go build ./...` passes
- [x] Frontend `npm run build` passes (16/16 routes, zero type errors)
- [x] API contract types match between Go server and TS client
- [ ] Login → JWT → Protected API flow works end-to-end
- [ ] All CRUD operations testable via UI
