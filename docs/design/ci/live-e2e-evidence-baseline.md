# Live E2E Evidence Baseline

> **Authority**: ADR-0058. This document defines the implementation-level
> evidence shape for `scripts/run_e2e_live.sh`.

## Purpose

The live E2E runner is the manual release-evidence path for proving the
master-flow against a real backend, PostgreSQL, and a real K8s/KubeVirt
cluster. It is intentionally outside required GitHub CI because it needs
operator-controlled credentials, a real cluster target, and a longer execution
window. A release claim needs durable evidence, not just a terminal success
line.

`live-e2e.evidence.json` is the machine-readable manifest that binds one run to
its result file, logs, Playwright structured report, backend guard outcome, and
non-secret cluster context.

## Artifact Layout

For a full run, the default artifact layout is:

| Artifact | Default path |
|----------|--------------|
| Evidence manifest | `.run/live-e2e/<date>/<run>/live-e2e.evidence.json` |
| Runner result | `.run/live-e2e/<date>/<run>/live-e2e.result` |
| Runner log | `.run/live-e2e/<date>/<run>/live-e2e.log` |
| Backend log | `.run/live-e2e/<date>/<run>/shepherd-e2e-server.log` |
| Playwright JSON report | `.run/live-e2e/<date>/<run>/playwright-results.json` |
| Playwright HTML report | `.run/live-e2e/<date>/<run>/playwright-report/` |
| Playwright test results | `.run/live-e2e/<date>/<run>/test-results/` |

`--evidence-file` and `RUN_EVIDENCE_FILE` may override only the evidence
manifest path. The rest of the run artifacts should remain in the run directory
for release evidence unless a CI artifact collector requires a copied path.
Manifest paths must stay relative to the repository root so release evidence does
not carry operator workstation paths.

## Manifest Fields

The manifest must contain these top-level fields:

| Field | Meaning |
|-------|---------|
| `schema_version` | Evidence schema version. Starts at `1`. |
| `generated_at` | ISO-8601 timestamp for manifest generation. |
| `mode` | `preflight` or `full`. |
| `status` | `passed`, `failed`, `preflight_passed`, or `preflight_failed`. |
| `exit_code` | Final shell exit code for the validation mode. |
| `playwright_exit_code` | Playwright process exit code for full runs, otherwise null. |
| `backend_guard_exit_code` | Backend critical-log guard exit code for full runs, otherwise null. |
| `run` | Non-secret run identity and local artifact root. |
| `artifacts` | Paths to result, logs, Playwright reports, and test results. |
| `policy_gates` | Whether release-critical gates ran, including live no-mock, master-flow test-matrix, and cluster discovery probes. |
| `cluster` | Kubeconfig source class, resolved current context, and non-secret Kubernetes/KubeVirt discovery probe results. |
| `playwright` | Project, reporter paths, and parsed JSON reporter stats when available. |

The manifest must not include:

* kubeconfig bytes
* database URLs
* passwords
* JWTs or API tokens
* TLS private keys or certificate bodies
* request/response payload bodies from live tests

## Failure Evidence

Full live E2E runs must write best-effort failure evidence after the run
directory is allocated, even when failure happens before Playwright starts. This
keeps background runs auditable when backend build, backend startup, seeding, or
late policy gates fail.

For early full-run failures:

* `mode` is `full`.
* `status` is `failed`.
* `exit_code` is the shell exit code.
* `playwright_exit_code` may be null if Playwright did not start.
* `backend_guard_exit_code` may be null if the backend guard did not run.
* `result_file.phase` records the last runner phase reached.

Early failure evidence is not completion evidence. It is a triage artifact.

## Completion Criteria

A real-cluster live E2E run is acceptable completion evidence only when:

1. `mode` is `full`.
2. `status` is `passed`.
3. `exit_code`, `playwright_exit_code`, and `backend_guard_exit_code` are all
   `0`.
4. `policy_gates.skipped` is `false`.
5. `policy_gates.cluster_probe` is `required`.
6. `artifacts.playwright_json` exists and contains Playwright JSON reporter
   output.
7. The evidence, result, runner log, backend log, and Playwright JSON artifact
   paths exist on disk.
