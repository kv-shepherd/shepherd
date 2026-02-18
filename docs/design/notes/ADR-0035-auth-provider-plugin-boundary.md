# ADR-0035 Implementation: Auth Provider Plugin Boundary and Type Discovery

> **Parent ADR**: [ADR-0035](../../adr/ADR-0035-auth-provider-plugin-boundary.md)  
> **Status**: Implementation specification draft (parent ADR status: **accepted**)  
> **Date**: 2026-02-15

---

## Summary

This note defines the concrete implementation for keeping auth-provider core
provider-agnostic, while allowing plugin-type onboarding through standardized
registration and API discovery.

---

## Delivered Artifacts

### 1. Runtime Registry Contract

* `internal/provider/auth_provider_admin_registry.go`

Delivered behavior:

* adapter registration by normalized `type` key
* duplicate type registration rejected
* adapter type discovery list API (`ListAuthProviderAdminAdapterTypes`)
* built-in compatibility plugins registered: `generic`, `oidc`, `ldap`, `sso`

### 2. API Discovery Endpoint

* `GET /api/v1/admin/auth-provider-types`
* OpenAPI schema:
  - `AuthProviderType`
  - `AuthProviderTypeList`

Delivered behavior:

* frontend/admin can discover supported provider types and config schema metadata
* runtime create/update remains contract-first and rejects unknown types

### 3. Frontend Alignment

* `web/src/features/admin-auth-providers/hooks/useAdminAuthProvidersController.ts`
* `web/src/features/admin-auth-providers/components/AdminAuthProvidersContent.tsx`

Delivered behavior:

* auth provider type options loaded from `GET /admin/auth-provider-types`
* create modal no longer hardcodes `oidc` as immutable default option list
* UI select is schema-driven from backend registry

### 4. CI Enforcement

* `docs/design/ci/scripts/check_auth_provider_plugin_boundary.go`

Blocking checks:

* runtime auth-provider handlers must resolve adapters via registry
* disallow provider-specific runtime branch patterns (`oidc`/`ldap`/`sso` hardcoded branches)
* frontend auth-provider controller must consume discovery API
* frontend must not reintroduce hardcoded provider-type options constant
* OpenAPI must not regress to OIDC/LDAP-only summary wording

### 5. Master-Flow Wording Alignment

* `docs/design/interaction-flows/master-flow.md` Stage 2.B/2.D updated to plugin-standard wording
* OIDC/LDAP retained as plugin examples, not core-only built-ins

### 6. Plugin Development Template and Auto-Registration Skeleton

* `pkg/authproviderplugin/admin.go` (public plugin contract wrapper)
* `plugins/authprovider/template/template.go` (minimal plugin template)
* `plugins/authprovider/example/plugin.go` (third-party style example plugin)
* `plugins/authprovider/autoreg/autoreg.go` (side-effect import loader)
* `plugins/authprovider/README.md` (integration instructions)

Delivered behavior:

* plugin authors implement the contract via `pkg/authproviderplugin`
* plugin package self-registers via `MustRegisterAdminAdapter`
* composition root imports `plugins/authprovider/autoreg` once for automatic plugin registration

---

## Acceptance Criteria

* `go run docs/design/ci/scripts/check_auth_provider_plugin_boundary.go` passes.
* `go run docs/design/ci/scripts/check_no_global_platform_admin_gate.go` passes.
* `go test ./internal/api/handlers -run 'TestListAuthProviderTypesAndRejectUnknownType|TestAuthProviderStage2CFlow|TestAdminUserRoleBindingAndAuthProviderCRUD'` passes (with PostgreSQL test harness).
* frontend typecheck and admin auth-provider hook tests pass.
* `master-flow.md` Stage 2.B narrative reflects plugin-standard model.
