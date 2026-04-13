#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=============================================="
echo " Shepherd Dev Container — Initial Bootstrap"
echo "=============================================="

if [[ -n "${CODESPACE_NAME:-}" ]]; then
    public_base_url="https://${CODESPACE_NAME}-3000.${GITHUB_CODESPACES_PORT_FORWARDING_DOMAIN:-app.github.dev}"
    echo "Detected Codespaces: ${public_base_url}"
else
    public_base_url="http://localhost:3000"
fi

export DEV_PUBLIC_BASE_URL="${public_base_url}"
export DEV_FRONTEND_MODE="host"
export DEV_ADMIN_PASSWORD="${DEV_ADMIN_PASSWORD:-admin}"

cd "${ROOT_DIR}"

# First boot for demos should start from a clean dev database and then load
# baseline + extended fixtures so the user lands on a populated environment.
bash start-dev.sh --clean-all --e2e-seed

echo ""
echo "=============================================="
echo " ✅ Shepherd demo environment is ready"
echo "=============================================="
echo " Web UI:  ${public_base_url}"
echo " Login:   admin / ${DEV_ADMIN_PASSWORD}"
