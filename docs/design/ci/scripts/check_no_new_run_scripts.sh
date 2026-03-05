#!/usr/bin/env bash
# check_no_new_run_scripts.sh — ADR-0039 §6 CI Gate
#
# Prevents new .go scripts from being added to docs/design/ci/scripts/.
# All new Go-based CI gates MUST be written as go/analysis.Analyzer entries
# in tools/shepherd-linter/ instead.
#
# Usage:
#   bash docs/design/ci/scripts/check_no_new_run_scripts.sh
#
# This script checks git for any newly staged .go files in the CI scripts
# directory. It allows files that existed before ADR-0039 acceptance.

set -euo pipefail

SCRIPTS_DIR="docs/design/ci/scripts"

# Known .go scripts that pre-date ADR-0039 and are grandfathered in.
# This list was generated from the repository state at 2026-03-02 (ADR-0039 creation date).
KNOWN_SCRIPTS=(
  "check_auth_provider_plugin_boundary.go"
  "check_changed_code_has_tests.sh"
  "check_critical_test_presence.go"
  "check_dead_tests.go"
  "check_doc_claims_consistency.go"
  "check_duplicate_guard_scope.go"
  "check_ent_codegen.go"
  "check_environment_isolation_enforcement.go"
  "check_forbidden_imports.go"
  "check_frontend_no_non_english_literals.go"
  "check_frontend_no_placeholder_pages.go"
  "check_frontend_openapi_usage.go"
  "check_frontend_route_shell_architecture.go"
  "check_handler_explicit_rbac_guards.go"
  "check_k8s_in_transaction.go"
  "check_kubevirt_ssa_compliance.go"
  "check_markdown_links.go"
  "check_master_flow_api_alignment.go"
  "check_master_flow_completion_readiness.go"
  "check_master_flow_test_matrix.go"
  "check_master_flow_traceability.go"
  "check_module_noop_hooks.go"
  "check_naked_goroutine.go"
  "check_no_global_platform_admin_gate.go"
  "check_no_gorm_import.go"
  "check_no_outbox_import.go"
  "check_no_runtime_mock.go"
  "check_no_runtime_placeholders.go"
  "check_no_sqlite_in_tests.go"
  "check_openapi_critical_contract.go"
  "check_openapi_critical_fingerprint.go"
  "check_provider_wiring.go"
  "check_repository_tests.go"
  "check_river_bypass.go"
  "check_river_job_args.go"
  "check_semaphore_usage.go"
  "check_stage3_admin_catalog_baseline.go"
  "check_stage4_system_service_baseline.go"
  "check_stage5c_behavior_tests.go"
  "check_stage5d_delete_baseline.go"
  "check_stage5e_batch_baseline.go"
  "check_stage6_vnc_baseline.go"
  "check_test_assertions.go"
  "check_transaction_boundary.go"
  "check_validate_spec.go"
  "check_vm_create_spec_completeness.go"
  "check_vm_create_status_progression.go"
)

if [ ! -d "$SCRIPTS_DIR" ]; then
  echo "OK: $SCRIPTS_DIR does not exist"
  exit 0
fi

# Build a set of known scripts for fast lookup
declare -A known_set
for s in "${KNOWN_SCRIPTS[@]}"; do
  known_set["$s"]=1
done

violations=0

for file in "$SCRIPTS_DIR"/*.go; do
  [ -f "$file" ] || continue
  basename=$(basename "$file")
  if [ -z "${known_set[$basename]+_}" ]; then
    echo "ERROR (ADR-0039): New .go script detected in $SCRIPTS_DIR: $basename"
    echo "  → New Go-based CI gates MUST be written as go/analysis.Analyzer"
    echo "    entries in tools/shepherd-linter/ (see ADR-0039 §6)."
    violations=$((violations + 1))
  fi
done

if [ "$violations" -gt 0 ]; then
  echo ""
  echo "FAIL: $violations new .go script(s) found in $SCRIPTS_DIR."
  echo "Move them to tools/shepherd-linter/analyzer/<name>/ as go/analysis Analyzers."
  exit 1
fi

echo "OK (ADR-0039): No new .go scripts in $SCRIPTS_DIR"
