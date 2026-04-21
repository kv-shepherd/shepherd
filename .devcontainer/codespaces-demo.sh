#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.codespaces.yml"
PROJECT_NAME="${CODESPACES_PROJECT_NAME:-shepherd-codespaces}"
HTTP_PORT="${CODESPACES_HTTP_PORT:-3000}"
API_PORT="${CODESPACES_API_PORT:-8080}"
ADMIN_USERNAME="${CODESPACES_ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${CODESPACES_ADMIN_PASSWORD:-admin}"
TEST_USERNAME="${CODESPACES_TEST_USERNAME:-test}"
TEST_PASSWORD="${CODESPACES_TEST_PASSWORD:-test}"
HOST_E2E_SEED_BIN="${REPO_ROOT}/build/bin/codespaces-e2e-seed"
GO_TOOLCHAIN_VERSION="${GO_TOOLCHAIN_VERSION:-$(awk '/^go [0-9]+\.[0-9]+\.[0-9]+$/ { print "go" $2; exit }' "${REPO_ROOT}/go.mod")}"
STATE_FILE="${SCRIPT_DIR}/.codespaces-demo.state"
SOURCE_FINGERPRINT_PATHS=(
    ".devcontainer/docker-compose.codespaces.yml"
    ".devcontainer/nginx.codespaces.conf"
    "Dockerfile"
    "api"
    "cmd"
    "deploy/prod/web.Dockerfile"
    "ent"
    "go.mod"
    "go.sum"
    "internal"
    "migrations"
    "pkg"
    "plugins"
    "web"
)
LAST_SOURCE_FINGERPRINT=""
LAST_SERVER_PUBLIC_BASE_URL=""
LAST_SERVER_ALLOWED_ORIGINS=""
LAST_HTTP_PORT=""
LAST_API_PORT=""
LAST_PROJECT_NAME=""

usage() {
    cat <<'EOF'
Usage: bash .devcontainer/codespaces-demo.sh [bootstrap|resume|rebuild]

bootstrap  Build the current source tree, reset the local Codespaces stack, and
           seed baseline + experience fixtures.
resume     Start the existing stack without reseeding data. Rebuild images
           automatically when the checked-out runtime source has changed.
rebuild    Rebuild the current source tree into fresh images and restart the
           stack without resetting data.
EOF
}

base64_file() {
    if base64 -w0 "$1" >/dev/null 2>&1; then
        base64 -w0 "$1"
    else
        base64 <"$1" | tr -d '\n'
    fi
}

load_state_file() {
    LAST_SOURCE_FINGERPRINT=""
    LAST_SERVER_PUBLIC_BASE_URL=""
    LAST_SERVER_ALLOWED_ORIGINS=""
    LAST_HTTP_PORT=""
    LAST_API_PORT=""
    LAST_PROJECT_NAME=""

    if [[ ! -f "${STATE_FILE}" ]]; then
        return 1
    fi

    # shellcheck disable=SC1090
    source "${STATE_FILE}"
    return 0
}

write_state_file() {
    local source_fingerprint="$1"
    local public_base_url="$2"
    local allowed_origins="$3"
    local tmp_file

    tmp_file="${STATE_FILE}.tmp"
    {
        printf 'LAST_SOURCE_FINGERPRINT=%q\n' "${source_fingerprint}"
        printf 'LAST_SERVER_PUBLIC_BASE_URL=%q\n' "${public_base_url}"
        printf 'LAST_SERVER_ALLOWED_ORIGINS=%q\n' "${allowed_origins}"
        printf 'LAST_HTTP_PORT=%q\n' "${HTTP_PORT}"
        printf 'LAST_API_PORT=%q\n' "${API_PORT}"
        printf 'LAST_PROJECT_NAME=%q\n' "${PROJECT_NAME}"
    } >"${tmp_file}"
    mv "${tmp_file}" "${STATE_FILE}"
}

compute_source_fingerprint() {
    local head

    if ! head="$(git -C "${REPO_ROOT}" rev-parse HEAD 2>/dev/null)"; then
        printf "nogit"
        return 0
    fi

    if [[ -n "$(git -C "${REPO_ROOT}" status --porcelain=v1 --untracked-files=all -- "${SOURCE_FINGERPRINT_PATHS[@]}")" ]]; then
        printf "%s-dirty" "${head}"
        return 0
    fi

    printf "%s" "${head}"
}

compute_public_base_url() {
    if [[ -n "${CODESPACE_NAME:-}" ]]; then
        printf "https://%s-%s.%s" \
            "${CODESPACE_NAME}" \
            "${HTTP_PORT}" \
            "${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-app.github.dev}"
    else
        printf "http://localhost:%s" "${HTTP_PORT}"
    fi
}

