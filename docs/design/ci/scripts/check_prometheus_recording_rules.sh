#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"
source docs/design/ci/scripts/promtool_lib.sh

fail() {
  echo "[prometheus-recording-rules] ERROR: $1" >&2
  exit 1
}

files=("$@")
if [[ ${#files[@]} -eq 0 ]]; then
  files=("deploy/monitoring/prometheus/shepherd-recording-rules.yml")
fi

expected_records=(
  shepherd:http_requests:rate5m
  shepherd:http_requests_5xx:rate5m
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_ready_jobs:sum
  shepherd:river_recent_discarded_jobs:sum
  shepherd:business_approval_pending:sum
  shepherd:business_approval_failed:sum
  shepherd:business_batch_approval_pending:sum
  shepherd:business_batch_approval_failed:sum
  shepherd:business_approval_failure_audit_actions:sum
)

allowed_metrics=(
  shepherd_http_requests_total
  shepherd_http_request_duration_seconds_bucket
  shepherd_openapi_validation_failures_total
  shepherd_river_ready_jobs
  shepherd_river_recent_terminal_jobs
  shepherd_business_approval_tickets
  shepherd_business_batch_approval_tickets
  shepherd_business_batch_approval_failed_children
  shepherd_business_approval_failure_audit_actions_recent
  shepherd:http_requests:rate5m
  shepherd:http_requests_5xx:rate5m
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_ready_jobs:sum
  shepherd:river_recent_discarded_jobs:sum
  shepherd:business_approval_pending:sum
  shepherd:business_approval_failed:sum
  shepherd:business_batch_approval_pending:sum
  shepherd:business_batch_approval_failed:sum
  shepherd:business_approval_failure_audit_actions:sum
)

required_source_metrics=(
  shepherd_http_requests_total
  shepherd_http_request_duration_seconds_bucket
  shepherd_openapi_validation_failures_total
  shepherd_river_ready_jobs
  shepherd_river_recent_terminal_jobs
  shepherd_business_approval_tickets
  shepherd_business_batch_approval_tickets
  shepherd_business_batch_approval_failed_children
  shepherd_business_approval_failure_audit_actions_recent
)

contains_allowed_metric() {
  local metric="$1"
  local allowed
  for allowed in "${allowed_metrics[@]}"; do
    [[ "${metric}" == "${allowed}" ]] && return 0
  done
  return 1
}

extract_record_block() {
  local file="$1"
  local record="$2"
  awk -v record="${record}" '
    $0 ~ "^[[:space:]]*- record: " record "$" { in_block = 1 }
    in_block && $0 ~ "^[[:space:]]*- record: " && $0 !~ "^[[:space:]]*- record: " record "$" { exit }
    in_block { print }
  ' "${file}"
}

for file in "${files[@]}"; do
  [[ -f "${file}" ]] || fail "rule file missing: ${file}"

  if promtool_bin="$(resolve_promtool)"; then
    "${promtool_bin}" check rules "${file}"
  elif [[ $? -eq 2 ]]; then
    echo "[prometheus-recording-rules] promtool not found; running structural checks only for ${file}"
  else
    fail "promtool discovery failed"
  fi

  rg -q '^groups:$' "${file}" || fail "${file}: missing top-level groups"
  rg -q '^[[:space:]]*- name: shepherd\.recording$' "${file}" || fail "${file}: missing shepherd.recording group"

  for record in "${expected_records[@]}"; do
    count="$(rg -c "^[[:space:]]*- record: ${record}$" "${file}" || true)"
    [[ "${count}" == "1" ]] || fail "${file}: expected exactly one ${record}, found ${count}"

    block="$(extract_record_block "${file}" "${record}")"
    [[ -n "${block}" ]] || fail "${file}: cannot extract block for ${record}"
    grep -Eq '^[[:space:]]+expr:' <<<"${block}" || fail "${file}: ${record} missing expr"
  done

  if rg -n '^[[:space:]]+(user|role|session|ticket|vm|namespace|cluster|path|query|header|method|route):' "${file}"; then
    fail "${file}: recording rules must not add sensitive or high-cardinality labels"
  fi

  if rg -n 'by[[:space:]]*\([^)]*(user|role|session|ticket|vm|namespace|cluster|path|query|header|method|route)[^)]*\)' "${file}"; then
    fail "${file}: recording rules must not group by sensitive or high-cardinality labels"
  fi

  if rg -n 'shepherd:[A-Za-z0-9_:]+' "${file}" | rg -v '^[0-9]+:[[:space:]]*- record: shepherd:'; then
    fail "${file}: recording rules must not depend on other recorded Shepherd series"
  fi

  while IFS= read -r metric; do
    [[ -z "${metric}" ]] && continue
    contains_allowed_metric "${metric}" || fail "${file}: unsupported Shepherd metric reference ${metric}"
  done < <(rg --no-filename -o 'shepherd[:_][A-Za-z0-9_:]+' "${file}" | sort -u)

  for metric in "${required_source_metrics[@]}"; do
    rg -q "${metric}" "${file}" || fail "${file}: required source metric ${metric} not referenced"
  done
done

echo "[prometheus-recording-rules] OK: ${#files[@]} rule file(s) checked"
