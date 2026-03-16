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
DEV_FRONTEND_MODE="${DEV_FRONTEND_MODE:-host}"
DEV_FRONTEND_PORT="${DEV_FRONTEND_PORT:-3001}"
DEV_INGRESS_PORT="${DEV_INGRESS_PORT:-3000}"
DEV_FRONTEND_RUNTIME="${DEV_FRONTEND_RUNTIME:-dev}" # dev|prod
DEV_FRONTEND_PROD_DIST_DIR="${DEV_FRONTEND_PROD_DIST_DIR:-.next-prod}"
# Dev-only tuning:
# - Source maps improve stack traces but can increase memory/CPU usage.
DEV_FRONTEND_DISABLE_SOURCE_MAPS="${DEV_FRONTEND_DISABLE_SOURCE_MAPS:-0}"
# Frontend OOM guard defaults:
# - enabled by default (no swap on many dev machines makes kernel OOM-kill likely)
# - applies only to the Next.js dev server process (host or docker)
DEV_FRONTEND_OOM_GUARD="${DEV_FRONTEND_OOM_GUARD:-1}"
DEV_FRONTEND_OOM_GUARD_MAX_OLD_SPACE_MB="${DEV_FRONTEND_OOM_GUARD_MAX_OLD_SPACE_MB:-3072}"
# Optional: override NODE_OPTIONS for the Next.js dev server only (host mode).
# Example:
#   DEV_FRONTEND_NODE_OPTIONS="--max-old-space-size=4096 --heapsnapshot-signal=SIGUSR2"
DEV_FRONTEND_NODE_OPTIONS="${DEV_FRONTEND_NODE_OPTIONS:-}"
FRONTEND_PID_FILE="${ROOT_DIR}/tmp/dev-web.pid"
FRONTEND_LOG_FILE="${ROOT_DIR}/tmp/dev-web.log"
KEEP_DB=0
# Default to webpack for stability; Turbopack can consume excessive memory in some dev scenarios.
DEV_FRONTEND_BUILDER="${DEV_FRONTEND_BUILDER:-webpack}"

usage() {
    cat <<'EOF'
Usage: ./start-dev.sh [options]

Options:
  --keep-db          Preserve the existing dev PostgreSQL container/data and only
                     recreate app services.
  --frontend-docker  Run the frontend inside Docker instead of the default host
                     Next.js dev server. This is slower but useful as a fallback.
  --frontend-prod    Run the host frontend in production mode:
                     - next build (into DEV_FRONTEND_PROD_DIST_DIR, default: .next-prod)
                     - next start (no HMR)
  --webpack          Use the webpack builder for the host Next.js dev server.
                     Useful when Turbopack exhibits high memory usage.
  --turbopack        Use Turbopack (Next.js default) for the host Next.js dev server.
  --no-oom-guard     Disable the default Next.js dev server heap limit guard.
  --disable-source-maps
                     Disable source maps for host Next.js dev server (lower memory/CPU, worse stack traces).
  -h, --help         Show this help message.
EOF
}

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

base64_file() {
    local file="$1"
    if base64 -w0 "$file" >/dev/null 2>&1; then
        base64 -w0 "$file"
    else
        base64 <"$file" | tr -d '\n'
    fi
}

json_string() {
    node -p 'JSON.stringify(process.argv[1])' "$1"
}

compute_allowed_dev_origins() {
    node -e '
        const os = require("os");
        const origins = new Set(["localhost", "127.0.0.1"]);
        const hostname = os.hostname();
        if (hostname) {
            origins.add(hostname);
        }
        for (const infos of Object.values(os.networkInterfaces())) {
            for (const info of infos || []) {
                if (info && info.family === "IPv4" && !info.internal) {
                    origins.add(info.address);
                }
            }
        }
        process.stdout.write(Array.from(origins).join(","));
    '
}

