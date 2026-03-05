#!/usr/bin/env bash
# check_no_legacy_batch1_invocations.sh — ADR-0039 cleanup guard
#
# Ensures Batch1+Batch2-migrated architecture checks are not invoked via legacy
# `go run docs/design/ci/scripts/check_*.go` calls in CI workflows/Makefile.
#
# Batch1+2 checks are enforced by `shepherd-arch` (golangci-lint module plugin).

set -euo pipefail

if ! command -v rg >/dev/null 2>&1; then
  echo "ERROR (ADR-0039): ripgrep (rg) is required for legacy invocation detection."
  exit 127
fi

TARGETS=(
  ".github/workflows/*.yml"
  ".github/workflows/*.yaml"
  "Makefile"
)

BATCH1_SCRIPTS=(
  "check_forbidden_imports.go"
  "check_naked_goroutine.go"
  "check_no_gorm_import.go"
  "check_no_outbox_import.go"
  "check_no_runtime_mock.go"
  "check_river_bypass.go"
  "check_river_job_args.go"
  "check_semaphore_usage.go"
  "check_transaction_boundary.go"
  # Batch 2 (ADR-0039 migration, 2026-03-03):
  "check_kubevirt_ssa_compliance.go"
  "check_k8s_in_transaction.go"
)


violations=0

for script in "${BATCH1_SCRIPTS[@]}"; do
  if rg -n --hidden --glob "${TARGETS[0]}" --glob "${TARGETS[1]}" --glob "${TARGETS[2]}" \
    "go run .*${script}|${script}" . >/tmp/legacy-batch1-match.txt; then
    while IFS= read -r line; do
      # Allow mentions in this guard script itself.
      if [[ "$line" == *"check_no_legacy_batch1_invocations.sh"* ]]; then
        continue
      fi
      echo "ERROR (ADR-0039): Legacy Batch1 script reference found: $line"
      violations=$((violations + 1))
    done </tmp/legacy-batch1-match.txt
  fi
done

rm -f /tmp/legacy-batch1-match.txt

if [ "$violations" -gt 0 ]; then
  echo ""
  echo "FAIL: found $violations legacy Batch1 script references in CI entrypoints."
  echo "Use shepherd-arch via golangci-lint module plugin instead."
  exit 1
fi

echo "OK (ADR-0039): no legacy Batch1 script invocations in CI entrypoints"
