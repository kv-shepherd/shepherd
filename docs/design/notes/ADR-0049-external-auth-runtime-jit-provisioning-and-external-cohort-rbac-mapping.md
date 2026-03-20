# Design Note: ADR-0049 — External Auth Runtime, JIT User Provisioning, and External Cohort-to-RBAC Mapping

> **Status**: Proposed (ADR-0049 under review until 2026-03-22)
> **Related ADR**: [ADR-0049](../../adr/ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md)
> **Owner**: @jindyzhao
> **Created**: 2026-03-20
> **Last Updated**: 2026-03-20

## Summary

This note captures the intended implementation shape for a unified external-auth
runtime standard:

* external providers plug into runtime login/callback through a shared contract
* successful external login uses JIT provisioning by default
* the canonical user center stores only logged-in or explicitly imported users
* external groups/departments/tags are normalized into `external cohorts`
* external cohorts may drive Shepherd RBAC automation, but never become direct
  runtime permissions

WeCom is the first provider practice for this standard, not a special core
exception.

## Scope

- In scope: runtime auth plugin seam for external providers
- In scope: JIT user provisioning on successful external login
- In scope: normalized external cohort projection and mapping into Shepherd RBAC
- In scope: display-only supplemental profile projection
- In scope: WeCom as the first provider implementation under the shared standard
- Out of scope: provider-specific workflow branches in core auth handlers
- Out of scope: default full-directory mirroring to construct the user center
- Out of scope: using external claims/cohorts directly during runtime authorization

---

## 1. Runtime Contract

### 1.1 Public plugin SDK

Implementation should extend the public plugin SDK, not `internal/provider`,
with a runtime contract similar to:

* `pkg/authproviderplugin/runtime.go`
* `internal/provider/auth_runtime.go`

The shared contract should stay small and stable:

```go
package provider

import "context"

type ExternalCohort struct {
    Kind        string `json:"kind"`
    Key         string `json:"key"`
    DisplayName string `json:"display_name,omitempty"`
}

type AuthProfileAttributes map[string]interface{}

type AuthResult struct {
    ExternalID        string                `json:"external_id"`
    Username          string                `json:"username"`
    DisplayName       string                `json:"display_name"`
    Email             string                `json:"email,omitempty"`
    Enabled           bool                  `json:"enabled"`
    Cohorts           []ExternalCohort      `json:"cohorts,omitempty"`
    ProfileAttributes AuthProfileAttributes `json:"profile_attributes,omitempty"`
}

type AuthStartRequest struct {
    LoginMode string `json:"login_mode,omitempty"`
    ReturnTo  string `json:"return_to,omitempty"`
}

type AuthStartResponse struct {
    RedirectURL string `json:"redirect_url,omitempty"`
}

type AuthCallbackRequest struct {
    Query  map[string][]string `json:"query,omitempty"`
    Form   map[string][]string `json:"form,omitempty"`
    Header map[string][]string `json:"header,omitempty"`
}

type AuthRuntimeCapability interface {
    StartLogin(ctx context.Context, config map[string]interface{}, req AuthStartRequest) (*AuthStartResponse, error)
    CompleteLogin(ctx context.Context, config map[string]interface{}, req AuthCallbackRequest) (*AuthResult, error)
}
```

Core must validate the canonical result shape, but must not understand
provider-specific callback fields.

### 1.2 Core-owned runtime steps

For external auth, the generic flow should be:

1. list login-capable providers
2. start provider login
3. receive callback on a generic provider callback path
4. pass callback envelope to provider
5. receive canonical `AuthResult`
6. perform JIT upsert
7. refresh external cohorts/profile projection
8. reconcile auto-managed RoleBindings when configured
9. issue Shepherd JWT/session
10. return a provider-agnostic frontend bridge response so browser-based login
    UIs can receive the Shepherd session without exposing the JWT in a callback
    query string

### 1.3 Provider-owned runtime steps

The provider owns:

* redirect URL construction
* provider callback parameter parsing
* token exchange
* remote userinfo lookup
* provider-specific nonce/state quirks
* provider-side retry/backoff rules

Core should not branch on `wecom`, `oidc`, or `ldap` in this flow.

---

## 2. JIT User Center Model

### 2.1 Default rule

The user center should default to:

* create/update user when external login succeeds
* keep only users who have logged in or were explicitly imported
* avoid full enterprise directory mirrors unless an admin explicitly uses
  directory sync

### 2.2 Canonical identity anchor

The JIT upsert rule should use:

