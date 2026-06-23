#!/usr/bin/env bash
# Collect a production deployment evidence bundle and manifest.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/collect_production_evidence.sh [options]

Options:
  --output-dir DIR             Evidence output directory (default: evidence/production-<timestamp>).
  --compose-file PATH          Compose file to use. Repeatable. Defaults to deploy/prod/docker-compose.prod.yml.
  --env-file PATH              Compose env file (default: deploy/prod/.env.prod).
  --public-base-url URL        Public Shepherd base URL. Defaults to SERVER_PUBLIC_BASE_URL from env or env file.
  --database-topology VALUE    bundled_postgres or external_postgres.
  --monitoring-overlay         Include deploy/prod/docker-compose.monitoring.yml and mark monitoring enabled.
  --release-version VALUE      Release version for the manifest.
  --server-image IMAGE         Server image for the manifest.
  --web-image IMAGE            Web image for the manifest.
  --rollback-image-version VAL Rollback image version for the manifest and rollback record.
  --live-e2e-evidence PATH     Full live E2E evidence manifest from the real-cluster run.
  --require-production-ready   Fail unless the generated manifest is production-ready.
  -h, --help                   Show this help.

The script records command output under the evidence directory and writes
production-deployment-evidence.json. It does not make deployment changes.
EOF
}

ROOT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "${ROOT_DIR}"

dir_timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
output_dir="${PRODUCTION_EVIDENCE_DIR:-evidence/production-${dir_timestamp}}"
env_file="${DEPLOY_ENV_FILE:-deploy/prod/.env.prod}"
public_base_url="${SERVER_PUBLIC_BASE_URL:-}"
database_topology="${PRODUCTION_DATABASE_TOPOLOGY:-}"
monitoring_overlay_enabled="false"
release_version="${SHEPHERD_VERSION:-${DEPLOY_RELEASE_VERSION:-}}"
server_image="${SERVER_IMAGE:-}"
web_image="${WEB_IMAGE:-}"
rollback_image_version="${ROLLBACK_IMAGE_VERSION:-}"
live_e2e_evidence="${LIVE_E2E_EVIDENCE_FILE:-}"
require_production_ready="${PRODUCTION_EVIDENCE_REQUIRE_READY:-false}"
compose_files=()

read_env_value() {
  local key="$1"
  local file="$2"
  [[ -f "${file}" ]] || return 1
  awk -F= -v key="${key}" '
    $0 !~ /^[[:space:]]*#/ && $1 == key {
      value = substr($0, index($0, "=") + 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
      gsub(/^"|"$/, "", value)
      gsub(/^'\''|'\''$/, "", value)
      print value
    }
  ' "${file}" | tail -n 1
}

value_arg() {
  local option="$1"
  local value="${2:-}"
  if [[ -z "${value}" ]]; then
    echo "ERROR: ${option} requires a non-empty value" >&2
    exit 2
  fi
  printf '%s' "${value}"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --output-dir)
      output_dir="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --compose-file)
      compose_files+=("$(value_arg "$1" "${2:-}")")
      shift 2
      ;;
    --env-file)
      env_file="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --public-base-url)
      public_base_url="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --database-topology)
      database_topology="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --monitoring-overlay)
      monitoring_overlay_enabled="true"
      shift
      ;;
    --release-version)
      release_version="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --server-image)
      server_image="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --web-image)
      web_image="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --rollback-image-version)
      rollback_image_version="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --live-e2e-evidence)
      live_e2e_evidence="$(value_arg "$1" "${2:-}")"
      shift 2
      ;;
    --require-production-ready)
      require_production_ready="true"
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

