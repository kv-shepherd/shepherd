---
# MADR 4.0 compatible metadata (YAML frontmatter)
status: "accepted"
date: 2026-03-21
deciders: ["@jindyzhao"]
consulted: ["@jindyzhao"]
informed: ["@jindyzhao"]
---

# ADR-0050: Upstream Identity Assertion Runtime Provider for Legacy Systems

> **Status**: Accepted<br>
> **Accepted On**: 2026-03-23<br>
> **Discussion**: [Issue #402](https://github.com/kv-shepherd/shepherd/issues/402)<br>
> **Extends**: `ADR-0035-auth-provider-plugin-boundary.md` *(applies the auth-provider plugin boundary to legacy upstream identity assertions as a runtime provider type)*<br>
> **Extends**: `ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md` *(adds a generic non-OIDC external-auth path without changing core canonical identity/RBAC rules)*<br>
> **Extends**: `ADR-0021-api-contract-first.md` *(requires contract-first runtime APIs for upstream assertion entry and callback/landing handling)*
>
> 📝 **Design Note**: implementation-facing details are in `docs/design/notes/ADR-0050-upstream-identity-assertion-runtime-provider.md`.

---

## Context and Problem Statement

Some enterprise environments already have legacy portals or internal systems that
authenticate users and expose some form of trusted identity assertion, but they
do not speak modern OIDC/SAML end to end.

Examples include:

* opaque token plus `userinfo` endpoint
* opaque token plus introspection endpoint
* trusted reverse-proxy header assertion behind a gateway

The project needs a way to integrate these systems without:

* hardcoding one specific legacy application into core
* trusting plain `username` query parameters
* forcing every legacy system to implement a brand-new Shepherd-specific token protocol before integration is possible

**Core question**: How should Shepherd support passwordless entry from legacy
upstream systems that can present a trusted identity assertion, while preserving
the existing auth-provider plugin boundary and canonical core identity model?

## Decision Drivers

* **ADR-0035 plugin boundary**: runtime auth must resolve providers via registry only and must not hardcode legacy-system branches in core.
* **ADR-0049 external-auth standard**: external login must still end in canonical `AuthResult`, JIT provisioning, and platform-owned RBAC.
* **Security boundary**: Shepherd must never trust unauthenticated `username` handoff from browser-controlled inputs.
* **Real-world deployability**: requiring a legacy system to implement a brand-new Shepherd-specific handoff protocol is often unrealistic.
* **Open-source generality**: the solution must be reusable for multiple legacy systems, not only one internal portal.
* **Contract-first governance**: entry/callback/landing APIs must be standardized before implementation.

## Considered Options

* **Option 1**: Hardcode an application-specific bridge flow in core
* **Option 2**: Introduce a generic upstream identity assertion runtime provider (chosen)
* **Option 3**: Require all legacy systems to migrate to OIDC/Keycloak before Shepherd integration
* **Option 4**: Trust browser-provided `username` or ad hoc headers directly

## Decision Outcome

**Chosen option**: **"Introduce a generic upstream identity assertion runtime
provider"**, because it preserves the plugin-standard runtime contract while
allowing Shepherd to consume already-existing enterprise trust signals without
turning one legacy system into a core architectural concept.

### Normative Decisions

#### 1. Legacy upstream identity entry must be modeled as a runtime auth provider type

Legacy-system integration must use the auth-provider registry and runtime auth
seam defined by ADR-0035 and ADR-0049.

Core must not add:

* application-specific branches
* one-off login handlers
* one-off token-verification code paths

Instead, a provider type may implement a generic upstream assertion pattern.

#### 2. Core only accepts trusted upstream identity assertions, never plain usernames

Shepherd must not authenticate users from:

* `?username=alice`
* arbitrary browser-supplied headers
* inferred knowledge of another system's session

An upstream identity assertion provider must verify identity through one of the
following provider-owned mechanisms:

* upstream token + `userinfo` endpoint
* upstream token + introspection endpoint
* trusted reverse-proxy headers from a deployment-controlled gateway

The provider may use a different internal mechanism, but core must only receive
a canonical `AuthResult` after verification succeeds.

#### 3. The provider owns verification workflow; core owns canonical identity handling

The provider owns:

* how an upstream assertion is obtained
* how the assertion is forwarded to upstream verification
* which endpoint or trust source is used
* how upstream response fields are mapped

The core owns:

* request routing
* request origin/state handling where applicable
* canonical `AuthResult` validation
* JIT user upsert
* Shepherd JWT/session issuance
* auditability and error normalization

#### 4. Upstream assertion providers must normalize into the ADR-0049 runtime contract

After successful verification, the provider must emit the same canonical
runtime auth result shape as any other external provider:

* `external_id`
* `username`
* `display_name`
* `email`
* `enabled`
* optional `cohorts`
* optional display-only profile attributes

This means legacy upstream systems are not a separate core identity model.

#### 5. Authorization remains platform-owned

Even if the upstream verification response contains groups, departments, or
roles, runtime authorization must still remain owned by Shepherd RBAC per
ADR-0049.

Upstream claims may be normalized into external cohorts or profile attributes,
but permission checks must not read them directly.

#### 6. Deployment-controlled trust sources are allowed, but their semantics remain provider-local

Some environments may use a gateway or reverse proxy that injects trusted user
headers after upstream authentication. This is allowed only when:

* the trust boundary is deployment-controlled
* the provider implementation verifies that the request is arriving from that trusted boundary
* the provider still normalizes the result into standard `AuthResult`

Core must not treat reverse-proxy headers as a universal runtime contract.

#### 7. This ADR does not require Shepherd to remove other runtime auth providers

LDAP login, WeCom login, OIDC login, and future providers may coexist.

The upstream assertion provider is an additional provider class for legacy
environments, not a replacement for the broader runtime-auth standard.

#### 8. Enterprise-specific secondary development may live in a separate private repository

Enterprise-specific runtime providers, deployment glue, and adapters may be
implemented in a separate private repository, as long as they implement the
public provider contracts and shared SDK exposed by Shepherd.

That private repository must not depend on Shepherd internal implementation
packages.

The open-source host repository should define:

* the provider contract
* the canonical auth-result boundary
* the security and RBAC rules
* the public SDK or common package that provider implementations import

The private repository may define:

* enterprise-specific upstream assertion transport
* enterprise-specific mapping rules
* deployment-specific trust integration

Public ADRs and core implementation must not encode any one enterprise
integration as a normative requirement.

#### 9. Shared plugin contracts must live in public packages, not host internals

If Shepherd supports private or external runtime provider implementations, the
interfaces, DTOs, and helper glue required by those providers must live in
public shared packages.

Private repositories must not import:

* `internal/...`
* host-only handlers/services
* host-only persistence packages as a substitute for provider contracts

They should import only the public provider SDK surface intended for external
implementation.

### Consequences

* ✅ Good, because legacy enterprise systems can be integrated without becoming core special cases.
* ✅ Good, because Shepherd can reuse existing trust anchors such as userinfo/introspection flows.
* ✅ Good, because security stays stronger than `username` passthrough or guessed sessions.
* ✅ Good, because runtime auth still converges on one canonical user-center/RBAC model.
* ✅ Good, because enterprise-specific secondary development can stay in a separate private repository without polluting the host core.
* ✅ Good, because the host can expose a stable plugin SDK while keeping internals private.
* 🟡 Neutral, because provider implementations may need deployment-specific configuration for how assertions arrive.
* 🟡 Neutral, because some environments may still prefer a gateway or future IdP migration instead of this provider type.
* ❌ Bad, because this does not magically solve environments where no trusted upstream assertion exists at all.

### Confirmation

* Runtime auth handlers resolve legacy upstream assertion providers via registry only.
* No one-off legacy-system names appear in core runtime auth branches.
* Tests prove Shepherd rejects plain username-only handoff.
* Tests prove a verified upstream assertion can normalize into canonical `AuthResult` and create/update users through JIT provisioning.
* Authorization tests prove upstream claims do not bypass Shepherd RBAC.

---

## Pros and Cons of the Options

### Option 1: Hardcode an application-specific bridge in core

* ✅ Good, because the first enterprise integration could be quick.
* ❌ Bad, because it breaks ADR-0035 and turns one internal system into a core concept.
* ❌ Bad, because the next legacy system would require another core exception.

### Option 2: Generic upstream identity assertion runtime provider (Chosen)

* ✅ Good, because it generalizes the legacy-integration pattern without hardcoding one system.
* ✅ Good, because it still fits the existing auth-provider seam.
* ✅ Good, because it keeps identity verification outside browser-controlled username passthrough.
* 🟡 Neutral, because providers still need configuration for endpoint mappings and trust mode.

### Option 3: Require all legacy systems to migrate to OIDC/Keycloak first

* ✅ Good, because the long-term standards story is cleaner.
* ❌ Bad, because it is often operationally or politically unrealistic as a prerequisite.
* ❌ Bad, because it blocks incremental adoption of Shepherd.

### Option 4: Trust browser-provided username or ad hoc headers directly

* ✅ Good, because it looks easy.
* ❌ Bad, because it is not a defensible trust model.
* ❌ Bad, because it bypasses the runtime auth standard entirely.

---

## More Information

### Related Decisions

* `ADR-0035-auth-provider-plugin-boundary.md` — provider registry and plugin-standard runtime boundary
* `ADR-0049-external-auth-runtime-jit-provisioning-and-external-cohort-rbac-mapping.md` — canonical external auth result, JIT provisioning, and RBAC rules
* `ADR-0026-idp-config-naming.md` — standardized provider output contract
* `ADR-0021-api-contract-first.md` — runtime APIs must be standardized before implementation

### References

* OAuth2-Proxy integration docs (Context7 `/oauth2-proxy/oauth2-proxy`) — trusted upstream header patterns behind deployment-controlled gateways
* Keycloak userinfo/introspection docs (Context7 `/keycloak/keycloak/26.5.2`) — standard identity assertion verification patterns
* HashiCorp go-plugin tutorial (Context7 `/hashicorp/go-plugin`) — shared SDK/contracts should stay stable while plugin implementations can evolve independently, potentially in separate repositories

### Implementation Notes

Revisit this ADR if:

* the project later standardizes a first-class shared-gateway mode across all deployments
* a future accepted ADR defines a stronger common abstraction over token-userinfo and trusted-header modes

---

## Changelog

| Date | Author | Change |
|------|--------|--------|
| 2026-03-21 | @jindyzhao | Initial draft |
| 2026-03-23 | @jindyzhao | Marked accepted after the 48-hour review window closed |
