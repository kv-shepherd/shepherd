---
status: "accepted"
date: 2026-05-28
deciders: ["@jindyzhao"]
consulted: []
informed: []
---

# ADR-0058: Live E2E Evidence Bundle Baseline

> **Extends**: [ADR-0030](./ADR-0030-design-documentation-layering-and-fullstack-governance.md)

---

## Context and Problem Statement

Real-cluster live E2E validation is valuable but too slow and environment-bound
to require in GitHub CI. Shepherd still needs a deterministic evidence format so
operator-run live validation can be reviewed without relying on screenshots,
terminal scrollback, or ambiguous latest-run selection.

## Decision Drivers

* Keep live E2E outside required GitHub CI.
* Make operator-run live validation evidence machine-readable.
* Prevent kubeconfig paths, tokens, private registries, and cluster-specific
  values from being committed.
* Distinguish readiness/preflight runs from full live completion evidence.

## Considered Options

* **Option 1**: Treat live E2E as ad hoc operator notes.
* **Option 2**: Emit a structured evidence manifest and validate fixtures in CI.
* **Option 3**: Require live E2E against a real cluster for every PR.

## Decision Outcome

**Chosen option**: "Option 2", because it keeps GitHub CI practical while making
manual live validation auditable when operators choose to run it.

### Normative Decisions

* Live E2E remains a manual/operator-run validation path, not a required GitHub
  CI job.
* A live run emits `live-e2e.evidence.json` plus structured Playwright artifacts
  under the run output directory.
* The manifest records run mode, result, timestamps, commit, environment
  fingerprint, artifact paths, and validation phases without embedding secrets.
* Full evidence must include a phase proving that KubeVirt API discovery succeeded
  before release completion is claimed.
* Latest-run lookup must select the latest full-run evidence when release
  completion requires a full live result.
* CI validates schema-compatible fixtures and negative fixtures, but does not
  require access to a real cluster.

### Consequences

* Live validation is reviewable when available.
* CI remains feasible for GitHub-hosted PRs.
* Release readiness can require real evidence without coupling every PR to a
  long-running cluster job.
* Operators must still run the real live path in an approved environment before
  claiming live-cluster completion.

### Confirmation

This ADR is implemented when:

* `scripts/run_e2e_live.sh` emits the evidence manifest and structured artifacts.
* Fixture validation covers passing, failing, preflight, secret-leak, and
  skipped-cluster-probe scenarios.
* The latest-full evidence selector ignores preflight-only runs.
* `make ci-live-e2e-evidence` and `make pr` pass without real cluster access.

## References

* [Live E2E evidence baseline](../design/ci/live-e2e-evidence-baseline.md)
* [Live E2E operations guide](../operations/live-e2e-validation.md)
