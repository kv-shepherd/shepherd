#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/dev/docker-compose.yml"
HOST_USER_ID="${USER_ID:-$(id -u)}"
HOST_GROUP_ID="${GROUP_ID:-$(id -g)}"
DEV_ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin123}"
NODE_MODULES_DIR="${ROOT_DIR}/web/node_modules"
LOCK_HASH_FILE="${NODE_MODULES_DIR}/.package-lock.hash"
SERVICES_TO_DELETE=("db" "server" "web" "nginx")
COMPOSE_CMD=(docker compose -f "${COMPOSE_FILE}")
DEV_INCLUDE_E2E_SEED="${DEV_INCLUDE_E2E_SEED:-1}"

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

json_string() {
    node -p 'JSON.stringify(process.argv[1])' "$1"
}

login_token() {
    local username="$1"
    local password="$2"
    local response=""

    response="$(
        curl -fsS http://localhost:8080/api/v1/auth/login \
            -H "Content-Type: application/json" \
            -d "{\"username\":$(json_string "$username"),\"password\":$(json_string "$password")}"
    )" || return 1

    printf '%s' "$response" | node -e '
        let data = "";
        process.stdin.on("data", (chunk) => { data += chunk; });
        process.stdin.on("end", () => {
            const parsed = JSON.parse(data);
            if (typeof parsed.token !== "string" || parsed.token.trim() === "") {
                process.exit(1);
            }
            process.stdout.write(parsed.token.trim());
        });
    '
}

rotate_default_admin_password() {
    local bootstrap_password="admin"
    local target_password="$1"
    local token=""

    if [[ "${target_password}" == "${bootstrap_password}" ]]; then
        echo " admin password left at bootstrap default"
        return 0
    fi

    if token="$(login_token admin "${target_password}" 2>/dev/null)"; then
        echo " admin password already rotated"
        return 0
    fi

    token="$(login_token admin "${bootstrap_password}")" || {
        echo " failed to login with bootstrap admin credentials"
        return 1
    }

    curl -fsS http://localhost:8080/api/v1/auth/change-password \
        -X POST \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "{\"old_password\":$(json_string "${bootstrap_password}"),\"new_password\":$(json_string "${target_password}")}" \
        >/dev/null

    echo " admin password rotated to admin/${target_password}"
}

require_cmd docker
require_cmd go
require_cmd npm
require_cmd node
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
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/e2e-seed ./cmd/e2e-seed/...
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

echo "Seeding development data..."
"${COMPOSE_CMD[@]}" exec -T server /usr/local/bin/seed >/dev/null
rotate_default_admin_password "${DEV_ADMIN_PASSWORD}"
if [[ "${DEV_INCLUDE_E2E_SEED}" == "1" ]]; then
    "${COMPOSE_CMD[@]}" exec -T server /usr/local/bin/e2e-seed >/dev/null
    echo " seed complete (baseline + extended fixtures)"
else
    echo " seed complete (baseline only; set DEV_INCLUDE_E2E_SEED=1 to include extended fixtures)"
fi

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
echo "  - Seeded users:        admin/${DEV_ADMIN_PASSWORD} (rotated from bootstrap admin/admin)"
if [[ "${DEV_INCLUDE_E2E_SEED}" == "1" ]]; then
    echo "                         e2e-admin/e2e-admin-123"
fi
