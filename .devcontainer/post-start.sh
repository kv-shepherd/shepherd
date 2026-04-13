#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=============================================="
echo " Shepherd Dev Container — Resume Services"
echo "=============================================="

if [[ -n "${CODESPACE_NAME:-}" ]]; then
    public_base_url="https://${CODESPACE_NAME}-3000.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-app.github.dev}"
else
    public_base_url="http://localhost:3000"
fi

export DEV_PUBLIC_BASE_URL="${public_base_url}"
export DEV_FRONTEND_MODE="host"
export DEV_ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin}"

if curl -fsS http://localhost:8080/api/v1/health/ready >/dev/null 2>&1 \
    && curl -fsS http://localhost:3000/ >/dev/null 2>&1; then
    echo "Backend and UI already ready; skipping restart."
    exit 0
fi

cd "${ROOT_DIR}"

# Resume from existing DB contents; do not reseed or rotate bootstrap state on
# every Codespaces/container start.
bash start-dev.sh --skip-seed

echo ""
echo "=============================================="
echo " ✅ Shepherd demo services resumed"
echo "=============================================="
echo " Web UI:  ${public_base_url}"
echo " Login:   admin / ${DEV_ADMIN_PASSWORD}"
