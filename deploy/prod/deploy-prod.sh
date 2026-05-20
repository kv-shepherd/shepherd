#!/usr/bin/env bash
# =============================================================================
# KubeVirt Shepherd — Production Deployment Script
# =============================================================================
# Usage:
#   bash deploy/prod/deploy-prod.sh                    # Public deploy (no bootstrap seed)
#   bash deploy/prod/deploy-prod.sh --with-seed        # Public deploy + bootstrap seed
#   bash deploy/prod/deploy-prod.sh --with-seed --with-experience-seed
#                                                 # Public deploy + baseline seed + experience fixtures
#   bash deploy/prod/deploy-prod.sh --enterprise       # Enterprise edition
#   bash deploy/prod/deploy-prod.sh --build-only       # Build images only
#   bash deploy/prod/deploy-prod.sh --seed-only        # Run bootstrap seed only (after services are up)
# =============================================================================
set -euo pipefail

if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
    SCRIPT_DIR="${DEPLOY_DIR:-$(pwd)}"
fi
if [[ -f "${SCRIPT_DIR}/Dockerfile" && -f "${SCRIPT_DIR}/web/package.json" ]]; then
    ROOT_DIR="${SCRIPT_DIR}"
elif ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." 2>/dev/null && pwd)" \
    && [[ -f "${ROOT_DIR}/Dockerfile" && -f "${ROOT_DIR}/web/package.json" ]]; then
    :
else
    ROOT_DIR="$(pwd)"
fi
SOURCE_TREE_AVAILABLE=0
if [[ -f "${ROOT_DIR}/Dockerfile" && -f "${ROOT_DIR}/web/package.json" ]]; then
    SOURCE_TREE_AVAILABLE=1
fi
COMPOSE_FILE="${DEPLOY_COMPOSE_FILE:-${SCRIPT_DIR}/docker-compose.prod.yml}"
ENV_FILE="${DEPLOY_ENV_FILE:-${SCRIPT_DIR}/.env.prod}"
ENV_EXAMPLE_FILE="${DEPLOY_ENV_EXAMPLE_FILE:-${SCRIPT_DIR}/.env.prod.example}"
TLS_DIR="${DEPLOY_TLS_DIR:-${SCRIPT_DIR}/tls}"
TLS_CERT_FILE="${TLS_CERT_FILE:-${TLS_DIR}/cert.pem}"
TLS_KEY_FILE="${TLS_KEY_FILE:-${TLS_DIR}/key.pem}"
COMPOSE_PROJECT_NAME="${DEPLOY_COMPOSE_PROJECT_NAME:-shepherd-prod}"
DEPLOY_ASSET_REF="${DEPLOY_ASSET_REF:-main}"
DEPLOY_RAW_BASE="${DEPLOY_RAW_BASE:-https://raw.githubusercontent.com/kv-shepherd/shepherd/${DEPLOY_ASSET_REF}}"
DEPLOY_RELEASE_VERSION="${DEPLOY_RELEASE_VERSION:-${SHEPHERD_VERSION:-}}"
RESOLVED_RELEASE_VERSION=""

strip_release_prefix() {
    local version="$1"
    version="${version#refs/tags/}"
    version="${version#v}"
    printf "%s" "${version}"
}

fetch_latest_release_tag() {
    local url="https://api.github.com/repos/kv-shepherd/shepherd/releases?per_page=1"
    local body
    if command -v curl >/dev/null 2>&1; then
        body="$(curl -fsSL "${url}")"
    elif command -v wget >/dev/null 2>&1; then
        body="$(wget -qO- "${url}")"
    else
        return 1
    fi
    printf "%s\n" "${body}" | sed -nE 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' | head -n 1
}

