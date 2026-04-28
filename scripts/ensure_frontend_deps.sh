#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WEB_DIR="${PROJECT_ROOT}/web"
NODE_MODULES_DIR="${WEB_DIR}/node_modules"
STAMP_FILE="${NODE_MODULES_DIR}/.pr-ci-deps.sha256"

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file}" | awk '{print $1}'
  else
    shasum -a 256 "${file}" | awk '{print $1}'
  fi
}

sha256_stream() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum | awk '{print $1}'
  else
    shasum -a 256 | awk '{print $1}'
  fi
}

dependency_fingerprint() {
  {
    printf 'node=%s\n' "$(node --version)"
    printf 'npm=%s\n' "$(npm --version)"
    printf 'package.json=%s\n' "$(sha256_file "${WEB_DIR}/package.json")"
    printf 'package-lock.json=%s\n' "$(sha256_file "${WEB_DIR}/package-lock.json")"
  } | sha256_stream
}

missing_dependency_path() {
  local required_paths=(
    "${NODE_MODULES_DIR}/.bin/next"
    "${NODE_MODULES_DIR}/.bin/vitest"
    "${NODE_MODULES_DIR}/@ant-design/icons/package.json"
    "${NODE_MODULES_DIR}/@testing-library/react/package.json"
    "${NODE_MODULES_DIR}/eslint/package.json"
    "${NODE_MODULES_DIR}/knip/package.json"
    "${NODE_MODULES_DIR}/next/dist/bin/next"
    "${NODE_MODULES_DIR}/next/package.json"
    "${NODE_MODULES_DIR}/react/package.json"
    "${NODE_MODULES_DIR}/typescript/bin/tsc"
    "${NODE_MODULES_DIR}/typescript/package.json"
    "${NODE_MODULES_DIR}/vitest/package.json"
    "${NODE_MODULES_DIR}/vitest/suppress-warnings.cjs"
  )

  local path
  for path in "${required_paths[@]}"; do
    if [[ ! -e "${path}" ]]; then
      printf '%s\n' "${path}"
      return 0
    fi
  done

  return 1
}

frontend_dependency_tree_ready() {
  local missing_path
  if missing_path="$(missing_dependency_path)"; then
    echo "Frontend dependency cache is missing ${missing_path}."
    return 1
  fi

  if ! (cd "${WEB_DIR}" && npm ls --depth=0 --silent >/dev/null 2>&1); then
    echo "Frontend dependency tree failed npm ls validation."
    return 1
  fi

  return 0
}

fingerprint="$(dependency_fingerprint)"

if [[ -d "${NODE_MODULES_DIR}" && -f "${STAMP_FILE}" && "$(cat "${STAMP_FILE}")" == "${fingerprint}" ]]; then
  if frontend_dependency_tree_ready; then
    echo "Reusing frontend dependencies from ${NODE_MODULES_DIR}."
    exit 0
  else
    echo "Frontend dependency cache failed validation; reinstalling."
  fi
fi

echo "Installing frontend dependencies with npm ci."
(cd "${WEB_DIR}" && npm ci)

settle_attempts="${FRONTEND_DEPS_SETTLE_ATTEMPTS:-10}"
for attempt in $(seq 1 "${settle_attempts}"); do
  if frontend_dependency_tree_ready; then
    printf '%s\n' "${fingerprint}" >"${STAMP_FILE}"
    exit 0
  fi
  if [[ "${attempt}" -lt "${settle_attempts}" ]]; then
    sleep 1
  fi
done

echo "Frontend dependency install did not pass validation after ${settle_attempts} attempts." >&2
exit 1