8. The runner log includes
   `cleanup review: namespace_vm_cleanup status=passed` for the live namespace.
9. `cluster.authenticated_context` is `true`.
10. `cluster.api_server_reachable` is `true`.
11. `cluster.kubevirt_api_available` is `true`, with at least one
   `cluster.kubevirt_api_versions` entry.

Preflight manifests are readiness evidence only. They do not prove the roadmap
live E2E item complete because no backend, browser suite, or real provider
workflow has run.

Release-evidence readiness must still prove the local tooling and cluster
target. The live runner therefore requires Atlas CLI, `kubectl`, a reachable
Kubernetes API server, an authenticated kubeconfig context, and KubeVirt API
discovery before it starts the backend/browser path.
`E2E_PREFLIGHT_CLUSTER_PROBE=0` is a local debugging
override only. The runner records that override as
`policy_gates.cluster_probe=skipped`, and strict full-pass validation rejects
the manifest even if later artifacts exist.

## Governance

`check_design_doc_governance.sh` protects the ADR-0058 contract by requiring:

* the ADR and this design document to exist
* `--evidence-file` to remain documented and implemented
* the default `live-e2e.evidence.json` path to remain documented
* Playwright JSON reporter wiring to remain available
* live E2E operations docs to include the manifest in pass criteria

`check_live_e2e_evidence_manifest.sh` protects the manifest schema itself. By
default it validates repository fixtures for full-pass, full early-failure, and
preflight-pass manifests. Operators can also point it at a real run:

```bash
bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  --require-full-pass \
  --require-existing-artifacts \
  .run/live-e2e/<date>/<run>/live-e2e.evidence.json
```

The real-run mode verifies that completion evidence is a full pass, policy gates
were not skipped, Playwright JSON output is present, and referenced artifact
paths exist on disk. For completion evidence it also requires the manifest to
record that cluster probing was not skipped plus a successful Kubernetes API
server probe, authenticated kubeconfig context, and KubeVirt API discovery
result. Strict full-pass validation with `--require-existing-artifacts` also
requires evidence, result, runner log, backend log, and Playwright JSON
artifacts to exist, namespace VM cleanup to be enabled, and the runner log to contain
`cleanup review: namespace_vm_cleanup status=passed`.

`find_latest_live_e2e_full_evidence.mjs` supports manual release evidence
selection. It scans `.run/live-e2e/`, ignores readiness-only preflight
manifests, and selects the newest manifest whose JSON body reports `mode=full`.
The selector uses `generated_at` when it is parseable and falls back to file
mtime for older or partial manifests.

## Manual Release Evidence

After an operator-controlled live E2E run, validate the latest full manifest
locally before attaching the evidence bundle to release notes:

```bash
make ci-live-e2e-latest-evidence
```

This target is not part of required GitHub CI. It is manual release evidence,
because GitHub-hosted CI cannot reliably provide the required real KubeVirt
cluster, kubeconfig provenance, cleanup review, and long-running browser window.

Archive the `.run/live-e2e/<date>/<run>/` directory, or copy it into the release
evidence store used by the deployment team. Failed live runs are triage
artifacts, not completion evidence.

For full-pass evidence, the gate also compares the structured manifest against
the runner result summary:

* `result_file.exit_code`, `result_file.playwright_exit_code`, and
  `result_file.backend_guard_exit_code` must match the top-level numeric exit
  codes.
* `result_file.failed` and `result_file.flaky` must be `0` when present.
* Playwright JSON `stats.unexpected` and `stats.flaky` must be `0` when the JSON
  reporter provides those fields.

The same script also has a `--self-test` mode. It validates the positive
fixtures and proves negative fixtures are rejected, including:

* a full-run failure before Playwright starts
* a manifest containing secret-like material
* a preflight manifest passed through `--require-full-pass`
* a full-pass manifest that reports flaky Playwright results
* a full-pass manifest without successful Kubernetes/KubeVirt discovery probes
* a full-pass manifest generated with cluster preflight probing skipped
* a full-pass manifest whose runner log lacks cleanup
  `namespace_vm_cleanup status=passed`
