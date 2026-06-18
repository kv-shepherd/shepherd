# Live E2E Validation SOP

> **Status**: Active operational guidance
> **Scope**: `scripts/run_e2e_live.sh`, Playwright live specs, real backend,
> PostgreSQL, and real KubeVirt provider validation.

## Purpose

Live E2E validation is the project-level acceptance path for the roadmap item
"login, VM request, approval, delivery, power, delete, VNC/serial console, and
directory-sync paths" against a real runtime. It is intentionally separate from
mock smoke tests.

Mock route handlers, `test.skip()` calls, and seeded-only assertions are not
valid evidence for live E2E completion.

The current Playwright configuration follows the standard CI pattern of explicit
projects, CI retries, first-retry traces, screenshots on failure, and a
`webServer` entry that starts a dedicated Next.js server. Live tests use the
`live-chromium` project and run with reduced retries because the suite creates
stateful resources.

## Required Inputs

| Input | Required | Notes |
|-------|----------|-------|
| PostgreSQL | Yes | Default runner starts isolated Docker PostgreSQL; `--no-db-wrapper` requires `DATABASE_URL` |
| KubeVirt-capable cluster | Yes | The cluster must support the provider operations under test |
| Kubeconfig | Yes | Set `E2E_KUBECONFIG_B64` or place `k8s-admin.yaml` at the repository root |
| `kubectl` | Yes | Required to resolve context, probe the Kubernetes API server, and prove KubeVirt API discovery |
| Atlas CLI | Yes | Required for startup migrations; set `ATLAS_EXEC_PATH`, install `atlas`, or run `make live-e2e-install-atlas` |
| Node dependencies | Yes | `npm ci --prefix web` |
| Playwright Chromium | Yes | `cd web && npx playwright install chromium` |
| Admin bootstrap password | Defaulted | Runner uses `admin/admin` then `E2E_NEW_PASSWORD=admin123` unless overridden |
| Cleanup policy | Default enabled | `E2E_CLEANUP_NAMESPACE_VMS=1` removes VMs in `E2E_NAMESPACE` |

Use a disposable namespace and cluster policy for live E2E. The runner defaults
to `E2E_NAMESPACE=e2e-live`.

## Preflight Gates

Run the standalone readiness gate before a release-evidence run:

```bash
bash scripts/run_e2e_live.sh --preflight-only
# or
make live-e2e-readiness
```

This validates local tooling, Atlas CLI availability, live E2E policy gates,
kubeconfig presence and basic shape, Kubernetes API reachability, KubeVirt API
discovery, PostgreSQL mode, fixed port conflicts, and Playwright availability
without starting PostgreSQL, the backend server, Next.js, or browser tests.

The live runner executes these gates unless `E2E_SKIP_PREFLIGHT_GATES=1` is set:

```bash
go run docs/design/ci/scripts/check_master_flow_test_matrix.go
bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
```

`check_master_flow_test_matrix.go` verifies required master-flow stages and live
step markers. `check_live_e2e_no_mock.sh` blocks Playwright network mocking and
`test.skip()` in `*-live.spec.ts` files.

Do not disable preflight gates for release evidence.
Do not set `E2E_PREFLIGHT_CLUSTER_PROBE=0` for release evidence; that debug
override skips the same API discovery proof required by the completion
manifest. The evidence manifest records the override as
`policy_gates.cluster_probe=skipped`, and strict full-pass validation rejects
that manifest.

## Default Isolated Run

Use the default mode when Docker is available and the machine can create a local
PostgreSQL container:

```bash
export E2E_KUBECONFIG_B64="$(base64 -w0 /path/to/kubeconfig)"
bash scripts/run_e2e_live.sh --background
```

The command starts a detached run and writes state under `.run/live-e2e/`.
Poll status without streaming the full log:

```bash
bash scripts/run_e2e_live.sh --status
```

Foreground mode is better for CI and debugging:

```bash
export E2E_KUBECONFIG_B64="$(base64 -w0 /path/to/kubeconfig)"
bash scripts/run_e2e_live.sh --foreground
```

## Existing Database Run

Use `--no-db-wrapper` only when a known PostgreSQL instance is already prepared:

```bash
export DATABASE_URL='postgres://shepherd:pass@127.0.0.1:5432/shepherd_test?sslmode=disable'
export E2E_KUBECONFIG_B64="$(base64 -w0 /path/to/kubeconfig)"
bash scripts/run_e2e_live.sh --foreground --no-db-wrapper
```

The runner builds and starts a local backend server, seeds baseline data, seeds
API-managed live fixtures, runs low-level E2E fixtures, then starts Playwright.

## Manual GitHub Workflow

After the workflow file is present on the default branch, maintainers can use
the manual **Live E2E Evidence** GitHub Actions workflow for operator-controlled
release evidence. It is a `workflow_dispatch` workflow only; it is not required
CI and does not run for pull requests or normal pushes.

Configure repository or organization secrets before dispatching the workflow:

| Secret | Required | Notes |
|--------|----------|-------|
| `E2E_KUBECONFIG_B64` | Yes | Base64-encoded kubeconfig for the disposable validation cluster |
| `E2E_ADMIN_USERNAME` | No | Overrides the default admin username |
| `E2E_ADMIN_PASSWORD` | No | Overrides the default initial admin password |
| `E2E_NEW_PASSWORD` | No | Overrides the default forced-change password |

The workflow inputs are:

| Input | Default | Notes |
|-------|---------|-------|
| `mode` | `full` | Use `preflight` for readiness only; use `full` for release evidence |
| `namespace` | `e2e-live` | Disposable namespace used by the run |
| `cleanup_namespace_vms` | `true` | Sets `E2E_CLEANUP_NAMESPACE_VMS` to clean up VMs on exit |

The workflow provisions PostgreSQL as a GitHub Actions service, installs the
go.mod-pinned Atlas CLI version with `make live-e2e-install-atlas`, installs
frontend dependencies and Playwright Chromium, then runs:

```bash
bash scripts/run_e2e_live.sh --preflight-only --no-db-wrapper
# or, for full release evidence:
bash scripts/run_e2e_live.sh --foreground --no-db-wrapper
make ci-live-e2e-latest-evidence
```

It uploads `.run/live-e2e/**` as a workflow artifact with hidden paths enabled
because the evidence directory is under `.run/`. Use a disposable validation
cluster and do not target an environment whose logs, resource names, or evidence
bundle metadata cannot be retained in GitHub Actions artifacts.

## Targeted Runs

Pass Playwright arguments after `--`:

```bash
bash scripts/run_e2e_live.sh --foreground -- \
  --project=live-chromium \
  web/tests/e2e/master-flow-live.spec.ts
```

Use targeted runs for debugging only. Release evidence should run the full
`live-chromium` project.

## Environment Controls

| Variable | Default | Purpose |
|----------|---------|---------|
| `E2E_USERNAME` | `admin` | Live admin user |
| `E2E_PASSWORD` | `admin` | Initial password before forced change |
| `E2E_NEW_PASSWORD` | `admin123` | Password after forced change |
| `E2E_NAMESPACE` | `e2e-live` | Namespace for VM requests and cleanup |
| `E2E_CLUSTER` | `e2e-cluster` | Cluster fixture name |
| `E2E_TEMPLATE` | `e2e-template` | Template fixture name |
| `E2E_SIZE` | `e2e-small` | Instance-size fixture name |
| `E2E_VM_RUNNING_ID` | `vm-e2e-running` | Seeded running VM ID used by console and power flows |
| `E2E_VM_STOPPED_ID` | `vm-e2e-stopped` | Seeded stopped VM ID used by lifecycle flows |
| `E2E_BACKEND_CRITICAL_GUARD` | `1` | Fail run on high-signal backend log errors |
| `E2E_BACKEND_STRICT_GUARD` | `0` | Optional stricter backend log pattern gate |
| `E2E_CLEANUP_NAMESPACE_VMS` | `1` | Cleanup VMs in `E2E_NAMESPACE` on exit |

## Evidence Requirements

A live E2E pass should record:

| Evidence | Where |
|----------|-------|
| Runner result | `.run/live-e2e/**/live-e2e.result` |
| Evidence manifest | `.run/live-e2e/**/live-e2e.evidence.json` |
| Runner log | `.run/live-e2e/**/live-e2e.log` |
| Backend log | Path printed as `backend log` |
| Playwright JSON report | `.run/live-e2e/**/playwright-results.json` |
| Playwright report | `.run/live-e2e/**/playwright-report/` when generated |
| Playwright test results | `.run/live-e2e/**/test-results/` |
| Cluster target | Redacted kubeconfig context, Kubernetes API probe, and KubeVirt API discovery result |
| Cleanup status | Runner log cleanup section |