stop_host_frontend() {
    if [[ ! -f "${FRONTEND_PID_FILE}" ]]; then
        return 0
    fi

    local pid=""
    pid="$(cat "${FRONTEND_PID_FILE}" 2>/dev/null || true)"
    if [[ -n "${pid}" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
        echo "Stopping existing host frontend (pid ${pid})..."
        # Kill the entire process group created by setsid.
        kill -- -"${pid}" >/dev/null 2>&1 || true
        for _ in {1..20}; do
            if ! kill -0 "${pid}" >/dev/null 2>&1; then
                break
            fi
            sleep 1
        done
    fi
    rm -f "${FRONTEND_PID_FILE}"
}

start_host_frontend() {
    local node_options="${DEV_FRONTEND_NODE_OPTIONS}"
    if [[ -z "${node_options}" ]] && [[ "${DEV_FRONTEND_OOM_GUARD}" == "1" ]]; then
        node_options="--max-old-space-size=${DEV_FRONTEND_OOM_GUARD_MAX_OLD_SPACE_MB} --heapsnapshot-signal=SIGUSR2"
    fi
    mkdir -p "$(dirname "${FRONTEND_PID_FILE}")"
    stop_host_frontend

    : > "${FRONTEND_LOG_FILE}"

    if [[ "${DEV_FRONTEND_RUNTIME}" == "prod" ]]; then
        echo "Building frontend on host (Next.js production build)..."
        echo "  - distDir: ${DEV_FRONTEND_PROD_DIST_DIR}"
        if [[ -n "${node_options}" ]]; then
            echo "  - NODE_OPTIONS: ${node_options}"
        fi

        (
            cd "${ROOT_DIR}/web"
            NEXT_PUBLIC_API_URL="/api/v1" \
            INTERNAL_API_URL="http://localhost:8080" \
            NEXT_DIST_DIR="${DEV_FRONTEND_PROD_DIST_DIR}" \
            NODE_OPTIONS="${node_options}" \
            ./node_modules/.bin/next build >>"${FRONTEND_LOG_FILE}" 2>&1
        ) || {
            echo " frontend build failed"
            tail -n 200 "${FRONTEND_LOG_FILE}" || true
            return 1
        }

        echo "Starting frontend on host (Next.js production server on :${DEV_FRONTEND_PORT})..."
        (
            cd "${ROOT_DIR}/web"
            NEXT_PUBLIC_API_URL="/api/v1" \
            INTERNAL_API_URL="http://localhost:8080" \
            NEXT_DIST_DIR="${DEV_FRONTEND_PROD_DIST_DIR}" \
            NODE_OPTIONS="${node_options}" \
            setsid ./node_modules/.bin/next start --hostname 0.0.0.0 --port "${DEV_FRONTEND_PORT}" >>"${FRONTEND_LOG_FILE}" 2>&1 < /dev/null &
            echo $! > "${FRONTEND_PID_FILE}"
        )
        return 0
    fi

    local allowed_origins=""
    allowed_origins="$(compute_allowed_dev_origins)"
    local next_args=()
    if [[ "${DEV_FRONTEND_BUILDER}" == "webpack" ]]; then
        next_args+=(--webpack)
    fi
    if [[ "${DEV_FRONTEND_DISABLE_SOURCE_MAPS}" == "1" ]]; then
        next_args+=(--disable-source-maps)
    fi

    echo "Starting frontend on host (Next.js dev server on :${DEV_FRONTEND_PORT})..."
    echo "  - builder: ${DEV_FRONTEND_BUILDER}"
    if [[ "${DEV_FRONTEND_DISABLE_SOURCE_MAPS}" == "1" ]]; then
        echo "  - source maps: disabled"
    fi
    if [[ -n "${node_options}" ]]; then
        echo "  - NODE_OPTIONS: ${node_options}"
    fi
    (
        cd "${ROOT_DIR}/web"
        DEV_ALLOWED_ORIGINS="${allowed_origins}" \
        NEXT_PUBLIC_API_URL="/api/v1" \
        INTERNAL_API_URL="http://localhost:8080" \
        NODE_OPTIONS="${node_options}" \
        setsid ./node_modules/.bin/next dev "${next_args[@]}" --hostname 0.0.0.0 --port "${DEV_FRONTEND_PORT}" >"${FRONTEND_LOG_FILE}" 2>&1 < /dev/null &
        echo $! > "${FRONTEND_PID_FILE}"
    )
}

wait_for_host_frontend() {
    local pid=""
    pid="$(cat "${FRONTEND_PID_FILE}" 2>/dev/null || true)"

    echo "Waiting for frontend (http://127.0.0.1:${DEV_FRONTEND_PORT})..."
    for _ in {1..45}; do
        if curl -fsS "http://127.0.0.1:${DEV_FRONTEND_PORT}/" >/dev/null; then
            echo " frontend ready"
            return 0
        fi
        if [[ -n "${pid}" ]] && ! kill -0 "${pid}" >/dev/null 2>&1; then
            echo " frontend exited unexpectedly"
            tail -n 200 "${FRONTEND_LOG_FILE}" || true
            return 1
        fi
        printf "."
        sleep 2
    done

    echo " frontend did not become ready in time"
    tail -n 200 "${FRONTEND_LOG_FILE}" || true
    return 1
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

while [[ $# -gt 0 ]]; do
    case "$1" in
        --keep-db)
            KEEP_DB=1
            shift
            ;;
        --frontend-docker)
            DEV_FRONTEND_MODE="docker"
            shift
            ;;
        --frontend-prod)
            DEV_FRONTEND_RUNTIME="prod"
            shift
            ;;
        --webpack)
            DEV_FRONTEND_BUILDER="webpack"
            shift
            ;;
        --turbopack)
            DEV_FRONTEND_BUILDER="turbopack"
            shift
            ;;
        --no-oom-guard)
            DEV_FRONTEND_OOM_GUARD=0
            shift
            ;;
        --disable-source-maps)
            DEV_FRONTEND_DISABLE_SOURCE_MAPS=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1"
            echo ""
            usage
            exit 1
            ;;
    esac