resolve_release_version() {
    if [[ -n "${RESOLVED_RELEASE_VERSION}" ]]; then
        printf "%s" "${RESOLVED_RELEASE_VERSION}"
        return 0
    fi

    local raw="${DEPLOY_RELEASE_VERSION:-${SHEPHERD_VERSION:-}}"
    if [[ -z "${raw}" && "${DEPLOY_ASSET_REF}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        raw="${DEPLOY_ASSET_REF}"
    fi
    if [[ -z "${raw}" ]]; then
        raw="$(fetch_latest_release_tag || true)"
    fi

    raw="$(strip_release_prefix "${raw}")"
    if [[ -z "${raw}" ]]; then
        echo "ERROR: could not resolve a Shepherd release image version." >&2
        echo "  Set SHEPHERD_VERSION=<version> or SERVER_IMAGE/WEB_IMAGE explicitly." >&2
        exit 1
    fi

    RESOLVED_RELEASE_VERSION="${raw}"
    printf "%s" "${RESOLVED_RELEASE_VERSION}"
}

resolve_image_defaults() {
    local release_version_requested=0
    if [[ -n "${DEPLOY_RELEASE_VERSION:-${SHEPHERD_VERSION:-}}" ]]; then
        release_version_requested=1
    fi

    if [[ "${SOURCE_TREE_AVAILABLE}" == "1" && "${release_version_requested}" == "0" ]]; then
        SERVER_IMAGE="${SERVER_IMAGE:-shepherd-server:latest}"
        WEB_IMAGE="${WEB_IMAGE:-shepherd-web:latest}"
    else
        local need_server_image=0
        local need_web_image=0
        local version=""

        if [[ -z "${SERVER_IMAGE:-}" ]] || { [[ "${release_version_requested}" == "1" ]] && [[ -z "${CONFIG_ENV_OVERRIDES[SERVER_IMAGE]+x}" ]]; }; then
            need_server_image=1
        fi
        if [[ -z "${WEB_IMAGE:-}" ]] || { [[ "${release_version_requested}" == "1" ]] && [[ -z "${CONFIG_ENV_OVERRIDES[WEB_IMAGE]+x}" ]]; }; then
            need_web_image=1
        fi

        if [[ "${need_server_image}" == "1" || "${need_web_image}" == "1" ]]; then
            version="$(resolve_release_version)"
        fi
        if [[ "${need_server_image}" == "1" ]]; then
            SERVER_IMAGE="ghcr.io/kv-shepherd/shepherd-server:${version}"
        fi
        if [[ "${need_web_image}" == "1" ]]; then
            WEB_IMAGE="ghcr.io/kv-shepherd/shepherd-web:${version}"
        fi
    fi
    export SERVER_IMAGE WEB_IMAGE
}

persist_resolved_images() {
    write_env_value SERVER_IMAGE "${SERVER_IMAGE}"
    write_env_value WEB_IMAGE "${WEB_IMAGE}"
}

CONFIG_ENV_KEYS=(
    POSTGRES_USER
    POSTGRES_IMAGE
    POSTGRES_PASSWORD
    POSTGRES_DB
    DATABASE_URL
    DEPLOY_BUNDLED_POSTGRES
    SECURITY_SESSION_SECRET
    SECURITY_ENCRYPTION_KEY
    SERVER_PORT
    SERVER_PUBLIC_BASE_URL
    SERVER_ALLOWED_ORIGINS
    SERVER_ALLOW_CREDENTIALS
    SERVER_UNSAFE_ALLOW_ALL_ORIGINS
    GIN_MODE
    LOG_LEVEL
    LOG_FORMAT
    DATABASE_AUTO_APPLY_VERSIONED_MIGRATIONS
    DATABASE_AUTO_MIGRATE
    RIVER_MAX_WORKERS
    RIVER_COMPLETED_JOB_RETENTION_PERIOD
    WORKER_GENERAL_POOL_SIZE
    WORKER_K8S_POOL_SIZE
    DEV_ADMIN_PASSWORD
    SERVER_IMAGE
    WEB_IMAGE
    NGINX_IMAGE
    NGINX_HTTP_PORT
    NGINX_HTTPS_PORT
)
declare -A CONFIG_ENV_OVERRIDES=()
for key in "${CONFIG_ENV_KEYS[@]}"; do
    if [[ ${!key+x} ]]; then
        CONFIG_ENV_OVERRIDES["${key}"]="${!key}"
    fi
done
export TLS_CERT_FILE TLS_KEY_FILE
ENTERPRISE_MODE=0
BUILD_ONLY=0
SEED_ONLY=0
RUN_SEED=0
SKIP_BUILD=0
DID_RUN_SEED=0
RUN_EXPERIENCE_SEED=0
DID_RUN_EXPERIENCE_SEED=0
BUNDLED_POSTGRES_MODE=""
BUNDLED_POSTGRES=1

usage() {
    cat <<'EOF'
Usage: deploy-prod.sh [options]

Options:
  --enterprise     Deploy enterprise edition (requires private repo)
  --build-only     Build images only, do not start services
  --with-seed      Run bootstrap seed after startup (roles + default admin)
  --with-experience-seed
                   Run extended experience fixtures after bootstrap seed
                   (admin/test accounts + sample system/service/catalog data)
  --seed-only      Run bootstrap seed only (assumes services are running)
  --skip-build     Skip image build, use existing images
  -h, --help       Show this help message

Environment overrides:
  DEPLOY_ENV_FILE              Alternate .env.prod path
  DEPLOY_DIR                   Directory used when running this script via stdin
  DEPLOY_ASSET_REF             Git ref used for auto-downloaded deploy assets
  SHEPHERD_VERSION             GHCR release image version for stdin/raw deploys.
                               If unset, deploy-prod.sh resolves the latest
                               GitHub Release tag.
  DEPLOY_TLS_DIR               Alternate TLS cert/key directory
  DEPLOY_COMPOSE_PROJECT_NAME  Alternate docker compose project name
  SERVER_IMAGE                 Server image tag to build/run
  WEB_IMAGE                    Web image tag to build/run
  POSTGRES_IMAGE               Bundled PostgreSQL image (default: postgres:18)
  NGINX_IMAGE                  Edge proxy image (default: nginx:1.30.1-alpine)

Deployment configuration values such as DATABASE_URL,
DEPLOY_BUNDLED_POSTGRES, SERVER_PUBLIC_BASE_URL, SECURITY_*,
DEV_ADMIN_PASSWORD, ports, and worker settings may be passed before bash.
They are persisted to the deployment .env file before services start.
EOF
}

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "ERROR: required command not found: $1"
        exit 1
    fi
}

require_docker_compose() {
    if ! docker compose version >/dev/null 2>&1; then
        echo "ERROR: Docker Compose v2 is required (the 'docker compose' plugin is missing or unavailable)."
        exit 1
    fi
}

download_file() {
    local url="$1"
    local dest="$2"
    mkdir -p "$(dirname "${dest}")"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL "${url}" -o "${dest}"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "${dest}" "${url}"
    else
        echo "ERROR: curl or wget is required to download missing deployment assets."
        exit 1
    fi
}

ensure_deploy_assets() {
    if [[ ! -f "${COMPOSE_FILE}" ]]; then
        echo "INFO: downloading production compose file to ${COMPOSE_FILE}"
        download_file "${DEPLOY_RAW_BASE}/deploy/prod/docker-compose.prod.yml" "${COMPOSE_FILE}"
    fi

    if [[ ! -f "${ENV_EXAMPLE_FILE}" ]]; then
        echo "INFO: downloading production environment template to ${ENV_EXAMPLE_FILE}"
        download_file "${DEPLOY_RAW_BASE}/deploy/prod/.env.prod.example" "${ENV_EXAMPLE_FILE}"
    fi

    local nginx_conf="${SCRIPT_DIR}/nginx/prod.conf"
    if [[ ! -f "${nginx_conf}" ]]; then
        echo "INFO: downloading nginx production config to ${nginx_conf}"
        download_file "${DEPLOY_RAW_BASE}/deploy/prod/nginx/prod.conf" "${nginx_conf}"
    fi
}

ensure_source_tree_for_build() {
    if [[ "${SOURCE_TREE_AVAILABLE}" == "1" ]]; then
        return
    fi

    echo "ERROR: source tree not found for local image build."
    echo "  Run this script from a full repository checkout for source-build deployment."
    exit 1
}

prepare_env_file() {
    if [[ -f "${ENV_FILE}" ]]; then
        return
    fi
    if [[ ! -f "${ENV_EXAMPLE_FILE}" ]]; then
        echo "ERROR: ${ENV_FILE} not found and template ${ENV_EXAMPLE_FILE} is missing."
        exit 1
    fi
    cp "${ENV_EXAMPLE_FILE}" "${ENV_FILE}"
    echo "INFO: ${ENV_FILE} not found."
    echo "  Generated a first-run template from:"
    echo "    ${ENV_EXAMPLE_FILE}"
}

read_env_value() {
    local key="$1"
    local line
    line=$(grep -E "^${key}=" "${ENV_FILE}" | tail -n 1 || true)
    if [[ -z "${line}" ]]; then
        return 0
    fi
    printf '%s' "${line#*=}"
}

write_env_value() {
    local key="$1"
    local value="$2"
    if grep -Eq "^${key}=" "${ENV_FILE}"; then
        local escaped_value="${value//\\/\\\\}"
        escaped_value="${escaped_value//&/\\&}"
        escaped_value="${escaped_value//|/\\|}"
        sed -i -e "s|^${key}=.*$|${key}=${escaped_value}|" "${ENV_FILE}"
    else
        printf '\n%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
    fi
}

persist_env_overrides() {
    local key
    for key in "${CONFIG_ENV_KEYS[@]}"; do
        if [[ ${CONFIG_ENV_OVERRIDES[${key}]+x} ]]; then
            write_env_value "${key}" "${CONFIG_ENV_OVERRIDES[${key}]}"
        fi
    done
}

is_placeholder_value() {
    local value="$1"
    case "${value}" in
        ""|CHANGE_ME*|change_me*|https://replace-me.example.com|https://your-domain.com)
            return 0
            ;;
    esac
    return 1
}

needs_generated_secret() {
    local key="$1"
    local value="$2"
    if is_placeholder_value "${value}"; then
        return 0
    fi

    case "${key}" in
        POSTGRES_PASSWORD|DEV_ADMIN_PASSWORD)
            [[ ${#value} -lt 16 ]] && return 0
            ;;
        SECURITY_SESSION_SECRET)
            [[ ${#value} -lt 32 ]] && return 0
            ;;
    esac

    return 1
}

ensure_generated_secret() {
    local key="$1"
    local bytes="$2"
    local current
    current="$(read_env_value "${key}")"
    if needs_generated_secret "${key}" "${current}"; then
        local generated
        generated="$(openssl rand -hex "${bytes}")"
        write_env_value "${key}" "${generated}"
        echo "INFO: generated ${key} and persisted it to ${ENV_FILE}."
    fi
}

base64_file() {
    base64 <"$1" | tr -d '\n'
}

host_goarch() {
    case "$(uname -m)" in
        x86_64|amd64)
            printf "amd64"
            ;;
        aarch64|arm64)
            printf "arm64"
            ;;
        *)
            echo "ERROR: unsupported host architecture: $(uname -m)"
            exit 1
            ;;
    esac
}

build_experience_seed_binary() {
    local binary_path="${ROOT_DIR}/build/bin/e2e-seed"
    local goarch
    goarch="$(host_goarch)"

    mkdir -p "${ROOT_DIR}/build/bin"
    echo "Building extended experience seeder (cmd/e2e-seed)..." >&2
    if command -v go >/dev/null 2>&1; then
        (
            cd "${ROOT_DIR}"
            GOOS=linux GOARCH="${goarch}" CGO_ENABLED=0 go build -ldflags="-s -w" -o build/bin/e2e-seed ./cmd/e2e-seed/...
        )
    else
        docker run --rm \
            -u "$(id -u):$(id -g)" \
            -e HOME=/tmp/go-home \
            -e GOCACHE=/tmp/go-build \
            -e GOMODCACHE=/tmp/go-mod \
            -e GOOS=linux \
            -e GOARCH="${goarch}" \
            -e CGO_ENABLED=0 \
            -v "${ROOT_DIR}:/workspace" \
            -w /workspace \
            golang:1.25.10-bookworm \
            /usr/local/go/bin/go build -ldflags="-s -w" -o build/bin/e2e-seed ./cmd/e2e-seed/...
    fi
    printf "%s" "${binary_path}"
}

run_experience_seed() {
    local binary_path server_container
    local -a e2e_seed_env
    e2e_seed_env=(
        -e "E2E_ADMIN_USERNAME=admin"
        -e "E2E_ADMIN_PASSWORD=admin"
        -e "E2E_ADMIN_EMAIL=admin@localhost"
        -e "E2E_SECOND_USERNAME=test"
        -e "E2E_SECOND_PASSWORD=test"
        -e "E2E_SECOND_EMAIL=test@localhost"
        -e "E2E_SECOND_DISPLAY_NAME=Test User"
        -e "E2E_SECOND_ROLE_NAME=TestEngineer"
    )

    if [[ -n "${E2E_KUBECONFIG_B64:-}" ]]; then
        e2e_seed_env+=(-e "E2E_KUBECONFIG_B64=${E2E_KUBECONFIG_B64}")
    elif [[ -n "${E2E_KUBECONFIG_PATH:-}" ]]; then
        if [[ ! -f "${E2E_KUBECONFIG_PATH}" ]]; then
            echo "ERROR: E2E_KUBECONFIG_PATH does not exist: ${E2E_KUBECONFIG_PATH}"
            exit 1
        fi
        e2e_seed_env+=(-e "E2E_KUBECONFIG_B64=$(base64_file "${E2E_KUBECONFIG_PATH}")")
    fi

    binary_path="$(build_experience_seed_binary)"
    server_container="$(docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" ps -q server)"
    if [[ -z "${server_container}" ]]; then
        echo "ERROR: could not resolve running server container for experience seed"
        exit 1
    fi

    echo "Running extended experience seed (sample catalog + admin/test users)..."
    docker cp "${binary_path}" "${server_container}:/tmp/e2e-seed"
    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" exec -T server chmod 0755 /tmp/e2e-seed >/dev/null
    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" \
        exec -T "${e2e_seed_env[@]}" server /tmp/e2e-seed >/dev/null
    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" exec -T server rm -f /tmp/e2e-seed >/dev/null 2>&1 || true
    echo "  ✓ Extended experience seed complete"
    DID_RUN_EXPERIENCE_SEED=1
}

login_token() {
    local username="$1"
    local password="$2"

    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" exec -T web \
        node -e "const [port, username, password] = process.argv.slice(1); (async () => { try { const res = await fetch('http://server:' + port + '/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ username, password }) }); const body = await res.json().catch(() => ({})); if (!res.ok || !body.token) process.exit(1); process.stdout.write(body.token); } catch { process.exit(2); } })();" \
        "${SERVER_PORT:-8080}" "${username}" "${password}" 2>/dev/null
}

change_password_via_api() {
    local token="$1"
    local old_password="$2"
    local new_password="$3"

    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" exec -T web \
        node -e "const [port, token, oldPassword, newPassword] = process.argv.slice(1); (async () => { try { const res = await fetch('http://server:' + port + '/api/v1/auth/change-password', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + token }, body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }) }); if (!res.ok) process.exit(1); } catch { process.exit(2); } })();" \
        "${SERVER_PORT:-8080}" "${token}" "${old_password}" "${new_password}" >/dev/null 2>&1
}

extract_database_host() {
    local url="$1"
    if [[ -z "${url}" ]]; then
        return 0
    fi
    local rest="${url#*://}"
    rest="${rest#*@}"
    rest="${rest%%/*}"
    rest="${rest%%\?*}"
    if [[ "${rest}" == \[*\]* ]]; then
        rest="${rest#\[}"
        printf "%s" "${rest%%]*}"
        return 0
    fi
    printf "%s" "${rest%%:*}"
}

sync_bundled_database_url() {
    local mode current host bundled_url pg_user pg_password pg_database
    mode="$(read_env_value DEPLOY_BUNDLED_POSTGRES)"
    current="$(read_env_value DATABASE_URL)"
    host="$(extract_database_host "${current}")"

    case "${mode:-auto}" in
        false|0|no|off)
            return
            ;;
    esac

    pg_user="$(read_env_value POSTGRES_USER)"
    pg_password="$(read_env_value POSTGRES_PASSWORD)"
    pg_database="$(read_env_value POSTGRES_DB)"
    pg_user="${pg_user:-shepherd}"
    pg_database="${pg_database:-shepherd_db}"

    bundled_url="postgres://${pg_user}:${pg_password}@db:5432/${pg_database}?sslmode=disable"

    if is_placeholder_value "${current}" || [[ "${host}" == "db" ]] || [[ "${mode:-auto}" =~ ^(true|1|yes|on)$ ]]; then
        if [[ "${current}" != "${bundled_url}" ]]; then
            write_env_value DATABASE_URL "${bundled_url}"
            echo "INFO: prepared DATABASE_URL for bundled PostgreSQL and persisted it to ${ENV_FILE}."
        fi
    fi
}