* primary external identity: `(auth_provider_id, external_id)`
* conflict checks: `username`, `email`

This aligns with the existing `User` invariants and avoids relying on database
unique failures as control flow.

### 2.3 Supplemental profile projection

Supplemental provider profile data should live in a dedicated projection such as
`UserDirectoryProfile`, not in authoritative `User` fields.

Preferred display-field whitelist:

* `given_name`
* `family_name`
* `preferred_name`
* `phone_number`
* `job_title`
* `organization`
* `organization_unit`
* `locale`
* `avatar_url`

Provider-specific raw attributes may still be retained in a raw blob, but
public UI should render only allowlisted fields.

---

## 3. External Cohorts and RBAC Mapping

### 3.1 Why `external cohort` instead of `group`

`group` is too narrow once Shepherd needs to support:

* OIDC groups
* LDAP groups
* LDAP OUs
* WeCom departments
* WeCom tags

The implementation should standardize these as typed external cohorts.

### 3.2 Mapping rule semantics

A mapping rule should mean:

* `external cohort selector` -> `Shepherd RBAC mutation policy`

Examples:

* cohort `group:ops` -> `viewer` in `test`
* cohort `department:finance` -> `operator` on selected systems
* cohort `tag:dba` -> database-management role

The runtime authorization engine still reads only persisted Shepherd RBAC
records.

### 3.3 Storage direction

The current group-specific naming is too narrow for this standard.
Implementation should prefer a clean generic model such as:

* `external_cohorts`
* `external_cohort_mappings`
* `external_cohort_grants`

instead of extending group-only tables and names.

Because the project is pre-launch, this should be done as a clean rename rather
than by adding compatibility layers.

### 3.4 Reconciliation model

Auto-managed RBAC reconciliation should be explicit:

* provider returns current normalized cohorts
* successful external login upserts the observed cohort set into a
  non-authoritative `external_cohorts` catalog for later admin mapping and
  filtering
* mapping service evaluates current cohort set
* Shepherd groups matching mappings by target binding key
* Shepherd updates persisted `RoleBinding` rows plus `external_cohort_grants`
  metadata
* manual bindings remain untouched

Runtime permission evaluation does not call back into the provider.

---

## 4. WeCom as the First Provider Practice

### 4.1 Positioning

WeCom is the first implementation under the shared standard. It does not define
the core contract.

### 4.2 Login modes

Recommended scope:

* primary: desktop/browser QR login
* secondary: in-WeCom web authorization when practical

Both belong to one `wecom` provider type and should appear as provider-defined
login modes, not separate core auth products.

### 4.3 Canonical field mapping

WeCom-specific user data should be adapted into the shared result:

* stable WeCom user ID -> `external_id`
* provider-configured username rule -> `username`
* Chinese or preferred display label -> `display_name`
* enterprise email if available -> `email`
* department/tag membership -> `cohorts`
* phone/localized names/department labels -> `profile_attributes`

### 4.4 Directory sync relationship

WeCom directory sync remains optional and scoped:

* use it for pre-provisioning, batch review, or admin-driven imports
* do not require it for successful login
* do not make it the default user-center source

This keeps WeCom aligned with the same standard as OIDC and LDAP.

---

## 5. API Direction

Contract-first implementation should likely add a provider-agnostic runtime API
surface similar to:

* `GET /auth/providers`
* `POST /auth/providers/{id}/login/start`
* `GET /auth/providers/{id}/callback`
* callback responses may be HTML bridge pages that `postMessage` the resulting
  Shepherd session back to the initiating frontend window

If additional endpoints are required, they should still preserve the rule that
core endpoints remain provider-agnostic and provider-specific callback/query
details stay inside the adapter.

---

## 6. Interaction with ADR-0048

ADR-0048 remains valid, but should align with this note by using the same
normalized `external cohort` terminology instead of a narrow `groups`-only
shape when the provider can return organizational membership during preview/sync.

Directory sync continues to be:

* optional
* scoped
* result-first
* non-authoritative for RBAC until mapped into Shepherd bindings

---

## 7. Acceptance Criteria

* runtime external-auth core paths use only provider registry/capabilities
* at least two different external providers can share the same start/callback
  core flow with different provider-owned callback semantics
* successful external login performs JIT create/update of canonical `User`
* supplemental profile data is stored outside authoritative core user fields
* runtime authorization code reads only Shepherd RBAC state
* external cohort mapping produces persisted Shepherd bindings instead of direct
  provider-driven authorization
* WeCom QR login works as the first provider without adding WeCom-specific
  branches to core auth runtime
