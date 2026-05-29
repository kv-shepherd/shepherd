#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"
source docs/design/ci/scripts/promtool_lib.sh

fail() {
  echo "[prometheus-rule-tests] ERROR: $1" >&2
  exit 1
}

test_file="${1:-deploy/monitoring/prometheus/shepherd-rules.test.yml}"

expected_records=(
  shepherd:http_requests:rate5m
  shepherd:http_requests_5xx:rate5m
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_ready_jobs:sum
  shepherd:river_recent_discarded_jobs:sum
)

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

[[ -f "${test_file}" ]] || fail "test file missing: ${test_file}"
[[ -f "deploy/monitoring/prometheus/shepherd-recording-rules.yml" ]] || fail "recording rule file missing"
[[ -f "deploy/monitoring/prometheus/shepherd-alerts.yml" ]] || fail "alert rule file missing"

if promtool_bin="$(resolve_promtool)"; then
  (
    cd deploy/monitoring/prometheus
    "${promtool_bin}" test rules "$(basename "${test_file}")"
  )
elif [[ $? -eq 2 ]]; then
  echo "[prometheus-rule-tests] promtool not found; running structural checks only for ${test_file}"
else
  fail "promtool discovery failed"
fi

rg -q '^rule_files:$' "${test_file}" || fail "${test_file}: missing rule_files"
rg -q '^[[:space:]]+- shepherd-recording-rules\.yml$' "${test_file}" \
  || fail "${test_file}: must include shepherd-recording-rules.yml"
rg -q '^[[:space:]]+- shepherd-alerts\.yml$' "${test_file}" \
  || fail "${test_file}: must include shepherd-alerts.yml"
rg -q '^evaluation_interval: 1m$' "${test_file}" \
  || fail "${test_file}: evaluation_interval must be 1m"
if rg -q '^fuzzy_compare:' "${test_file}"; then
  fail "${test_file}: fuzzy_compare is not supported by the Ubuntu 24.04 promtool baseline"
fi
rg -q '^group_eval_order:$' "${test_file}" \
  || fail "${test_file}: missing group_eval_order"
rg -q '^[[:space:]]+- shepherd\.recording$' "${test_file}" \
  || fail "${test_file}: group_eval_order must include shepherd.recording"
rg -q '^[[:space:]]+- shepherd\.baseline$' "${test_file}" \
  || fail "${test_file}: group_eval_order must include shepherd.baseline"

recording_line="$(rg -n '^[[:space:]]+- shepherd\.recording$' "${test_file}" | cut -d: -f1 | head -n1)"
baseline_line="$(rg -n '^[[:space:]]+- shepherd\.baseline$' "${test_file}" | cut -d: -f1 | head -n1)"
[[ -n "${recording_line}" && -n "${baseline_line}" ]] \
  || fail "${test_file}: cannot determine group_eval_order lines"
(( recording_line < baseline_line )) \
  || fail "${test_file}: shepherd.recording must be evaluated before shepherd.baseline"

for record in "${expected_records[@]}"; do
  count="$(rg -c "^[[:space:]]+- expr: ${record}$" "${test_file}" || true)"
  [[ "${count}" == "1" ]] || fail "${test_file}: expected exactly one promql test for ${record}, found ${count}"
  rg -q "labels: '${record}" "${test_file}" || fail "${test_file}: missing expected sample label for ${record}"
done

for alert in "${expected_alerts[@]}"; do
  count="$(rg -c "^[[:space:]]+- alertname: ${alert}$" "${test_file}" || true)"
  [[ "${count}" == "1" ]] || fail "${test_file}: expected exactly one alert test for ${alert}, found ${count}"
done

if rg -n '(user|role|session|ticket|vm|namespace|cluster|path|query|header)=' "${test_file}"; then
  fail "${test_file}: rule tests must not introduce sensitive or high-cardinality sample labels"
fi

echo "[prometheus-rule-tests] OK: ${test_file} checked"