ensure_public_base_url() {
    local current
    current="$(read_env_value SERVER_PUBLIC_BASE_URL)"
    if is_placeholder_value "${current}"; then
        write_env_value SERVER_PUBLIC_BASE_URL "https://localhost"
        echo "INFO: defaulted SERVER_PUBLIC_BASE_URL to https://localhost in ${ENV_FILE}."
    fi
}

should_prepare_bundled_postgres_values() {
    local mode current host
    mode="$(read_env_value DEPLOY_BUNDLED_POSTGRES)"
    current="$(read_env_value DATABASE_URL)"
    host="$(extract_database_host "${current}")"

    case "${mode:-auto}" in
        false|0|no|off)
            return 1
            ;;
        true|1|yes|on)
            return 0
            ;;
    esac

    if is_placeholder_value "${current}" || [[ "${host}" == "db" ]]; then
        return 0
    fi
    return 1
}

prepare_runtime_values() {
    ensure_public_base_url
    if should_prepare_bundled_postgres_values; then
        ensure_generated_secret POSTGRES_PASSWORD 16
    fi
    ensure_generated_secret SECURITY_SESSION_SECRET 32
    ensure_generated_secret SECURITY_ENCRYPTION_KEY 32
    ensure_generated_secret DEV_ADMIN_PASSWORD 16
    sync_bundled_database_url
}

