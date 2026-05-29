#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"
source docs/design/ci/scripts/promtool_lib.sh

fail() {
  echo "[prometheus-alert-rules] ERROR: $1" >&2
  exit 1
}

files=("$@")
if [[ ${#files[@]} -eq 0 ]]; then
  files=("deploy/monitoring/prometheus/shepherd-alerts.yml")
fi

expected_alerts=(
  ShepherdMetricsTargetDown
  ShepherdHighHTTP5xxRate
  ShepherdHighHTTPP95Latency
  ShepherdOpenAPIValidationFailures
  ShepherdPostgresStatsScrapeFailed
  ShepherdRiverDeadTupleRatioHigh
  ShepherdRiverQueueStatsScrapeFailed
  ShepherdRiverQueueBacklogAgeHigh
  ShepherdRiverJobsDiscarded
)

allowed_metrics=(
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_recent_discarded_jobs:sum
  shepherd_postgres_table_stats_scrape_success
  shepherd_river_dead_tuple_ratio
  shepherd_river_queue_stats_scrape_success
  shepherd_river_oldest_ready_job_age_seconds
)

required_metrics=(
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_recent_discarded_jobs:sum
  shepherd_postgres_table_stats_scrape_success
  shepherd_river_dead_tuple_ratio
  shepherd_river_queue_stats_scrape_success
  shepherd_river_oldest_ready_job_age_seconds
)

extract_alert_block() {
  local file="$1"
  local alert="$2"
  awk -v alert="${alert}" '
    $0 ~ "^[[:space:]]*- alert: " alert "$" { in_block = 1 }
    in_block && $0 ~ "^[[:space:]]*- alert: " && $0 !~ "^[[:space:]]*- alert: " alert "$" { exit }
    in_block { print }
  ' "${file}"
}

contains_allowed_metric() {
  local metric="$1"
  local allowed
  for allowed in "${allowed_metrics[@]}"; do
    [[ "${metric}" == "${allowed}" ]] && return 0
  done
  return 1
}

for file in "${files[@]}"; do
  [[ -f "${file}" ]] || fail "rule file missing: ${file}"

  if promtool_bin="$(resolve_promtool)"; then
    "${promtool_bin}" check rules "${file}"
  elif [[ $? -eq 2 ]]; then
    echo "[prometheus-alert-rules] promtool not found; running structural checks only for ${file}"
  else
    fail "promtool discovery failed"
  fi

  rg -q '^groups:$' "${file}" || fail "${file}: missing top-level groups"
  rg -q '^[[:space:]]*- name: shepherd\.baseline$' "${file}" || fail "${file}: missing shepherd.baseline group"

  for alert in "${expected_alerts[@]}"; do
    count="$(rg -c "^[[:space:]]*- alert: ${alert}$" "${file}" || true)"
    [[ "${count}" == "1" ]] || fail "${file}: expected exactly one ${alert}, found ${count}"

    block="$(extract_alert_block "${file}" "${alert}")"
    [[ -n "${block}" ]] || fail "${file}: cannot extract block for ${alert}"
    grep -Eq '^[[:space:]]+expr:' <<<"${block}" || fail "${file}: ${alert} missing expr"
    grep -Eq '^[[:space:]]+for:' <<<"${block}" || fail "${file}: ${alert} missing for"
    grep -Eq '^[[:space:]]+labels:' <<<"${block}" || fail "${file}: ${alert} missing labels"
    grep -Eq '^[[:space:]]+severity: (warning|critical)$' <<<"${block}" || fail "${file}: ${alert} missing valid severity"
    grep -Eq '^[[:space:]]+service: shepherd$' <<<"${block}" || fail "${file}: ${alert} missing service label"
    grep -Eq '^[[:space:]]+annotations:' <<<"${block}" || fail "${file}: ${alert} missing annotations"
    grep -Eq '^[[:space:]]+summary:' <<<"${block}" || fail "${file}: ${alert} missing summary annotation"
    grep -Eq '^[[:space:]]+description:' <<<"${block}" || fail "${file}: ${alert} missing description annotation"
    grep -Eq '^[[:space:]]+runbook_url:' <<<"${block}" || fail "${file}: ${alert} missing runbook_url annotation"
  done

  if rg -n '^[[:space:]]+(user|role|session|ticket|vm|namespace|cluster|path|query|header):' "${file}"; then
    fail "${file}: alert labels must not add sensitive or high-cardinality dimensions"
  fi

  while IFS= read -r metric; do
    [[ -z "${metric}" ]] && continue
    contains_allowed_metric "${metric}" || fail "${file}: unsupported Shepherd metric reference ${metric}"
  done < <(rg --no-filename -o 'shepherd[:_][A-Za-z0-9_:]+' "${file}" | sort -u)

  for metric in "${required_metrics[@]}"; do
    rg -q "${metric}" "${file}" || fail "${file}: required metric ${metric} not referenced"
  done
done

echo "[prometheus-alert-rules] OK: ${#files[@]} rule file(s) checked"
