---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"  # proposed | accepted | deprecated | superseded by ADR-XXXX
date: 2026-02-15
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0035: Auth Provider Plugin Boundary and Type Discovery

> **Review Period**: Until 2026-02-17 (48-hour minimum)<br>
> **Discussion**: TBD (Issue link required before acceptance)<br>
> **Amends**: [ADR-0026](./ADR-0026-idp-config-naming.md#standard-provider-output-contract)

---

## Context and Problem Statement

The project must treat `auth providers` as a stable core contract, while allowing
new provider implementations to be added without changing core auth/RBAC logic.
Runtime and frontend had drift risk from provider-specific assumptions
(OIDC/LDAP wording and defaults), which weakens extensibility.

## Decision Drivers

* Keep core auth/RBAC logic provider-agnostic.
* Allow new provider plugins to integrate through a single standard contract.
* Make provider types discoverable by API for frontend schema-driven UI.
* Enforce this boundary with CI, not convention only.

## Considered Options

* **Option 1**: Keep OIDC/LDAP-first flow in core and extend ad hoc.
* **Option 2**: Introduce strict plugin boundary with runtime type registry + discovery API.
* **Option 3**: Move all provider logic into core with static enum and feature flags.

## Decision Outcome

**Chosen option**: "Introduce strict plugin boundary with runtime type registry + discovery API".

### Consequences

* ✅ Good, because core only depends on standard adapter interface.
* ✅ Good, because frontend reads provider types from API, not hardcoded lists.
* ✅ Good, because unknown/non-registered provider types are rejected explicitly.
* 🟡 Neutral, because built-in compatibility types (`oidc`, `ldap`, `sso`, `generic`) remain pre-registered.
* ❌ Bad, because plugin developers must register type metadata before usage.

### Confirmation

* CI gate `check_auth_provider_plugin_boundary.go` must pass.
* Backend exposes `GET /admin/auth-provider-types` and frontend consumes it.
* Runtime auth-provider handlers must resolve adapters via registry only.
* No OIDC/LDAP hardcoded branch logic in runtime auth-provider paths.

---

## Pros and Cons of the Options

### Option 1: Keep OIDC/LDAP-first flow in core

* ✅ Good, because implementation is straightforward in short term.
* ❌ Bad, because every new provider requires core edits.
* ❌ Bad, because frontend/backed drift risk increases over time.

### Option 2: Plugin boundary + discovery API

* ✅ Good, because extension is standardized and discoverable.
* ✅ Good, because it aligns with contract-first and schema-driven frontend.
* ❌ Bad, because initial migration requires API/CI updates.

### Option 3: Static enum + feature flags

* ✅ Good, because behavior is predictable.
* ❌ Bad, because it blocks third-party provider expansion without core release.

---

## More Information

### Related Decisions

* [ADR-0026](./ADR-0026-idp-config-naming.md) - standardized provider output contract
* [ADR-0021](./ADR-0021-api-contract-first.md) - API contract-first governance
* ADR-0034 (proposed, tracked in issue #235) - test-first/spec-driven enforcement

### References

* OpenAPI 3.1 specification: https://spec.openapis.org/oas/v3.1.0.html

### Implementation Notes

* Add registry-backed provider type discovery endpoint: `GET /admin/auth-provider-types`.
* Auth-provider create/update validation must require registered type.
* Frontend create flow must query provider type list from API and remove hardcoded defaults.
* CI blocks provider-specific hardcoded branching in runtime/auth-provider flow.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-02-15 | @jindyzhao | Initial draft |
| 2026-02-15 | @jindyzhao | Status set to proposed pending public review/issue linkage |
