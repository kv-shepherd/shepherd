#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

if ! command -v node >/dev/null 2>&1; then
  echo "[live-e2e-evidence] ERROR: node is required" >&2
  exit 1
fi

node - "$@" <<'NODE'
const fs = require('fs');
const os = require('os');
const path = require('path');

const defaultManifests = [
  'docs/design/ci/fixtures/live-e2e-evidence-full.passed.json',
  'docs/design/ci/fixtures/live-e2e-evidence-full.failed-early.json',
  'docs/design/ci/fixtures/live-e2e-evidence-preflight.passed.json',
];

const args = process.argv.slice(2);
let requireFullPass = false;
let requireExistingArtifacts = false;
let selfTest = false;
let showHelp = false;
const manifests = [];

for (const arg of args) {
  if (arg === '--require-full-pass') {
    requireFullPass = true;
  } else if (arg === '--require-existing-artifacts') {
    requireExistingArtifacts = true;
  } else if (arg === '--self-test') {
    selfTest = true;
  } else if (arg === '-h' || arg === '--help') {
    showHelp = true;
  } else {
    manifests.push(arg);
  }
}

if (showHelp) {
  console.log(`Usage:
  bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh [options] [manifest...]

Options:
  --require-full-pass          Require mode=full, status=passed, exit codes 0, and policy gates enabled
  --require-existing-artifacts Require referenced artifact paths to exist on disk
  --self-test                  Validate positive fixtures and prove negative fixtures are rejected
`);
  process.exit(0);
}

if (selfTest && manifests.length > 0) {
  console.error('[live-e2e-evidence] ERROR: --self-test does not accept manifest paths');
  process.exit(1);
}

if (manifests.length === 0) {
  if (!selfTest && (requireFullPass || requireExistingArtifacts)) {
    console.error('[live-e2e-evidence] ERROR: strict validation requires at least one manifest path');
    process.exit(1);
  }
  if (!selfTest) {
    manifests.push(...defaultManifests);
  }
}

const topLevelKeys = new Set([
  'schema_version',
  'generated_at',
  'mode',
  'status',
  'exit_code',
  'playwright_exit_code',
  'backend_guard_exit_code',
  'run',
  'artifacts',
  'result_file',
  'policy_gates',
  'cluster',
  'playwright',
  'cleanup',
]);
const artifactKeys = [
  'evidence',
  'result',
  'runner_log',
  'backend_log',
  'playwright_json',
  'playwright_report',
  'playwright_test_results',
];
const disallowedSecretKeys = new Set([
  'database_url',
  'password',
  'token',
  'jwt',
  'secret',
  'kubeconfig',
  'kubeconfig_b64',
  'client_certificate_data',
  'client_key_data',
  'certificate_authority_data',
  'private_key',
]);

const isObject = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value);
const isStringOrNull = (value) => typeof value === 'string' || value === null;
const isNumberOrNull = (value) => typeof value === 'number' || value === null;
const isAbsoluteManifestPath = (value) => typeof value === 'string' && path.isAbsolute(value);
const parseIntegerLike = (value) => {
  if (typeof value === 'number' && Number.isInteger(value)) return value;
  if (typeof value === 'string' && /^-?\d+$/.test(value.trim())) {
    return Number(value.trim());
  }
  return null;
};
const add = (errors, manifestPath, location, message) => {
  errors.push(`${manifestPath}: ${location}: ${message}`);
};
const valueAt = (root, keys) => {
  let current = root;
  for (const key of keys) {
    if (!isObject(current) || !(key in current)) return undefined;
    current = current[key];
  }
  return current;
};
const requireObject = (errors, manifestPath, root, keys) => {
  const value = valueAt(root, keys);
  const location = keys.join('.');
  if (!isObject(value)) {
    add(errors, manifestPath, location, 'must be an object');
    return null;
  }
  return value;
};
const requireType = (errors, manifestPath, root, keys, predicate, description) => {
  const value = valueAt(root, keys);
  const location = keys.join('.');
  if (!predicate(value)) {
    add(errors, manifestPath, location, `must be ${description}`);
    return false;
  }
  return true;
};
const requireEnum = (errors, manifestPath, root, keys, allowed) => {
  const value = valueAt(root, keys);
  if (!allowed.includes(value)) {
    add(errors, manifestPath, keys.join('.'), `must be one of: ${allowed.join(', ')}`);
    return false;
  }
  return true;
};
const scanForSecrets = (errors, manifestPath, value, location = '$') => {
  if (typeof value === 'string') {
    if (/postgres(?:ql)?:\/\//i.test(value)) {
      add(errors, manifestPath, location, 'must not contain a PostgreSQL DSN');
    }
    if (/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(value)) {
      add(errors, manifestPath, location, 'must not contain private key material');
    }
    if (/^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/.test(value)) {
      add(errors, manifestPath, location, 'must not contain JWT-looking token material');
    }
    return;
  }
  if (Array.isArray(value)) {
    value.forEach((entry, index) => scanForSecrets(errors, manifestPath, entry, `${location}[${index}]`));
    return;
  }
  if (!isObject(value)) return;
  for (const [key, entry] of Object.entries(value)) {
    const normalized = key.toLowerCase().replace(/-/g, '_');
    if (normalized !== 'kubeconfig_source' && disallowedSecretKeys.has(normalized)) {
      add(errors, manifestPath, `${location}.${key}`, 'must not be present in evidence manifest');
    }
    scanForSecrets(errors, manifestPath, entry, `${location}.${key}`);
  }
};

