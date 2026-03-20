---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-03-19
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0048: Directory Sync Capability for Auth Providers

> **Status**: 🔍 Public Review — 48-hour minimum comment window<br>
> **Review Open**: 2026-03-19<br>
> **Review Closes**: 2026-03-21 (earliest merge date)<br>
> **Discussion**: [Issue #396](https://github.com/kv-shepherd/shepherd/issues/396)<br>
> **Extends**: `ADR-0035-auth-provider-plugin-boundary.md` *(adds optional directory-sync capability within the auth-provider plugin boundary)*<br>
> **Extends**: `ADR-0024-provider-interface-capability-composition.md` *(keeps directory sync as a small optional capability, not a base-interface expansion)*
>
> 📝 **Design Note**: Implementation-facing details are in `docs/design/notes/ADR-0048-directory-sync-capability.md`. The implementation PR must follow the contract in this ADR, not the superseded process-first draft.

---

## Context and Problem Statement

Enterprise deployments often need to import and synchronize users from external
directories such as WeCom, Feishu, DingTalk, Azure AD, LDAP-backed org trees, or
future SCIM-compatible systems.

The platform already treats auth providers as plugins with a stable core
contract. The open question is not whether directory sync should exist, but
**what the core is allowed to standardize**.

The project philosophy is strict:

* Core defines canonical standards and invariant outcomes.
* Peripheral adapters may keep provider-specific workflows and quirks.
* Core must require result compatibility, but must not hardcode vendor process.

The earlier process-first draft assumed all directory providers fit the same
"department tree + attribute picker + preview + manual sync" flow. That is too
prescriptive for this codebase's plugin boundary.

**Core question**: How should the platform support directory-based user import
while keeping provider-specific workflow at the edge and standardizing only the
canonical import result in core?

## Decision Drivers

* **ADR-0035 plugin boundary**: Core auth/RBAC runtime must remain plugin-standard and provider-agnostic.
* **ADR-0024 capability composition**: Directory sync must remain an optional capability, not a mandatory expansion of `AuthProviderAdminAdapter`.
* **ADR-0026 standardized provider output**: Core must consume canonical identity fields, not vendor-specific field names or flow steps.
* **ADR-0049 external auth/runtime standard**: normalized external cohorts may be carried for display and mapping input, but they are not direct permissions.
* **ADR-0021 contract-first governance**: The directory-sync API must stay provider-agnostic at the HTTP contract boundary even when providers use different request shapes.
* **Core philosophy**: Core only governs consistent results; provider-specific process belongs at the edge.
* **Identity safety**: Sync must respect global `User` invariants such as username/email uniqueness and stable external identity linkage.
* **ADR-0006 async model**: Bulk directory import remains an async job.
* **Schema-driven UI reuse**: Core may render a generic form from plugin-supplied schema, but must not assume one universal directory workflow.

## Considered Options

* **Option 1**: Provider-specific sync modules with custom endpoints and UI
* **Option 2**: Generic process contract with fixed steps (`ListDepartments`, `AvailableAttributes`, `Preview`, `Sync`)
* **Option 3**: Generic normalized import contract with provider-owned request schema/workflow (chosen)

## Decision Outcome

**Chosen option**: **"Option 3: Generic normalized import contract with
provider-owned request schema/workflow"**, because it matches the project's core
philosophy: the core standardizes canonical user-import results and job
semantics, while each provider keeps control of how it gathers, filters, and
shapes source data before adaptation.

### Normative Decisions

#### 1. `DirectorySyncCapability` remains optional, but moves to the result boundary

Auth-provider adapters that support directory import **MAY** implement
`DirectorySyncCapability`.

The capability contract must standardize:

* provider-owned sync request schema
* canonical preview/import result items
* canonical sync job status and summary

The capability contract must **not** standardize vendor workflow primitives such
as "departments", "organizational units", "attribute picker", or any other
provider-specific selection process as universal first-class core concepts.

#### 2. Providers own sync workflow input; core accepts only opaque provider request payload

Core APIs carry a `provider_request` JSON object whose structure is defined by
the adapter's `request_schema`.

Examples:

* WeCom may use department IDs and field-selection options.
* LDAP may use base DN, filters, and nested search flags.
* Azure AD may use group selectors or delta tokens.
* A future push-oriented bridge may use a minimal request or scheduled cursor.

Core handlers, workers, and database schemas must treat `provider_request` as an
opaque adapter-owned payload. Core must not inspect provider-specific keys.

#### 3. Core standardizes canonical import result records

All providers must normalize imported users into a canonical record shape before
core persistence:

| Field | Purpose |
|-------|---------|
| `external_id` | Stable provider-side identity key |
| `username` | Canonical Shepherd username candidate |
| `display_name` | Canonical Shepherd display name |
| `email` | Canonical Shepherd email candidate |
| `cohorts` | Normalized external cohort refs if provider can supply them |
| `attributes` | Provider-specific raw attributes for display/audit only |

Core auth/RBAC/approval/runtime logic must depend only on canonical identity
fields and must not branch on provider-specific attribute keys or cohort kinds.
Normalized cohorts are informational and may be consumed only by explicit
mapping or batch-management services that produce Shepherd RBAC records.

#### 4. Conflict classification is a core responsibility

Directory sync must classify identity conflicts before persistence. At minimum,
the contract must distinguish:

* same `(auth_provider_id, external_id)` identity
* username collision
* email collision
* ambiguous existing local user

`conflict_resolution` policies apply only after classification. Core must not
rely on database unique-constraint failures as the primary decision mechanism.

#### 5. Provider-specific raw attributes must not be persisted on `User` core fields

Canonical `User` fields remain authoritative for auth and governance. Raw
directory attributes are stored in a dedicated provider-profile projection, not
in `User.metadata`.

This projection is informational only and must not drive:

* authentication
* RBAC
* approval policy
* runtime orchestration
* validation rules

#### 6. Async job contract remains core-owned

Directory imports run asynchronously via River Queue. Core owns:

* job enqueue semantics
* job lifecycle/status
* job summary counters
* auditability of request snapshot and result summary

Providers own only source-data acquisition and adaptation into canonical import
records.

#### 7. API surface is reduced to standardized result-oriented endpoints

The approved API surface is:

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/admin/auth-providers/{id}/directory/descriptor` | Return capability metadata and `request_schema` |
| `POST` | `/admin/auth-providers/{id}/directory/preview` | Preview canonical import result for `provider_request` |
| `POST` | `/admin/auth-providers/{id}/directory/sync` | Trigger async sync job using `provider_request` |
| `GET` | `/admin/auth-providers/{id}/directory/sync-jobs` | List sync jobs |
| `GET` | `/admin/auth-providers/{id}/directory/sync-jobs/{jobId}` | Get one sync job |

`preview` is a synchronous read-only operation over provider-owned request
input. `sync` enqueues an asynchronous import job per ADR-0006.

The earlier dedicated `/departments` and `/attributes` endpoints are rejected as
universal core concepts.

### Consequences

* ✅ Good, because core now standardizes only canonical results and job semantics.
* ✅ Good, because WeCom/LDAP/Azure AD/Feishu can keep different request workflows without core API churn.
* ✅ Good, because the existing schema-driven UI pattern can still render a generic request form from provider-supplied schema.
* ✅ Good, because raw provider data is isolated from canonical `User` fields.
* ✅ Good, because future providers with non-department-based selection models no longer require a new core abstraction.
* 🟡 Neutral, because provider authors must define a request schema in addition to producing normalized records.
* 🟡 Neutral, because the core preview table becomes canonical-field-first rather than vendor-workflow-first.
* ❌ Bad, because this ADR rejects the earlier simpler process-first draft and requires rework of the proposed note/API/storage model before implementation.

### Confirmation

* Core directory-sync handlers and workers never inspect provider-specific request keys.
* No runtime path hardcodes WeCom/LDAP/Azure/Feishu-specific workflow branches.
* Preview/sync tests prove two materially different providers can use the same endpoints with different `provider_request` shapes.
* Conflict-resolution tests cover `external_id`, `username`, and `email` collision paths.
* Raw directory attributes are stored outside core `User` fields and are explicitly non-authoritative.
* CI architecture checks continue to enforce auth-provider plugin boundary rules.

---

## Pros and Cons of the Options

### Option 1: Provider-specific sync modules

Each provider gets its own API endpoints, handlers, and UI flow.

* ✅ Good, because every provider can model its own workflow perfectly.
* ❌ Bad, because it breaks ADR-0035 by forcing core/API/frontend changes for every new provider.
* ❌ Bad, because canonical identity conflict rules would drift across providers.

### Option 2: Generic process contract with fixed workflow steps

Assume every provider exposes the same department/attribute/preview workflow.

* ✅ Good, because the first implementation feels straightforward.
* ❌ Bad, because it standardizes process instead of standardizing result.
* ❌ Bad, because it cannot cleanly represent providers that select by groups, filters, cursors, or non-browse flows.
* ❌ Bad, because it pushes vendor workflow vocabulary into core API and UI.

### Option 3: Generic normalized import contract with provider-owned workflow (Chosen)

Providers define input shape and acquisition process; core persists only
canonical import results.

* ✅ Good, because it matches the project's "core defines standards, edge owns process" philosophy.
* ✅ Good, because it keeps the auth-provider seam extensible without process leakage.
* ✅ Good, because it reuses schema-driven UI without pretending all providers share the same workflow.
* 🟡 Neutral, because adapters do more adaptation work.

---

## More Information

### Related Decisions

* `ADR-0035-auth-provider-plugin-boundary.md` — Auth providers remain plugin-standard and discoverable.
* `ADR-0024-provider-interface-capability-composition.md` — Optional capability composition pattern.
* `ADR-0021-api-contract-first.md` — Contract-first governance for the standardized directory-sync API surface.
* `ADR-0026-idp-config-naming.md` — Canonical provider output contract and adapter-only normalization.
* `ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md` — JIT user-center rule and normalized external-cohort semantics.
* `ADR-0041-power-operation-approval-requirement-service.md` — Parallel precedent: core keeps canonical semantics, provider routing stays peripheral.
* `ADR-0006-unified-async-model.md` — Async execution model for sync jobs.

### References

* [SCIM 2.0 RFC 7644](https://tools.ietf.org/html/rfc7644)
* [Go Specification - Interface Embedding](https://github.com/golang/go/blob/master/doc/go_spec.html)
* [HashiCorp go-plugin tutorial](https://github.com/hashicorp/go-plugin/blob/main/docs/extensive-go-plugin-tutorial.md)
* [Backstage external integrations](https://github.com/backstage/backstage/blob/master/docs/features/software-catalog/external-integrations.md)

### Implementation Notes

Revisit this ADR if:

* frontend plugin surfaces become available and providers want custom admin UX beyond schema-driven forms
* multi-provider user linking becomes a first-class identity model
* push-based sync becomes a first-class runtime mode

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-19 | @jindyzhao | Initial process-first draft published for review |
| 2026-03-19 | @jindyzhao | Reworked to result-first contract: provider-owned workflow, core-owned canonical import semantics |
| 2026-03-20 | @jindyzhao | Aligned canonical import wording with normalized external-cohort standard from ADR-0049 draft |