if [[ ${#compose_files[@]} -eq 0 ]]; then
  compose_files=("deploy/prod/docker-compose.prod.yml")
fi
if [[ "${monitoring_overlay_enabled}" == "true" ]]; then
  compose_files+=("deploy/prod/docker-compose.monitoring.yml")
fi

deployment_database_url="${DATABASE_URL:-}"
if [[ -z "${deployment_database_url}" ]]; then
  deployment_database_url="$(read_env_value DATABASE_URL "${env_file}" || true)"
fi
postgres_ops_database_url="${POSTGRES_OPS_DATABASE_URL:-${deployment_database_url}}"

if [[ -z "${public_base_url}" ]]; then
  public_base_url="$(read_env_value SERVER_PUBLIC_BASE_URL "${env_file}" || true)"
fi
if [[ -z "${server_image}" ]]; then
  server_image="$(read_env_value SERVER_IMAGE "${env_file}" || true)"
fi
if [[ -z "${web_image}" ]]; then
  web_image="$(read_env_value WEB_IMAGE "${env_file}" || true)"
fi
if [[ -z "${database_topology}" ]]; then
  deploy_bundled_postgres="$(read_env_value DEPLOY_BUNDLED_POSTGRES "${env_file}" || true)"
  if [[ "${deploy_bundled_postgres}" == "true" || "${deploy_bundled_postgres}" == "auto" || -z "${deployment_database_url}" ]]; then
    database_topology="bundled_postgres"
  else
    database_topology="external_postgres"
  fi
fi
if [[ -z "${release_version}" ]]; then
  release_version="unknown"
fi
if [[ -z "${server_image}" ]]; then
  server_image="unknown"
fi
if [[ -z "${web_image}" ]]; then
  web_image="unknown"
fi
if [[ -z "${rollback_image_version}" ]]; then
  rollback_image_version="unknown"
fi

mkdir -p "${output_dir}"

compose_cmd=(docker compose)
for file in "${compose_files[@]}"; do
  compose_cmd+=(-f "${file}")
done
compose_cmd+=(--env-file "${env_file}")

declare -A check_status=()
declare -A artifact_path=()
declare -A artifact_exists=()

set_status() {
  check_status["$1"]="$2"
}

set_artifact() {
  local key="$1"
  local path="$2"
  artifact_path["${key}"]="${path}"
  if [[ -e "${path}" ]]; then
    artifact_exists["${key}"]="true"
  else
    artifact_exists["${key}"]="false"
  fi
}

run_logged_check() {
  local name="$1"
  local log_path="$2"
  shift 2
  if "$@" >"${log_path}" 2>&1; then
    set_status "${name}" "passed"
  else
    set_status "${name}" "failed"
  fi
}

run_logged_check static_governance "${output_dir}/static-governance.log" bash docs/design/ci/scripts/check_design_doc_governance.sh
run_logged_check monitoring_assets "${output_dir}/monitoring-assets.log" make ci-monitoring-assets

if [[ -n "${postgres_ops_database_url}" ]]; then
  run_logged_check postgres_ops "${output_dir}/postgres-ops-check.log" env DATABASE_URL="${postgres_ops_database_url}" make postgres-ops-check
else
  printf 'DATABASE_URL or POSTGRES_OPS_DATABASE_URL was not set.\n' >"${output_dir}/postgres-ops-check.log"
  set_status postgres_ops pending
fi

if command -v docker >/dev/null 2>&1; then
  if "${compose_cmd[@]}" config --quiet >"${output_dir}/compose-config.txt" 2>&1; then
    printf 'docker compose config --quiet passed\n' >>"${output_dir}/compose-config.txt"
    set_status compose_config passed
  else
    set_status compose_config failed
  fi
  if "${compose_cmd[@]}" ps --all --format json >"${output_dir}/compose-ps.json" 2>"${output_dir}/compose-ps.err"; then
    set_status compose_ps passed
  else
    set_status compose_ps failed
  fi
  if "${compose_cmd[@]}" images --format json >"${output_dir}/compose-images.json" 2>"${output_dir}/compose-images.err"; then
    set_status compose_images passed
  else
    set_status compose_images failed
  fi
else
  printf 'docker command was not found.\n' >"${output_dir}/compose-config.txt"
  printf '[]\n' >"${output_dir}/compose-ps.json"
  printf '[]\n' >"${output_dir}/compose-images.json"
  set_status compose_config failed
  set_status compose_ps failed
  set_status compose_images failed
fi

if [[ -n "${public_base_url}" ]]; then
  if curl -fsS "${public_base_url%/}/api/v1/health/live" >"${output_dir}/health-live.txt" 2>&1; then
    set_status health_live passed
  else
    set_status health_live failed
  fi
  if curl -fsS "${public_base_url%/}/api/v1/health/ready" >"${output_dir}/health-ready.txt" 2>&1; then
    set_status health_ready passed
  else
    set_status health_ready failed
  fi
else
  printf 'SERVER_PUBLIC_BASE_URL was not set.\n' >"${output_dir}/health-live.txt"
  printf 'SERVER_PUBLIC_BASE_URL was not set.\n' >"${output_dir}/health-ready.txt"
  set_status health_live pending
  set_status health_ready pending
fi

if [[ -n "${live_e2e_evidence}" ]]; then
  if bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh --require-full-pass --require-existing-artifacts "${live_e2e_evidence}" >"${output_dir}/live-e2e-evidence-check.log" 2>&1; then
    set_status live_e2e_full passed
  else
    set_status live_e2e_full failed
  fi
else
  printf 'LIVE_E2E_EVIDENCE_FILE or --live-e2e-evidence was not set.\n' >"${output_dir}/live-e2e-evidence-check.log"
  set_status live_e2e_full pending
fi

if [[ "${rollback_image_version}" != "unknown" ]]; then
  {
    printf 'rollback_image_version=%s\n' "${rollback_image_version}"
    printf 'recorded_at=%s\n' "${generated_at}"
  } >"${output_dir}/rollback-record.txt"
  set_status rollback_record passed
else
  printf 'ROLLBACK_IMAGE_VERSION or --rollback-image-version was not set.\n' >"${output_dir}/rollback-record.txt"
  set_status rollback_record pending
fi

live_result_path=""
manual_live_bundle=""
playwright_json_path=""
backend_log_path=""
if [[ -n "${live_e2e_evidence}" && -f "${live_e2e_evidence}" ]]; then
  live_result_path="$(node -e 'const fs=require("fs"); const p=process.argv[1]; const m=JSON.parse(fs.readFileSync(p,"utf8")); process.stdout.write(m.artifacts?.result?.path || "");' "${live_e2e_evidence}" 2>/dev/null || true)"
  playwright_json_path="$(node -e 'const fs=require("fs"); const p=process.argv[1]; const m=JSON.parse(fs.readFileSync(p,"utf8")); process.stdout.write(m.artifacts?.playwright_json?.path || "");' "${live_e2e_evidence}" 2>/dev/null || true)"
  backend_log_path="$(node -e 'const fs=require("fs"); const p=process.argv[1]; const m=JSON.parse(fs.readFileSync(p,"utf8")); process.stdout.write(m.artifacts?.backend_log?.path || "");' "${live_e2e_evidence}" 2>/dev/null || true)"
  manual_live_bundle="$(dirname "${live_e2e_evidence}")"
fi

set_artifact compose_config "${output_dir}/compose-config.txt"
set_artifact compose_ps "${output_dir}/compose-ps.json"
set_artifact compose_images "${output_dir}/compose-images.json"
set_artifact postgres_ops_log "${output_dir}/postgres-ops-check.log"
set_artifact live_e2e_evidence "${live_e2e_evidence:-.run/live-e2e/YYYYMMDD/HHMM/live-e2e.evidence.json}"
set_artifact live_e2e_result "${live_result_path:-.run/live-e2e/YYYYMMDD/HHMM/live-e2e.result}"
set_artifact manual_live_e2e_bundle "${manual_live_bundle:-.run/live-e2e/YYYYMMDD/HHMM}"
set_artifact playwright_json "${playwright_json_path:-.run/live-e2e/YYYYMMDD/HHMM/playwright-results.json}"
set_artifact backend_log "${backend_log_path:-.run/live-e2e/YYYYMMDD/HHMM/shepherd-e2e-server.log}"
set_artifact rollback_record "${output_dir}/rollback-record.txt"

MANIFEST_PATH="${output_dir}/production-deployment-evidence.json" \
GENERATED_AT="${generated_at}" \
RELEASE_VERSION="${release_version}" \
SERVER_IMAGE_VALUE="${server_image}" \
WEB_IMAGE_VALUE="${web_image}" \
ROLLBACK_IMAGE_VERSION_VALUE="${rollback_image_version}" \
DATABASE_TOPOLOGY_VALUE="${database_topology}" \
MONITORING_OVERLAY_ENABLED_VALUE="${monitoring_overlay_enabled}" \
PUBLIC_BASE_URL_VALUE="${public_base_url:-unknown}" \
CHECK_STATIC_GOVERNANCE="${check_status[static_governance]:-pending}" \
CHECK_MONITORING_ASSETS="${check_status[monitoring_assets]:-pending}" \
CHECK_POSTGRES_OPS="${check_status[postgres_ops]:-pending}" \
CHECK_COMPOSE_CONFIG="${check_status[compose_config]:-pending}" \
CHECK_COMPOSE_PS="${check_status[compose_ps]:-pending}" \
CHECK_COMPOSE_IMAGES="${check_status[compose_images]:-pending}" \
CHECK_HEALTH_LIVE="${check_status[health_live]:-pending}" \
CHECK_HEALTH_READY="${check_status[health_ready]:-pending}" \
CHECK_LIVE_E2E_FULL="${check_status[live_e2e_full]:-pending}" \
CHECK_ROLLBACK_RECORD="${check_status[rollback_record]:-pending}" \
ARTIFACT_COMPOSE_CONFIG_PATH="${artifact_path[compose_config]}" \
ARTIFACT_COMPOSE_CONFIG_EXISTS="${artifact_exists[compose_config]}" \
ARTIFACT_COMPOSE_PS_PATH="${artifact_path[compose_ps]}" \
ARTIFACT_COMPOSE_PS_EXISTS="${artifact_exists[compose_ps]}" \
ARTIFACT_COMPOSE_IMAGES_PATH="${artifact_path[compose_images]}" \
ARTIFACT_COMPOSE_IMAGES_EXISTS="${artifact_exists[compose_images]}" \
ARTIFACT_POSTGRES_OPS_LOG_PATH="${artifact_path[postgres_ops_log]}" \
ARTIFACT_POSTGRES_OPS_LOG_EXISTS="${artifact_exists[postgres_ops_log]}" \
ARTIFACT_LIVE_E2E_EVIDENCE_PATH="${artifact_path[live_e2e_evidence]}" \
ARTIFACT_LIVE_E2E_EVIDENCE_EXISTS="${artifact_exists[live_e2e_evidence]}" \
ARTIFACT_LIVE_E2E_RESULT_PATH="${artifact_path[live_e2e_result]}" \
ARTIFACT_LIVE_E2E_RESULT_EXISTS="${artifact_exists[live_e2e_result]}" \
ARTIFACT_MANUAL_LIVE_E2E_BUNDLE_PATH="${artifact_path[manual_live_e2e_bundle]}" \
ARTIFACT_MANUAL_LIVE_E2E_BUNDLE_EXISTS="${artifact_exists[manual_live_e2e_bundle]}" \
ARTIFACT_PLAYWRIGHT_JSON_PATH="${artifact_path[playwright_json]}" \
ARTIFACT_PLAYWRIGHT_JSON_EXISTS="${artifact_exists[playwright_json]}" \
ARTIFACT_BACKEND_LOG_PATH="${artifact_path[backend_log]}" \
ARTIFACT_BACKEND_LOG_EXISTS="${artifact_exists[backend_log]}" \
ARTIFACT_ROLLBACK_RECORD_PATH="${artifact_path[rollback_record]}" \
ARTIFACT_ROLLBACK_RECORD_EXISTS="${artifact_exists[rollback_record]}" \
node <<'NODE'
const fs = require('fs');
const env = process.env;
const bool = (value) => value === 'true';
const check = (name, status) => ({ name, status });
const artifact = (prefix) => ({
  path: env[`ARTIFACT_${prefix}_PATH`],
  exists: bool(env[`ARTIFACT_${prefix}_EXISTS`]),
});
const manifest = {
  schema_version: 1,
  generated_at: env.GENERATED_AT,
  release: {
    version: env.RELEASE_VERSION,
    server_image: env.SERVER_IMAGE_VALUE,
    web_image: env.WEB_IMAGE_VALUE,
    rollback_image_version: env.ROLLBACK_IMAGE_VERSION_VALUE,
  },
  deployment: {
    topology: 'docker_compose',
    database_topology: env.DATABASE_TOPOLOGY_VALUE,
    monitoring_overlay_enabled: bool(env.MONITORING_OVERLAY_ENABLED_VALUE),
    public_base_url: env.PUBLIC_BASE_URL_VALUE,
  },
  checks: [
    check('static_governance', env.CHECK_STATIC_GOVERNANCE),
    check('monitoring_assets', env.CHECK_MONITORING_ASSETS),
    check('postgres_ops', env.CHECK_POSTGRES_OPS),
    check('compose_config', env.CHECK_COMPOSE_CONFIG),
    check('compose_ps', env.CHECK_COMPOSE_PS),
    check('compose_images', env.CHECK_COMPOSE_IMAGES),
    check('health_live', env.CHECK_HEALTH_LIVE),
    check('health_ready', env.CHECK_HEALTH_READY),
    check('live_e2e_full', env.CHECK_LIVE_E2E_FULL),
    check('rollback_record', env.CHECK_ROLLBACK_RECORD),
  ],
  artifacts: {
    compose_config: artifact('COMPOSE_CONFIG'),
    compose_ps: artifact('COMPOSE_PS'),
    compose_images: artifact('COMPOSE_IMAGES'),
    postgres_ops_log: artifact('POSTGRES_OPS_LOG'),
    live_e2e_evidence: artifact('LIVE_E2E_EVIDENCE'),
    live_e2e_result: artifact('LIVE_E2E_RESULT'),
    manual_live_e2e_bundle: artifact('MANUAL_LIVE_E2E_BUNDLE'),
    playwright_json: artifact('PLAYWRIGHT_JSON'),
    backend_log: artifact('BACKEND_LOG'),
    rollback_record: artifact('ROLLBACK_RECORD'),
  },
};
fs.writeFileSync(env.MANIFEST_PATH, `${JSON.stringify(manifest, null, 2)}\n`);
NODE

production_evidence_check_args=(--file "${output_dir}/production-deployment-evidence.json")
if [[ "${require_production_ready}" == "true" || "${require_production_ready}" == "1" ]]; then
  production_evidence_check_args+=(--require-production-ready --require-existing-artifacts --require-live-e2e-pass)
fi

bash scripts/check_production_evidence.sh "${production_evidence_check_args[@]}"
echo "Production evidence manifest: ${output_dir}/production-deployment-evidence.json"
