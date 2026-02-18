#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

PG_IMAGE="${PG_IMAGE:-postgres:16}"
PG_USER="${PG_USER:-shepherd}"
PG_PASSWORD="${PG_PASSWORD:-shepherd}"
PG_DB="${PG_DB:-shepherd_test}"
PG_HOST="${PG_HOST:-127.0.0.1}"
PG_WAIT_TIMEOUT_SEC="${PG_WAIT_TIMEOUT_SEC:-90}"
KEEP_CONTAINER=0

usage() {
  cat <<'EOF'
Usage:
  scripts/run_with_docker_pg.sh [options] [-- command...]

Options:
  --keep                Keep container after command exits (for debugging)
  --image <image>       PostgreSQL image (default: postgres:16)
  --timeout <seconds>   Health wait timeout (default: 90)
  -h, --help            Show this help

Environment overrides:
  PG_IMAGE, PG_USER, PG_PASSWORD, PG_DB, PG_HOST, PG_WAIT_TIMEOUT_SEC

Default command (when no command is provided):
  go test -count=1 ./internal/api/handlers ./internal/governance/approval ./internal/usecase ./internal/jobs ./internal/repository/sqlc ./internal/service

Examples:
  scripts/run_with_docker_pg.sh
  scripts/run_with_docker_pg.sh -- make master-flow-strict
  scripts/run_with_docker_pg.sh -- go test -count=1 ./internal/repository/sqlc -run TestMarkTicketApprovedAtomic
EOF
}

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker command not found"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "ERROR: docker daemon is not available"
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
    echo "INFO: keeping container ${CONTAINER_NAME}"
    return
  fi
  docker rm -f "${CONTAINER_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "INFO: starting PostgreSQL test container ${CONTAINER_NAME} (${PG_IMAGE})"
docker run -d \
  --name "${CONTAINER_NAME}" \
  -e POSTGRES_USER="${PG_USER}" \
  -e POSTGRES_PASSWORD="${PG_PASSWORD}" \
  -e POSTGRES_DB="${PG_DB}" \
  -p "${PG_HOST}::5432" \
  --health-cmd "pg_isready -U ${PG_USER} -d ${PG_DB}" \
  --health-interval 1s \
  --health-timeout 3s \
  --health-retries 60 \
  "${PG_IMAGE}" >/dev/null

PG_PORT=""
for _ in $(seq 1 30); do
  RAW_PORT="$(docker port "${CONTAINER_NAME}" 5432/tcp 2>/dev/null | tail -n 1 || true)"
  if [[ -n "${RAW_PORT}" ]]; then
    PG_PORT="${RAW_PORT##*:}"
    break
  fi
  sleep 1
done

if [[ -z "${PG_PORT}" ]]; then
  echo "ERROR: unable to determine mapped PostgreSQL port"
  docker logs "${CONTAINER_NAME}" || true
  exit 1
fi

DEADLINE=$((SECONDS + PG_WAIT_TIMEOUT_SEC))
while true; do
  HEALTH="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${CONTAINER_NAME}" 2>/dev/null || true)"
  if [[ "${HEALTH}" == "healthy" ]]; then
    break
  fi
  if [[ "${HEALTH}" == "unhealthy" ]]; then
    echo "ERROR: PostgreSQL container became unhealthy"
    docker logs "${CONTAINER_NAME}" || true
    exit 1
  fi
  if (( SECONDS >= DEADLINE )); then
    echo "ERROR: timed out waiting for PostgreSQL health (${PG_WAIT_TIMEOUT_SEC}s)"
    docker logs "${CONTAINER_NAME}" || true
    exit 1
  fi
  sleep 1
done

TEST_DSN="postgres://${PG_USER}:${PG_PASSWORD}@${PG_HOST}:${PG_PORT}/${PG_DB}?sslmode=disable"
echo "INFO: PostgreSQL is healthy on ${PG_HOST}:${PG_PORT}"
echo "INFO: running command: ${COMMAND[*]}"

TEST_DATABASE_URL="${TEST_DSN}" DATABASE_URL="${TEST_DSN}" "${COMMAND[@]}"
