#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

NO_DB_WRAPPER=0
BACKGROUND=0
FOREGROUND=0
STATUS_ONLY=0
BG_LOG_FILE=""
BG_PID_FILE=""
BG_RESULT_FILE=""
BG_OUTPUT_DIR="${ROOT_DIR}/.run/live-e2e"
BG_STATE_FILE=""
PASSTHRU_ARGS=()

usage() {
  cat <<'EOF'
Usage:
  scripts/run_e2e_live.sh [options] [-- playwright-args...]

Options:
  (default)              Start detached in background, run with a fresh ephemeral PG18 container, write files into .run/live-e2e/
  --foreground           Run in current shell (useful for CI or debugging)
  --no-db-wrapper        Use existing DATABASE_URL instead of auto-starting Docker PostgreSQL
  --background           Force detached background mode
  --status               Read run status only (no log content), for low-token polling
  --output-dir <path>    Background run output root (default: .run/live-e2e/, subfolders: YYYYMMDD/HHMM)
  --log-file <path>      Background mode log file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.log)
  --pid-file <path>      Background mode pid file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.pid)
  --result-file <path>   Background mode result file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.result)
  --state-file <path>    Status metadata file (default: <output-dir>/latest.env)
  -h, --help             Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-db-wrapper)
      NO_DB_WRAPPER=1
      shift
      ;;
    --background)
      BACKGROUND=1
      shift
      ;;
    --foreground)
      FOREGROUND=1
      shift
      ;;
    --status)
      STATUS_ONLY=1
      shift
      ;;
    --output-dir)
      BG_OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --log-file)
      BG_LOG_FILE="${2:-}"
      shift 2
      ;;
    --pid-file)
      BG_PID_FILE="${2:-}"
      shift 2
      ;;
    --result-file)
      BG_RESULT_FILE="${2:-}"
      shift 2
      ;;
    --state-file)
      BG_STATE_FILE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      PASSTHRU_ARGS+=("$@")
      break
      ;;
    *)
      PASSTHRU_ARGS+=("$1")
      shift
      ;;
  esac
done

BG_STATE_FILE="${BG_STATE_FILE:-${BG_OUTPUT_DIR}/latest.env}"

extract_result_summary() {
  local log_file="$1"
  local line failed flaky passed total pass_rate

  line="$(rg -N "Playwright Run Summary::" "${log_file}" 2>/dev/null | tail -n 1 || true)"
  if [[ -z "${line}" ]]; then
    return 1
  fi

  failed="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ failed' | head -n 1 | awk '{print $1}')"
  flaky="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ flaky' | head -n 1 | awk '{print $1}')"
  passed="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ passed' | head -n 1 | awk '{print $1}')"
  failed="${failed:-0}"
  flaky="${flaky:-0}"
  passed="${passed:-0}"
  total=$((failed + flaky + passed))
  if (( total > 0 )); then
    pass_rate="$(awk -v p="${passed}" -v t="${total}" 'BEGIN { printf "%.2f", (p*100)/t }')"
  else
    pass_rate="0.00"
  fi

  echo "failed=${failed} flaky=${flaky} passed=${passed} total=${total} pass_rate=${pass_rate}%"
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:|\\.)${port}$"; then
      return 0
    fi
  fi

  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
  fi

  if command -v netstat >/dev/null 2>&1; then
    if netstat -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:|\\.)${port}$"; then
      return 0
    fi
  fi

  return 1
}