resolve_bundled_postgres_mode() {
    local mode="${DEPLOY_BUNDLED_POSTGRES:-auto}"
    case "${mode}" in
        true|1|yes|on)
            BUNDLED_POSTGRES_MODE="forced-bundled"
            BUNDLED_POSTGRES=1
            ;;
        false|0|no|off)
            BUNDLED_POSTGRES_MODE="forced-external"
            BUNDLED_POSTGRES=0
            ;;
        auto|"")
            local db_host=""
            db_host="$(extract_database_host "${DATABASE_URL}")"
            if [[ "${db_host}" == "db" ]]; then
                BUNDLED_POSTGRES_MODE="auto-bundled"
                BUNDLED_POSTGRES=1
            else
                BUNDLED_POSTGRES_MODE="auto-external"
                BUNDLED_POSTGRES=0
            fi
            ;;
        *)
            echo "ERROR: DEPLOY_BUNDLED_POSTGRES must be one of: auto, true, false"
            exit 1
            ;;
    esac
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --enterprise)
            ENTERPRISE_MODE=1
            shift
            ;;
        --build-only)
            BUILD_ONLY=1
            shift
            ;;
        --with-seed|--bootstrap)
            RUN_SEED=1
            shift
            ;;
        --with-experience-seed)
            RUN_EXPERIENCE_SEED=1
            RUN_SEED=1
            shift
            ;;
        --seed-only)
            SEED_ONLY=1
            shift
            ;;
        --skip-build)
            SKIP_BUILD=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown argument: $1"
            usage
            exit 1
            ;;
    esac