done

if ! [[ "${HOST_USER_ID}" =~ ^[0-9]+$ ]] || ! [[ "${HOST_GROUP_ID}" =~ ^[0-9]+$ ]]; then
    echo "USER_ID/GROUP_ID must be numeric. USER_ID=${HOST_USER_ID}, GROUP_ID=${HOST_GROUP_ID}"
    exit 1
fi

if [[ "${DEV_FRONTEND_MODE}" != "host" && "${DEV_FRONTEND_MODE}" != "docker" ]]; then
    echo "DEV_FRONTEND_MODE must be 'host' or 'docker'. Current value: ${DEV_FRONTEND_MODE}"
    exit 1
fi

if [[ "${DEV_FRONTEND_RUNTIME}" != "dev" && "${DEV_FRONTEND_RUNTIME}" != "prod" ]]; then
    echo "DEV_FRONTEND_RUNTIME must be 'dev' or 'prod'. Current value: ${DEV_FRONTEND_RUNTIME}"
    exit 1
fi

if [[ "${DEV_FRONTEND_BUILDER}" != "webpack" && "${DEV_FRONTEND_BUILDER}" != "turbopack" ]]; then
    echo "DEV_FRONTEND_BUILDER must be 'webpack' or 'turbopack'. Current value: ${DEV_FRONTEND_BUILDER}"
    exit 1
fi

if [[ "${DEV_FRONTEND_OOM_GUARD}" != "0" && "${DEV_FRONTEND_OOM_GUARD}" != "1" ]]; then
    echo "DEV_FRONTEND_OOM_GUARD must be '0' or '1'. Current value: ${DEV_FRONTEND_OOM_GUARD}"
    exit 1
fi

if [[ "${DEV_FRONTEND_DISABLE_SOURCE_MAPS}" != "0" && "${DEV_FRONTEND_DISABLE_SOURCE_MAPS}" != "1" ]]; then
    echo "DEV_FRONTEND_DISABLE_SOURCE_MAPS must be '0' or '1'. Current value: ${DEV_FRONTEND_DISABLE_SOURCE_MAPS}"
    exit 1
fi

if [[ "${DEV_FRONTEND_RUNTIME}" == "prod" && "${DEV_FRONTEND_MODE}" != "host" ]]; then
    echo "--frontend-prod only supports the host frontend. Remove --frontend-docker or set DEV_FRONTEND_MODE=host."
    exit 1
fi

WEB_UPSTREAM="host.docker.internal:${DEV_FRONTEND_PORT}"
COMPOSE_SERVICES=("db" "server" "nginx")
if [[ "${DEV_FRONTEND_MODE}" == "docker" ]]; then
    WEB_UPSTREAM="web:3000"
    COMPOSE_SERVICES=("db" "server" "web" "nginx")
fi

echo "Checking development environment status..."
stop_host_frontend
if [[ "${KEEP_DB}" == "1" ]]; then
    echo "Resetting development environment (preserve DB container/data)..."
    for svc in server web nginx; do
        echo "  Removing service: $svc"
        "${COMPOSE_CMD[@]}" rm -s -f -v "$svc" || true
    done
    echo "  Preserving service: db"
else
    echo "Resetting development environment (clear DB data every run)..."
    for svc in "${SERVICES_TO_DELETE[@]}"; do
        echo "  Removing service: $svc"
        "${COMPOSE_CMD[@]}" rm -s -f -v "$svc" || true
    done
    "${COMPOSE_CMD[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
fi
echo "Cleanup complete."

echo "Building backend binaries on host (reuse local Go cache)..."
mkdir -p "${ROOT_DIR}/build/bin"
(
    cd "${ROOT_DIR}"
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/shepherd ./cmd/server/...
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/seed ./cmd/seed/...
    GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/e2e-seed ./cmd/e2e-seed/...
)

echo "Packaging backend image (shepherd-server)..."
DOCKER_BUILDKIT=1 docker build --network=host \
    --target dev-runtime \
    -t shepherd-server -f "${ROOT_DIR}/Dockerfile" "${ROOT_DIR}"

