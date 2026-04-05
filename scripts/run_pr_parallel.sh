#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LOG_DIR="${PROJECT_ROOT}/tmp/pr-parallel-logs"

mkdir -p "${LOG_DIR}"
rm -f "${LOG_DIR}"/*.log

run_lane() {
  local name="$1"
  shift
  (
    set -euo pipefail
    echo "==> ${name}"
    "$@"
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
    local result="${entry##*:}"
    printf '  - %-11s %s\n' "${name}" "${result}"
  done
  echo "Overall result: ${overall}"
}

echo "Preparing local PR CI workspace..."
(cd "${PROJECT_ROOT}" && make ci-prep)
(cd "${PROJECT_ROOT}" && make ci-api-sync)

run_lane backend bash -lc "cd '${PROJECT_ROOT}' && make ci-backend"
backend_pid="${RUN_LANE_PID}"
run_lane governance bash -lc "cd '${PROJECT_ROOT}' && make ci-governance"
governance_pid="${RUN_LANE_PID}"
run_lane frontend bash -lc "cd '${PROJECT_ROOT}' && make ci-frontend"
frontend_pid="${RUN_LANE_PID}"
run_lane e2e-smoke bash -lc "cd '${PROJECT_ROOT}' && make ci-e2e-smoke"
e2e_pid="${RUN_LANE_PID}"

failures=0
lane_results=()
for entry in \
  "backend:${backend_pid}" \
  "governance:${governance_pid}" \
  "frontend:${frontend_pid}" \
  "e2e-smoke:${e2e_pid}"
do
  name="${entry%%:*}"
  pid="${entry##*:}"
  if ! wait "${pid}"; then
    failures=1
    lane_results+=("${name}:FAIL")
    echo "Lane '${name}' failed. Recent log output:"
    tail -n 120 "${LOG_DIR}/${name}.log" || true
  else
    lane_results+=("${name}:PASS")
    echo "Lane '${name}' passed."
  fi
done

if [ "${failures}" -ne 0 ]; then
  print_summary "FAIL" "${lane_results[@]}"
  echo "Parallel PR CI failed. Full logs are under ${LOG_DIR}."
  exit 1
fi

print_summary "PASS" "${lane_results[@]}"
echo "Parallel PR CI passed. Logs are under ${LOG_DIR}."
