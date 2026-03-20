---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-03-20
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0049: External Auth Runtime, JIT User Provisioning, and External Cohort-to-RBAC Mapping

> **Status**: 🔍 Public Review — 48-hour minimum comment window<br>
> **Review Open**: 2026-03-20<br>
> **Review Closes**: 2026-03-22 (earliest merge date)<br>
> **Discussion**: [Issue #400](https://github.com/kv-shepherd/shepherd/issues/400)<br>
> **Amends**: `ADR-0026-idp-config-naming.md#standard-provider-output-contract` *(clarifies runtime auth result shape, non-authoritative profile data, and cohort normalization)*<br>
> **Amends**: `ADR-0035-auth-provider-plugin-boundary.md` *(extends the auth-provider boundary from admin/discovery into runtime login and callback execution)*<br>
> **Amends**: `ADR-0015-governance-model-v2.md` *(clarifies that external identity data may drive RoleBinding automation, but permissions remain platform-owned)*<br>
> **Extends**: `ADR-0048-directory-sync-capability.md` *(keeps directory sync optional/scoped instead of the default user-center construction path)*
>
> 📝 **Design Note**: implementation-facing details and the WeCom first-provider plan are captured in `docs/design/notes/ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md`.

---

## Context and Problem Statement

The project already treats auth-provider configuration and admin discovery as a
plugin boundary, but runtime login is still not governed by the same standard.
At the same time, enterprise providers such as WeCom, OIDC, LDAP, and future
SSO adapters expose different login workflows and different organizational data
shapes.

The platform needs a stricter rule set before implementing the first external
runtime provider:

* external login workflow must stay at the provider edge
* the core user center must remain canonical and minimal
* platform permissions must remain owned by Shepherd RBAC, not by vendor claims
* external organization data must help administrators batch-manage RBAC without
  becoming direct runtime authority

**Core question**: how should Shepherd standardize runtime external
authentication, user-center provisioning, and external organization mapping so
that WeCom can be the first provider implementation without pushing vendor
workflow into core?

## Decision Drivers

* **ADR-0035 plugin boundary**: auth providers must remain plugin-standard and provider-agnostic at runtime, not only in admin CRUD.
* **ADR-0026 normalized identity contract**: core must consume canonical identity data instead of provider-specific claims or callback fields.
* **ADR-0015 platform RBAC**: authorization decisions must remain based on Shepherd roles and bindings.
* **ADR-0048 optional sync capability**: directory import is optional and scoped, not the default user-center construction path.
* **Core philosophy**: core only defines stable standards and outcomes; provider-specific processes stay at the edge.
* **Operational fit**: storing a full enterprise directory by default is wasteful for V1 and creates avoidable sync/conflict overhead.
* **Admin usability**: external groups/departments/tags should still help batch authorization workflows.
* **Contract-first governance**: runtime auth endpoints and DTOs must be standardized before implementation.

## Considered Options

* **Option 1**: Keep runtime login hardcoded in core and add external providers case by case
* **Option 2**: Standardize runtime auth plugins, JIT user provisioning, and platform-owned RBAC mapping from normalized external cohorts (chosen)
* **Option 3**: Mirror full external directories into Shepherd and authorize directly from provider groups/departments

## Decision Outcome

**Chosen option**: **"Option 2: Standardize runtime auth plugins, JIT user
provisioning, and platform-owned RBAC mapping from normalized external
cohorts"**, because it keeps the core canonical and provider-agnostic while
still letting external identity systems improve administrator workflows.

### Normative Decisions

#### 1. External auth runtime must use the auth-provider plugin boundary end to end

For external auth providers, runtime login must resolve a provider adapter via
the auth-provider registry and must not hardcode vendor branches in core.

The provider owns:

* login-mode specifics
* redirect/callback parameter semantics
* token exchange
* remote user-info fetch
* provider-specific validation quirks

The core owns:

* request routing
* CSRF/state correlation primitives
* canonical auth-result validation
* JIT user upsert
* Shepherd JWT/session issuance
* auditability and error normalization

#### 2. JIT provisioning is the default user-center rule for all external auth providers

All external auth providers must default to **just-in-time provisioning**:

* a user is created or updated when they successfully log in
* the platform stores users who have logged in or who were explicitly imported
  by an administrator
* full-directory mirroring is not the default construction path for the user
  center

`DirectorySyncCapability` remains optional per ADR-0048 and is used only for
scoped pre-provisioning or administrative import workflows.

#### 3. Core user identity remains canonical and minimal

The authoritative identity model remains the canonical Shepherd `User` plus the
stable external link:

| Field | Purpose |
|-------|---------|
| `auth_provider_id` | Which provider instance authenticated the user |
| `external_id` | Stable provider-side identity key |
| `username` | Canonical Shepherd username |
| `display_name` | Canonical Shepherd display name |
| `email` | Canonical Shepherd email candidate |
| `enabled` | Whether the user is active in Shepherd |

Provider-specific claims or profile blobs must not become authoritative user
fields in core.

#### 4. External organization data must be normalized as external cohorts

Core must not standardize vendor terms such as "OIDC groups", "LDAP OUs",
"WeCom departments", or "enterprise tags" as separate first-class core models.

Instead, providers may normalize them into a common **external cohort** shape:

| Field | Purpose |
|-------|---------|
| `kind` | Cohort type such as `group`, `department`, `ou`, `tag` |
| `key` | Stable provider-scoped cohort identifier |
| `display_name` | Human-readable label |

External cohorts are **not permissions**. They are input for display, filtering,
and explicit RBAC mapping only.

#### 5. Platform RBAC remains the sole authorization authority

Runtime authorization checks must read only Shepherd RBAC state such as:

* `Role`
* `RoleBinding`
* `ResourceRoleBinding`

Runtime authorization must not directly read:

* OIDC claims/groups
* LDAP groups/OUs
* WeCom departments/tags
* raw profile attributes

External cohorts may only affect access after they are transformed into
Shepherd-managed RBAC records through explicit mapping rules or administrator
actions.

#### 6. External cohort mapping is allowed, but it maps into Shepherd RBAC

The platform may support mapping rules such as:

* external cohort -> Shepherd role
* external cohort -> allowed environments
* external cohort -> scoped resource binding policy

But the persisted runtime result must always be Shepherd RBAC records.

This means:

* external identity data may trigger or maintain auto-managed RoleBindings
* administrators may batch-grant RBAC using cohort membership as a selector
* removing or changing a mapping changes future Shepherd-managed bindings
* permission evaluation still reads only Shepherd RBAC tables

#### 7. Supplemental external profile data is display-only and stored outside authoritative core fields

Providers may return supplemental profile data such as:

* phone number
* preferred name
* localized name
* job title
* department label
* avatar URL

These values must be stored in a dedicated projection separate from
authoritative `User` fields. They are informational only and must not drive:

* authentication success
* RBAC
* approval policy
* runtime orchestration
* validation rules

#### 8. One provider may expose multiple login modes without changing the core contract

A provider may support multiple login modes under the same provider type.

Example:

* WeCom QR login for desktop/browser use
* WeCom in-app web authorization for users already inside the WeCom client

Core must see these only as provider-defined login modes behind one runtime
contract. Core must not fork business logic per vendor login mode.

### Consequences

* ✅ Good, because WeCom/OIDC/LDAP can share one runtime auth standard without flattening their provider workflows into core.
* ✅ Good, because the user center stores only users who matter to the platform by default.
* ✅ Good, because administrators can still batch-manage permissions using external organization signals.
* ✅ Good, because platform authorization remains auditable and deterministic in Shepherd RBAC.
* ✅ Good, because directory sync remains optional and scoped instead of becoming a mandatory enterprise directory mirror.
* 🟡 Neutral, because providers now need both runtime-auth adaptation and optional sync adaptation.
* 🟡 Neutral, because existing group-only wording in older ADRs/design docs must be generalized to cohort wording over time.
* ❌ Bad, because this ADR rejects the simpler but weaker "read provider claims directly during authorization" pattern.

### Confirmation

* Runtime external-auth handlers resolve providers via registry only.
* No runtime path hardcodes WeCom/OIDC/LDAP vendor branches in core auth flow.
* JIT login tests prove successful external login can create and update canonical users.
* Authorization tests prove permission checks do not read external cohorts or raw profile attributes directly.
* Mapping tests prove external cohorts only affect access through persisted Shepherd RBAC records.
* Directory-sync tests continue to prove sync is optional and scoped, not the default user-center construction path.
* CI architecture checks block provider-specific workflow leakage into runtime auth and directory-sync core paths.

---

## Pros and Cons of the Options

### Option 1: Keep runtime login hardcoded in core

Continue using provider-specific branches and special cases in runtime login.

* ✅ Good, because the first provider can be added quickly.
* ❌ Bad, because each new provider reopens core auth flow and increases drift.
* ❌ Bad, because it conflicts with ADR-0035's plugin-standard direction.

### Option 2: Runtime plugin standard + JIT + platform-owned RBAC mapping (Chosen)

Providers own login workflow; core owns canonical user records and RBAC state.

* ✅ Good, because it cleanly separates provider process from core standards.
* ✅ Good, because it scales from WeCom to OIDC/LDAP without redefining the core user center.
* ✅ Good, because it lets departments/groups/tags help admin workflows without becoming direct permissions.
* 🟡 Neutral, because provider and mapping contracts must be defined carefully up front.

### Option 3: Full mirrored directory + direct external authorization

Mirror whole directories by default and treat external org data as runtime
authority.

* ✅ Good, because some enterprise admin operations appear convenient at first.
* ❌ Bad, because it makes the core user center vendor-shaped and operationally heavy.
* ❌ Bad, because runtime permissions become dependent on external provider semantics instead of Shepherd RBAC.
* ❌ Bad, because it conflicts with the project's "core standard, edge process" philosophy.

---

## More Information

### Related Decisions

* `ADR-0015-governance-model-v2.md` — Shepherd RBAC remains authoritative.
* `ADR-0021-api-contract-first.md` — runtime auth APIs and DTOs must be standardized before implementation.
* `ADR-0026-idp-config-naming.md` — normalized provider output remains the adapter contract.
* `ADR-0035-auth-provider-plugin-boundary.md` — auth providers remain plugin-standard.
* `ADR-0048-directory-sync-capability.md` — directory sync remains optional and result-first.

### References

* [HashiCorp go-plugin tutorial](https://github.com/hashicorp/go-plugin/blob/main/docs/extensive-go-plugin-tutorial.md)
* [Backstage external integrations](https://github.com/backstage/backstage/blob/master/docs/features/software-catalog/external-integrations.md)
* [SCIM Core Schema RFC 7643](https://www.rfc-editor.org/rfc/rfc7643.html)

### Implementation Notes

Revisit this ADR if:

* Shepherd decides to support true multi-provider account linking as a first-class identity model
* external providers need custom hosted login UIs beyond redirect/callback patterns
* tenant boundaries require organization data to affect more than display, filtering, and mapping inputs

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-20 | @jindyzhao | Initial draft |
| 2026-03-20 | @jindyzhao | Published for 48-hour public review |