current_lock_hash="$(compute_sha256 "${ROOT_DIR}/web/package-lock.json")"
if [ ! -d "${NODE_MODULES_DIR}" ] || [ ! -f "${LOCK_HASH_FILE}" ] || [ "$(cat "${LOCK_HASH_FILE}" 2>/dev/null || true)" != "${current_lock_hash}" ]; then
    echo "Installing frontend dependencies into ${NODE_MODULES_DIR}..."
    (cd "${ROOT_DIR}/web" && npm ci)
    mkdir -p "${NODE_MODULES_DIR}"
    printf "%s" "${current_lock_hash}" > "${LOCK_HASH_FILE}"
else
    echo "Reusing frontend dependencies from ${NODE_MODULES_DIR}..."
fi

if [[ "${DEV_FRONTEND_MODE}" == "docker" ]]; then
    echo "Packaging frontend image (shepherd-web)..."
    DOCKER_BUILDKIT=1 docker build --network=host \
        --build-arg "USER_ID=${HOST_USER_ID}" \
        --build-arg "GROUP_ID=${HOST_GROUP_ID}" \
        -t shepherd-web -f "${ROOT_DIR}/deploy/dev/web.Dockerfile" "${ROOT_DIR}/web"
fi

echo "Starting development environment (${COMPOSE_SERVICES[*]})..."
USER_ID="${HOST_USER_ID}" \
GROUP_ID="${HOST_GROUP_ID}" \
WEB_UPSTREAM="${WEB_UPSTREAM}" \
"${COMPOSE_CMD[@]}" up -d "${COMPOSE_SERVICES[@]}"

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
if [[ "${backend_ready}" != "true" ]]; then
    echo " backend did not become ready in time"
    "${COMPOSE_CMD[@]}" logs --tail=200 server || true
    exit 1
fi

echo "Seeding development data..."
"${COMPOSE_CMD[@]}" exec -T server /usr/local/bin/seed >/dev/null
rotate_default_admin_password "${DEV_ADMIN_PASSWORD}"
if [[ "${DEV_INCLUDE_E2E_SEED}" == "1" ]]; then
    E2E_SEED_ENV=()
    DEV_KUBECONFIG_FILE="${ROOT_DIR}/k8s-admin.yaml"
    if [[ -f "${DEV_KUBECONFIG_FILE}" ]]; then
        echo " importing live dev cluster from ${DEV_KUBECONFIG_FILE}"
        E2E_SEED_ENV=(-e "E2E_KUBECONFIG_B64=$(base64_file "${DEV_KUBECONFIG_FILE}")")
    else
        echo " no local k8s-admin.yaml found; e2e seed will register an unreachable stub cluster"
    fi
    "${COMPOSE_CMD[@]}" exec -T "${E2E_SEED_ENV[@]}" server /usr/local/bin/e2e-seed >/dev/null
    echo " seed complete (baseline + extended fixtures)"
else
    echo " seed complete (baseline only; set DEV_INCLUDE_E2E_SEED=1 to include extended fixtures)"
fi

if [[ "${DEV_FRONTEND_MODE}" == "host" ]]; then
    start_host_frontend
    wait_for_host_frontend
fi

echo "Waiting for ingress (http://localhost:${DEV_INGRESS_PORT})..."
for _ in {1..30}; do
    if curl -fsS "http://localhost:${DEV_INGRESS_PORT}/" >/dev/null; then
        echo " ingress ready"
        break
    fi
    printf "."
    sleep 2
done

echo "Prewarming common routes..."
for route in / /login /dashboard; do
    curl -fsS "http://localhost:${DEV_INGRESS_PORT}${route}" >/dev/null || true
done
echo " warmup complete"

echo ""
echo "Development environment is UP"
echo "  - Web (nginx ingress): http://localhost:${DEV_INGRESS_PORT}"
echo "  - Backend direct:      http://localhost:8080"
echo "  - DB:                  localhost:5432"
echo "  - Frontend mode:       ${DEV_FRONTEND_MODE}"
if [[ "${DEV_FRONTEND_MODE}" == "host" ]]; then
    echo "  - Frontend direct:     http://localhost:${DEV_FRONTEND_PORT}"
    echo "  - Frontend log:        ${FRONTEND_LOG_FILE}"
fi
if [[ "${KEEP_DB}" == "1" ]]; then
    echo "  - DB reset mode:       preserved (--keep-db)"
else
    echo "  - DB reset mode:       rebuilt (default)"
fi
echo "  - Seeded users:        admin/${DEV_ADMIN_PASSWORD} (rotated from bootstrap admin/admin)"
if [[ "${DEV_INCLUDE_E2E_SEED}" == "1" ]]; then
    echo "                         e2e-admin/e2e-admin-123"
fi
