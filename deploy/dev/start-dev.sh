#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/dev/docker-compose.yml"
HOST_USER_ID="${USER_ID:-$(id -u)}"
HOST_GROUP_ID="${GROUP_ID:-$(id -g)}"
DEV_ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin}"
NODE_MODULES_DIR="${ROOT_DIR}/web/node_modules"
LOCK_HASH_FILE="${NODE_MODULES_DIR}/.package-lock.hash"
SERVICES_TO_DELETE=("db" "server" "web" "nginx")
COMPOSE_CMD=(docker compose -f "${COMPOSE_FILE}")
DEV_INCLUDE_E2E_SEED="${DEV_INCLUDE_E2E_SEED:-0}"
DEV_FRONTEND_MODE="${DEV_FRONTEND_MODE:-host}"
DEV_FRONTEND_PORT="${DEV_FRONTEND_PORT:-3001}"
DEV_INGRESS_PORT="${DEV_INGRESS_PORT:-3000}"
DEV_HTTPS_INGRESS_PORT="${DEV_HTTPS_INGRESS_PORT:-3443}"
DEV_PUBLIC_BASE_URL="${DEV_PUBLIC_BASE_URL:-}"
DEV_NGINX_TLS_DIR="${DEV_NGINX_TLS_DIR:-${ROOT_DIR}/tmp/dev-nginx-tls}"
DEV_NGINX_TLS_CERT_FILE="${DEV_NGINX_TLS_DIR}/cert.pem"
DEV_NGINX_TLS_KEY_FILE="${DEV_NGINX_TLS_DIR}/key.pem"
DEV_NGINX_TLS_HOSTS_FILE="${DEV_NGINX_TLS_DIR}/hosts.txt"
DEV_FRONTEND_RUNTIME="${DEV_FRONTEND_RUNTIME:-dev}" # dev|prod
DEV_FRONTEND_PROD_DIST_DIR="${DEV_FRONTEND_PROD_DIST_DIR:-.next-prod}"
# Dev-only tuning:
# - Source maps improve stack traces but can increase memory/CPU usage.
DEV_FRONTEND_DISABLE_SOURCE_MAPS="${DEV_FRONTEND_DISABLE_SOURCE_MAPS:-0}"
# Frontend OOM guard defaults:
# - enabled by default (no swap on many dev machines makes kernel OOM-kill likely)
# - applies only to the Next.js dev server process (host or docker)
DEV_FRONTEND_OOM_GUARD="${DEV_FRONTEND_OOM_GUARD:-1}"
DEV_FRONTEND_OOM_GUARD_MAX_OLD_SPACE_MB="${DEV_FRONTEND_OOM_GUARD_MAX_OLD_SPACE_MB:-4096}"
DEV_FRONTEND_OOM_DIAGNOSTICS="${DEV_FRONTEND_OOM_DIAGNOSTICS:-1}"
DEV_FRONTEND_HEAPSNAPSHOT_NEAR_LIMIT_COUNT="${DEV_FRONTEND_HEAPSNAPSHOT_NEAR_LIMIT_COUNT:-2}"
DEV_FRONTEND_DIAGNOSTIC_DIR="${DEV_FRONTEND_DIAGNOSTIC_DIR:-${ROOT_DIR}/tmp/node-diagnostics/dev-web}"
# Optional: override NODE_OPTIONS for the Next.js dev server only (host mode).
# Example:
#   DEV_FRONTEND_NODE_OPTIONS="--max-old-space-size=6144 --heapsnapshot-signal=SIGUSR2"
DEV_FRONTEND_NODE_OPTIONS="${DEV_FRONTEND_NODE_OPTIONS:-}"
FRONTEND_PID_FILE="${ROOT_DIR}/tmp/dev-web.pid"
FRONTEND_LOG_FILE="${ROOT_DIR}/tmp/dev-web.log"
CLEAN_ALL=0
SKIP_SEED=0
# Default to webpack for stability; Turbopack can consume excessive memory in some dev scenarios.
DEV_FRONTEND_BUILDER="${DEV_FRONTEND_BUILDER:-webpack}"

