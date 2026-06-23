#!/usr/bin/env bash
# Validate the machine-readable production deployment evidence manifest.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/check_production_evidence.sh [--file PATH] [--require-production-ready] [--require-existing-artifacts] [--require-live-e2e-pass] [--self-test]

Options:
  --file PATH                  Evidence JSON to validate.
  --require-production-ready   Require every required go-live check to be passed.
  --require-existing-artifacts Require artifact paths to exist on disk.
  --require-live-e2e-pass      Require the referenced live E2E evidence to be a full pass.
  --self-test                  Generate positive and negative fixtures and validate strict mode.
  -h, --help                   Show this help.

Without --file, the checked-in example manifest is schema-validated only.
EOF
}

evidence_file="docs/operations/production-deployment-evidence.example.json"
require_production_ready=0
require_existing_artifacts=0
require_live_e2e_pass=0
self_test=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --file)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        echo "ERROR: --file requires a non-empty path" >&2
        exit 2
      fi
      evidence_file="$2"
      shift 2
      ;;
    --require-production-ready)
      require_production_ready=1
      shift
      ;;
    --require-existing-artifacts)
      require_existing_artifacts=1
      shift
      ;;
    --require-live-e2e-pass)
      require_live_e2e_pass=1
      shift
      ;;
    --self-test)
      self_test=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v node >/dev/null 2>&1; then
  echo "ERROR: node is required for production evidence validation" >&2
  exit 2
fi

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "${ROOT_DIR}"

if [[ "${self_test}" -eq 1 ]]; then
  if [[ "${evidence_file}" != "docs/operations/production-deployment-evidence.example.json" \
    || "${require_production_ready}" -ne 0 \
    || "${require_existing_artifacts}" -ne 0 \
    || "${require_live_e2e_pass}" -ne 0 ]]; then
    echo "ERROR: --self-test does not accept --file or strict validation flags" >&2
    exit 2
  fi

  fixture_root=".run/production-evidence-validator-self-test-$$"
  cleanup() {
    rm -rf "${fixture_root}"
  }
  trap cleanup EXIT

  SELF_TEST_ROOT="${fixture_root}" node <<'NODE'
const fs = require('fs');
const path = require('path');

const root = process.env.SELF_TEST_ROOT;
const liveRoot = path.join(root, 'live-e2e');
fs.mkdirSync(liveRoot, { recursive: true });

const clone = (value) => JSON.parse(JSON.stringify(value));
const touchFile = (file, contents = 'ok\n') => {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, contents);
  return file;
};
const touchDir = (dir) => {
  fs.mkdirSync(dir, { recursive: true });
  return dir;
};

const liveFixture = JSON.parse(fs.readFileSync('docs/design/ci/fixtures/live-e2e-evidence-full.passed.json', 'utf8'));
liveFixture.run.directory = liveRoot;
liveFixture.artifacts.evidence.path = path.join(liveRoot, 'live-e2e.evidence.json');
liveFixture.artifacts.result.path = touchFile(path.join(liveRoot, 'live-e2e.result'), 'exit_code=0\nplaywright_exit_code=0\nbackend_guard_exit_code=0\nfailed=0\nflaky=0\n');
liveFixture.artifacts.runner_log.path = touchFile(path.join(liveRoot, 'live-e2e.log'), 'cleanup review: namespace_vm_cleanup status=passed namespace=e2e-live\n');
liveFixture.artifacts.backend_log.path = touchFile(path.join(liveRoot, 'shepherd-e2e-server.log'), 'backend completed\n');
liveFixture.artifacts.playwright_json.path = touchFile(path.join(liveRoot, 'playwright-results.json'), '{"stats":{"unexpected":0,"flaky":0}}\n');
liveFixture.artifacts.playwright_report.path = touchDir(path.join(liveRoot, 'playwright-report'));
liveFixture.artifacts.playwright_test_results.path = touchDir(path.join(liveRoot, 'test-results'));
for (const artifact of Object.values(liveFixture.artifacts)) artifact.exists = true;
fs.writeFileSync(liveFixture.artifacts.evidence.path, `${JSON.stringify(liveFixture, null, 2)}\n`);