For failed runs, keep the evidence manifest, result file, backend log, and
Playwright trace before retrying. If the run fails before Playwright starts, the
failed evidence manifest may have null Playwright/backend-guard exit codes and
`result_file.phase` records the failing runner phase. The default Playwright
settings capture trace on first retry and screenshots on failure.

Use `--evidence-file <path>` only when a CI artifact collector requires a fixed
manifest path. Keep evidence paths relative to the repository root; the default
per-run path is preferred for local/background runs.

Validate a completed manifest before attaching it to release evidence:

```bash
bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  --require-full-pass \
  --require-existing-artifacts \
  .run/live-e2e/<date>/<run>/live-e2e.evidence.json
```

Live E2E is outside required GitHub CI. The run is long-running and depends on a
real KubeVirt-capable cluster, kubeconfig provenance, operator cleanup review,
and runtime credentials that GitHub-hosted CI should not own. Use the
latest-manifest validator manually:

```bash
make ci-live-e2e-latest-evidence
```

Preserve `.run/live-e2e/<date>/<run>/` with the release evidence bundle. The
latest-manifest validator only selects manifests whose JSON body reports
`mode=full`; preflight manifests are ignored because they are readiness
evidence, not completion evidence. In short, preflight manifests are ignored by
the latest full-pass validator.

## Pass Criteria

The run is acceptable release evidence only when all conditions hold:

1. `live-e2e.result` reports `exit_code=0`.
2. `live-e2e.evidence.json` reports `mode=full`, `status=passed`, and
   `exit_code=0`.
3. `playwright-results.json` exists for the run and the evidence validator sees
   no Playwright `unexpected` or `flaky` tests in JSON stats.
4. `live-e2e.result` and `live-e2e.evidence.json` agree on the final,
   Playwright, and backend-guard exit codes.
5. Playwright has no failed or flaky tests.
6. The backend critical guard reports `backend_guard_exit_code=0`.
7. `check_live_e2e_no_mock.sh` and `check_master_flow_test_matrix.go` both
   passed during the run.
8. `policy_gates.cluster_probe=required`.
9. `cluster.api_server_reachable=true`, `cluster.kubevirt_api_available=true`,
   and `cluster.kubevirt_api_versions` is non-empty.
10. Namespace cleanup either succeeded or has an explicit, ticketed follow-up for
   leftover live test resources.

## Failure Triage

| Symptom | First check |
|---------|-------------|
| Kubeconfig rejected | Verify `E2E_KUBECONFIG_B64` decodes to a kubeconfig without local file references, exec plugins, proxy URLs, or unsafe TLS settings |
| No cluster option in approval | Confirm the live cluster fixture is enabled and health detection reached `HEALTHY` |
| VM never reaches `RUNNING` | Check CDI image import, storage class defaults, KubeVirt events, and worker log lines in the backend log |
| Console request fails | Confirm the target VM is running and the environment requires the expected approval branch |
| Cleanup fails | Use the logged VM IDs, approve pending delete tickets, then confirm the namespace is empty |

## Related Files

| File | Role |
|------|------|
| [run_e2e_live.sh](../../scripts/run_e2e_live.sh) | Live E2E orchestration script |
| [live-e2e-evidence-baseline.md](../design/ci/live-e2e-evidence-baseline.md) | ADR-0058 evidence manifest contract |
| [check_live_e2e_evidence_manifest.sh](../design/ci/scripts/check_live_e2e_evidence_manifest.sh) | Evidence manifest schema and release-pass validation gate |
| [playwright.config.ts](../../web/playwright.config.ts) | Playwright projects, server startup, retries, trace, and screenshot policy |
| [check_live_e2e_no_mock.sh](../design/ci/scripts/check_live_e2e_no_mock.sh) | No-mock/no-skip policy gate |
| [check_master_flow_test_matrix.go](../design/ci/scripts/check_master_flow_test_matrix.go) | Master-flow stage and live marker coverage gate |
| [master-flow-tests.json](../design/traceability/master-flow-tests.json) | Stage-to-test mapping |