cleanup_residual_pg_on_port() {
  local target_port="$1"
  local cid image network_mode host_ports args
  local to_remove=()

  if ! command -v docker >/dev/null 2>&1; then
    return 0
  fi

  while IFS= read -r cid; do
    [[ -z "${cid}" ]] && continue
    image="$(docker inspect -f '{{.Config.Image}}' "${cid}" 2>/dev/null || true)"
    if [[ "${image}" != postgres* ]]; then
      continue
    fi

    host_ports="$(docker inspect -f '{{range $k, $v := .NetworkSettings.Ports}}{{if $v}}{{(index $v 0).HostPort}} {{end}}{{end}}' "${cid}" 2>/dev/null || true)"
    network_mode="$(docker inspect -f '{{.HostConfig.NetworkMode}}' "${cid}" 2>/dev/null || true)"
    args="$(docker inspect -f '{{range .Args}}{{.}} {{end}}' "${cid}" 2>/dev/null || true)"

    if [[ " ${host_ports} " == *" ${target_port} "* ]]; then
      to_remove+=("${cid}")
      continue
    fi

    if [[ "${network_mode}" == "host" ]] && [[ " ${args} " == *" -p ${target_port} "* ]]; then
      to_remove+=("${cid}")
      continue
    fi
  done < <(docker ps -aq 2>/dev/null || true)

  if (( ${#to_remove[@]} > 0 )); then
    echo "INFO: removing residual PostgreSQL containers on port ${target_port}: ${to_remove[*]}"
    docker rm -f "${to_remove[@]}" >/dev/null 2>&1 || true
  fi
}

if [[ "${BACKGROUND}" -eq 1 && "${FOREGROUND}" -eq 1 ]]; then
  echo "ERROR: --background and --foreground cannot be used together"
  exit 1
fi

if [[ "${STATUS_ONLY}" -eq 0 && "${BACKGROUND}" -eq 0 && "${FOREGROUND}" -eq 0 ]]; then
  BACKGROUND=1
fi

if [[ "$STATUS_ONLY" -eq 1 ]]; then
  if [[ -z "${BG_LOG_FILE}" || -z "${BG_PID_FILE}" ]]; then
    if [[ -f "${BG_STATE_FILE}" ]]; then
      # shellcheck disable=SC1090
      source "${BG_STATE_FILE}"
    fi
  fi

  if [[ -z "${BG_LOG_FILE}" || -z "${BG_PID_FILE}" ]]; then
    echo "ERROR: status mode needs --log-file and --pid-file, or a valid --state-file"
    exit 1
  fi

  pid="N/A"
  running="no"
  if [[ -f "${BG_PID_FILE}" ]]; then
    pid="$(cat "${BG_PID_FILE}" 2>/dev/null || echo N/A)"
  fi
  if [[ "${pid}" != "N/A" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    running="yes"
  fi

  log_size_bytes=0
  log_mtime="N/A"
  if [[ -f "${BG_LOG_FILE}" ]]; then
    log_size_bytes="$(wc -c <"${BG_LOG_FILE}")"
    log_mtime="$(stat -c '%y' "${BG_LOG_FILE}" | cut -d'.' -f1)"
  fi

  echo "STATUS: running=${running} pid=${pid} log=${BG_LOG_FILE} size_bytes=${log_size_bytes} updated_at=${log_mtime}"

  if [[ "${running}" == "no" ]]; then
    if [[ -z "${BG_RESULT_FILE}" ]]; then
      if [[ -f "${BG_STATE_FILE}" ]]; then
        # shellcheck disable=SC1090
        source "${BG_STATE_FILE}"
      fi
    fi
    if [[ -n "${BG_RESULT_FILE}" && -f "${BG_RESULT_FILE}" ]]; then
      echo "RESULT_FILE: ${BG_RESULT_FILE}"
      cat "${BG_RESULT_FILE}"
    elif [[ -f "${BG_LOG_FILE}" ]] && result_line="$(extract_result_summary "${BG_LOG_FILE}")"; then
      echo "RESULT: ${result_line}"
    else
      echo "RESULT: summary line not found in log"
    fi
  fi
  exit 0
fi

if [[ "$BACKGROUND" -eq 1 ]]; then
  run_day="$(date +%Y%m%d)"
  run_hm="$(date +%H%M)"
  run_dir_base="${BG_OUTPUT_DIR}/${run_day}/${run_hm}"
  run_dir="${run_dir_base}"
  suffix=1
  while [[ -e "${run_dir}" ]]; do
    run_dir="${run_dir_base}-${suffix}"
    suffix=$((suffix + 1))
  done

  mkdir -p "${BG_OUTPUT_DIR}"
  mkdir -p "${run_dir}"
  BG_LOG_FILE="${BG_LOG_FILE:-${run_dir}/live-e2e.log}"
  BG_PID_FILE="${BG_PID_FILE:-${run_dir}/live-e2e.pid}"
  BG_RESULT_FILE="${BG_RESULT_FILE:-${run_dir}/live-e2e.result}"
  BG_STATE_FILE="${BG_STATE_FILE:-${BG_OUTPUT_DIR}/latest.env}"
  mkdir -p "$(dirname "${BG_LOG_FILE}")"
  mkdir -p "$(dirname "${BG_PID_FILE}")"
  mkdir -p "$(dirname "${BG_RESULT_FILE}")"
  mkdir -p "$(dirname "${BG_STATE_FILE}")"

  CMD=(bash "${ROOT_DIR}/scripts/run_e2e_live.sh")
  CMD+=(--foreground)
  if [[ "$NO_DB_WRAPPER" -eq 1 ]]; then
    CMD+=(--no-db-wrapper)
  fi
  if [[ "${#PASSTHRU_ARGS[@]}" -gt 0 ]]; then
    CMD+=(-- "${PASSTHRU_ARGS[@]}")
  fi

  RUN_RESULT_FILE="${BG_RESULT_FILE}" RUN_LOG_FILE="${BG_LOG_FILE}" nohup "${CMD[@]}" >"${BG_LOG_FILE}" 2>&1 &
  bg_pid=$!
  echo "${bg_pid}" >"${BG_PID_FILE}"
  cat >"${BG_STATE_FILE}" <<EOF
BG_LOG_FILE=${BG_LOG_FILE}
BG_PID_FILE=${BG_PID_FILE}
BG_RESULT_FILE=${BG_RESULT_FILE}
BG_RUN_DIR=${run_dir}
EOF
  echo "INFO: started background live e2e run"
  echo "INFO: pid=${bg_pid}"
  echo "INFO: output root: ${BG_OUTPUT_DIR}"
  echo "INFO: run dir: ${run_dir}"
  echo "INFO: pid file: ${BG_PID_FILE}"
  echo "INFO: log file: ${BG_LOG_FILE}"
  echo "INFO: result file: ${BG_RESULT_FILE}"
  echo "INFO: reminder: poll status every 5 minutes (not log content) until completion"
  echo "INFO: command: bash scripts/run_e2e_live.sh --status --state-file ${BG_STATE_FILE}"
  exit 0
fi

set -- "${PASSTHRU_ARGS[@]}"

if [[ "$NO_DB_WRAPPER" -eq 0 ]]; then
  # Live E2E default: always run against an isolated PostgreSQL test container.
  PG_IMAGE="${PG_IMAGE:-postgres:18}"
  E2E_PG_PORT="${E2E_PG_PORT:-55432}"
  cleanup_residual_pg_on_port "${E2E_PG_PORT}"
  if port_in_use "${E2E_PG_PORT}"; then
    echo "ERROR: port ${E2E_PG_PORT} is still in use after residual cleanup; stop the process and retry"
    exit 1
  fi
  exec env PG_PORT="${E2E_PG_PORT}" ./scripts/run_with_docker_pg.sh --image "${PG_IMAGE}" -- bash ./scripts/run_e2e_live.sh --foreground --no-db-wrapper "$@"
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  echo "ERROR: DATABASE_URL is required when --no-db-wrapper is set"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl command not found"
  exit 1
fi

pick_free_port() {
  local candidate
  for _ in $(seq 1 80); do
    candidate=$((RANDOM % 10000 + 18080))
    if ! port_in_use "$candidate"; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}

if [[ -n "${SERVER_PORT:-}" ]]; then
  SERVER_PORT="$SERVER_PORT"
elif [[ -n "${E2E_BACKEND_PORT:-}" ]]; then
  SERVER_PORT="$E2E_BACKEND_PORT"
else
  SERVER_PORT="$(pick_free_port || true)"
  if [[ -z "$SERVER_PORT" ]]; then
    echo "ERROR: unable to allocate free backend port"
    exit 1
  fi
fi

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:${SERVER_PORT}}"
SERVER_LOG="${E2E_SERVER_LOG:-/tmp/shepherd-e2e-server.log}"
INTERNAL_API_URL="${INTERNAL_API_URL:-http://127.0.0.1:${SERVER_PORT}}"
SERVER_BIN="${E2E_SERVER_BIN:-/tmp/shepherd-e2e-server-bin}"
# Use same-origin API path by default to avoid browser CORS between Playwright web port
# and backend random port. Next.js rewrite (INTERNAL_API_URL) forwards /api/v1 to backend.
# Keep env override support for explicit direct-base testing when needed.
NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-/api/v1}"
if [[ -n "${PW_WEB_PORT:-}" ]]; then
  PW_WEB_PORT="$PW_WEB_PORT"
else
  PW_WEB_PORT="$(pick_free_port || true)"
  if [[ -z "$PW_WEB_PORT" ]]; then
    echo "ERROR: unable to allocate free Playwright web port"
    exit 1
  fi
fi
PW_BASE_URL="${PW_BASE_URL:-http://127.0.0.1:${PW_WEB_PORT}}"

export SERVER_PORT
export INTERNAL_API_URL
export NEXT_PUBLIC_API_URL
export PW_WEB_PORT
export PW_BASE_URL
export DATABASE_AUTO_MIGRATE="${DATABASE_AUTO_MIGRATE:-true}"
export SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET:-0123456789abcdef0123456789abcdef}"
export SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
# Strict live e2e runs on random Playwright web ports; allow all origins in this
# test harness to prevent CORS false negatives unrelated to product behavior.
export SERVER_UNSAFE_ALLOW_ALL_ORIGINS="${SERVER_UNSAFE_ALLOW_ALL_ORIGINS:-true}"
export E2E_USERNAME="${E2E_USERNAME:-${E2E_ADMIN_USERNAME:-e2e-admin}}"
export E2E_PASSWORD="${E2E_PASSWORD:-${E2E_ADMIN_PASSWORD:-e2e-admin-123}}"

SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT INT TERM

if port_in_use "$SERVER_PORT"; then
  echo "ERROR: backend port ${SERVER_PORT} is already in use"
  exit 1
fi

echo "INFO: building backend server binary"
go build -o "$SERVER_BIN" ./cmd/server

echo "INFO: starting backend server on :${SERVER_PORT}"
"$SERVER_BIN" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

echo "INFO: waiting for backend readiness (${API_BASE_URL}/api/v1/health/live)"
READY=0
for _ in $(seq 1 120); do
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    echo "ERROR: backend server process exited before readiness"
    echo "INFO: tailing server log ($SERVER_LOG)"
    tail -n 120 "$SERVER_LOG" || true
    exit 1
  fi
  if curl -fsS "${API_BASE_URL}/api/v1/health/live" >/dev/null; then
    READY=1
    break
  fi
  sleep 1
done

if [[ "$READY" -ne 1 ]]; then
  echo "ERROR: backend server did not become ready"
  echo "INFO: tailing server log ($SERVER_LOG)"
  tail -n 120 "$SERVER_LOG" || true
  exit 1
fi

echo "INFO: seeding baseline data"
go run ./cmd/seed
go run ./cmd/e2e-seed

echo "INFO: running live Playwright E2E suite (no mock routes)"
set +e
CI=1 npm --prefix web run test:e2e:live -- "$@"
RUN_EXIT_CODE=$?
set -e

if [[ -n "${RUN_RESULT_FILE:-}" ]]; then
  {
    echo "exit_code=${RUN_EXIT_CODE}"
    if [[ -n "${RUN_LOG_FILE:-}" ]] && result_line="$(extract_result_summary "${RUN_LOG_FILE}")"; then
      echo "${result_line}"
    else
      echo "summary=unavailable"
    fi
  } >"${RUN_RESULT_FILE}"
fi

exit "${RUN_EXIT_CODE}"