const artifactPaths = {
  compose_config: touchFile(path.join(root, 'compose-config.txt'), 'docker compose config --quiet passed\n'),
  compose_ps: touchFile(path.join(root, 'compose-ps.json'), '[]\n'),
  compose_images: touchFile(path.join(root, 'compose-images.json'), '[]\n'),
  postgres_ops_log: touchFile(path.join(root, 'postgres-ops-check.log'), 'OK: PostgreSQL/River operations validation passed\n'),
  live_e2e_evidence: liveFixture.artifacts.evidence.path,
  live_e2e_result: liveFixture.artifacts.result.path,
  manual_live_e2e_bundle: liveRoot,
  playwright_json: liveFixture.artifacts.playwright_json.path,
  backend_log: liveFixture.artifacts.backend_log.path,
  rollback_record: touchFile(path.join(root, 'rollback-record.txt'), 'rollback_image_version=0.0.0-previous\n'),
};

const checks = [
  'static_governance',
  'monitoring_assets',
  'postgres_ops',
  'compose_config',
  'compose_ps',
  'compose_images',
  'health_live',
  'health_ready',
  'live_e2e_full',
  'rollback_record',
].map((name) => ({ name, status: 'passed' }));

const manifest = {
  schema_version: 1,
  generated_at: '2026-06-21T00:00:00Z',
  release: {
    version: 'self-test',
    server_image: 'ghcr.io/kv-shepherd/shepherd-server:self-test',
    web_image: 'ghcr.io/kv-shepherd/shepherd-web:self-test',
    rollback_image_version: 'self-test-previous',
  },
  deployment: {
    topology: 'docker_compose',
    database_topology: 'external_postgres',
    monitoring_overlay_enabled: true,
    public_base_url: 'https://shepherd.example.com',
  },
  checks,
  artifacts: Object.fromEntries(Object.entries(artifactPaths).map(([key, artifactPath]) => [
    key,
    { path: artifactPath, exists: true },
  ])),
};

fs.writeFileSync(path.join(root, 'production.valid.json'), `${JSON.stringify(manifest, null, 2)}\n`);

const pendingManifest = clone(manifest);
pendingManifest.checks = pendingManifest.checks.map((check) => (
  check.name === 'postgres_ops' ? { ...check, status: 'pending' } : check
));
fs.writeFileSync(path.join(root, 'production.pending.invalid.json'), `${JSON.stringify(pendingManifest, null, 2)}\n`);

const invalidLiveFixture = clone(liveFixture);
invalidLiveFixture.cleanup.namespace = '';
const invalidLivePath = path.join(liveRoot, 'live-e2e.invalid.json');
fs.writeFileSync(invalidLivePath, `${JSON.stringify(invalidLiveFixture, null, 2)}\n`);
const invalidLiveManifest = clone(manifest);
invalidLiveManifest.artifacts.live_e2e_evidence.path = invalidLivePath;
fs.writeFileSync(path.join(root, 'production.live-invalid.json'), `${JSON.stringify(invalidLiveManifest, null, 2)}\n`);
NODE

  bash "$0" \
    --file "${fixture_root}/production.valid.json" \
    --require-production-ready \
    --require-existing-artifacts \
    --require-live-e2e-pass >/dev/null

  if bash "$0" \
    --file "${fixture_root}/production.pending.invalid.json" \
    --require-production-ready \
    --require-existing-artifacts \
    --require-live-e2e-pass >/dev/null 2>&1; then
    echo "[production-evidence] ERROR: pending production evidence fixture was accepted" >&2
    exit 1
  fi

  if bash "$0" \
    --file "${fixture_root}/production.live-invalid.json" \
    --require-production-ready \
    --require-existing-artifacts \
    --require-live-e2e-pass >/dev/null 2>&1; then
    echo "[production-evidence] ERROR: invalid live E2E evidence fixture was accepted" >&2
    exit 1
  fi

  echo "[production-evidence] OK: self-test passed"
  exit 0
