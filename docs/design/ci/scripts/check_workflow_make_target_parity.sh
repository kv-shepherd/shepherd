#!/usr/bin/env bash

set -euo pipefail

failures=0

require_line() {
  local file="$1"
  local needle="$2"
  if ! grep -Fq "$needle" "$file"; then
    echo "Missing required workflow line in ${file}: ${needle}" >&2
    failures=1
  fi
}

forbid_line() {
  local file="$1"
  local needle="$2"
  if grep -Fq "$needle" "$file"; then
    echo "Forbidden workflow line in ${file}: ${needle}" >&2
    failures=1
  fi
}

ci_workflow=".github/workflows/ci.yml"
frontend_workflow=".github/workflows/frontend-tests.yml"
api_workflow=".github/workflows/api-contract.yaml"

require_line "$ci_workflow" "run: make secrets-scan"
require_line "$ci_workflow" "run: make govulncheck"
require_line "$ci_workflow" "run: make ci-go-lint"
require_line "$ci_workflow" "run: make ci-go-build"
require_line "$ci_workflow" "run: make ci-go-test"
require_line "$ci_workflow" "run: make ci-master-flow-backend"
require_line "$ci_workflow" "run: make ci-governance"

forbid_line "$ci_workflow" "uses: golangci/golangci-lint-action"
forbid_line "$ci_workflow" "run: go build ./..."
forbid_line "$ci_workflow" "run: go test -race -coverprofile=coverage.out -covermode=atomic ./..."

require_line "$frontend_workflow" "run: make -C .. ci-frontend-deadcode"
require_line "$frontend_workflow" "run: make -C .. ci-frontend-unit"
require_line "$frontend_workflow" "run: make -C .. ci-e2e-smoke"

forbid_line "$frontend_workflow" "run: npm audit --audit-level=high"
forbid_line "$frontend_workflow" "run: npm run knip"
forbid_line "$frontend_workflow" "run: npm run typecheck"
forbid_line "$frontend_workflow" "run: npm run test:run"
forbid_line "$frontend_workflow" "run: npm run build"

require_line "$api_workflow" "run: make ci-api-lint"
require_line "$api_workflow" 'run: make ci-api-breaking BASE_REF=${{ github.base_ref }}'
require_line "$api_workflow" "run: make ci-api-generated-sync"
require_line "$api_workflow" "run: make ci-api-contract"

forbid_line "$api_workflow" "run: make api-compat-generate"
forbid_line "$api_workflow" "run: make api-generate"
forbid_line "$api_workflow" "run: make api-compat"
forbid_line "$api_workflow" "run: npm run typecheck"
forbid_line "$api_workflow" "run: go test ./internal/api/middleware/... -run OpenAPI -count=1"

if [[ "$failures" -ne 0 ]]; then
  echo "FAIL: workflow/make target parity check failed" >&2
  exit 1
fi

echo "OK: workflow/make target parity check passed"