done

# ---- Preflight checks ----
require_cmd docker
require_docker_compose
require_cmd openssl

echo "=============================================="
echo "  KubeVirt Shepherd — Production Deployment"
echo "=============================================="
echo ""

ensure_deploy_assets
prepare_env_file
persist_env_overrides

# Source env so generated values can reuse the current topology inputs.
set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a
resolve_image_defaults
persist_resolved_images

prepare_runtime_values

# Re-source env after first-run generation/backfill.
set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a
resolve_image_defaults
persist_resolved_images

# Validate required variables
for var in DATABASE_URL SERVER_PUBLIC_BASE_URL SERVER_IMAGE WEB_IMAGE; do
    val="${!var:-}"
    if is_placeholder_value "${val}"; then
        echo "ERROR: ${var} is not set or still has a placeholder value in ${ENV_FILE}"
        exit 1
    fi
done

resolve_bundled_postgres_mode

if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
    val="${POSTGRES_PASSWORD:-}"
    if is_placeholder_value "${val}"; then
        echo "ERROR: POSTGRES_PASSWORD is required when bundled PostgreSQL is enabled"
        exit 1
    fi
fi

if [[ ${#SECURITY_SESSION_SECRET} -lt 32 ]]; then
    echo "ERROR: SECURITY_SESSION_SECRET must be at least 32 characters in ${ENV_FILE}"
    exit 1
fi
if [[ ! "${SECURITY_ENCRYPTION_KEY}" =~ ^[0-9A-Fa-f]{64}$ ]]; then
    echo "ERROR: SECURITY_ENCRYPTION_KEY must be a 64-character hex-encoded AES-256 key in ${ENV_FILE}"
    exit 1
fi

# Check TLS certificate
if [[ ! -f "${TLS_CERT_FILE}" ]] || [[ ! -f "${TLS_KEY_FILE}" ]]; then
    echo "WARNING: TLS certificates not found in ${TLS_DIR}/"
    echo "  Generating self-signed certificate for development/testing..."
    mkdir -p "$(dirname "${TLS_CERT_FILE}")" "$(dirname "${TLS_KEY_FILE}")"
    openssl req \
        -x509 -nodes -newkey rsa:2048 -days 365 \
        -keyout "${TLS_KEY_FILE}" \
        -out "${TLS_CERT_FILE}" \
        -subj "/CN=localhost" \
        -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" \
        >/dev/null 2>&1
    echo "  Self-signed certificate generated. Replace with real certs for production."
fi

# ---- Build phase ----
if [[ "${SEED_ONLY}" == "1" ]]; then
    echo "Skipping build phase (--seed-only)."
elif [[ "${SKIP_BUILD}" == "1" ]]; then
    echo "Skipping build phase (--skip-build)."
    if [[ "${BUILD_ONLY}" == "1" ]]; then
        echo ""
        echo "Build phase skipped. Images selected:"
        echo "  - ${SERVER_IMAGE}"
        echo "  - ${WEB_IMAGE}"
        echo "  - ${NGINX_IMAGE:-nginx:1.30.1-alpine}"
        exit 0
    fi
elif [[ "${SOURCE_TREE_AVAILABLE}" != "1" ]]; then
    echo "Skipping build phase (release-image deployment; no source tree found)."
    if [[ "${BUILD_ONLY}" == "1" ]]; then
        echo ""
        echo "Release images selected:"
        echo "  - ${SERVER_IMAGE}"
        echo "  - ${WEB_IMAGE}"
        echo "  - ${NGINX_IMAGE:-nginx:1.30.1-alpine}"
        exit 0
    fi
else
    ensure_source_tree_for_build

    echo ""
    echo "--- Phase 1: Building Images ---"
    echo ""

    # Build Go backend
    echo "[1/3] Building Go backend image (${SERVER_IMAGE})..."
    DOCKER_BUILDKIT=1 docker build \
        --network=host \
        --target runtime \
        -t "${SERVER_IMAGE}" \
        -f "${ROOT_DIR}/Dockerfile" \
        "${ROOT_DIR}"
    echo "  ✓ ${SERVER_IMAGE} built"

    # Build Next.js frontend
    echo "[2/3] Building Next.js frontend image (${WEB_IMAGE})..."
    DOCKER_BUILDKIT=1 docker build \
        --network=host \
        --target runner \
        -t "${WEB_IMAGE}" \
        -f "${SCRIPT_DIR}/web.Dockerfile" \
        "${ROOT_DIR}/web"
    echo "  ✓ ${WEB_IMAGE} built"

    echo "[3/3] Pulling ${NGINX_IMAGE:-nginx:1.30.1-alpine}..."
    docker pull "${NGINX_IMAGE:-nginx:1.30.1-alpine}" >/dev/null 2>&1 || true
    echo "  ✓ ${NGINX_IMAGE:-nginx:1.30.1-alpine} ready"
    if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
        echo "  ✓ bundled PostgreSQL topology selected (${BUNDLED_POSTGRES_MODE})"
    else
        echo "  ✓ external PostgreSQL topology selected (${BUNDLED_POSTGRES_MODE})"
    fi

    if [[ "${BUILD_ONLY}" == "1" ]]; then
        echo ""
        echo "Build complete. Images ready:"
        echo "  - ${SERVER_IMAGE}"
        echo "  - ${WEB_IMAGE}"
        echo "  - ${NGINX_IMAGE:-nginx:1.30.1-alpine}"
        exit 0
    fi
fi

# ---- Deploy phase ----
if [[ "${SEED_ONLY}" != "1" ]]; then
    echo ""
    echo "--- Phase 2: Starting Services ---"
    echo ""

    if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
        docker compose \
            -f "${COMPOSE_FILE}" \
            --env-file "${ENV_FILE}" \
            -p "${COMPOSE_PROJECT_NAME}" \
            up -d db

        echo ""
        echo "Waiting for bundled database to be ready..."
        for _ in {1..30}; do
            if docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" \
                exec -T db pg_isready -U "${POSTGRES_USER:-shepherd}" -d "${POSTGRES_DB:-shepherd_db}" >/dev/null 2>&1; then
                echo "  ✓ Database ready"
                break
            fi
            printf "."
            sleep 2
        done

        docker compose \
            -f "${COMPOSE_FILE}" \
            --env-file "${ENV_FILE}" \
            -p "${COMPOSE_PROJECT_NAME}" \
            up -d server web nginx
    else
        echo ""
        echo "Skipping bundled database startup (external DATABASE_URL detected)."

        docker compose \
            -f "${COMPOSE_FILE}" \
            --env-file "${ENV_FILE}" \
            -p "${COMPOSE_PROJECT_NAME}" \
            up -d server web nginx
    fi

    echo "Waiting for backend to be ready..."
    for _ in {1..60}; do
        if docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" \
            exec -T web node -e "const http=require('http');http.get('http://server:${SERVER_PORT:-8080}/api/v1/health/ready',res=>process.exit(res.statusCode===200?0:1)).on('error',()=>process.exit(2));" >/dev/null 2>&1; then
            echo "  ✓ Backend ready"
            break
        fi
        printf "."
        sleep 2
    done
fi

# ---- Seed phase ----
if [[ "${SEED_ONLY}" == "1" || "${RUN_SEED}" == "1" ]]; then
    echo ""
    echo "--- Phase 3: Running Bootstrap Seed ---"
    echo ""

    echo "Running bootstrap seed (built-in roles + default admin)..."
    docker compose -f "${COMPOSE_FILE}" -p "${COMPOSE_PROJECT_NAME}" \
        exec -T server /usr/local/bin/seed >/dev/null 2>&1 || {
        echo "  ⚠ bootstrap seed failed or was already reconciled (non-fatal)"
    }
    echo "  ✓ Bootstrap seed complete"
    DID_RUN_SEED=1
    if [[ "${RUN_EXPERIENCE_SEED}" == "1" ]]; then
        run_experience_seed
    fi
else
    echo ""
    echo "--- Phase 3: Skipping Bootstrap Seed ---"
    echo ""
    echo "Skipping bootstrap seed by default."
    echo "  Use --with-seed for first install/bootstrap or role-baseline reconciliation."
    if [[ "${RUN_EXPERIENCE_SEED}" == "1" ]]; then
        echo "ERROR: --with-experience-seed requires bootstrap seed to run."
        exit 1
    fi
fi

# Rotate admin password if needed
ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin}"
if [[ "${DID_RUN_SEED}" == "1" && "${ADMIN_PASSWORD}" != "admin" ]]; then
    echo "Rotating default admin password..."
    TOKEN=""
    # Try login with target password first
    TOKEN="$(login_token admin "${ADMIN_PASSWORD}" || true)"

    if [[ -n "${TOKEN}" ]]; then
        echo "  ✓ Admin password already set"
    else
        # Login with bootstrap password and rotate
        TOKEN="$(login_token admin admin || true)"

        if [[ -n "${TOKEN}" ]]; then
            if change_password_via_api "${TOKEN}" admin "${ADMIN_PASSWORD}"; then
                echo "  ✓ Admin password rotated"
            else
                echo "  ⚠ Could not rotate admin password (non-fatal)"
            fi
        else
            echo "  ⚠ Could not rotate admin password (non-fatal)"
        fi
    fi
fi

# ---- Summary ----
echo ""
echo "=============================================="
echo "  Deployment Complete!"
echo "=============================================="
echo ""
if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
    db_summary="bundled PostgreSQL 18 (compose service)"
else
    db_summary="external PostgreSQL via DATABASE_URL"
fi
echo "  Services:"
echo "    Web:     ${SERVER_PUBLIC_BASE_URL}"
echo "    Backend: http://localhost:${SERVER_PORT:-8080} (internal)"
echo "    DB:      ${db_summary}"
echo ""
if [[ "${DID_RUN_SEED}" == "1" ]]; then
    if [[ "${DID_RUN_EXPERIENCE_SEED}" == "1" ]]; then
        echo "  Login:     admin / ${ADMIN_PASSWORD}"
        echo "             test / test"
        if [[ -n "${E2E_KUBECONFIG_B64:-}" || -n "${E2E_KUBECONFIG_PATH:-}" ]]; then
            echo "  Note:      experience fixtures seeded against the provided cluster kubeconfig"
        else
            echo "  Note:      no cluster kubeconfig provided; the seeded cluster is intentionally unreachable"
            echo "             so cluster-backed VM actions remain illustrative until a real cluster is configured"
        fi
        echo ""
    else
        echo "  Login:     admin / ${ADMIN_PASSWORD}"
        echo "  Note:      first sign-in requires a password change"
        echo ""
    fi
else
    echo "  Bootstrap seed was skipped."
    echo "  Run with --with-seed (or --seed-only) if this is a fresh database."
    echo ""
fi
echo "  Management:"
echo "    docker compose -f ${COMPOSE_FILE} -p ${COMPOSE_PROJECT_NAME} logs -f"
echo "    docker compose -f ${COMPOSE_FILE} -p ${COMPOSE_PROJECT_NAME} ps"
echo "    docker compose -f ${COMPOSE_FILE} -p ${COMPOSE_PROJECT_NAME} down"
echo ""
if [[ "${ENTERPRISE_MODE}" == "1" ]]; then
    echo "  Enterprise Auth Setup:"
    echo "    cd <enterprise-extension-repo>"
    echo "    make auth-bootstrap-apply"
    echo "    make auth-plan"
    echo "    make auth-apply"
fi