fi

if [[ ! -f "${evidence_file}" ]]; then
  echo "ERROR: production evidence file not found: ${evidence_file}" >&2
  exit 2
fi

node - "${evidence_file}" "${require_production_ready}" "${require_existing_artifacts}" "${require_live_e2e_pass}" <<'NODE'
const fs = require('fs');
const path = require('path');

const [manifestPath, requireProductionReadyArg, requireExistingArtifactsArg, requireLiveE2EPassArg] = process.argv.slice(2);
const requireProductionReady = requireProductionReadyArg === '1';
const requireExistingArtifacts = requireExistingArtifactsArg === '1';
const requireLiveE2EPass = requireLiveE2EPassArg === '1';

const requiredChecks = [
  'static_governance',
  'monitoring_assets',
  'postgres_ops',
  'compose_config',
  'compose_ps',
  'compose_images',
  'health_live',
  'health_ready',
  'live_e2e_full',
  'rollback_record',
];

const requiredArtifacts = [
  'compose_config',
  'compose_ps',
  'compose_images',
  'postgres_ops_log',
  'live_e2e_evidence',
  'live_e2e_result',
  'manual_live_e2e_bundle',
  'playwright_json',
  'backend_log',
  'rollback_record',
];

const allowedStatuses = new Set(['passed', 'failed', 'pending', 'deferred']);
const allowedTopologies = new Set(['docker_compose', 'helm']);
const allowedDatabaseTopologies = new Set(['bundled_postgres', 'external_postgres']);

const errors = [];
const add = (field, message) => errors.push(`${field}: ${message}`);
const isObject = (value) => value !== null && typeof value === 'object' && !Array.isArray(value);
const nonEmptyString = (value) => typeof value === 'string' && value.trim() !== '';

const readJSON = (file) => {
  try {
    return JSON.parse(fs.readFileSync(file, 'utf8'));
  } catch (error) {
    add(file, `failed to read JSON: ${error.message}`);
    return null;
  }
};

const manifest = readJSON(manifestPath);
if (!manifest) {
  console.error(errors.map((error) => ` - ${error}`).join('\n'));
  process.exit(1);
}

if (manifest.schema_version !== 1) add('schema_version', 'must be 1');
if (!nonEmptyString(manifest.generated_at) || Number.isNaN(Date.parse(manifest.generated_at))) {
  add('generated_at', 'must be an ISO-8601 timestamp string');
}

if (!isObject(manifest.release)) {
  add('release', 'must be an object');
} else {
  for (const key of ['version', 'server_image', 'web_image', 'rollback_image_version']) {
    if (!nonEmptyString(manifest.release[key])) add(`release.${key}`, 'must be a non-empty string');
  }
}

if (!isObject(manifest.deployment)) {
  add('deployment', 'must be an object');
} else {
  if (!allowedTopologies.has(manifest.deployment.topology)) {
    add('deployment.topology', `must be one of ${Array.from(allowedTopologies).join(', ')}`);
  }
  if (!allowedDatabaseTopologies.has(manifest.deployment.database_topology)) {
    add('deployment.database_topology', `must be one of ${Array.from(allowedDatabaseTopologies).join(', ')}`);
  }
  if (typeof manifest.deployment.monitoring_overlay_enabled !== 'boolean') {
    add('deployment.monitoring_overlay_enabled', 'must be a boolean');
  }
  if (!nonEmptyString(manifest.deployment.public_base_url)) {
    add('deployment.public_base_url', 'must be a non-empty string');
  }
}

