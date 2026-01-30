---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "proposed"
date: 2026-01-30
deciders: []
consulted: []
informed: []
---

# ADR-0026: Auth Provider Naming and Standardized Provider Output

> **Review Period**: Until 2026-02-01
> **Discussion**: [Issue #74](https://github.com/kv-shepherd/shepherd/issues/74)
> **Amends**: [ADR-0018](./ADR-0018-instance-size-abstraction.md) (naming alignment)

---

## Context and Problem Statement

Design docs currently use both `auth_providers` and `idp_configs` to represent
OIDC/LDAP provider configuration. The platform intent is to standardize provider
output across OIDC, LDAP, enterprise SSO, Feishu, DingTalk, WeCom, etc., so that
only the adapter/plugin layer changes while core logic remains stable.

## Decision Drivers

* Standardized provider output across many auth backends
* Minimal change to core auth/rbac logic when adding providers
* Consistent naming across UI/API/DB
* Alignment with existing V1 design in ADR-0018

## Considered Options

* **Option 1**: Use `auth_providers` as canonical name
* **Option 2**: Use `idp_configs` as canonical name
* **Option 3**: Keep both with compatibility layer

## Decision Outcome

**Chosen option**: "Use `auth_providers` as canonical name", because it aligns with
V1 design intent for a standardized provider abstraction and avoids long-term
naming drift.

### Consequences

* ✅ Good, because core auth logic depends on a single standardized provider model
* ✅ Good, because adding new auth backends is adapter-only
* 🟡 Neutral, because existing references to `idp_configs` must be reconciled
* ❌ Bad, because any prior `idp_configs` references in docs must be amended

### Confirmation

* Schema definitions include `auth_providers` with OIDC/LDAP fields
* API endpoints and UI labels use "Auth Provider" terminology
* Any `idp_configs` mentions are amended/aliased in design docs

---

## Pros and Cons of the Options

### Option 1: Use `auth_providers`

* ✅ Good, because matches standardized provider abstraction intent
* ✅ Good, because aligns with ADR-0018 table naming
* ❌ Bad, because ADR-0015 uses `idp_config` naming and needs amendment

### Option 2: Use `idp_configs`

* ✅ Good, because matches IdP terminology in ADR-0015
* ❌ Bad, because conflicts with standardized provider abstraction naming

### Option 3: Keep both with compatibility layer

* ✅ Good, because avoids immediate renames
* ❌ Bad, because doubles complexity and invites drift

---

## More Information

### Related Decisions

* [ADR-0015](./ADR-0015-governance-model-v2.md) - IdP schema naming (amended by this ADR)
* [ADR-0018](./ADR-0018-instance-size-abstraction.md) - `auth_providers` table design

### References

* OpenID Connect Discovery: https://openid.net/specs/openid-connect-discovery-1_0.html
* OpenID Connect Core: https://openid.net/specs/openid-connect-core-1_0.html

### Implementation Notes

* Rename remaining docs and examples to `auth_providers`
* Adapter/plugin layer maps external systems to standardized provider output

### Standard Provider Output (Contract)

Adapters MUST normalize all external providers into a common output payload:

| Field | Type | Description |
|-------|------|-------------|
| `provider_id` | string | `auth_providers.id` |
| `auth_type` | string | `oidc` / `ldap` / `sso` / `wecom` / `feishu` / `dingtalk` |
| `external_id` | string | Stable subject identifier from provider |
| `email` | string | User email (may be empty if provider lacks) |
| `display_name` | string | Human-readable name |
| `groups` | string[] | Normalized group list for RBAC mapping |
| `raw_claims` | json | Raw provider claims/attributes (optional, for audit/debug) |

**Rules**:
- Core auth/RBAC logic consumes only this normalized output.
- Provider-specific fields must be mapped in the adapter layer.

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-01-30 | @jindyzhao | Initial draft |
