# ADR-0050 Design Note: Upstream Identity Assertion Runtime Provider

> **Status**: Accepted (ADR-0050 accepted on 2026-03-23)
> **Related ADR**: [ADR-0050](../../adr/ADR-0050-upstream-identity-assertion-runtime-provider.md)

## Purpose

This note captures implementation-facing details for a runtime auth provider
that verifies identity from an upstream system without making that system a core
concept.

## Design Goals

* Reuse the existing runtime auth provider seam from ADR-0049.
* Support legacy upstream systems that already expose a usable trust signal.
* Avoid unsafe username passthrough.
* Keep platform authorization owned by Shepherd RBAC.

## Supported Trust Modes

The provider may support one or more of these provider-local trust modes:

1. `token_userinfo`
   - client presents an upstream token
   - provider calls configured `userinfo` endpoint
2. `token_introspection`
   - client presents an upstream token
   - provider calls configured introspection endpoint
3. `trusted_gateway_headers`
   - provider reads deployment-controlled upstream headers after gateway authentication

These modes are provider-internal implementation details. Core sees only one
runtime provider contract.

## Recommended Provider Configuration Shape

Provider-local config may include:

* `login_entry_url`
* `callback_param_name`
* `state_param_name`
* `return_to_param_name`
* `trust_mode`
* `userinfo_endpoint`
* `introspection_endpoint`
* `incoming_token_transport`
  - `authorization_bearer`
  - `query`
  - `cookie`
  - `header`
* `incoming_token_name`
* `upstream_token_transport`
  - `authorization_bearer`
  - `query`
  - `header`
  - `form`
* `upstream_token_name`
* `trusted_header_username`
* `trusted_header_email`
* `trusted_header_external_id`
* `trusted_header_display_name`
* `trusted_header_enabled`
* `trusted_header_cohorts`
* `trusted_header_cohort_kind`
* `trusted_gateway_cidrs`
* `username_path`
* `display_name_path`
* `email_path`
* `external_id_path`
* `enabled_path`
* `active_path`
* `cohort_path`
* `cohort_kind`
* `profile_attribute_paths`
* `request_timeout_seconds`

This config belongs to the provider, not to core runtime auth.

## Runtime Flow

### Flow A: token -> userinfo

1. Browser arrives at a Shepherd landing/start endpoint with an upstream token already available through deployment-approved means.
2. Core routes to the configured provider.
3. Provider forwards the token to configured `userinfo` endpoint.
4. Provider validates upstream response shape.
5. Provider maps upstream fields into canonical `AuthResult`.
6. Core applies canonical validation, JIT upsert, JWT issuance, and audit.

### Flow B: token -> introspection

1. Browser arrives with upstream token.
2. Provider calls configured introspection endpoint.
3. Provider requires `active == true` or equivalent upstream success contract.
4. Provider maps identity fields into canonical `AuthResult`.
5. Core continues with standard runtime flow.

### Flow C: trusted gateway headers

1. Gateway authenticates the request upstream.
2. Gateway injects trusted headers.
3. Provider verifies request origin is within configured trust boundary.
4. Provider maps trusted header values into canonical `AuthResult`.
5. Core continues with standard runtime flow.

Current implementation note:

* trusted-header mode relies on the callback/runtime request envelope carrying
  the request `RemoteAddr`, so provider-local CIDR checks can be performed
  without pushing gateway trust logic into core auth semantics

## Security Constraints

The implementation must reject:

* plain `?username=...` without verification
* arbitrary browser headers
* direct dependence on another system's private session store
* legacy-system-specific code branches in core handlers/services

For trusted gateway mode, the provider must never assume that all headers are
trusted. It must verify the deployment trust boundary explicitly.

## Core/Edge Boundary

### Core-owned

* runtime routing
* CSRF/state handling when applicable
* canonical auth result validation
* JIT provisioning
* Shepherd session/JWT issuance
* RBAC enforcement

### Provider-owned

* assertion retrieval details
* userinfo/introspection invocation
* trusted-header parsing
* upstream field mapping
* provider-local validation quirks

## Private Repository Support

This provider class is explicitly intended to support enterprise-specific
secondary development in a separate private repository.

Recommended split:

### Public host repository

Owns:

* runtime auth provider contract
* canonical `AuthResult`
* JIT user provisioning service
* RBAC and security rules
* public provider SDK/common package

### Private integration repository

Owns:

* concrete upstream integration adapters
* deployment-specific trust glue
* enterprise mapping defaults
* enterprise runbooks and deployment manifests

The private repository should depend on the public shared SDK/contracts rather
than copying or forking core runtime auth semantics.

It must not import Shepherd `internal/...` packages.

## Recommended Public SDK Surface

At minimum, the public SDK surface for this provider class should expose:

* runtime auth provider interface
* canonical auth result DTOs
* any public helper DTOs needed for provider registration or capability description

If additional helper logic is needed for external implementations, it should be
published through stable public packages rather than by widening host internals.

### Current repository implication

The current repository already exposes an initial public provider surface under:

* `pkg/authproviderplugin`

That package is the correct direction for external/private provider
implementations. As private-repository support becomes formalized, the project
should strengthen this surface so that it serves as the stable SDK for provider
implementations, rather than requiring any direct dependency on host internals.

### Current SDK gaps to close

To make separate-repository runtime providers practical, the public SDK should
be strengthened in these areas:

1. clearly document that provider registration currently happens through the
   public admin registration surface, while runtime capability is discovered via
   optional interface implementation
2. re-export provider-facing error types such as runtime-start validation
   errors so external implementations do not need `internal/...`
3. keep runtime capability DTOs and helper types stable under `pkg/...`
4. document a single public import path for provider authors

## Testing Requirements

Minimum tests:

* rejects plain username-only handoff
* verifies token -> userinfo success path
* verifies token -> introspection success path if supported
* verifies trusted-header mode rejects untrusted origin
* proves canonical `AuthResult` reaches JIT provisioning path
* proves upstream roles/groups are not used directly in permission checks

## First Implementation Guidance

The first implementation may target a generic `upstream_token_userinfo`
provider, because that fits the most common legacy-environment pattern best.

Even in that first implementation:

* naming must remain generic
* config fields must remain provider-local
* runtime core must never mention any concrete legacy system by name
