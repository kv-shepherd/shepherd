#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LOG_DIR="${PROJECT_ROOT}/tmp/pr-parallel-logs"

mkdir -p "${LOG_DIR}"
rm -f "${LOG_DIR}"/*.duration "${LOG_DIR}"/*.log

RECORDED_LANE_DURATION="-"

format_duration() {
  local seconds="$1"
  printf '%dm%02ds' "$((seconds / 60))" "$((seconds % 60))"
}

run_lane() {
  local name="$1"
  shift
  (
    set -uo pipefail
    local started
    local status
    started="$(date +%s)"
    echo "==> ${name}"
    "$@"
    status="$?"
    format_duration "$(( $(date +%s) - started ))" >"${LOG_DIR}/${name}.duration"
    exit "${status}"
  ) >"${LOG_DIR}/${name}.log" 2>&1 &
  RUN_LANE_PID="$!"
}

print_summary() {
  local overall="$1"
  shift

  echo
  echo "Parallel PR CI lane summary:"
  for entry in "$@"; do
    local name="${entry%%:*}"
    local rest="${entry#*:}"
    local result="${rest%%:*}"
    local duration="${rest#*:}"
    printf '  - %-11s %-4s %s\n' "${name}" "${result}" "${duration}"
  done
  echo "Overall result: ${overall}"
}

record_lane_result() {
  local name="$1"
  local result="$2"
  local duration="-"
  if [ -f "${LOG_DIR}/${name}.duration" ]; then
    duration="$(cat "${LOG_DIR}/${name}.duration")"
  fi
  RECORDED_LANE_DURATION="${duration}"
  lane_results+=("${name}:${result}:${duration}")
}

echo "Preparing local PR CI workspace..."
prep_started="$(date +%s)"
(cd "${PROJECT_ROOT}" && make ci-prep)
(cd "${PROJECT_ROOT}" && make ci-api-sync-local)
echo "Local PR CI preflight passed in $(format_duration "$(( $(date +%s) - prep_started ))")."

failures=0
lane_results=()

wait_lane() {
  local name="$1"
  local pid="$2"
  if ! wait "${pid}"; then
    failures=1
    record_lane_result "${name}" "FAIL"
    echo "Lane '${name}' failed. Recent log output:"
    tail -n 120 "${LOG_DIR}/${name}.log" || true
  else
    record_lane_result "${name}" "PASS"
    echo "Lane '${name}' passed in ${RECORDED_LANE_DURATION}."
  fi
}

run_lane backend bash -lc "cd '${PROJECT_ROOT}' && make ci-backend"
backend_pid="${RUN_LANE_PID}"
run_lane governance bash -lc "cd '${PROJECT_ROOT}' && make ci-governance"
governance_pid="${RUN_LANE_PID}"
# Keep frontend tests in one Vitest shard while backend/governance lanes run in
# parallel. This avoids local CPU starvation without reducing test coverage.
run_lane frontend bash -lc "cd '${PROJECT_ROOT}' && FRONTEND_LOCAL_TEST_SHARDS='${FRONTEND_LOCAL_TEST_SHARDS:-1}' make ci-frontend"
frontend_pid="${RUN_LANE_PID}"

if wait "${frontend_pid}"; then
  record_lane_result "frontend" "PASS"
  echo "Lane 'frontend' passed in ${RECORDED_LANE_DURATION}."
  # The frontend lane has already produced a production Next.js build.
  # Reuse it for local smoke tests to avoid a second concurrent Next build.
  run_lane e2e-smoke bash -lc "cd '${PROJECT_ROOT}' && PW_USE_EXISTING_BUILD=1 make ci-e2e-smoke"
  e2e_pid="${RUN_LANE_PID}"
else
  failures=1
  record_lane_result "frontend" "FAIL"
  lane_results+=("e2e-smoke:SKIP:-")
  echo "Lane 'frontend' failed. Recent log output:"
  tail -n 120 "${LOG_DIR}/frontend.log" || true
fi

wait_lane backend "${backend_pid}"
wait_lane governance "${governance_pid}"

if [ -n "${e2e_pid:-}" ]; then
  wait_lane e2e-smoke "${e2e_pid}"
fi

if [ "${failures}" -ne 0 ]; then
  print_summary "FAIL" "${lane_results[@]}"
  echo "Parallel PR CI failed. Full logs are under ${LOG_DIR}."
  exit 1
fi

print_summary "PASS" "${lane_results[@]}"
echo "Parallel PR CI passed. Logs are under ${LOG_DIR}."
