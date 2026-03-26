# Design Note: Supplemental CI Scanners

> Status: Proposed
> Related ADRs: ADR-0020, ADR-0039
> Owner: @jindyzhao
> Date: 2026-03-26

## Summary

This note records the staged adoption of supplemental scanners:

- `govulncheck` for reachable Go vulnerability analysis
- `knip` for frontend dead-code and dependency-hygiene analysis
- `gitleaks` for repository-content secret detection
- `npm audit --audit-level=high` for blocking frontend dependency advisory checks

No new ADR is required. These scanners extend existing CI/tooling governance and do not change runtime architecture, API contracts, provider boundaries, or deployment topology.

## Scope

- In scope:
  - local entrypoints in `Makefile`
  - blocking GitHub Actions jobs
  - CI cache policy for Next.js build outputs used by frontend unit/build and Playwright smoke lanes
  - dependency/version tracking in `docs/design/DEPENDENCIES.md`
  - CI policy documentation in `docs/design/ci/README.md`
- Out of scope:
  - changing branch protection / required-check policy
  - upgrading the Go baseline to clear standard-library vulnerability findings
  - broad dead-code cleanup across the frontend

## Pending Changes (Not Yet Normative)

- Affected docs:
  - `docs/design/ci/README.md`
  - `docs/design/DEPENDENCIES.md`
- Affected components:
  - `Makefile`
  - `.github/workflows/ci.yml`
  - `.github/workflows/frontend-tests.yml`
  - `web/package.json`
  - `web/knip.json`

## Rollout Policy

### `govulncheck`

- Entry point: `make govulncheck`
- Current CI mode: blocking
- Local mirror: included in `make pr` / `make pr-ci`

Reasoning:

- `govulncheck` is high-signal and is now viable as a blocking gate because the repository baseline has been upgraded to patched Go `1.25.8`
- blocking it before the patch-level upgrade would have redlined every PR without a code-level remediation path

### `knip`

- Entry point: `make frontend-deadcode-scan`
- Current CI mode: blocking
- Local mirror: included in `make pr` / `make pr-ci`

Reasoning:

- `knip` is valuable for frontend drift and unused exports
- initial dependency declaration issues and dead-code noise have been reduced to zero in the current tree
- the generated OpenAPI TypeScript artifact is explicitly ignored, keeping the gate focused on hand-maintained frontend code

### `gitleaks`

- Entry point: `make secrets-scan`
- Current CI mode: blocking
- Local mirror: included in `make lint`, `make ci-governance`, and `make pr` / `make pr-ci`

Reasoning:

- `gitleaks` adds a fast repository-content secret scan without introducing heavy environment dependencies
- current-tree scanning with `--no-git` keeps the gate focused on the content being submitted, not on historical repository archaeology
- a small repository-local `.gitleaks.toml` allowlist excludes generated artifacts, local cache directories, and deterministic dev/test fixtures with no production value

### `npm audit --audit-level=high`

- Entry point: `make frontend-security-audit`
- Current CI mode: blocking
- Local mirror: included in `make lint`, `make ci-frontend`, and `make pr` / `make pr-ci`

Reasoning:

- the current `web` dependency graph has been reduced to zero high/critical advisories under `npm audit`
- `npm audit --audit-level=high` adds a fast dependency-security gate with no extra external tooling bootstrap
- keeping it in the frontend hygiene lane avoids adding a redundant standalone required job while still making the check blocking

### Deferred: `trivy fs`

- Current CI mode: not enabled

Reasoning:

- `trivy fs` would add database/bootstrap overhead and overlap heavily with the combination of `govulncheck`, `npm audit`, and `gitleaks`
- in the current repository state it is more likely to duplicate known dependency findings than to add unique blocking value
- it becomes more attractive after frontend dependency audit debt is lower and if we want broader filesystem misconfiguration scanning

## Compatibility Notes

- `make pr` remains the canonical local mirror of required GitHub Actions jobs; `make pr-ci` is an alias
- scanner jobs must not silently swallow output; scan results should remain visible in CI logs
- generated OpenAPI TypeScript artifacts should stay ignored by `knip`
- Next.js build caches are persisted for both explicit frontend builds and Playwright smoke's dedicated E2E build output
- Playwright browser binaries are intentionally not cached; Playwright's CI guidance notes that browser-cache restore cost on Linux is often not better than reinstalling

## Open Questions

- Which currently exported frontend helpers are intentionally public seams and deserve allowlisting before `knip` is made blocking?
