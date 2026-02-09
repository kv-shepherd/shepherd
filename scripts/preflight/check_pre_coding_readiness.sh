#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

print_pass() {
  PASS_COUNT=$((PASS_COUNT + 1))
  echo "[PASS] $1"
}

print_fail() {
  FAIL_COUNT=$((FAIL_COUNT + 1))
  echo "[FAIL] $1"
}

print_warn() {
  WARN_COUNT=$((WARN_COUNT + 1))
  echo "[WARN] $1"
}

check_file() {
  local path="$1"
  local desc="$2"
  if [[ -f "${path}" ]]; then
    print_pass "${desc}: ${path}"
  else
    print_fail "${desc}: ${path} (missing)"
  fi
}

check_dir() {
  local path="$1"
  local desc="$2"
  if [[ -d "${path}" ]]; then
    print_pass "${desc}: ${path}"
  else
    print_fail "${desc}: ${path} (missing)"
  fi
}

check_content() {
  local path="$1"
  local pattern="$2"
  local desc="$3"
  if [[ ! -f "${path}" ]]; then
    print_fail "${desc}: ${path} (missing)"
    return
  fi
  if rg -q "${pattern}" "${path}"; then
    print_pass "${desc}"
  else
    print_fail "${desc}: pattern '${pattern}' not found in ${path}"
  fi
}

run_optional_check() {
  local desc="$1"
  shift
  if "$@"; then
    print_pass "${desc}"
  else
    print_warn "${desc}: command failed"
  fi
}

echo "== Pre-Coding Readiness Check (Local, Non-CI) =="
echo "Project root: ${ROOT_DIR}"
echo

echo "## A. Core bootstrap artifacts (Phase 0 hard gate)"
check_file "go.mod" "Go module file"
check_content "go.mod" "^module[[:space:]]+kv-shepherd\\.io/shepherd$" "Go module path uses ADR-0016 vanity import"
check_file "cmd/server/main.go" "Server entrypoint"
check_file ".github/workflows/ci.yml" "Main CI workflow"
check_file ".golangci.yml" "golangci-lint config"

echo
echo "## B. Design docs governance baseline (ADR-0030)"
check_dir "docs/design/frontend" "Frontend design layer directory"
check_file "docs/design/frontend/README.md" "Frontend design index"
check_file "docs/design/frontend/FRONTEND.md" "Frontend baseline spec"
check_dir "docs/design/database" "Database design layer directory"
check_file "docs/design/database/README.md" "Database design index"
check_content "docs/design/README.md" "\\./frontend/README\\.md" "Design README links frontend index"
check_content "docs/design/README.md" "\\./frontend/FRONTEND\\.md" "Design README links frontend baseline"
check_content "docs/design/interaction-flows/master-flow.md" "\\.\\./frontend/FRONTEND\\.md" "master-flow references canonical frontend path"
run_optional_check "Design docs governance script" bash -c "bash docs/design/ci/scripts/check_design_doc_governance.sh >/dev/null 2>&1"

echo
echo "## C. Design-phase API contract artifacts (ADR-0021, ADR-0029)"
check_file "docs/design/ci/workflows/api-contract.yaml" "Design-phase API contract workflow template"
check_file "docs/design/ci/makefile/api.mk" "Design-phase API make targets"
check_file "docs/design/ci/vacuum/.vacuum.yaml" "Vacuum ruleset template"
check_file "docs/design/ci/api-templates/openapi.yaml" "OpenAPI template"
check_file "docs/design/ci/scripts/api-check.sh" "API sync check script"

echo
echo "## D. Coding-phase landing targets (informational, not auto-migrated)"
if [[ -f ".github/workflows/docs-governance.yaml" ]]; then
  print_warn "Coding-phase docs governance workflow already exists in .github/workflows"
else
  print_warn "Coding-phase docs governance workflow not landed yet (.github/workflows/docs-governance.yaml)"
fi
if [[ -f ".github/workflows/api-contract.yaml" ]]; then
  print_warn "Coding-phase API contract workflow already exists in .github/workflows"
else
  print_warn "Coding-phase API contract workflow not landed yet (.github/workflows/api-contract.yaml)"
fi
if [[ -f "build/api.mk" ]]; then
  print_warn "build/api.mk already exists"
else
  print_warn "build/api.mk not landed yet (still in docs/design/ci/makefile/api.mk)"
fi
if [[ -f "scripts/check-sqlc-usage.sh" ]]; then
  print_warn "scripts/check-sqlc-usage.sh already exists"
else
  print_warn "scripts/check-sqlc-usage.sh not landed yet"
fi

echo
echo "== Summary =="
echo "PASS: ${PASS_COUNT}"
echo "FAIL: ${FAIL_COUNT}"
echo "WARN: ${WARN_COUNT}"

if (( FAIL_COUNT > 0 )); then
  exit 1
fi
