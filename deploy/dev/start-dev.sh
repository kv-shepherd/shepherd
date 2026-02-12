#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/dev/docker-compose.yml"
HOST_USER_ID="${USER_ID:-$(id -u)}"
HOST_GROUP_ID="${GROUP_ID:-$(id -g)}"
NODE_MODULES_DIR="${ROOT_DIR}/web/node_modules"
LOCK_HASH_FILE="${NODE_MODULES_DIR}/.package-lock.hash"
SERVICES_TO_DELETE=("db" "server" "web" "nginx")
COMPOSE_CMD=(docker compose -f "${COMPOSE_FILE}")

require_cmd() {
    local cmd="$1"
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo "Missing required command: $cmd"
        exit 1
    fi
}

compute_sha256() {
    local file="$1"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

require_cmd docker
require_cmd go
require_cmd npm
require_cmd curl

if ! [[ "$HOST_USER_ID" =~ ^[0-9]+$ ]] || ! [[ "$HOST_GROUP_ID" =~ ^[0-9]+$ ]]; then
    echo "USER_ID/GROUP_ID must be numeric. USER_ID=${HOST_USER_ID}, GROUP_ID=${HOST_GROUP_ID}"
    exit 1
fi

echo "Checking development environment status..."
echo "Resetting development environment (clear DB data every run)..."

for svc in "${SERVICES_TO_DELETE[@]}"; do
    echo "  Removing service: $svc"
    "${COMPOSE_CMD[@]}" rm -s -f -v "$svc" || true
done

"${COMPOSE_CMD[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
echo "Cleanup complete."

echo "Building backend binaries on host (reuse local Go cache)..."
mkdir -p "${ROOT_DIR}/build/bin"
(
    cd "$ROOT_DIR"
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/shepherd ./cmd/server/...
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/seed ./cmd/seed/...
)

echo "Packaging backend image (shepherd-server)..."
DOCKER_BUILDKIT=1 docker build --network=host \
    --target dev-runtime \
    -t shepherd-server -f "${ROOT_DIR}/Dockerfile" "${ROOT_DIR}"

current_lock_hash="$(compute_sha256 "${ROOT_DIR}/web/package-lock.json")"
if [ ! -d "$NODE_MODULES_DIR" ] || [ ! -f "$LOCK_HASH_FILE" ] || [ "$(cat "$LOCK_HASH_FILE" 2>/dev/null || true)" != "$current_lock_hash" ]; then
    echo "Installing frontend dependencies into ${NODE_MODULES_DIR}..."
    (cd "${ROOT_DIR}/web" && npm ci)
    mkdir -p "$NODE_MODULES_DIR"
    printf "%s" "$current_lock_hash" > "$LOCK_HASH_FILE"
else
    echo "Reusing frontend dependencies from ${NODE_MODULES_DIR}..."
fi

echo "Packaging frontend image (shepherd-web)..."
DOCKER_BUILDKIT=1 docker build --network=host \
    --build-arg "USER_ID=${HOST_USER_ID}" \
    --build-arg "GROUP_ID=${HOST_GROUP_ID}" \
    -t shepherd-web -f "${ROOT_DIR}/deploy/dev/web.Dockerfile" "${ROOT_DIR}/web"

echo "Starting development environment (db -> server -> web -> nginx)..."
USER_ID="$HOST_USER_ID" GROUP_ID="$HOST_GROUP_ID" "${COMPOSE_CMD[@]}" up -d

echo "Waiting for database..."
until "${COMPOSE_CMD[@]}" exec -T db pg_isready -U shepherd -d shepherd_db >/dev/null 2>&1; do
    printf "."
    sleep 2
done
echo " db ready"

echo "Waiting for backend (http://localhost:8080/api/v1/health/live)..."
backend_ready=false
for _ in {1..30}; do
    if curl -fsS http://localhost:8080/api/v1/health/live >/dev/null; then
        backend_ready=true
        echo " backend ready"
        break
    fi
    printf "."
    sleep 2
done
if [ "$backend_ready" != "true" ]; then
    echo " backend did not become ready in time"
    "${COMPOSE_CMD[@]}" logs --tail=200 server || true
    exit 1
fi

echo "Seeding default development data (admin/admin)..."
"${COMPOSE_CMD[@]}" exec -T server /usr/local/bin/seed >/dev/null
echo " seed complete"

echo "Waiting for ingress (http://localhost:3000)..."
for _ in {1..30}; do
    if curl -fsS http://localhost:3000/ >/dev/null; then
        echo " ingress ready"
        break
    fi
    printf "."
    sleep 2
done

echo "Prewarming common routes..."
for route in / /login /dashboard; do
    curl -fsS "http://localhost:3000${route}" >/dev/null || true
done
echo " warmup complete"

echo ""
echo "Development environment is UP"
echo "  - Web (nginx ingress): http://localhost:3000"
echo "  - Backend direct:      http://localhost:8080"
echo "  - DB:                  localhost:5432"
