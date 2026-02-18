#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

NO_DB_WRAPPER=0
if [[ "${1:-}" == "--no-db-wrapper" ]]; then
  NO_DB_WRAPPER=1
  shift
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  if [[ "$NO_DB_WRAPPER" -eq 1 ]]; then
    echo "ERROR: DATABASE_URL is required when --no-db-wrapper is set"
    exit 1
  fi
  exec ./scripts/run_with_docker_pg.sh -- bash ./scripts/run_e2e_live.sh --no-db-wrapper "$@"
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "ERROR: curl command not found"
  exit 1
fi

port_in_use() {
  local port="$1"
  ss -ltn | awk '{print $4}' | grep -Eq "(^|:|\\.)${port}$"
}

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
CI=1 npm --prefix web run test:e2e:live -- "$@"