const validateArtifact = (errors, manifestPath, manifest, key) => {
  const artifact = requireObject(errors, manifestPath, manifest, ['artifacts', key]);
  if (!artifact) return;
  for (const unknown of Object.keys(artifact)) {
    if (unknown !== 'path' && unknown !== 'exists') {
      add(errors, manifestPath, `artifacts.${key}.${unknown}`, 'unknown artifact property');
    }
  }
  requireType(errors, manifestPath, manifest, ['artifacts', key, 'path'], isStringOrNull, 'a string or null');
  requireType(errors, manifestPath, manifest, ['artifacts', key, 'exists'], (v) => typeof v === 'boolean', 'a boolean');
  const artifactPath = artifact.path;
  if (artifact.exists === true && artifactPath === null) {
    add(errors, manifestPath, `artifacts.${key}`, 'exists=true requires a non-null path');
  }
  if (isAbsoluteManifestPath(artifactPath)) {
    add(errors, manifestPath, `artifacts.${key}.path`, 'must be relative to the repository root');
  }
  if (requireExistingArtifacts && artifactPath !== null && !fs.existsSync(artifactPath)) {
    add(errors, manifestPath, `artifacts.${key}.path`, `does not exist on disk: ${artifactPath}`);
  }
};

const requireResultMatchesTopLevel = (errors, manifestPath, manifest, resultKey, topLevelKey) => {
  const resultValue = manifest.result_file?.[resultKey];
  if (resultValue === undefined) {
    add(errors, manifestPath, `result_file.${resultKey}`, 'is required for full-pass evidence');
    return;
  }
  const parsed = parseIntegerLike(resultValue);
  if (parsed === null) {
    add(errors, manifestPath, `result_file.${resultKey}`, 'must be an integer-like value');
    return;
  }
  if (parsed !== manifest[topLevelKey]) {
    add(errors, manifestPath, `result_file.${resultKey}`, `must match ${topLevelKey}=${manifest[topLevelKey]}`);
  }
};

const requireOptionalResultZero = (errors, manifestPath, manifest, resultKey) => {
  const resultValue = manifest.result_file?.[resultKey];
  if (resultValue === undefined) return;
  const parsed = parseIntegerLike(resultValue);
  if (parsed === null) {
    add(errors, manifestPath, `result_file.${resultKey}`, 'must be an integer-like value when present');
    return;
  }
  if (parsed !== 0) {
    add(errors, manifestPath, `result_file.${resultKey}`, 'must be 0 for full-pass evidence');
  }
};

const requireOptionalStatsZero = (errors, manifestPath, manifest, statsKey) => {
  const stats = manifest.playwright?.json_report?.stats;
  if (!isObject(stats) || !(statsKey in stats)) return;
  const parsed = parseIntegerLike(stats[statsKey]);
  if (parsed === null) {
    add(errors, manifestPath, `playwright.json_report.stats.${statsKey}`, 'must be an integer-like value when present');
    return;
  }
  if (parsed !== 0) {
    add(errors, manifestPath, `playwright.json_report.stats.${statsKey}`, 'must be 0 for full-pass evidence');
  }
};