if (!Array.isArray(manifest.checks)) {
  add('checks', 'must be an array');
} else {
  const byName = new Map();
  for (const [index, check] of manifest.checks.entries()) {
    if (!isObject(check)) {
      add(`checks[${index}]`, 'must be an object');
      continue;
    }
    if (!nonEmptyString(check.name)) add(`checks[${index}].name`, 'must be a non-empty string');
    if (!allowedStatuses.has(check.status)) {
      add(`checks[${index}].status`, `must be one of ${Array.from(allowedStatuses).join(', ')}`);
    }
    if (nonEmptyString(check.name)) byName.set(check.name, check);
  }
  for (const name of requiredChecks) {
    const check = byName.get(name);
    if (!check) {
      add(`checks.${name}`, 'is required');
    } else if (requireProductionReady && check.status !== 'passed') {
      add(`checks.${name}.status`, 'must be passed for production-ready evidence');
    }
  }
}

if (!isObject(manifest.artifacts)) {
  add('artifacts', 'must be an object');
} else {
  for (const key of requiredArtifacts) {
    const artifact = manifest.artifacts[key];
    if (!isObject(artifact)) {
      add(`artifacts.${key}`, 'is required');
      continue;
    }
    if (!nonEmptyString(artifact.path)) {
      add(`artifacts.${key}.path`, 'must be a non-empty string');
    }
    if (typeof artifact.exists !== 'boolean') {
      add(`artifacts.${key}.exists`, 'must be a boolean');
    }
    if (requireExistingArtifacts) {
      if (artifact.exists !== true) add(`artifacts.${key}.exists`, 'must be true');
      if (nonEmptyString(artifact.path) && !fs.existsSync(artifact.path)) {
        add(`artifacts.${key}.path`, 'must exist on disk');
      }
    }
  }
}

if (requireLiveE2EPass) {
  const liveArtifact = manifest.artifacts?.live_e2e_evidence;
  if (!isObject(liveArtifact) || !nonEmptyString(liveArtifact.path) || !fs.existsSync(liveArtifact.path)) {
    add('artifacts.live_e2e_evidence.path', 'must point to an existing live E2E evidence manifest');
  } else {
    const live = readJSON(liveArtifact.path);
    if (live) {
      if (live.mode !== 'full') add('live_e2e.mode', 'must be full');
      if (live.status !== 'passed') add('live_e2e.status', 'must be passed');
      if (live.exit_code !== 0 || live.playwright_exit_code !== 0 || live.backend_guard_exit_code !== 0) {
        add('live_e2e.exit_codes', 'exit_code, playwright_exit_code, and backend_guard_exit_code must be 0');
      }
      if (live.policy_gates?.skipped !== false || live.policy_gates?.cluster_probe !== 'required') {
        add('live_e2e.policy_gates', 'must be required and not skipped');
      }
      if (live.cluster?.authenticated_context !== true) add('live_e2e.cluster.authenticated_context', 'must be true');
      if (live.cluster?.api_server_reachable !== true) add('live_e2e.cluster.api_server_reachable', 'must be true');
      if (live.cluster?.kubevirt_api_available !== true) add('live_e2e.cluster.kubevirt_api_available', 'must be true');
      if (!Array.isArray(live.cluster?.kubevirt_api_versions) || live.cluster.kubevirt_api_versions.length === 0) {
        add('live_e2e.cluster.kubevirt_api_versions', 'must be non-empty');
      }
    }
  }
}

if (errors.length > 0) {
  console.error('[production-evidence] ERROR: invalid production deployment evidence');
  for (const error of errors) console.error(` - ${error}`);
  process.exit(1);
}

console.log(`[production-evidence] OK: ${manifestPath}`);
NODE

if [[ "${require_live_e2e_pass}" -eq 1 ]]; then
  live_e2e_evidence_file="$(
    node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); process.stdout.write(m.artifacts?.live_e2e_evidence?.path || "");' \
      "${evidence_file}"
  )"
  if [[ -z "${live_e2e_evidence_file}" ]]; then
    echo "ERROR: production evidence live_e2e_evidence artifact path is empty" >&2
    exit 1
  fi
  bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
    --require-full-pass \
    --require-existing-artifacts \
    "${live_e2e_evidence_file}"
fi
