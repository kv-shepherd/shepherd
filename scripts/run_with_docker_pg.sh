#!/usr/bin/env bash

set -euo pipefail

LOG_TS_FORMAT="${LOG_TS_FORMAT:-%Y-%m-%dT%H:%M:%S%z}"
ts_now() {
  date +"${LOG_TS_FORMAT}"
}

log_info() {
  printf '%s INFO: %s\n' "$(ts_now)" "$*"
}

log_warn() {
  printf '%s WARN: %s\n' "$(ts_now)" "$*"
}

log_error() {
  printf '%s ERROR: %s\n' "$(ts_now)" "$*" >&2
}

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

PG_IMAGE="${PG_IMAGE:-postgres:18}"
PG_USER="${PG_USER:-shepherd}"
PG_PASSWORD="${PG_PASSWORD:-shepherd}"
PG_DB="${PG_DB:-shepherd_test}"
PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-}"
PG_WAIT_TIMEOUT_SEC="${PG_WAIT_TIMEOUT_SEC:-90}"
KEEP_CONTAINER=0

usage() {
  cat <<'EOF'
Usage:
  scripts/run_with_docker_pg.sh [options] [-- command...]

Options:
  --keep                Keep container after command exits (for debugging)
  --image <image>       PostgreSQL image (default: postgres:18)
  --port <port>         Fixed host port mapping (default: random host port)
  --timeout <seconds>   Health wait timeout (default: 90)
  -h, --help            Show this help

Environment overrides:
  PG_IMAGE, PG_USER, PG_PASSWORD, PG_DB, PG_HOST, PG_PORT, PG_WAIT_TIMEOUT_SEC

Default command (when no command is provided):
  go test -count=1 ./internal/api/handlers ./internal/governance/approval ./internal/usecase ./internal/jobs ./internal/repository/sqlc ./internal/service

Examples:
  scripts/run_with_docker_pg.sh
  scripts/run_with_docker_pg.sh -- make master-flow-strict
  scripts/run_with_docker_pg.sh -- go test -count=1 ./internal/repository/sqlc -run TestMarkTicketApprovedAtomic
EOF
}

if ! command -v docker >/dev/null 2>&1; then
  log_error "docker command not found"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  log_error "docker daemon is not available"
  exit 1
fi

COMMAND=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --keep)
      KEEP_CONTAINER=1
      shift
      ;;
    --image)
      PG_IMAGE="$2"
      shift 2
      ;;
    --port)
      PG_PORT="$2"
      shift 2
      ;;
    --timeout)
      PG_WAIT_TIMEOUT_SEC="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      COMMAND=("$@")
      break
      ;;
    *)
      COMMAND+=("$1")
      shift
      ;;
  esac
done

if [[ "${#COMMAND[@]}" -eq 0 ]]; then
  COMMAND=(
    go test -count=1
    ./internal/api/handlers
    ./internal/governance/approval
    ./internal/usecase
    ./internal/jobs
    ./internal/repository/sqlc
    ./internal/service
  )
fi

CONTAINER_NAME="shepherd-test-pg-$(date +%s)-$RANDOM"

cleanup() {
  if [[ "$KEEP_CONTAINER" -eq 1 ]]; then
    log_info "keeping container ${CONTAINER_NAME}"
    return
  fi
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

log_info "starting PostgreSQL test container ${CONTAINER_NAME} (${PG_IMAGE})"

DOCKER_PORT_ARGS=()
if [[ -n "${PG_PORT}" ]]; then
  DOCKER_PORT_ARGS=(-p "${PG_HOST}:${PG_PORT}:5432")
else
  DOCKER_PORT_ARGS=(-p "${PG_HOST}::5432")
fi

RUN_ERR_LOG="$(mktemp)"
HOST_NETWORK_MODE=0
if ! docker run -d \
  --name "${CONTAINER_NAME}" \
  -e POSTGRES_USER="${PG_USER}" \
  -e POSTGRES_PASSWORD="${PG_PASSWORD}" \
  -e POSTGRES_DB="${PG_DB}" \
  "${DOCKER_PORT_ARGS[@]}" \
  --health-cmd "pg_isready -U ${PG_USER} -d ${PG_DB}" \
  --health-interval 1s \
  --health-timeout 3s \
  --health-retries 60 \
  "${PG_IMAGE}" >/dev/null 2>"${RUN_ERR_LOG}"; then
  if [[ -n "${PG_PORT}" ]] && rg -q "Unable to enable OPEN PORT rule|failed to set up container networking" "${RUN_ERR_LOG}"; then
    log_warn "bridge networking failed; retrying PostgreSQL container in host mode on port ${PG_PORT}"
    docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
    if ! docker run -d \
      --name "${CONTAINER_NAME}" \
      --network host \
      -e POSTGRES_USER="${PG_USER}" \
      -e POSTGRES_PASSWORD="${PG_PASSWORD}" \
      -e POSTGRES_DB="${PG_DB}" \
      --health-cmd "pg_isready -U ${PG_USER} -d ${PG_DB} -p ${PG_PORT}" \
      --health-interval 1s \
      --health-timeout 3s \
      --health-retries 60 \
      "${PG_IMAGE}" -p "${PG_PORT}" >/dev/null 2>"${RUN_ERR_LOG}"; then
      cat "${RUN_ERR_LOG}" >&2
      rm -f "${RUN_ERR_LOG}"
      exit 1
    fi
    HOST_NETWORK_MODE=1
  else
    cat "${RUN_ERR_LOG}" >&2
    rm -f "${RUN_ERR_LOG}"
    exit 1
  fi
fi
rm -f "${RUN_ERR_LOG}"

if [[ -z "${PG_PORT}" ]]; then
  for _ in $(seq 1 30); do
    RAW_PORT="$(docker port "${CONTAINER_NAME}" 5432/tcp 2>/dev/null | tail -n 1 || true)"
    if [[ -n "${RAW_PORT}" ]]; then
      PG_PORT="${RAW_PORT##*:}"
      break
    fi
    sleep 1
  done

  if [[ -z "${PG_PORT}" ]]; then
    log_error "unable to determine mapped PostgreSQL port"
    docker logs "${CONTAINER_NAME}" || true
    exit 1
  fi
fi

DEADLINE=$((SECONDS + PG_WAIT_TIMEOUT_SEC))
while true; do
  HEALTH="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${CONTAINER_NAME}" 2>/dev/null || true)"
  if [[ "${HEALTH}" == "healthy" ]]; then
    break
  fi
  if [[ "${HEALTH}" == "unhealthy" ]]; then
    log_error "PostgreSQL container became unhealthy"
    docker logs "${CONTAINER_NAME}" || true
    exit 1
  fi
  if (( SECONDS >= DEADLINE )); then
    log_error "timed out waiting for PostgreSQL health (${PG_WAIT_TIMEOUT_SEC}s)"
    docker logs "${CONTAINER_NAME}" || true
    exit 1
  fi
  sleep 1
done

TEST_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DB}?sslmode=disable"
if [[ "${HOST_NETWORK_MODE}" -eq 1 ]]; then
  log_info "PostgreSQL is healthy on ${PG_HOST}:${PG_PORT} (host network mode)"
else
  log_info "PostgreSQL is healthy on ${PG_HOST}:${PG_PORT}"
fi
log_info "running command: ${COMMAND[*]}"

TEST_DATABASE_URL="${TEST_DSN}" DATABASE_URL="${TEST_DSN}" "${COMMAND[@]}"
