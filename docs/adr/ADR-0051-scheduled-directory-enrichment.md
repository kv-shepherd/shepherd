---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-03-21
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0051: Scheduled Directory Enrichment for Existing Users

> **Status**: Accepted<br>
> **Accepted On**: 2026-03-23<br>
> **Discussion**: [Issue #402](https://github.com/kv-shepherd/shepherd/issues/402)<br>
> **Extends**: `ADR-0048-directory-sync-capability.md` *(adds a scheduled enrichment mode on top of provider-owned directory sync without turning full mirroring into the default)*<br>
> **Extends**: `ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md` *(keeps JIT user-center construction as the default while allowing scheduled profile/cohort enrichment for existing users)*
>
> 📝 **Design Note**: implementation-facing details are in `docs/design/notes/ADR-0051-scheduled-directory-enrichment.md`.

---

## Context and Problem Statement

Some enterprise deployments authenticate users through one source, but need to
enrich those same users from another source.

Typical example:

* users authenticate via LDAP or a legacy upstream system
* directory metadata such as departments or phone numbers is more complete in
  WeCom or another enterprise directory
* usernames or another stable join key can link the records

The project already decided that:

* JIT provisioning is the default user-center construction rule
* directory sync is optional and provider-owned
* external organization data cannot become direct authorization authority

The open question is how to support scheduled external directory enrichment
without drifting into full mirrored-directory mode or vendor-specific core
logic.

## Decision Drivers

* **ADR-0049 JIT rule**: the platform should not mirror full external directories by default.
* **ADR-0048 sync boundary**: provider-specific directory workflows stay at the edge; core standardizes results.
* **Practical usability**: administrators still need richer department/profile data than the primary auth source may provide.
* **Identity safety**: enrichment should link to existing canonical users through explicit join rules, not silent heuristic takeover.
* **RBAC safety**: external directory metadata must remain input to mapping workflows, not direct runtime authority.
* **Operational efficiency**: the common case is to enrich existing users, not to precreate thousands of never-used accounts.

## Considered Options

* **Option 1**: Full scheduled mirror of external directory into canonical users
* **Option 2**: Scheduled enrichment of existing users linked by explicit join key (chosen)
* **Option 3**: Manual one-off imports only, no scheduler

## Decision Outcome

**Chosen option**: **"Scheduled enrichment of existing users linked by explicit
join key"**, because it preserves the JIT user-center model while still solving
the practical need to supplement user profiles and cohort catalogs from richer
directory sources.

### Normative Decisions

#### 1. Scheduled enrichment is an optional directory-sync mode, not a new core identity model

Providers that already support directory sync may also support scheduled
enrichment.

This does not introduce a second user-center authority. It remains an optional
mode of provider-owned directory sync under ADR-0048.

#### 2. The default scheduled mode is `enrich_existing_only`

The default scheduled enrichment behavior must be:

* fetch records from the external directory
* match them to existing Shepherd users by explicit join rule
* enrich matched users only

The platform must not default to creating canonical users for every synced
directory record.

#### 3. Matching must use an explicit join key rule

Scheduled enrichment must use a provider-configured join rule such as:

* `username`
* `employee_id`
* another explicit stable key

Implicit fuzzy matching is not allowed as the default.

If multiple users match or no user matches, the result must be classified
explicitly rather than silently attached.

#### 4. Scheduled enrichment writes only to non-authoritative projections and cohort catalogs

Scheduled enrichment may update:

* display-only external profile projection
* normalized external cohort catalog
* admin-facing sample or sync-result surfaces

Scheduled enrichment must not directly rewrite:

* the runtime auth authority model
* RBAC decisions
* approval policy state
* core resource state

#### 5. External directory metadata remains non-authoritative for authorization

Departments, tags, groups, OUs, and similar data synced on a schedule may help:

* filtering
* batch selection
* explicit cohort-to-RBAC mapping

They still must not become direct runtime permission inputs.

#### 6. Runtime auth and scheduled enrichment are independent capabilities

A provider such as WeCom may support:

* runtime login
* directory sync
* scheduled enrichment

These capabilities may coexist, but they are independent. Enabling scheduled
enrichment does not require runtime login, and disabling runtime login does not
disable enrichment.

#### 7. Provider-owned scheduling inputs stay outside the core result model

Provider-specific scheduling and selection inputs remain provider-owned, for
example:

* department selectors
* nested-sync flags
* field selection
* schedule cadence or cursor semantics

Core owns only:

* job lifecycle
* canonical match classification
* canonical projection/cohort write rules
* result summary and auditability

#### 8. Enterprise-specific enrichment implementations may live in a separate private repository

Enterprise-specific scheduled enrichment logic may be implemented in a separate
private repository, as long as it targets the public directory-sync and
enrichment contracts defined by Shepherd.

That private repository must not depend on Shepherd internal implementation
packages.

The open-source host repository should own:

* the canonical enrichment semantics
* the join/match safety rules
* the projection/cohort boundary
* the public SDK or common packages for enrichment providers

The private repository may own:

* enterprise-specific directory source adapters
* enterprise-specific field mapping and scheduling defaults
* enterprise deployment wiring

### Consequences

* ✅ Good, because richer external directories can supplement canonical users without becoming the primary identity authority.
* ✅ Good, because the platform can default to enriching real users instead of mirroring unused enterprise populations.
* ✅ Good, because WeCom and similar sources remain provider-specific at workflow level while core stays provider-neutral.
* ✅ Good, because this pattern is reusable beyond WeCom for HR or other directory sources.
* ✅ Good, because enterprise-specific enrichment logic can evolve in a private repository without changing public core semantics.
* ✅ Good, because public enrichment contracts can remain stable while private adapters change independently.
* 🟡 Neutral, because administrators must configure explicit join keys and sync scope carefully.
* 🟡 Neutral, because some organizations may later choose a more aggressive pre-provisioning mode, which should remain opt-in.
* ❌ Bad, because environments lacking a stable cross-system join key may need manual review workflows before enrichment is safe.

### Confirmation

* Scheduled enrichment tests prove the default mode enriches existing users only.
* Match-classification tests prove unmatched and ambiguous records are surfaced explicitly.
* Projection/cohort tests prove scheduled enrichment does not directly change runtime RBAC evaluation.
* CI architecture checks continue to block vendor workflow leakage into core handlers and services.

---

## Pros and Cons of the Options

### Option 1: Full scheduled mirror of the external directory

* ✅ Good, because administrators can see the whole directory immediately.
* ❌ Bad, because it conflicts with ADR-0049's JIT-first user-center model.
* ❌ Bad, because it creates unnecessary identity, sync, and conflict overhead.

### Option 2: Scheduled enrichment of existing users linked by explicit join key (Chosen)

* ✅ Good, because it solves the common "LDAP authenticates, WeCom enriches" case cleanly.
* ✅ Good, because it keeps enrichment provider-specific and user-center authority canonical.
* ✅ Good, because it scales to multiple directory sources without changing core identity rules.
* 🟡 Neutral, because unmatched records still need explicit handling or admin review.

### Option 3: Manual one-off imports only

* ✅ Good, because it is simple.
* ❌ Bad, because it does not solve the drift problem for changing departments/profile data.
* ❌ Bad, because it increases admin toil in exactly the environments that need enrichment most.

---

## More Information

### Related Decisions

* `ADR-0048-directory-sync-capability.md` — result-first directory sync boundary
* `ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md` — JIT user-center rule and external cohort semantics
* `ADR-0026-idp-config-naming.md` — standardized provider output and adapter-only normalization

### References

* Keycloak userinfo docs (Context7 `/keycloak/keycloak/26.5.2`) — standard model for retrieving identity/profile data from an upstream identity source
* HashiCorp go-plugin tutorial (Context7 `/hashicorp/go-plugin`) — stable host contracts and separate plugin implementation repositories

### Implementation Notes

Revisit this ADR if:

* the project later accepts a first-class "preprovision missing users" mode as a separate, opt-in policy
* manual review queues become necessary for ambiguous enrichment joins

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-21 | @jindyzhao | Initial draft |
| 2026-03-23 | @jindyzhao | Marked accepted after the 48-hour review window closed |