compute_allowed_origins() {
    local public_base_url="$1"
    printf "%s,http://localhost:%s,http://127.0.0.1:%s,http://localhost:%s,http://127.0.0.1:%s" \
        "${public_base_url}" \
        "${HTTP_PORT}" \
        "${HTTP_PORT}" \
        "${API_PORT}" \
        "${API_PORT}"
}

compose_cmd() {
    COMPOSE_PROJECT_NAME="${PROJECT_NAME}" docker compose -f "${COMPOSE_FILE}" "$@"
}

compose_up() {
    local mode="$1"
    local -a args
    args=(up -d)
    if [[ "${mode}" == "bootstrap" || "${mode}" == "rebuild" ]]; then
        args+=(--build)
    fi

    DATABASE_URL="${DATABASE_URL}" \
    POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
    SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET}" \
    SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY}" \
    SERVER_PUBLIC_BASE_URL="${SERVER_PUBLIC_BASE_URL}" \
    SERVER_ALLOWED_ORIGINS="${SERVER_ALLOWED_ORIGINS}" \
    HTTP_PORT="${HTTP_PORT}" \
    API_PORT="${API_PORT}" \
    COMPOSE_PROJECT_NAME="${PROJECT_NAME}" \
        docker compose -f "${COMPOSE_FILE}" "${args[@]}"
}

compose_exec() {
    compose_cmd exec -T "$@"
}

build_codespaces_e2e_seed() {
    mkdir -p "$(dirname "${HOST_E2E_SEED_BIN}")"
    (
        cd "${REPO_ROOT}"
        GOTOOLCHAIN="${GO_TOOLCHAIN_VERSION}" \
        GOOS=linux \
        GOARCH="$(go env GOARCH)" \
        CGO_ENABLED=0 \
            go build -ldflags="-s -w" -o "${HOST_E2E_SEED_BIN}" ./cmd/e2e-seed/...
    )
}

run_codespaces_e2e_seed() {
    local server_container
    local -a e2e_seed_env
    e2e_seed_env=(
        -e "E2E_ADMIN_USERNAME=${ADMIN_USERNAME}"
        -e "E2E_ADMIN_PASSWORD=${ADMIN_PASSWORD}"
        -e "E2E_ADMIN_EMAIL=${ADMIN_USERNAME}@localhost"
        -e "E2E_SECOND_USERNAME=${TEST_USERNAME}"
        -e "E2E_SECOND_PASSWORD=${TEST_PASSWORD}"
        -e "E2E_SECOND_EMAIL=${TEST_USERNAME}@localhost"
        -e "E2E_SECOND_DISPLAY_NAME=Test User"
        -e "E2E_SECOND_ROLE_NAME=TestEngineer"
    )

    if [[ -n "${E2E_KUBECONFIG_B64:-}" ]]; then
        e2e_seed_env+=(-e "E2E_KUBECONFIG_B64=${E2E_KUBECONFIG_B64}")
    elif [[ -n "${E2E_KUBECONFIG_PATH:-}" ]]; then
        if [[ ! -f "${E2E_KUBECONFIG_PATH}" ]]; then
            echo "ERROR: E2E_KUBECONFIG_PATH does not exist: ${E2E_KUBECONFIG_PATH}" >&2
            return 1
        fi
        e2e_seed_env+=(-e "E2E_KUBECONFIG_B64=$(base64_file "${E2E_KUBECONFIG_PATH}")")
    fi

    server_container="$(compose_cmd ps -q server)"
    if [[ -z "${server_container}" ]]; then
        echo "Unable to resolve the server container for experience seeding." >&2
        return 1
    fi

    echo "Building extended experience seeder from the current source tree..."
    build_codespaces_e2e_seed

    echo "Injecting extended experience seeder into the running server container..."
    docker cp "${HOST_E2E_SEED_BIN}" "${server_container}:/tmp/e2e-seed"

    echo "Seeding experience fixtures (admin/test accounts + sample catalog data)..."
    compose_exec \
        "${e2e_seed_env[@]}" \
        server /tmp/e2e-seed >/dev/null
    compose_exec server rm -f /tmp/e2e-seed >/dev/null 2>&1 || true
}

wait_for_url() {
    local url="$1"
    local label="$2"
    echo "Waiting for ${label}..."
    for _ in {1..60}; do
        if curl -fsS "${url}" >/dev/null 2>&1; then
            echo " ${label} ready"
            return 0
        fi
        printf "."
        sleep 2
    done
    echo
    echo "${label} did not become ready in time" >&2
    return 1
}

prewarm_routes() {
    local base_url="$1"
    echo "Prewarming common routes..."
    for route in / /login /dashboard; do
        curl -fsS "${base_url}${route}" >/dev/null 2>&1 || true
    done
    echo " warmup complete"
}

