# Private Repository Guide

This guide describes how to maintain enterprise auth-provider implementations in
a separate private repository without turning Shepherd into a long-lived fork.

Related ADRs:

- `ADR-0035`
- `ADR-0050`
- `ADR-0051`

## Summary

Shepherd exposes a public auth-provider SDK surface under
`pkg/authproviderplugin`, while enterprise-specific provider implementations may
live in a separate private repository. The goal is to keep public changes
focused on stable capability boundaries and keep enterprise-specific behavior in
private code, without letting the private repository depend on Shepherd
`internal/...`.

## Scope

- In scope:
  - how to split work between the public repository and a private provider
    repository
  - how to sequence public SDK changes and private implementation changes
  - how to version and validate private-repository upgrades against the public
    host
- Out of scope:
  - new plugin architecture decisions
  - enterprise-specific provider behavior or deployment defaults
  - release automation for downstream private repositories

## Repository Roles

### Public Repository Responsibilities

The public Shepherd repository owns:

- the host product and runtime behavior
- the public provider SDK surface in `pkg/authproviderplugin`
- canonical provider-facing DTOs, capability interfaces, and structured errors
- CI boundary enforcement proving external consumers can compile without
  `internal/...`
- general provider author guidance

### Private Repository Responsibilities

The private repository owns:

- enterprise-specific provider implementations
- deployment-specific trust integration, transport, and mapping rules
- enterprise defaults, rollout wiring, and local operational glue
- private CI that proves the implementation still compiles and tests against the
  selected public-host version

The private repository must not import Shepherd `internal/...`.

## Current Packaging Model

The public repository now provides a reusable server entrypoint seam under:

- `kv-shepherd.io/shepherd/pkg/serverbootstrap`

That means a private repository can build an enterprise server entrypoint by:

1. blank-importing its own provider registration packages
2. calling `serverbootstrap.Main()` from its own `cmd/server-enterprise`

The private repository still must not import Shepherd `internal/...`.

At the current KubeVirt `v1.8.0` baseline, downstream enterprise repositories
must also carry the same `k8s.io/kube-openapi` replace used by the public host
module, because that upstream dependency is not yet consumable as a plain tagged
version through the transitive graph.

Do not hide a private repository inside the public repository with `.gitignore`;
keep enterprise packaging in the private repository and depend on public
packages only.

## Change Routing Rules

Use the following rule of thumb when new work arrives:

1. If the change introduces or adjusts a reusable provider capability boundary,
   it belongs in the public repository first.
2. If the change only affects one enterprise integration's mapping, transport,
   or deployment semantics, it belongs in the private repository.
3. If the private repository cannot proceed without touching Shepherd
   `internal/...`, treat that as a public-SDK gap and fix the public repository
   first.

Applied example:

- generic WeCom multi-department sync behavior belongs in the public host
- enterprise-specific department-name defaults or local field mapping belong in
  the private repository
- runtime auth-provider rows remain runtime/business configuration rather than
  deployment-time `config.yaml`

## Recommended Delivery Sequence

1. Evaluate whether the requested behavior is general or enterprise-specific.
2. If general, land the public repository change first:
   - SDK/API boundary update
   - docs or note update if needed
   - CI smoke or boundary gate update
3. Tag or otherwise pin the public repository version after the change lands.
4. Update the private repository dependency to that public version.
5. Implement the enterprise-specific provider logic in the private repository.
6. Validate both:
   - public repository CI still proves SDK external consumability
   - private repository CI proves the enterprise implementation still compiles
     and behaves correctly

## Versioning and Upgrade Discipline

- Private repositories should depend on an explicit public version or commit, not
  float on public `main`.
- Public `pkg/...` changes should aim for source-compatible evolution whenever
  possible.
- If a public SDK change would force private implementation rewrites, document
  the migration in the public repository before upgrading the private one.

## CI Expectations

### Public Repository

The public repository should continue to block on:

- external-consumer smoke for `pkg/authproviderplugin`
- plugin-boundary checks that reject `internal/...` imports from public examples
  or smoke modules

### Private Repository

The private repository should add its own checks for:

- accidental imports of public-host `internal/...`
- provider implementation compile/test coverage against the pinned public
  version
- enterprise-specific integration validation

## Operating Principle

The public repository is not a general SDK product like `client-go`; it remains
the host product plus a small, stable extension surface. The private repository
extends that surface, but should never need to understand the host's internal
package layout.
