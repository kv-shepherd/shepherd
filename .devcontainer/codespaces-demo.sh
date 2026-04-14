#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.codespaces.yml"
PROJECT_NAME="${CODESPACES_PROJECT_NAME:-shepherd-codespaces}"
HTTP_PORT="${CODESPACES_HTTP_PORT:-3000}"
API_PORT="${CODESPACES_API_PORT:-8080}"
ADMIN_USERNAME="${CODESPACES_ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${CODESPACES_ADMIN_PASSWORD:-admin}"
GHCR_REPO="${CODESPACES_RELEASE_REPO:-kv-shepherd/shepherd}"
DEFAULT_RELEASE_TAG="${CODESPACES_RELEASE_TAG:-v0.1.0-alpha.1}"

usage() {
    cat <<'EOF'
Usage: bash .devcontainer/codespaces-demo.sh [bootstrap|resume]

bootstrap  Start a fresh Codespaces demo stack, seed baseline + demo fixtures,
           and wait for the UI to become ready.
resume     Start the stack without reseeding, reusing the existing demo data.
EOF
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

resolve_release_tag() {
    if command -v gh >/dev/null 2>&1; then
        local tag=""
        tag="$(
            gh release list \
                --repo "${GHCR_REPO}" \
                --limit 20 \
                --json tagName,isDraft,isPrerelease \
                --jq '[.[] | select(.isDraft == false)] | .[0].tagName'
        )" || true
        if [[ -n "${tag}" && "${tag}" != "null" ]]; then
            printf "%s" "${tag}"
            return 0
        fi
    fi

    printf "%s" "${DEFAULT_RELEASE_TAG}"
}

docker_login_ghcr() {
    if ! command -v gh >/dev/null 2>&1; then
        echo "gh CLI is required to authenticate GHCR pulls in Codespaces." >&2
        return 1
    fi

    local token="${GH_TOKEN:-${GITHUB_TOKEN:-}}"
    local username
    if [[ -z "${token}" ]]; then
        if ! token="$(gh auth token)"; then
            echo "Unable to read a GitHub auth token for GHCR pulls. Sign in to gh inside the Codespace and retry." >&2
            return 1
        fi
    fi
    if ! username="$(gh api user --jq .login)"; then
        echo "Unable to resolve the GitHub username for GHCR login." >&2
        return 1
    fi
    printf '%s' "${token}" | docker login ghcr.io -u "${username}" --password-stdin >/dev/null
}

compose_up() {
    local pull_policy="$1"

    SERVER_IMAGE="${SERVER_IMAGE}" \
    WEB_IMAGE="${WEB_IMAGE}" \
    HTTP_PORT="${HTTP_PORT}" \
    API_PORT="${API_PORT}" \
    DATABASE_URL="${DATABASE_URL}" \
    POSTGRES_PASSWORD="${POSTGRES_PASSWORD}" \
    SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET}" \
    SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY}" \
    SERVER_PUBLIC_BASE_URL="${SERVER_PUBLIC_BASE_URL}" \
    SERVER_ALLOWED_ORIGINS="${SERVER_ALLOWED_ORIGINS}" \
    COMPOSE_PROJECT_NAME="${PROJECT_NAME}" \
        docker compose -f "${COMPOSE_FILE}" up -d --pull "${pull_policy}"
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

seed_demo() {
    echo "Seeding baseline data..."
    COMPOSE_PROJECT_NAME="${PROJECT_NAME}" \
        docker compose -f "${COMPOSE_FILE}" exec -T server /usr/local/bin/seed >/dev/null

    echo "Seeding demo fixtures..."
    COMPOSE_PROJECT_NAME="${PROJECT_NAME}" \
        docker compose -f "${COMPOSE_FILE}" exec -T \
        -e "E2E_ADMIN_USERNAME=${ADMIN_USERNAME}" \
        -e "E2E_ADMIN_PASSWORD=${ADMIN_PASSWORD}" \
        server /usr/local/bin/e2e-seed >/dev/null
}

start_stack() {
    local mode="$1"
    local public_base_url
    local allowed_origins
    local release_tag
    local release_version

    public_base_url="$(compute_public_base_url)"

    if [[ "${mode}" == "resume" ]] && stack_ready; then
        echo "Backend and UI already ready; skipping resume bootstrap."
        echo " Web UI: ${public_base_url}"
        echo " Login:  ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
        return 0
    fi

    allowed_origins="$(compute_allowed_origins "${public_base_url}")"
    release_tag="$(resolve_release_tag)"
    release_version="${release_tag#v}"

    SERVER_IMAGE="ghcr.io/kv-shepherd/shepherd-server:${release_version}"
    WEB_IMAGE="ghcr.io/kv-shepherd/shepherd-web:${release_version}"
    DATABASE_URL="postgres://shepherd:shepherd_password@db:5432/shepherd_db?sslmode=disable"
    POSTGRES_PASSWORD="shepherd_password"
    SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET:-codespaces-session-secret-0123456789abcdef0123456789abcdef}"
    SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
    SERVER_PUBLIC_BASE_URL="${public_base_url}"
    SERVER_ALLOWED_ORIGINS="${allowed_origins}"

    echo "=============================================="
    echo " Shepherd Codespaces Demo"
    echo "=============================================="
    echo " Release tag: ${release_tag}"
    echo " Server image: ${SERVER_IMAGE}"
    echo " Web image:    ${WEB_IMAGE}"
    echo " Web UI:       ${public_base_url}"
    echo " Login:        ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"

    docker_login_ghcr

    if [[ "${mode}" == "bootstrap" ]]; then
        echo "Resetting demo volume state for a clean first boot..."
        COMPOSE_PROJECT_NAME="${PROJECT_NAME}" docker compose -f "${COMPOSE_FILE}" down -v --remove-orphans >/dev/null 2>&1 || true
    fi

    if [[ "${mode}" == "bootstrap" ]]; then
        compose_up "always"
    else
        compose_up "missing"
    fi

    wait_for_url "http://localhost:${API_PORT}/api/v1/health/ready" "backend"

    if [[ "${mode}" == "bootstrap" ]]; then
        seed_demo
    else
        echo "Skipping seed on resume."
    fi

    wait_for_url "http://localhost:${HTTP_PORT}/" "web UI"
    prewarm_routes "http://localhost:${HTTP_PORT}"

    echo ""
    echo "=============================================="
    echo " ✅ Shepherd Codespaces demo is ready"
    echo "=============================================="
    echo " Web UI:  ${public_base_url}"
    echo " API:     http://localhost:${API_PORT}"
    echo " Login:   ${ADMIN_USERNAME} / ${ADMIN_PASSWORD}"
}

main() {
    case "${1:-bootstrap}" in
        -h|--help|help)
            usage
            ;;
        bootstrap|resume)
            start_stack "$1"
            ;;
        *)
            usage >&2
            exit 1
            ;;
    esac
}

main "$@"