stack_ready() {
    curl -fsS "http://localhost:${API_PORT}/api/v1/health/ready" >/dev/null 2>&1 \
        && curl -fsS "http://localhost:${HTTP_PORT}/" >/dev/null 2>&1
}

seed_codespaces() {
    echo "Seeding baseline data..."
    compose_exec server /usr/local/bin/seed >/dev/null
    run_codespaces_e2e_seed
}

start_stack() {
    local mode="$1"
    local public_base_url
    local allowed_origins
    local source_fingerprint
    local effective_mode

    public_base_url="$(compute_public_base_url)"
    allowed_origins="$(compute_allowed_origins "${public_base_url}")"
    source_fingerprint="$(compute_source_fingerprint)"
    effective_mode="${mode}"

    if [[ "${mode}" == "resume" ]]; then
        if ! load_state_file; then
            echo "Codespaces build state missing; rebuilding once to align containers with the current source tree."
            effective_mode="rebuild"
        elif [[ "${LAST_SOURCE_FINGERPRINT}" != "${source_fingerprint}" ]]; then
            echo "Codespaces source drift detected (${LAST_SOURCE_FINGERPRINT} -> ${source_fingerprint}); rebuilding images."
            effective_mode="rebuild"
        elif stack_ready \
            && [[ "${LAST_PROJECT_NAME}" == "${PROJECT_NAME}" ]] \
            && [[ "${LAST_SERVER_PUBLIC_BASE_URL}" == "${public_base_url}" ]] \
            && [[ "${LAST_SERVER_ALLOWED_ORIGINS}" == "${allowed_origins}" ]] \
            && [[ "${LAST_HTTP_PORT}" == "${HTTP_PORT}" ]] \
            && [[ "${LAST_API_PORT}" == "${API_PORT}" ]]; then
            echo "Backend and UI already ready; source + runtime config unchanged; skipping resume bootstrap."
            echo " Web UI: ${public_base_url}"
            echo " Login:  ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
            echo " User:   ${TEST_USERNAME} / ${TEST_PASSWORD}"
            return 0
        else
            echo "Codespaces runtime config changed or services are not ready; reconciling containers without reseeding data."
        fi
    fi

    DATABASE_URL="postgres://shepherd:shepherd_password@db:5432/shepherd_db?sslmode=disable"
    POSTGRES_PASSWORD="shepherd_password"
    SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET:-codespaces-session-secret-0123456789abcdef0123456789abcdef}"
    SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
    SERVER_PUBLIC_BASE_URL="${public_base_url}"
    SERVER_ALLOWED_ORIGINS="${allowed_origins}"

    echo "=============================================="
    echo " Shepherd Codespaces"
    echo "=============================================="
    echo " Mode:          ${effective_mode}"
    echo " Source root:   ${REPO_ROOT}"
    echo " Web UI:        ${public_base_url}"
    echo " Login(admin):  ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
    echo " Login(test):   ${TEST_USERNAME} / ${TEST_PASSWORD}"
    if [[ -n "${E2E_KUBECONFIG_B64:-${E2E_KUBECONFIG_PATH:-}}" ]]; then
        echo " Cluster seed:  configured from operator-provided kubeconfig"
    else
        echo " Cluster seed:  stub cluster only (no live KubeVirt backing configured)"
    fi

    if [[ "${effective_mode}" == "bootstrap" ]]; then
        echo "Resetting Codespaces state for a clean first boot..."
        compose_cmd down -v --remove-orphans >/dev/null 2>&1 || true
    fi

    compose_up "${effective_mode}"
    wait_for_url "http://localhost:${API_PORT}/api/v1/health/ready" "backend"

    case "${effective_mode}" in
        bootstrap)
            seed_codespaces
            ;;
        rebuild)
            echo "Skipping seed on rebuild."
            ;;
        resume)
            echo "Skipping seed on resume."
            ;;
    esac

    wait_for_url "http://localhost:${HTTP_PORT}/" "web UI"
    prewarm_routes "http://localhost:${HTTP_PORT}"
    write_state_file "${source_fingerprint}" "${public_base_url}" "${allowed_origins}"

    echo
    echo "=============================================="
    echo " Shepherd Codespaces is ready"
    echo "=============================================="
    echo " Web UI:  ${public_base_url}"
    echo " API:     http://localhost:${API_PORT}"
    echo " Admin:   ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
    echo " Test:    ${TEST_USERNAME} / ${TEST_PASSWORD}"
}

main() {
    case "${1:-bootstrap}" in
        -h|--help|help)
            usage
            ;;
        bootstrap|resume|rebuild)
            start_stack "$1"
            ;;
        *)
            usage >&2
            exit 1
            ;;
    esac
}

main "$@"
