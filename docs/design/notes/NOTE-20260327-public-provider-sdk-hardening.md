# Design Note: Public Provider SDK Hardening Follow-Up

> Status: Active hardening (initial execution slice completed)
> Related ADRs: ADR-0035, ADR-0050, ADR-0051
> Owner: @jindyzhao
> Date: 2026-03-27

## Summary

The provider plugin boundary is already an accepted architectural decision.
What remains incomplete is execution hardening: proving that a separate
repository can consume the public auth-provider SDK without touching
`internal/...`, and tightening CI so provider-facing contracts do not drift back
into host-only packages.

This note does not introduce a new decision. It records the remaining follow-up
work required to finish the execution of existing ADRs.

## Scope

- In scope:
  - strengthen `pkg/authproviderplugin` as the only public import path for
    runtime/admin/directory provider authors
  - add an external-consumer smoke module that compiles against `pkg/...`
  - enforce the external-consumer smoke in CI
  - extend plugin-boundary CI checks so public examples/templates/smoke modules
    do not import `internal/...`
- Out of scope:
  - new plugin architecture decisions
  - moving enterprise-specific adapters into this repository
  - widening private runtime services or persistence packages for external use

## Why This Matters

`pkg/authproviderplugin` already exists and is the correct public seed, but the
repository still lacks an end-to-end proof that:

1. a separate repository can implement providers by importing only `pkg/...`
2. CI will fail if provider authors are forced back onto `internal/...`

Without those checks, the current state remains "documented intent" rather than
"enforced contract."

## Completed in This Slice

1. `pkg/authproviderplugin` remains the single public SDK entrypoint for
   provider authors.
2. A nested external-consumer compile smoke module now exists under
   `tools/sdk-smoke/authproviderplugin-external` and imports only
   `kv-shepherd.io/shepherd/pkg/authproviderplugin`.
3. A public server entrypoint seam now exists under `pkg/serverbootstrap` so a
   private repository can compile its own `cmd/server-enterprise` without
   importing Shepherd `internal/...`.
4. `make ci-governance` now runs the external-consumer smoke module as a
   blocking check.
5. Plugin-boundary CI validation now fails if public smoke code imports
   `internal/...`.
6. Separate-repository author guidance is now documented in
   `plugins/authprovider/README.md` and `plugins/authprovider/PRIVATE_REPOSITORY_GUIDE.md`.

## Remaining Long-Term Hardening

1. Keep the public `pkg/authproviderplugin` surface source-compatible as
   provider-facing requirements evolve.
2. Revisit whether currently aliased public DTOs should graduate from
   `internal/provider/*contract` into first-class public definitions if SDK
   churn becomes frequent.
3. Add equivalent external-consumer smoke coverage if additional public plugin
   SDK surfaces are introduced beyond auth-provider plugins.

## Non-Goals

This note does not require all provider DTO definitions to move out of
`internal/...` immediately. The short-term requirement is stronger: the public
SDK must be consumable and stable from outside the repository. Internal package
layout can continue to evolve as long as the public package remains source
compatible.

## Acceptance Signals

- A nested external-consumer module compiles and tests successfully using only
  `pkg/authproviderplugin`.
- A private-repository style `cmd/server-enterprise` can compile against
  `pkg/serverbootstrap` without importing `internal/...`.
- `make ci-governance` includes the smoke module check.
- `check_auth_provider_plugin_boundary.go` fails if public provider examples or
  smoke modules import `internal/...`.
- Provider authors can be pointed to one public import path instead of a mix of
  `pkg/...` and host internals.