const validateManifest = (manifestPath) => {
  const errors = [];
  let manifest;

  try {
    manifest = JSON.parse(fs.readFileSync(manifestPath, 'utf8'));
  } catch (error) {
    return [`${manifestPath}: $: failed to parse JSON: ${error.message}`];
  }

  if (!isObject(manifest)) {
    return [`${manifestPath}: $: manifest must be an object`];
  }

  for (const key of topLevelKeys) {
    if (!(key in manifest)) {
      add(errors, manifestPath, key, 'is required');
    }
  }
  for (const key of Object.keys(manifest)) {
    if (!topLevelKeys.has(key)) {
      add(errors, manifestPath, key, 'unknown top-level property');
    }
  }

  requireType(errors, manifestPath, manifest, ['schema_version'], (v) => v === 1, 'the number 1');
  requireType(errors, manifestPath, manifest, ['generated_at'], (v) => typeof v === 'string' && !Number.isNaN(Date.parse(v)), 'an ISO-8601 timestamp string');
  requireEnum(errors, manifestPath, manifest, ['mode'], ['full', 'preflight']);
  requireEnum(errors, manifestPath, manifest, ['status'], ['passed', 'failed', 'preflight_passed', 'preflight_failed']);
  requireType(errors, manifestPath, manifest, ['exit_code'], (v) => typeof v === 'number', 'a number');
  requireType(errors, manifestPath, manifest, ['playwright_exit_code'], isNumberOrNull, 'a number or null');
  requireType(errors, manifestPath, manifest, ['backend_guard_exit_code'], isNumberOrNull, 'a number or null');

  const run = requireObject(errors, manifestPath, manifest, ['run']);
  if (run) {
    requireType(errors, manifestPath, manifest, ['run', 'id'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['run', 'directory'], isStringOrNull, 'a string or null');
    if (isAbsoluteManifestPath(run.directory)) {
      add(errors, manifestPath, 'run.directory', 'must be relative to the repository root');
    }
  }

  const artifacts = requireObject(errors, manifestPath, manifest, ['artifacts']);
  if (artifacts) {
    for (const key of artifactKeys) {
      if (!(key in artifacts)) {
        add(errors, manifestPath, `artifacts.${key}`, 'is required');
      }
    }
    for (const key of Object.keys(artifacts)) {
      if (!artifactKeys.includes(key)) {
        add(errors, manifestPath, `artifacts.${key}`, 'unknown artifact');
      }
    }
    for (const key of artifactKeys) {
      validateArtifact(errors, manifestPath, manifest, key);
    }
  }

  requireObject(errors, manifestPath, manifest, ['result_file']);

  const policy = requireObject(errors, manifestPath, manifest, ['policy_gates']);
  if (policy) {
    requireType(errors, manifestPath, manifest, ['policy_gates', 'skipped'], (v) => typeof v === 'boolean', 'a boolean');
    requireEnum(errors, manifestPath, manifest, ['policy_gates', 'master_flow_test_matrix'], ['required', 'skipped']);
    requireEnum(errors, manifestPath, manifest, ['policy_gates', 'live_e2e_no_mock'], ['required', 'skipped']);
    requireEnum(errors, manifestPath, manifest, ['policy_gates', 'cluster_probe'], ['required', 'skipped']);
  }

  const cluster = requireObject(errors, manifestPath, manifest, ['cluster']);
  if (cluster) {
    requireType(errors, manifestPath, manifest, ['cluster', 'kubeconfig_source'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['cluster', 'current_context'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['cluster', 'api_server_reachable'], (v) => typeof v === 'boolean' || v === null, 'a boolean or null');
    requireType(errors, manifestPath, manifest, ['cluster', 'kubernetes_version'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['cluster', 'kubevirt_api_available'], (v) => typeof v === 'boolean' || v === null, 'a boolean or null');
    requireType(errors, manifestPath, manifest, ['cluster', 'kubevirt_api_versions'], (v) => Array.isArray(v) && v.every((entry) => typeof entry === 'string'), 'an array of strings');
  }

  const playwright = requireObject(errors, manifestPath, manifest, ['playwright']);
  if (playwright) {
    requireType(errors, manifestPath, manifest, ['playwright', 'project'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['playwright', 'json_report'], (v) => v === null || isObject(v), 'an object or null');
  }

  const cleanup = requireObject(errors, manifestPath, manifest, ['cleanup']);
  if (cleanup) {
    requireType(errors, manifestPath, manifest, ['cleanup', 'namespace'], isStringOrNull, 'a string or null');
    requireType(errors, manifestPath, manifest, ['cleanup', 'namespace_vm_cleanup_enabled'], (v) => typeof v === 'boolean', 'a boolean');
    requireType(errors, manifestPath, manifest, ['cleanup', 'review_log_required'], (v) => typeof v === 'boolean', 'a boolean');
  }

  if (manifest.mode === 'full') {
    if (!['passed', 'failed'].includes(manifest.status)) {
      add(errors, manifestPath, 'status', 'full mode requires status passed or failed');
    }
    if (manifest.status === 'passed') {
      if (manifest.playwright_exit_code === null) {
        add(errors, manifestPath, 'playwright_exit_code', 'full passed mode requires a number');
      }
      if (manifest.backend_guard_exit_code === null) {
        add(errors, manifestPath, 'backend_guard_exit_code', 'full passed mode requires a number');
      }
      if (manifest.playwright?.project === null) {
        add(errors, manifestPath, 'playwright.project', 'full passed mode requires a project name');
      }
    }
  }
  if (manifest.mode === 'preflight') {
    if (!['preflight_passed', 'preflight_failed'].includes(manifest.status)) {
      add(errors, manifestPath, 'status', 'preflight mode requires status preflight_passed or preflight_failed');
    }
    if (manifest.playwright_exit_code !== null) {
      add(errors, manifestPath, 'playwright_exit_code', 'preflight mode requires null');
    }
    if (manifest.backend_guard_exit_code !== null) {
      add(errors, manifestPath, 'backend_guard_exit_code', 'preflight mode requires null');
    }
  }

  if (requireFullPass) {
    if (manifest.mode !== 'full') add(errors, manifestPath, 'mode', 'must be full');
    if (manifest.status !== 'passed') add(errors, manifestPath, 'status', 'must be passed');
    if (manifest.exit_code !== 0) add(errors, manifestPath, 'exit_code', 'must be 0');
    if (manifest.playwright_exit_code !== 0) add(errors, manifestPath, 'playwright_exit_code', 'must be 0');
    if (manifest.backend_guard_exit_code !== 0) add(errors, manifestPath, 'backend_guard_exit_code', 'must be 0');
    requireResultMatchesTopLevel(errors, manifestPath, manifest, 'exit_code', 'exit_code');
    requireResultMatchesTopLevel(errors, manifestPath, manifest, 'playwright_exit_code', 'playwright_exit_code');
    requireResultMatchesTopLevel(errors, manifestPath, manifest, 'backend_guard_exit_code', 'backend_guard_exit_code');
    requireOptionalResultZero(errors, manifestPath, manifest, 'failed');
    requireOptionalResultZero(errors, manifestPath, manifest, 'flaky');
    if (manifest.policy_gates?.skipped !== false) add(errors, manifestPath, 'policy_gates.skipped', 'must be false');
    if (manifest.policy_gates?.cluster_probe !== 'required') add(errors, manifestPath, 'policy_gates.cluster_probe', 'must be required');
    if (manifest.artifacts?.playwright_json?.exists !== true) add(errors, manifestPath, 'artifacts.playwright_json.exists', 'must be true');
    if (manifest.playwright?.json_report === null) add(errors, manifestPath, 'playwright.json_report', 'must be present');
    if (manifest.playwright?.json_report?.parse_error) add(errors, manifestPath, 'playwright.json_report.parse_error', 'must be absent');
    requireOptionalStatsZero(errors, manifestPath, manifest, 'unexpected');
    requireOptionalStatsZero(errors, manifestPath, manifest, 'flaky');
    if (manifest.cluster?.current_context === null) add(errors, manifestPath, 'cluster.current_context', 'must be present');
    if (manifest.cluster?.api_server_reachable !== true) add(errors, manifestPath, 'cluster.api_server_reachable', 'must be true');
    if (manifest.cluster?.kubevirt_api_available !== true) add(errors, manifestPath, 'cluster.kubevirt_api_available', 'must be true');
    if (!Array.isArray(manifest.cluster?.kubevirt_api_versions) || manifest.cluster.kubevirt_api_versions.length === 0) {
      add(errors, manifestPath, 'cluster.kubevirt_api_versions', 'must be a non-empty array for full-pass evidence');
    }
    if (manifest.cleanup?.review_log_required !== true) add(errors, manifestPath, 'cleanup.review_log_required', 'must be true');
  }

  scanForSecrets(errors, manifestPath, manifest);
  return errors;
};

const runValidation = (paths) => {
  let failures = [];
  for (const manifestPath of paths) {
    failures = failures.concat(validateManifest(manifestPath));
  }
  return failures;
};

if (selfTest) {
  const originalRequireFullPass = requireFullPass;
  const originalRequireExistingArtifacts = requireExistingArtifacts;

  requireFullPass = false;
  requireExistingArtifacts = false;
  let failures = runValidation(defaultManifests);
  if (failures.length > 0) {
    console.error('[live-e2e-evidence] ERROR: positive fixture validation failed');
    for (const failure of failures) {
      console.error(` - ${failure}`);
    }
    process.exit(1);
  }

  const negativeSecretFailures = runValidation([
    'docs/design/ci/fixtures/live-e2e-evidence-secret.invalid.json',
  ]);
  if (negativeSecretFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: negative secret fixture was not rejected');
    process.exit(1);
  }

  requireFullPass = true;
  requireExistingArtifacts = false;
  const negativePreflightFailures = runValidation([
    'docs/design/ci/fixtures/live-e2e-evidence-preflight.passed.json',
  ]);
  if (negativePreflightFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: preflight fixture was accepted as full-pass evidence');
    process.exit(1);
  }

  const negativeFlakyFailures = runValidation([
    'docs/design/ci/fixtures/live-e2e-evidence-flaky.invalid.json',
  ]);
  if (negativeFlakyFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: flaky fixture was accepted as full-pass evidence');
    process.exit(1);
  }

  const negativeClusterProbeFailures = runValidation([
    'docs/design/ci/fixtures/live-e2e-evidence-cluster-probe.invalid.json',
  ]);
  if (negativeClusterProbeFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: cluster probe fixture was accepted as full-pass evidence');
    process.exit(1);
  }

  const negativeClusterProbeSkippedFailures = runValidation([
    'docs/design/ci/fixtures/live-e2e-evidence-cluster-probe-skipped.invalid.json',
  ]);
  if (negativeClusterProbeSkippedFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: cluster probe skipped fixture was accepted as full-pass evidence');
    process.exit(1);
  }

  const absolutePathFixture = JSON.parse(fs.readFileSync('docs/design/ci/fixtures/live-e2e-evidence-full.passed.json', 'utf8'));
  absolutePathFixture.run.directory = path.resolve('.run/live-e2e/20260528/1300-12345');
  absolutePathFixture.artifacts.evidence.path = path.resolve('.run/live-e2e/20260528/1300-12345/live-e2e.evidence.json');
  const absolutePathFixtureDir = fs.mkdtempSync(path.join(os.tmpdir(), 'shepherd-live-e2e-evidence-'));
  const absolutePathFixturePath = path.join(absolutePathFixtureDir, 'absolute-path.invalid.json');
  fs.writeFileSync(absolutePathFixturePath, `${JSON.stringify(absolutePathFixture, null, 2)}\n`);
  const negativeAbsolutePathFailures = runValidation([absolutePathFixturePath]);
  fs.rmSync(absolutePathFixtureDir, { recursive: true, force: true });
  if (negativeAbsolutePathFailures.length === 0) {
    console.error('[live-e2e-evidence] ERROR: absolute-path fixture was accepted');
    process.exit(1);
  }

  requireFullPass = originalRequireFullPass;
  requireExistingArtifacts = originalRequireExistingArtifacts;
  console.log('[live-e2e-evidence] OK: self-test passed');
  process.exit(0);
}

let failures = runValidation(manifests);

if (failures.length > 0) {
  console.error('[live-e2e-evidence] ERROR: invalid evidence manifest');
  for (const failure of failures) {
    console.error(` - ${failure}`);
  }
  process.exit(1);
}

console.log(`[live-e2e-evidence] OK: ${manifests.length} manifest(s) checked`);
NODE
