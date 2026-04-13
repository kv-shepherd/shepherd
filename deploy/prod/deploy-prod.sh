#!/usr/bin/env bash
# =============================================================================
# KubeVirt Shepherd — Production Deployment Script
# =============================================================================
# Usage:
#   bash deploy/prod/deploy-prod.sh                    # Public deploy (no bootstrap seed)
#   bash deploy/prod/deploy-prod.sh --with-seed        # Public deploy + bootstrap seed
#   bash deploy/prod/deploy-prod.sh --enterprise       # Enterprise edition
#   bash deploy/prod/deploy-prod.sh --build-only       # Build images only
#   bash deploy/prod/deploy-prod.sh --seed-only        # Run bootstrap seed only (after services are up)
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.prod.yml"
ENV_FILE="${SCRIPT_DIR}/.env.prod"
ENTERPRISE_MODE=0
BUILD_ONLY=0
SEED_ONLY=0
RUN_SEED=0
SKIP_BUILD=0
DID_RUN_SEED=0
BUNDLED_POSTGRES_MODE=""
BUNDLED_POSTGRES=1

usage() {
    cat <<'EOF'
Usage: deploy-prod.sh [options]

Options:
  --enterprise     Deploy enterprise edition (requires private repo)
  --build-only     Build images only, do not start services
  --with-seed      Run bootstrap seed after startup (roles + default admin)
  --seed-only      Run bootstrap seed only (assumes services are running)
  --skip-build     Skip image build, use existing images
  -h, --help       Show this help message
EOF
}