usage() {
    cat <<'EOF'
Usage: ./start-dev.sh [options]

Options:
  --clean-all        Reset the development stack from a clean database and remove
                     the existing dev PostgreSQL container/data.
  --skip-seed        Do not run baseline/e2e seed or rotate the bootstrap admin
                     password. Useful for resume flows that should keep existing
                     demo data untouched.
  --e2e-seed         Run extended local fixtures (`cmd/e2e-seed`) after the
                     baseline development seed (`cmd/seed`).
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

url_origin() {
    node -p '(() => { try { return new URL(process.argv[1] || "").origin; } catch { return ""; } })()' -- "$1"
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

append_dev_origin_host() {
    local csv="$1"
    local candidate_url="$2"
    if [[ -z "${candidate_url}" ]]; then
        printf "%s" "${csv}"
        return 0
    fi
    CSV_INPUT="${csv}" CANDIDATE_URL="${candidate_url}" node -e '
        const csv = process.env.CSV_INPUT || "";
        const raw = process.env.CANDIDATE_URL || "";
        const values = csv.split(",").map(v => v.trim()).filter(Boolean);
        try {
            const parsed = new URL(raw);
            const host = parsed.hostname;
            if (host && !values.some(v => v.toLowerCase() === host.toLowerCase())) {
                values.push(host);
            }
        } catch {}
        process.stdout.write(values.join(","));
    '
}

compute_allowed_dev_origin_urls() {
    DEV_FRONTEND_PORT="${DEV_FRONTEND_PORT}" DEV_INGRESS_PORT="${DEV_INGRESS_PORT}" DEV_HTTPS_INGRESS_PORT="${DEV_HTTPS_INGRESS_PORT}" node -e '
        const os = require("os");
        const hosts = new Set(["localhost", "127.0.0.1"]);
        const hostname = os.hostname();
        if (hostname) {
            hosts.add(hostname);
        }
        for (const infos of Object.values(os.networkInterfaces())) {
            for (const info of infos || []) {
                if (info && info.family === "IPv4" && !info.internal) {
                    hosts.add(info.address);
                }
            }
        }
        const ports = new Set([
            process.env.DEV_INGRESS_PORT || "3000",
            process.env.DEV_HTTPS_INGRESS_PORT || "3443",
            process.env.DEV_FRONTEND_PORT || "3001",
        ]);
        const origins = [];
        for (const host of hosts) {
            for (const port of ports) {
                origins.push(`http://${host}:${port}`);
                if (port === (process.env.DEV_HTTPS_INGRESS_PORT || "3443")) {
                    origins.push(`https://${host}:${port}`);
                }
            }
        }
        process.stdout.write(origins.join(","));
    '
}

compute_public_dev_base_url() {
    if [[ -n "${DEV_PUBLIC_BASE_URL}" ]]; then
        printf "%s" "${DEV_PUBLIC_BASE_URL}"
        return 0
    fi
    DEV_HTTPS_INGRESS_PORT="${DEV_HTTPS_INGRESS_PORT}" node -e '
        const os = require("os");
        const port = process.env.DEV_HTTPS_INGRESS_PORT || "3443";
        let selectedHost = "";
        for (const infos of Object.values(os.networkInterfaces())) {
            for (const info of infos || []) {
                if (info && info.family === "IPv4" && !info.internal) {
                    selectedHost = info.address;
                    break;
                }
            }
            if (selectedHost) {
                break;
            }
        }
        if (!selectedHost) {
            const hostname = os.hostname();
            selectedHost = hostname || "localhost";
        }
        process.stdout.write(`https://${selectedHost}:${port}`);
    '
}

compute_dev_tls_hosts() {
    node -e '
        const os = require("os");
        const values = new Set(["localhost", "127.0.0.1"]);
        const hostname = os.hostname();
        if (hostname) {
            values.add(hostname);
        }
        for (const infos of Object.values(os.networkInterfaces())) {
            for (const info of infos || []) {
                if (info && info.family === "IPv4" && !info.internal) {
                    values.add(info.address);
                }
            }
        }
        process.stdout.write(Array.from(values).sort().join("\n"));
    '
}

generate_dev_tls_certificate() {
    local hosts=""
    hosts="$(compute_dev_tls_hosts)"
    mkdir -p "${DEV_NGINX_TLS_DIR}"

    local regenerate="0"
    if [[ ! -f "${DEV_NGINX_TLS_CERT_FILE}" || ! -f "${DEV_NGINX_TLS_KEY_FILE}" ]]; then
        regenerate="1"
    elif [[ ! -f "${DEV_NGINX_TLS_HOSTS_FILE}" ]]; then
        regenerate="1"
    elif [[ "$(cat "${DEV_NGINX_TLS_HOSTS_FILE}")" != "${hosts}" ]]; then
        regenerate="1"
    fi

    if [[ "${regenerate}" != "1" ]]; then
        return 0
    fi

    local config_file="${DEV_NGINX_TLS_DIR}/openssl.cnf"
    local alt_names_file="${DEV_NGINX_TLS_DIR}/alt_names.cnf"
    : > "${alt_names_file}"
    local dns_index=1
    local ip_index=1
    while IFS= read -r host; do
        [[ -z "${host}" ]] && continue
        if [[ "${host}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            printf "IP.%d = %s\n" "${ip_index}" "${host}" >> "${alt_names_file}"
            ip_index=$((ip_index + 1))
        else
            printf "DNS.%d = %s\n" "${dns_index}" "${host}" >> "${alt_names_file}"
            dns_index=$((dns_index + 1))
        fi
    done <<< "${hosts}"

    cat > "${config_file}" <<EOF
[ req ]
default_bits = 2048
prompt = no
distinguished_name = req_distinguished_name
x509_extensions = v3_req

[ req_distinguished_name ]
CN = localhost

[ v3_req ]
subjectAltName = @alt_names

[ alt_names ]
$(cat "${alt_names_file}")
EOF

    echo "Generating development TLS certificate (${DEV_NGINX_TLS_CERT_FILE})..."
    openssl req \
        -x509 \
        -nodes \
        -newkey rsa:2048 \
        -days 365 \
        -keyout "${DEV_NGINX_TLS_KEY_FILE}" \
        -out "${DEV_NGINX_TLS_CERT_FILE}" \
        -config "${config_file}" \
        >/dev/null 2>&1

    printf "%s" "${hosts}" > "${DEV_NGINX_TLS_HOSTS_FILE}"
}

append_origin_url() {
    local csv="$1"
    local candidate="$2"
    if [[ -z "${candidate}" ]]; then
        printf "%s" "${csv}"
        return 0
    fi
    if [[ -z "${csv}" ]]; then
        printf "%s" "${candidate}"
        return 0
    fi
    CSV_INPUT="${csv}" CANDIDATE_INPUT="${candidate}" node -e '
        const csv = process.env.CSV_INPUT || "";
        const candidate = process.env.CANDIDATE_INPUT || "";
        const values = csv.split(",").map(v => v.trim()).filter(Boolean);
        if (!values.some(v => v.toLowerCase() === candidate.toLowerCase())) {
            values.push(candidate);
        }
        process.stdout.write(values.join(","));
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
        if [[ "${DEV_FRONTEND_OOM_DIAGNOSTICS}" == "1" ]]; then
            node_options="${node_options} --diagnostic-dir=${DEV_FRONTEND_DIAGNOSTIC_DIR} --report-on-fatalerror --report-dir=${DEV_FRONTEND_DIAGNOSTIC_DIR} --heapsnapshot-near-heap-limit=${DEV_FRONTEND_HEAPSNAPSHOT_NEAR_LIMIT_COUNT}"
        fi
    fi
    mkdir -p "$(dirname "${FRONTEND_PID_FILE}")"
    if [[ "${DEV_FRONTEND_OOM_DIAGNOSTICS}" == "1" ]]; then
        mkdir -p "${DEV_FRONTEND_DIAGNOSTIC_DIR}"
    fi
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
            NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE="1" \
            NEXT_PUBLIC_DEV_SECURE_ORIGIN="${DEV_PUBLIC_BASE_URL}" \
            NEXT_PUBLIC_DEV_HTTP_INGRESS_PORT="${DEV_INGRESS_PORT}" \
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
        if [[ "${DEV_FRONTEND_OOM_DIAGNOSTICS}" == "1" ]]; then
            echo "  - diagnostics: ${DEV_FRONTEND_DIAGNOSTIC_DIR}"
        fi
        (
            cd "${ROOT_DIR}/web"
            NEXT_PUBLIC_API_URL="/api/v1" \
            NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE="1" \
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
    allowed_origins="$(append_dev_origin_host "${allowed_origins}" "${DEV_PUBLIC_BASE_URL}")"
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
    if [[ "${DEV_FRONTEND_OOM_DIAGNOSTICS}" == "1" ]]; then
        echo "  - diagnostics: ${DEV_FRONTEND_DIAGNOSTIC_DIR}"
    fi
    (
        cd "${ROOT_DIR}/web"
        DEV_ALLOWED_ORIGINS="${allowed_origins}" \
        NEXT_PUBLIC_API_URL="/api/v1" \
        NEXT_PUBLIC_DEV_BROWSER_LOG_BRIDGE="1" \
        NEXT_PUBLIC_DEV_SECURE_ORIGIN="${DEV_PUBLIC_BASE_URL}" \
        NEXT_PUBLIC_DEV_HTTP_INGRESS_PORT="${DEV_INGRESS_PORT}" \
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
require_cmd openssl

while [[ $# -gt 0 ]]; do
    case "$1" in
        --clean-all)
            CLEAN_ALL=1
            shift
            ;;
        --skip-seed)
            SKIP_SEED=1
            shift
            ;;
        --e2e-seed)
            DEV_INCLUDE_E2E_SEED=1
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

if [[ "${DEV_INCLUDE_E2E_SEED}" != "0" && "${DEV_INCLUDE_E2E_SEED}" != "1" ]]; then
    echo "DEV_INCLUDE_E2E_SEED must be '0' or '1'. Current value: ${DEV_INCLUDE_E2E_SEED}"
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
if [[ "${CLEAN_ALL}" == "1" ]]; then
    echo "Resetting development environment (clean all services and DB data)..."
    for svc in "${SERVICES_TO_DELETE[@]}"; do
        echo "  Removing service: $svc"
        "${COMPOSE_CMD[@]}" rm -s -f -v "$svc" || true
    done
    "${COMPOSE_CMD[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
else
    echo "Resetting development environment (preserve DB container/data)..."
    for svc in server web nginx; do
        echo "  Removing service: $svc"
        "${COMPOSE_CMD[@]}" rm -s -f -v "$svc" || true
    done
    echo "  Preserving service: db"
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
generate_dev_tls_certificate
dev_allowed_origin_urls="$(compute_allowed_dev_origin_urls)"
dev_public_base_url="$(compute_public_dev_base_url)"
DEV_PUBLIC_BASE_URL="${dev_public_base_url}"
dev_public_origin="$(url_origin "${dev_public_base_url}")"
dev_allowed_origin_urls="$(append_origin_url "${dev_allowed_origin_urls}" "${dev_public_origin}")"
echo "  - backend allowed origins: ${dev_allowed_origin_urls}"
echo "  - backend public base url: ${dev_public_base_url}"
USER_ID="${HOST_USER_ID}" \
GROUP_ID="${HOST_GROUP_ID}" \
WEB_UPSTREAM="${WEB_UPSTREAM}" \
DEV_ALLOWED_ORIGIN_URLS="${dev_allowed_origin_urls}" \
DEV_PUBLIC_BASE_URL="${dev_public_base_url}" \
DEV_HTTPS_INGRESS_PORT="${DEV_HTTPS_INGRESS_PORT}" \
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

if [[ "${SKIP_SEED}" == "1" ]]; then
    echo "Skipping development seed/bootstrap rotation (--skip-seed)."
else
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
        echo " seed complete (baseline only)"
    fi
fi

if [[ "${DEV_FRONTEND_MODE}" == "host" ]]; then
    start_host_frontend
    wait_for_host_frontend
fi

echo "Waiting for ingress (https://localhost:${DEV_HTTPS_INGRESS_PORT})..."
for _ in {1..30}; do
    if curl -kfsS "https://localhost:${DEV_HTTPS_INGRESS_PORT}/" >/dev/null; then
        echo " ingress ready"
        break
    fi
    printf "."
    sleep 2
done

echo "Prewarming common routes..."
for route in / /login /dashboard; do
    curl -kfsS "https://localhost:${DEV_HTTPS_INGRESS_PORT}${route}" >/dev/null || true
done
echo " warmup complete"

echo ""
echo "Development environment is UP"
echo "  - Web (nginx redirect): http://localhost:${DEV_INGRESS_PORT}"
echo "  - Web (nginx ingress):  https://localhost:${DEV_HTTPS_INGRESS_PORT}"
echo "  - Backend direct:      http://localhost:8080"
echo "  - DB:                  localhost:5432"
echo "  - Frontend mode:       ${DEV_FRONTEND_MODE}"
if [[ "${DEV_FRONTEND_MODE}" == "host" ]]; then
    echo "  - Frontend direct:     http://localhost:${DEV_FRONTEND_PORT}"
    echo "  - Frontend log:        ${FRONTEND_LOG_FILE}"
fi
if [[ "${CLEAN_ALL}" == "1" ]]; then
    echo "  - DB reset mode:       rebuilt (--clean-all)"
else
    echo "  - DB reset mode:       preserved (default)"
fi
if [[ "${SKIP_SEED}" == "1" ]]; then
    echo "  - Seed/bootstrap:      skipped (--skip-seed)"
else
    echo "  - Seeded users:        admin/${DEV_ADMIN_PASSWORD} (rotated from bootstrap admin/admin)"
    if [[ "${DEV_INCLUDE_E2E_SEED}" == "1" ]]; then
echo "                         e2e-admin/e2e-admin-123"
    fi
fi
echo "  - Note:                accept the local TLS certificate once in the browser for noVNC"