extract_database_host() {
    local url="$1"
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
echo "=============================================="
echo "  KubeVirt Shepherd — Production Deployment"
echo "=============================================="
echo ""

if [[ ! -f "${ENV_FILE}" ]]; then
    echo "ERROR: ${ENV_FILE} not found."
    echo "  Create it from the example:"
    echo "    cp ${SCRIPT_DIR}/.env.prod.example ${ENV_FILE}"
    exit 1
fi

# Source env for validation
set -a
# shellcheck source=/dev/null
source "${ENV_FILE}"
set +a

# Validate required variables
for var in DATABASE_URL SECURITY_SESSION_SECRET SECURITY_ENCRYPTION_KEY SERVER_PUBLIC_BASE_URL; do
    val="${!var:-}"
    if [[ -z "${val}" ]] || [[ "${val}" == CHANGE_ME* ]]; then
        echo "ERROR: ${var} is not set or still has placeholder value in ${ENV_FILE}"
        exit 1
    fi
done

resolve_bundled_postgres_mode

if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
    val="${POSTGRES_PASSWORD:-}"
    if [[ -z "${val}" ]] || [[ "${val}" == CHANGE_ME* ]]; then
        echo "ERROR: POSTGRES_PASSWORD is required when bundled PostgreSQL is enabled"
        exit 1
    fi
fi

# Check TLS certificate
TLS_DIR="${SCRIPT_DIR}/tls"
if [[ ! -f "${TLS_DIR}/cert.pem" ]] || [[ ! -f "${TLS_DIR}/key.pem" ]]; then
    echo "WARNING: TLS certificates not found in ${TLS_DIR}/"
    echo "  Generating self-signed certificate for development/testing..."
    mkdir -p "${TLS_DIR}"
    openssl req \
        -x509 -nodes -newkey rsa:2048 -days 365 \
        -keyout "${TLS_DIR}/key.pem" \
        -out "${TLS_DIR}/cert.pem" \
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
else
    echo ""
    echo "--- Phase 1: Building Images ---"
    echo ""

    # Build Go backend
    echo "[1/3] Building Go backend image (shepherd-server)..."
    DOCKER_BUILDKIT=1 docker build \
        --network=host \
        -t shepherd-server:latest \
        -f "${ROOT_DIR}/Dockerfile" \
        "${ROOT_DIR}"
    echo "  ✓ shepherd-server:latest built"

    # Build Next.js frontend
    echo "[2/3] Building Next.js frontend image (shepherd-web)..."
    DOCKER_BUILDKIT=1 docker build \
        --network=host \
        -t shepherd-web:latest \
        -f "${SCRIPT_DIR}/web.Dockerfile" \
        "${ROOT_DIR}/web"
    echo "  ✓ shepherd-web:latest built"

    echo "[3/3] Pulling nginx:1.27-alpine..."
    docker pull nginx:1.27-alpine >/dev/null 2>&1 || true
    echo "  ✓ nginx:1.27-alpine ready"
    if [[ "${BUNDLED_POSTGRES}" == "1" ]]; then
        echo "  ✓ bundled PostgreSQL topology selected (${BUNDLED_POSTGRES_MODE})"
    else
        echo "  ✓ external PostgreSQL topology selected (${BUNDLED_POSTGRES_MODE})"
    fi

    if [[ "${BUILD_ONLY}" == "1" ]]; then
        echo ""
        echo "Build complete. Images ready:"
        echo "  - shepherd-server:latest"
        echo "  - shepherd-web:latest"
        echo "  - nginx:1.27-alpine"
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
            -p shepherd-prod \
            up -d db

        echo ""
        echo "Waiting for bundled database to be ready..."
        for _ in {1..30}; do
            if docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
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
            -p shepherd-prod \
            up -d server web nginx
    else
        echo ""
        echo "Skipping bundled database startup (external DATABASE_URL detected)."

        docker compose \
            -f "${COMPOSE_FILE}" \
            --env-file "${ENV_FILE}" \
            -p shepherd-prod \
            up -d server web nginx
    fi

    echo "Waiting for backend to be ready..."
    for _ in {1..60}; do
        if docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
            exec -T server wget --no-verbose --tries=1 --spider \
                "http://localhost:${SERVER_PORT:-8080}/api/v1/health/ready" >/dev/null 2>&1; then
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
    docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
        exec -T server /usr/local/bin/seed >/dev/null 2>&1 || {
        echo "  ⚠ bootstrap seed failed or was already reconciled (non-fatal)"
    }
    echo "  ✓ Bootstrap seed complete"
    DID_RUN_SEED=1
else
    echo ""
    echo "--- Phase 3: Skipping Bootstrap Seed ---"
    echo ""
    echo "Skipping bootstrap seed by default."
    echo "  Use --with-seed for first install/bootstrap or role-baseline reconciliation."
fi

# Rotate admin password if needed
ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin}"
if [[ "${DID_RUN_SEED}" == "1" && "${ADMIN_PASSWORD}" != "admin" ]]; then
    echo "Rotating default admin password..."
    TOKEN=""
    # Try login with target password first
    TOKEN=$(docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
        exec -T server wget -qO- \
            --header="Content-Type: application/json" \
            --post-data="{\"username\":\"admin\",\"password\":\"${ADMIN_PASSWORD}\"}" \
            "http://localhost:${SERVER_PORT:-8080}/api/v1/auth/login" 2>/dev/null | \
        grep -o '"token":"[^"]*"' | head -1 | sed 's/"token":"//;s/"//' || true)

    if [[ -n "${TOKEN}" ]]; then
        echo "  ✓ Admin password already set"
    else
        # Login with bootstrap password and rotate
        TOKEN=$(docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
            exec -T server wget -qO- \
                --header="Content-Type: application/json" \
                --post-data='{"username":"admin","password":"admin"}' \
                "http://localhost:${SERVER_PORT:-8080}/api/v1/auth/login" 2>/dev/null | \
            grep -o '"token":"[^"]*"' | head -1 | sed 's/"token":"//;s/"//' || true)

        if [[ -n "${TOKEN}" ]]; then
            docker compose -f "${COMPOSE_FILE}" -p shepherd-prod \
                exec -T server wget -qO- \
                    --header="Authorization: Bearer ${TOKEN}" \
                    --header="Content-Type: application/json" \
                    --post-data="{\"old_password\":\"admin\",\"new_password\":\"${ADMIN_PASSWORD}\"}" \
                    "http://localhost:${SERVER_PORT:-8080}/api/v1/auth/change-password" >/dev/null 2>&1 || true
            echo "  ✓ Admin password rotated"
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
    echo "  Login:     admin / ${ADMIN_PASSWORD}"
    echo "  Note:      first sign-in requires a password change"
    echo ""
else
    echo "  Bootstrap seed was skipped."
    echo "  Run with --with-seed (or --seed-only) if this is a fresh database."
    echo ""
fi
echo "  Management:"
echo "    docker compose -f ${COMPOSE_FILE} -p shepherd-prod logs -f"
echo "    docker compose -f ${COMPOSE_FILE} -p shepherd-prod ps"
echo "    docker compose -f ${COMPOSE_FILE} -p shepherd-prod down"
echo ""
if [[ "${ENTERPRISE_MODE}" == "1" ]]; then
    echo "  Enterprise Auth Setup:"
    echo "    cd private/shepherd-enterprise"
    echo "    make auth-bootstrap-apply"
    echo "    make auth-plan"
    echo "    make auth-apply"
fi
