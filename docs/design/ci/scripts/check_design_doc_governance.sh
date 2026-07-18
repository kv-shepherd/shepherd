#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

legacy_refs_file="$(mktemp)"
trap 'rm -f "${legacy_refs_file}"' EXIT

fail() {
  echo "[design-doc-governance] ERROR: $1" >&2
  exit 1
}

check_file_exists() {
  local path="$1"
  [[ -f "$path" ]] || fail "Required file missing: $path"
}

# Required docs paths (ADR-0030 layering)
check_file_exists "docs/design/frontend/README.md"
check_file_exists "docs/design/frontend/FRONTEND.md"
check_file_exists "docs/design/frontend/architecture/README.md"
check_file_exists "docs/design/frontend/features/batch-operations-queue.md"
check_file_exists "docs/design/frontend/contracts/README.md"
check_file_exists "docs/design/frontend/testing/README.md"
check_file_exists "docs/design/database/README.md"
check_file_exists "docs/design/database/schema-catalog.md"
check_file_exists "docs/design/database/lifecycle-retention.md"
check_file_exists "docs/design/database/transactions-consistency.md"
check_file_exists "docs/design/database/migrations.md"
check_file_exists "docs/design/observability/README.md"
check_file_exists "docs/design/observability/metrics-baseline.md"
check_file_exists "docs/design/observability/river-queue-metrics-baseline.md"
check_file_exists "docs/design/observability/request-correlation-logging-baseline.md"
check_file_exists "docs/design/observability/river-worker-correlation-logging-baseline.md"
check_file_exists "docs/design/observability/alerts-baseline.md"
check_file_exists "docs/design/observability/alert-runbook-link-baseline.md"
check_file_exists "docs/design/observability/dashboards-baseline.md"
check_file_exists "docs/design/observability/grafana-dashboard-promql-baseline.md"
check_file_exists "docs/design/observability/tracing-baseline.md"
check_file_exists "docs/design/observability/recording-rules-baseline.md"
check_file_exists "docs/design/observability/rule-tests-baseline.md"
check_file_exists "docs/design/observability/prometheus-config-validation-baseline.md"
check_file_exists "docs/design/observability/prometheus-operator-baseline.md"
check_file_exists "docs/design/observability/prometheus-operator-rule-parity-baseline.md"
check_file_exists "docs/design/observability/compose-monitoring-baseline.md"
check_file_exists "docs/design/ci/live-e2e-evidence-baseline.md"
check_file_exists "docs/design/ci/GATE_HARDENING_CHECKLIST.md"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-full.passed.json"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-full.failed-early.json"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-preflight.passed.json"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-secret.invalid.json"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-flaky.invalid.json"
check_file_exists "docs/design/ci/fixtures/live-e2e-evidence-cluster-probe.invalid.json"
check_file_exists "docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh"
check_file_exists "docs/design/ci/scripts/find_latest_live_e2e_full_evidence.mjs"
check_file_exists "docs/operations/README.md"
check_file_exists "docs/operations/production-deployment.md"
check_file_exists "docs/operations/live-e2e-validation.md"
check_file_exists "docs/adr/ADR-0054-minimal-prometheus-observability-baseline.md"
check_file_exists "docs/adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md"
check_file_exists "docs/adr/ADR-0056-observability-deployment-packaging-baseline.md"
check_file_exists "docs/adr/ADR-0057-opentelemetry-and-correlation-logging-baseline.md"
check_file_exists "docs/adr/ADR-0058-live-e2e-evidence-bundle-baseline.md"
check_file_exists "deploy/prod/docker-compose.monitoring.yml"
check_file_exists "deploy/monitoring/prometheus/prometheus.yml"
check_file_exists "deploy/monitoring/prometheus/shepherd-recording-rules.yml"
check_file_exists "deploy/monitoring/prometheus/shepherd-alerts.yml"
check_file_exists "deploy/monitoring/prometheus/shepherd-rules.test.yml"
check_file_exists "docs/design/ci/scripts/check_prometheus_config.sh"
check_file_exists "docs/design/ci/scripts/check_prometheus_alert_runbooks.sh"
check_file_exists "docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh"
check_file_exists "docs/design/ci/scripts/check_grafana_dashboard_promql.sh"
check_file_exists "deploy/monitoring/prometheus-operator/shepherd-service-monitor.yml"
check_file_exists "deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml"
check_file_exists "deploy/monitoring/grafana/dashboards/shepherd-overview.json"
check_file_exists "deploy/monitoring/grafana/provisioning/datasources/prometheus.yml"
check_file_exists "deploy/monitoring/grafana/provisioning/dashboards/shepherd.yml"

# Traceability manifest (ADR-0032)
check_file_exists "docs/design/traceability/master-flow.json"

# Retired path must not be used as markdown link target in design/i18n/adr docs.
if rg -n "\]\((docs/design/FRONTEND\.md|\.\./FRONTEND\.md|\./FRONTEND\.md|\.\./design/FRONTEND\.md|\.\./\.\./\.\./\.\./design/FRONTEND\.md)\)" docs/design docs/i18n docs/adr \
  --glob '!docs/design/frontend/**' \
  --glob '!docs/adr/ADR-0030-*.md' >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Found legacy FRONTEND.md path references"
fi

# Canonical link checks
rg -q "\[frontend/README\.md\]\(\./frontend/README\.md\)" docs/design/README.md \
  || fail "docs/design/README.md must link to ./frontend/README.md"

rg -q "\[frontend/FRONTEND\.md\]\(\./frontend/FRONTEND\.md\)" docs/design/README.md \
  || fail "docs/design/README.md must link to ./frontend/FRONTEND.md"

rg -q "\[database/README\.md\]\(\./database/README\.md\)" docs/design/README.md \
  || fail "docs/design/README.md must link to ./database/README.md"

rg -q "\[Operations Guides\]\(\.\./operations/README\.md\)" docs/design/README.md \
  || fail "docs/design/README.md must link to ../operations/README.md"

rg -q "\.\./frontend/FRONTEND\.md" docs/design/interaction-flows/master-flow.md \
  || fail "master-flow.md must reference ../frontend/FRONTEND.md"

rg -q "\.\./database/lifecycle-retention\.md" docs/design/interaction-flows/master-flow.md \
  || fail "master-flow.md must reference ../database/lifecycle-retention.md"

rg -q "\.\./database/README\.md" docs/design/interaction-flows/README.md \
  || fail "interaction-flows/README.md must reference ../database/README.md"

# Dependency and toolchain versions must stay centralized in DEPENDENCIES.md.
# These high-level current-state docs are frequently used as implementation
# summaries, so they should link to DEPENDENCIES.md instead of repeating pins.
version_pin_pattern='React [0-9]|Next\.js [0-9]|Go `?[0-9]+\.[0-9]+|PostgreSQL [0-9]+|postgres/[0-9]+|client-go`? `?v[0-9]+|k8s\.io/\*`? `?v[0-9]+|OpenAPI `?[0-9]+\.[0-9]+|Vitest [0-9]|Playwright [0-9]|MSW [0-9]|TypeScript [0-9]|Ant Design [0-9]|Zustand [0-9]|TanStack Query [0-9]|Tailwind CSS [0-9]|Zod [0-9]|react-i18next [0-9]'
if rg -n "${version_pin_pattern}" \
  docs/design/README.md \
  docs/design/CURRENT_STATE.md \
  docs/design/frontend/FRONTEND.md \
  docs/design/phases/05-auth-api-frontend.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current-state design docs must link to DEPENDENCIES.md instead of hardcoding dependency/toolchain versions"
fi

# Checklist governance statement
rg -q "Global Single Standard" docs/design/checklist/README.md \
  || fail "checklist/README.md must declare CHECKLIST.md as global single standard"

if rg -n "dedicated review UI" docs/design/DEFERRED_FOLLOWUPS.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "DEFERRED_FOLLOWUPS.md must not describe the pending-adoption review UI as deferred after /admin/pending-adoptions shipped"
fi

if rg -n 'pending_adoptions.*schema only|schema only.*pending_adoptions' docs/design \
  --glob '!docs/design/ci/scripts/check_design_doc_governance.sh' >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Design docs must not describe pending_adoptions as schema-only after discovery/API/UI adoption workflow shipped"
fi

rg -q "VM adoption discovery scan" docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must include the VM adoption discovery scan worker in the runtime snapshot"

rg -q "Resource adoption" docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must include the shipped resource adoption capability"

rg -q 'pending_adoptions' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must document pending_adoptions persistence as part of resource adoption"

rg -q '/admin/pending-adoptions' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must document the admin pending-adoptions review UI"

if rg -n 'P3 — prod overcommit warning UX' docs/design/CHECKLIST.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "CHECKLIST.md must not list prod overcommit warning UX as an open P3 item after the approval UI warning shipped"
fi

rg -q 'P3-done — prod overcommit warning UX is surfaced in the approval UI' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md must mark the prod overcommit warning UX as shipped in the master-flow status table"

if rg -n 'P3 — provider hard idempotency' docs/design/CHECKLIST.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "CHECKLIST.md must not list provider hard idempotency as open after the pre-SSA create ownership guard shipped"
fi

rg -q 'P3-done — provider create ownership guard rejects unowned same-name VMs before SSA apply' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md must mark provider-side VM create idempotency as complete in the master-flow status table"

rg -q 'Provider-side hard idempotency now guards create-style SSA with a same-name ownership check before apply' docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4 checklist must document the provider-side VM create ownership guard"

if rg -n 'P2 — API \+ child execution dispatch \+ two-layer throttling' docs/design/CHECKLIST.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "CHECKLIST.md must not list the completed Stage 5.E batch baseline as an open P2 item"
fi

rg -q 'P2-done — API \+ child execution dispatch \+ two-layer throttling' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md must mark the Stage 5.E batch baseline as complete in the master-flow status table"

if rg -n 'P3 — V1 inbox flow complete' docs/design/CHECKLIST.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "CHECKLIST.md must not list the completed Stage 5.F V1 inbox notification baseline as an open P3 item"
fi

rg -q 'P3-done — V1 inbox flow complete' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md must mark the Stage 5.F V1 inbox notification baseline as complete in the master-flow status table"

rg -F -q '| 5.B | Admin Approval | ✅ 95% | Prod overcommit informational warning already surfaced in approval UI; template lifecycle follow-ups remain deferred | P3-done |' docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4 checklist must mark the shipped Stage 5.B admin approval baseline as P3-done"

rg -F -q '| 5.F | Notification System | ✅ 95% | V1 inbox notification flow implemented end-to-end (API + triggers + InboxSender + NotificationBell + 90-day retention cleanup) | P3-done |' docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4 checklist must mark the shipped Stage 5.F notification baseline as P3-done"

if rg -n 'P2 — noVNC proxy internals|proxy internals \+ (VNC )?active revocation remain deferred \| P2 \|' \
  docs/design/CHECKLIST.md docs/design/checklist/phase-4-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Stage 6 noVNC proxy internals and VNC active revocation are V2+ scope, not current open P2 checklist debt"
fi

rg -F -q 'V1-baseline done — request/status/open API, approval guard, audit, encrypted single-use bootstrap credential, shared replay marker, and frontend approved-target validation are shipped; noVNC proxy internals and VNC active revocation remain V2+' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md must mark the shipped Stage 6 V1 console baseline while keeping noVNC internals and VNC active revocation as V2+"

rg -F -q '| 6 | VNC Console Access | ⚠️ 96% | Stage 6 V1 baseline + shared PG replay marker + AES-256-GCM encrypted token envelope implemented; noVNC proxy internals + VNC active revocation remain V2+ scope | V2+ |' docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4 checklist must keep Stage 6 proxy internals and VNC active revocation in V2+ scope"

if rg -n 'Active JWT session revocation and session listing|active revocation is RFC-0008 future scope|active revocation deferred to|active session revocation remains RFC-0008|active JWT[[:space:]]+revocation' \
  docs/design/DEFERRED_FOLLOWUPS.md \
  docs/design/CURRENT_STATE.md \
  docs/design/checklist/phase-0-checklist.md \
  docs/design/phases/04-governance.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "JWT session-version revocation is implemented; docs must only defer session listing/admin session-management APIs"
fi

rg -F -q 'Session listing and administrative session-management API | RFC / V2 scope | Session-version JWT revocation is implemented' docs/design/DEFERRED_FOLLOWUPS.md \
  || fail "DEFERRED_FOLLOWUPS.md must defer session listing/admin APIs without re-deferring implemented JWT revocation"

rg -F -q 'Auth token/session semantics | Complete via JWT + DB-bootstrapped secrets + session-version revocation; session listing/admin APIs remain' docs/design/checklist/phase-0-checklist.md \
  || fail "phase-0 checklist must document implemented session-version revocation and defer only session listing/admin APIs"

rg -F -q '| §22 Authentication | ✅ Done | Section 8 (this doc) | Local/JWT, session-version revocation, external provider runtime, JIT provisioning, directory sync, and cohort mapping implemented; session listing/admin APIs remain RFC-0008 |' docs/design/phases/04-governance.md \
  || fail "04-governance.md must document the implemented session-version revocation baseline"

if rg -n 'remove domain[^[:alnum:]]+`?PENDING`?|remove domain-level[^[:alnum:]]+`?PENDING`?|`?PENDING`? status should not exist at VM domain level|Domain `PENDING` status is redundant' \
  docs/design \
  --glob '!docs/design/ci/scripts/check_design_doc_governance.sh' >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Design docs must not ask to remove VM PENDING; it is the K8s/KubeVirt scheduler-wait state, not pre-approval request state"
fi

workflow_storage_docs=(
  docs/design/database/vm-lifecycle-write-model.md
  docs/design/checklist/phase-4-checklist.md
  docs/design/phases/04-governance.md
  docs/design/interaction-flows/master-flow.md
  docs/i18n/zh-CN/design/interaction-flows/master-flow.md
)
if rg -n 'approval_tickets' "${workflow_storage_docs[@]}" >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current workflow docs must use the physical tickets table, not the retired approval_tickets name"
fi
current_ticket_docs=(
  docs/design/checklist/phase-1-checklist.md
  docs/design/checklist/phase-3-checklist.md
  docs/design/checklist/phase-4-checklist.md
  docs/design/ci/README.md
  docs/design/examples/README.md
  docs/design/examples/domain/event.go
  docs/design/examples/usecase/create_vm.go
  docs/design/phases/01-contracts.md
  docs/design/phases/03-service-layer.md
  docs/design/phases/04-governance.md
  docs/design/interaction-flows/master-flow.md
  docs/i18n/zh-CN/design/interaction-flows/master-flow.md
)
if rg -n 'ApprovalTicket|ent/schema/approval_ticket\.go' "${current_ticket_docs[@]}" >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current workflow docs must use the Ticket entity and ent/schema/ticket.go; accepted historical ADR/RFC text is intentionally outside this gate"
fi
if rg -n 'CreateApprovalTicket|GetApprovalTicket|UpdateApprovalTicket' \
  docs/design/examples/usecase/create_vm.go docs/design/phases/03-service-layer.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current sqlc documentation/examples must use generated Ticket methods such as InsertTicket and ApproveCreateTicket"
fi
if rg -n 'ticket\.(Payload|Namespace)|Ticket\.(Payload|Namespace)' "${current_ticket_docs[@]}" >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current workflow docs must read immutable request fields from DomainEvent.payload, not the Ticket entity"
fi
if rg -n 'CREATE/DELETE|`CREATE`/`DELETE`|enum \(`CREATE`, `DELETE`\)' \
  docs/design/checklist/phase-4-checklist.md docs/design/phases/04-governance.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current phase-4 docs must not collapse Ticket.operation_type to the retired CREATE/DELETE subset"
fi
for doc in docs/design/checklist/phase-4-checklist.md docs/design/phases/01-contracts.md docs/design/phases/04-governance.md; do
  rg -F -q '`CREATE`, `MODIFY`, `DELETE`, `POWER`, `VNC_ACCESS`' "$doc" \
    || fail "$doc must document the complete Ticket.operation_type enum"
done
rg -F -q 'Values("CREATE", "MODIFY", "DELETE", "POWER", "VNC_ACCESS")' ent/schema/ticket.go \
  || fail "ent/schema/ticket.go must retain the complete Ticket.operation_type enum"
if rg -n 'selected_template_version' \
  internal/repository/sqlc/schema.sql \
  internal/repository/sqlc/models.go \
  ent/schema/ticket.go \
  "${current_ticket_docs[@]}" >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Current Ticket schemas/docs must not expose the removed selected_template_version field"
fi
if rg -n 'template_version' ent/schema/ticket.go >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Ticket schema comments must describe the implemented snapshot fields, not a removed template_version field"
fi
rg -F -q '| Approval template-version decision | Accepted ADR-0017' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must record the accepted ADR-0017 approval template-version contract debt"
rg -F -q 'persists an effective `template_snapshot` but exposes no independent selected-version input or field' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must distinguish snapshot-only implementation from the accepted selected-version contract"
rg -F -q 'accept a superseding ADR/amendment before treating snapshot-only behavior as aligned' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must require a superseding accepted decision for snapshot-only behavior"
rg -F -q 'approval template-version mismatches above are accepted-ADR contract debt' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must not imply that batch limits are the only accepted-ADR contract debt"
rg -F -q '**Known accepted-ADR contract debt:** ADR-0017 and ADR-0015 require the' docs/design/phases/04-governance.md \
  || fail "04-governance.md must disclose the approval template-version accepted-ADR debt"
rg -F -q 'identifies two accepted-ADR contract debts:' docs/design/README.md \
  || fail "design README must report both accepted-ADR contract debts"
rg -F -q 'template version (`selected_template_version`), while the current approval' docs/design/README.md \
  || fail "design README must include the approval template-version contract debt"
rg -F -q 'Each debt requires a dedicated issue and a new accepted ADR/amendment' docs/design/README.md \
  || fail "design README must require an issue and accepted amendment for each contract debt"
if rg -n 'PENDING_APPROVAL.*APPROVED|Ticket.*PENDING_APPROVAL' \
  docs/design/phases/04-governance.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Raw Ticket approval transitions must start at PENDING; PENDING_APPROVAL is only a public projection status"
fi
rg -F -q 'Insert `tickets` (`CREATE`, `PENDING`)' docs/design/database/vm-lifecycle-write-model.md \
  || fail "VM lifecycle write model must document the physical tickets/PENDING submission row"
rg -F -q 'are best-effort and are not part of the atomic Event/Ticket write set' docs/design/database/vm-lifecycle-write-model.md \
  || fail "VM lifecycle write model must keep audit/notification side effects outside the atomic Event/Ticket write set"
rg -F -q 'After commit: best-effort audit + approval routing + notification trigger' docs/design/interaction-flows/master-flow.md \
  || fail "English master flow must document submission audit/notification work as post-commit best-effort"
rg -F -q '提交后：best-effort 审计 + 审批路由 + 通知触发' docs/i18n/zh-CN/design/interaction-flows/master-flow.md \
  || fail "Chinese master flow must document submission audit/notification work as post-commit best-effort"

frontend_page_count="$(find web/src/app -type f -name 'page.tsx' | wc -l | tr -d '[:space:]')"
openapi_operation_count="$(rg -c '^[[:space:]]+operationId:' api/openapi.yaml | tr -d '[:space:]')"
ent_schema_count="$(find ent/schema -maxdepth 1 -type f -name '*.go' | wc -l | tr -d '[:space:]')"
page_count_docs=(
  docs/design/README.md
  docs/design/CURRENT_STATE.md
  docs/design/CHECKLIST.md
  docs/design/checklist/phase-5-checklist.md
  docs/design/phases/05-auth-api-frontend.md
)
for doc in "${page_count_docs[@]}"; do
  rg -q "${frontend_page_count} App Router .*page" "$doc" \
    || fail "$doc must reference the current frontend App Router page count (${frontend_page_count})"
done
if rg -n '[0-9]+ App Router .*page' "${page_count_docs[@]}" >"${legacy_refs_file}"; then
  stale_page_count_refs="$(awk -v count="${frontend_page_count}" '$0 !~ count " App Router" { print }' "${legacy_refs_file}")"
  if [[ -n "${stale_page_count_refs}" ]]; then
    printf '%s\n' "${stale_page_count_refs}" >&2
    fail "Found stale frontend App Router page count; expected ${frontend_page_count}"
  fi
fi

schema_count_docs=(
  docs/design/README.md
  docs/design/CURRENT_STATE.md
  docs/design/CHECKLIST.md
  docs/design/phases/01-contracts.md
)
for doc in "${schema_count_docs[@]}"; do
  rg -q "${ent_schema_count} (Ent schemas|schema files)" "$doc" \
    || fail "$doc must reference the current Ent schema count (${ent_schema_count})"
done
if rg -n '[0-9]+ (Ent schemas|schema files)' "${schema_count_docs[@]}" >"${legacy_refs_file}"; then
  stale_schema_refs="$(awk -v count="${ent_schema_count}" '$0 !~ count " (Ent schemas|schema files)" { print }' "${legacy_refs_file}")"
  if [[ -n "${stale_schema_refs}" ]]; then
    printf '%s\n' "${stale_schema_refs}" >&2
    fail "Found stale Ent schema count; expected ${ent_schema_count}"
  fi
fi

operation_count_docs=(
  docs/design/README.md
  docs/design/CURRENT_STATE.md
  docs/design/CHECKLIST.md
  docs/design/checklist/phase-1-checklist.md
  docs/design/checklist/phase-5-checklist.md
  docs/design/phases/01-contracts.md
  docs/design/phases/04-governance.md
  docs/design/phases/05-auth-api-frontend.md
)
for doc in "${operation_count_docs[@]}"; do
  if [[ "$doc" == "docs/design/checklist/phase-5-checklist.md" ]]; then
    rg -q "${openapi_operation_count} .*endpoints|${openapi_operation_count} .*operationId" "$doc" \
      || fail "$doc must reference the current OpenAPI operation count (${openapi_operation_count})"
    continue
  fi
  rg -q "${openapi_operation_count} .*operationId|${openapi_operation_count} .*endpoints" "$doc" \
    || fail "$doc must reference the current OpenAPI operation count (${openapi_operation_count})"
done
if rg -n '[0-9]+[[:space:]]+`?(operationIds?|endpoints)' "${operation_count_docs[@]}" >"${legacy_refs_file}"; then
  stale_operation_refs="$(awk -v count="${openapi_operation_count}" '$0 !~ count "[[:space:]]+`?(operationId|operationIds|endpoints)" { print }' "${legacy_refs_file}")"
  if [[ -n "${stale_operation_refs}" ]]; then
    printf '%s\n' "${stale_operation_refs}" >&2
    fail "Found stale OpenAPI operation count; expected ${openapi_operation_count}"
  fi
fi

if rg -n '\[ \] If 3\.1-only features are used|\[ \] CI blocks merges unless `make api-check` passes|\[ \] ADR-0029 Compliance' \
  docs/design/checklist/phase-1-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-1 checklist must not list shipped API contract-first CI gates as unchecked work"
fi

[[ -s api/openapi.compat.yaml ]] \
  || fail "api/openapi.compat.yaml must exist as the generated OpenAPI 3.0 compatibility artifact"

rg -F -q 'REQUIRE_OPENAPI_COMPAT=1 bash ./docs/design/ci/scripts/api-check.sh' Makefile \
  || fail "Makefile ci-api-generated-sync-check must enforce REQUIRE_OPENAPI_COMPAT=1 api-check"

rg -F -q 'run: make ci-api-lint' .github/workflows/api-contract-validation.yml \
  || fail "API contract workflow must run the API lint gate"

rg -F -q 'run: make ci-api-generated-sync' .github/workflows/api-contract-validation.yml \
  || fail "API contract workflow must run the generated-sync gate"

rg -F -q 'run: make ci-api-contract' .github/workflows/api-contract-validation.yml \
  || fail "API contract workflow must run the libopenapi contract test gate"

rg -F -q 'validatorconfig.WithStrictMode()' internal/api/middleware/openapi_validator.go \
  || fail "OpenAPI runtime validator must use libopenapi-validator StrictMode"

rg -F -q 'github.com/pb33f/libopenapi-validator' go.mod \
  || fail "go.mod must retain libopenapi-validator for ADR-0029 runtime contract validation"

if rg -n '\[ \] \*\*Transaction Management\*\* per ADR-0012|\[ \] \*\*Test Infrastructure\*\* \(PostgreSQL via testcontainers-go\)|\[ \] \*\*PostgreSQL Test Infrastructure\*\*|\[ \] `go\.mod` uses vanity import path|\[ \] All internal imports use vanity path' \
  docs/design/checklist/phase-1-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-1 checklist must not list shipped transaction/test-infrastructure/vanity-path baselines as unchecked work"
fi

rg -F -q 'module kv-shepherd.io/shepherd' go.mod \
  || fail "go.mod must use the kv-shepherd.io/shepherd vanity module path"

if rg -n '"(github\.com|gitlab\.com|localhost|kubevirt-shepherd|shepherd-platform)/[^" ]*/internal|"kubevirt-shepherd/internal|"shepherd/internal' \
  --glob '*.go' . >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "Go source must not use legacy non-vanity internal import paths"
fi

rg -F -q 'testutil.MustStartDockerPG(m)' internal/api/handlers/testmain_test.go \
  || fail "API handler DB-backed tests must use shared PostgreSQL test infrastructure"

rg -F -q 'testutil.MustStartDockerPG(m)' internal/jobs/jobs_test.go \
  || fail "job DB-backed tests must use shared PostgreSQL test infrastructure"

rg -F -q 'func OpenEntPostgres(' internal/testutil/postgres_ent.go \
  || fail "shared PostgreSQL test helper must expose OpenEntPostgres"

rg -F -q 'func MustStartDockerPG(' internal/testutil/docker_pg.go \
  || fail "shared PostgreSQL test helper must expose MustStartDockerPG"

rg -F -q 'check_sqlc_usage.sh' Makefile \
  || fail "ci-governance must run the ADR-0012 sqlc usage gate"

rg -F -q 'check_validate_spec.go' Makefile \
  || fail "ci-governance must run the ValidateSpec transaction gate"

rg -F -q '"kv-shepherd.io/shepherd-linter/analyzer/txboundary"' tools/shepherd-linter/plugin.go \
  || fail "shepherd-linter must wire txboundary analyzer for ADR-0012 transaction boundaries"

rg -F -q '"kv-shepherd.io/shepherd-linter/analyzer/k8sintransaction"' tools/shepherd-linter/plugin.go \
  || fail "shepherd-linter must wire k8sintransaction analyzer for ADR-0012 K8s transaction boundaries"

if rg -n '\[ \] \*\*Schema Definition Standards\*\* followed|remaining work is Ent schema/transaction/test standards|Ent schema/transaction/test standards and deferred V2 schemas remain' \
  docs/design/CHECKLIST.md docs/design/checklist/phase-1-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-1 docs must not list shipped Ent schema definition standards as unchecked work"
fi

if rg -n '\[ \] Business logic does not contain excessive `if ptr != nil` checks for optional fields|remaining current-scope work is optional-field pointer-use audit|ADR-0028 generated-type gate' \
  docs/design/CHECKLIST.md docs/design/checklist/phase-1-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-1 docs must not list the ADR-0028 optional pointer audit as unchecked work after critical contract enforcement shipped"
fi

rg -F -q 'allowedGeneratedOptionalPointerFields' docs/design/ci/scripts/check_openapi_critical_contract.go \
  || fail "OpenAPI critical contract must freeze the generated optional pointer exception set"

if rg -n '\| Phase 1 \| 🔄 Partial \(~98%\)' docs/design/CHECKLIST.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "CHECKLIST.md progress tracking must use the reconciled Phase 1 ~99% status"
fi

rg -F -q '| Phase 1 | 🔄 Partial (~99%) | - | 33 Ent schemas + Ent schema standards/codegen/dynamic-query safety gates + Go/TS API types + ADR-0028 generated-type/pointer-audit gates + frontend testing toolchain + cluster credential runtime + provider plugin config boundary done; remaining work is deferred V2 schemas/deployment items |' docs/design/CHECKLIST.md \
  || fail "CHECKLIST.md progress tracking must reflect the current Phase 1 pointer-audit and standards status"

: >"${legacy_refs_file}"
while IFS= read -r schema_file; do
  schema_base="$(basename "${schema_file}")"
  [[ "${schema_base}" == "mixin.go" ]] && continue

  if ! rg -q 'field\.String\("id"\)' "${schema_file}"; then
    echo "${schema_file}: missing field.String(\"id\")" >>"${legacy_refs_file}"
  elif ! awk '/field\.String\("id"\)/{seen=1} seen&&/Unique\(\)/{unique=1} seen&&/Immutable\(\)/{immutable=1} END{exit !(seen&&unique&&immutable)}' "${schema_file}"; then
    echo "${schema_file}: id field must be unique and immutable" >>"${legacy_refs_file}"
  fi

  if ! rg -q 'func \([^)]+\) Mixin\(\) \[\]ent\.Mixin' "${schema_file}"; then
    echo "${schema_file}: missing Mixin() []ent.Mixin" >>"${legacy_refs_file}"
  elif ! rg -q 'TimeMixin\{\}|AuditMixin\{\}' "${schema_file}"; then
    echo "${schema_file}: concrete schemas must use TimeMixin{} or AuditMixin{}" >>"${legacy_refs_file}"
  fi

  if rg -n 'field\.Time\("(created_at|updated_at)"\)' "${schema_file}" >>"${legacy_refs_file}"; then
    :
  fi
done < <(find ent/schema -maxdepth 1 -type f -name '*.go' -print | sort)

if [[ -s "${legacy_refs_file}" ]]; then
  cat "${legacy_refs_file}" >&2
  fail "Ent schema definition standards check failed"
fi

if rg -n '^- \[ \] `CreateVMSnapshot` create snapshot$|^- \[ \] `GetVMSnapshot`, `ListVMSnapshots` query snapshots$|^- \[ \] `DeleteVMSnapshot` delete snapshot$|^- \[ \] `RestoreVMFromSnapshot` restore from snapshot$|^- \[ \] `CloneVM` clone from VM$|^- \[ \] Support cloning from snapshot$|^- \[ \] `GetVMClone`, `ListVMClones` status query$|^- \[ \] `MigrateVM` initiate migration$|^- \[ \] `GetVMMigration`, `ListVMMigrations` status query$|^- \[ \] `CancelVMMigration` cancel migration$' \
  docs/design/checklist/phase-2-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-2 checklist must mark snapshot/clone/migration provider methods as RFC-backed future scope, not unqualified current V1 work"
fi

rg -F -q 'VM snapshot, full VM clone, and live migration workflows | RFC-backed future scope' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must keep snapshot/clone/migration workflows in RFC-backed future scope"

if rg -n '^- \[ \] List-Watch pattern implemented \(deferred V2 acceleration\)$|^- \[ \] \*\*410 Gone Complete Handling\*\*:$|^[[:space:]]+- \[ \] Clear `resourceVersion` \(force full Re-list\)$|^[[:space:]]+- \[ \] Notify `CacheService` to invalidate cache$|^[[:space:]]+- \[ \] Don'\''t count toward circuit breaker$|^[[:space:]]+- \[ \] \*\*Read Request Degradation Strategy\*\* implemented$|^- \[ \] Exponential backoff reconnect \(with jitter\)$|^- \[ \] Circuit breaker configured$' \
  docs/design/checklist/phase-2-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-2 ResourceWatcher rows must be explicitly marked as RFC-backed deferred V2 acceleration"
fi

rg -F -q 'ResourceWatcher | - | Deferred | Optional V2 acceleration only; see [RFC-0020](../../rfc/RFC-0020-k8s-watch-acceleration.md), not V1 baseline (ADR-0038)' docs/design/phases/02-providers.md \
  || fail "Phase 2 provider spec must keep ResourceWatcher in deferred optional V2 scope"

rg -F -q 'Optional K8s watch acceleration / ResourceWatcher | RFC / V2 scope | [RFC-0020]' docs/design/DEFERRED_FOLLOWUPS.md \
  || fail "DEFERRED_FOLLOWUPS.md must keep ResourceWatcher in RFC/V2 scope"

rg -F -q 'ResourceWatcher` is the canonical VM status path | `internal/jobs/vm_status_sync.go` is the authoritative ResourceVersion-aware polling path.' docs/design/CURRENT_STATE.md \
  || fail "CURRENT_STATE.md must identify vm_status_sync as the authoritative status path instead of ResourceWatcher"

if rg -n '^- \[ \] Context timeout handling$|^- \[ \] KubeVirtProvider unit tests pass \(using Mock Client\)' \
  docs/design/checklist/phase-2-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-2 checklist must not list shipped provider timeout handling or provider unit tests as unchecked work"
fi

rg -F -q 'Context timeout handling — provider-owned K8s operations and cluster health probes use bounded `k8s.operation_timeout` contexts, enforced by `shepherd-arch/k8stimeout`' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must document provider K8s operation timeout enforcement"

rg -F -q 'KubeVirtProvider unit tests pass (using fake/mock client interfaces) — verified 2026-06-19 with `go test -count=1 ./internal/provider`' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must document the current provider unit-test lane"

rg -F -q 'return context.WithTimeout(ctx, p.operationTimeout)' internal/provider/kubevirt.go \
  || fail "KubeVirt provider operations must retain bounded operation-timeout contexts"

rg -F -q 'opCtx, cancel := context.WithTimeout(ctx, c.operationTimeout)' internal/provider/health_checker.go \
  || fail "cluster health checker must retain bounded operation-timeout probes"

rg -F -q '"kv-shepherd.io/shepherd-linter/analyzer/k8stimeout"' tools/shepherd-linter/plugin.go \
  || fail "shepherd-linter must wire k8stimeout analyzer for provider operation-timeout enforcement"

if rg -n '^- \[ \] \*\*Concurrency Control\*\* with queue-wait mechanism$|^- \[ \] i18n Standards verified$|^- \[ \] Database migration scripts \(Atlas — Phase 4\)$|^- \[ \] Cache service \(Ent local query, no Redis\)$' \
  docs/design/checklist/phase-2-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-2 General/Atlas rows must not reappear as unqualified current unchecked work"
fi

rg -F -q 'V1 uses River queue `MaxWorkers`; per-cluster queue-wait/semaphore remains deferred to [RFC-0015]' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must document the V1 River concurrency baseline and RFC-0015 queue-wait scope"

rg -F -q 'CacheService-based `CLUSTER_REBUILDING` UX is tracked in [DEFERRED_FOLLOWUPS.md]' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must keep CacheService rebuilding UX in deferred scope"

rg -F -q 'i18n Standards verified — frontend non-English literal and repository Chinese-character allowlist checks pass' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must document i18n verification evidence"

rg -F -q 'Database migration scripts (Atlas — Phase 4) — `migrations/atlas/atlas.hcl`, checked-in Atlas SQL, and startup migration tests are present' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist must document Atlas migration-script evidence"

rg -F -q 'Partial (~73%) — Basic VM CRUD + SSAApplier + VMRenderer + AuthProvider Admin + KubeVirt instance type/preference catalog reads + label-based pending adoption discovery/periodic scan + admin adoption management, bounded provider K8s operation timeouts, provider unit test lane, V1 River queue concurrency, i18n, and Atlas baselines done' docs/design/checklist/phase-2-checklist.md \
  || fail "phase-2 checklist status must include the reconciled General and Atlas baselines"

rg -F -q 'check_frontend_no_non_english_literals.go' Makefile \
  || fail "Makefile must retain frontend non-English literal i18n hygiene gate"

rg -F -q 'migrations/atlas/atlas.hcl' docs/design/DEPENDENCIES.md \
  || fail "DEPENDENCIES.md must document Atlas migration config"

rg -F -q 'func EnsureStartupMigrations(' internal/infrastructure/startup_migrations.go \
  || fail "startup migration path must retain EnsureStartupMigrations"

if rg -n '^- \[ \] Version number auto-increment$|^- \[ \] Supports diff calculation$|^- \[ \] YAML compressed storage$' \
  docs/design/checklist/phase-4-checklist.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "phase-4 RevisionService rich diff/compression items must be marked as RFC-backed future scope"
fi

rg -F -q 'Rich VM revision diff/compressed YAML service | RFC / V2 candidate' docs/design/DEFERRED_FOLLOWUPS.md \
  || fail "DEFERRED_FOLLOWUPS.md must keep rich VM revision diff/compressed YAML service in RFC/V2 scope"

rg -F -q 'full resource reconciler, rich revision diff/compression service, and template lifecycle states deferred' docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4 checklist status must mention deferred rich revision diff/compression scope"

# Batch parent-child alignment markers
rg -q "parent-child" docs/design/phases/04-governance.md \
  || fail "04-governance.md must describe parent-child batch model"

rg -q "two-layer rate limiting" docs/design/phases/04-governance.md \
  || fail "04-governance.md must describe two-layer rate limiting"

# Master-flow (product truth) alignment for phase/checklist/examples
rg -q "master-flow\\.md#stage-5e-batch-operations" docs/design/phases/04-governance.md \
  || fail "04-governance.md must reference master-flow Stage 5.E"

rg -q "master-flow\\.md#stage-5-d" docs/design/phases/04-governance.md \
  || fail "04-governance.md must reference master-flow Stage 5.D"

rg -q "master-flow\\.md#stage-6-vnc-console-access" docs/design/phases/04-governance.md \
  || fail "04-governance.md must reference master-flow Stage 6"

rg -q "adr-0015-vnc-v1-addendum" docs/adr/ADR-0015-governance-model-v2.md \
  || fail "ADR-0015 must include V1 VNC scope addendum anchor"

rg -q "ADR-0015.*18\\.1.*addendum" docs/design/phases/04-governance.md \
  || fail "04-governance.md must reference ADR-0015 \u00a718.1 addendum for V1 VNC scope"

rg -q "/api/v1/vms/\\{vm_id\\}/vnc" docs/design/interaction-flows/master-flow.md \
  || fail "master-flow.md must document canonical VNC websocket endpoint path"

if rg -n "/api/v1/vms/\\{vm_id\\}/vnc\\?token=" docs/design/interaction-flows/master-flow.md docs/i18n/zh-CN/design/interaction-flows/master-flow.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "VNC flow docs must not use URI query token transport"
fi

if rg -n "GET /vnc/\\{vm_id\\}\\?token=\\{vnc_jwt\\}" docs/design/interaction-flows/master-flow.md docs/i18n/zh-CN/design/interaction-flows/master-flow.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "VNC flow docs must not use legacy /vnc/{vm_id} endpoint path"
fi

rg -q "/api/v1/vms/\\{vm_id\\}/console/request" docs/design/phases/04-governance.md \
  || fail "04-governance.md must use canonical VNC endpoint placeholder {vm_id}"

rg -q "/api/v1/vms/\\{vm_id\\}/vnc" docs/design/phases/04-governance.md \
  || fail "04-governance.md must use canonical VNC websocket endpoint"

if rg -n "/api/v1/vms/\\{vm_id\\}/vnc\\?token=" docs/design/phases/04-governance.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "04-governance.md must not document VNC URI query token transport"
fi

if rg -n "tracked in Redis" docs/design/phases/04-governance.md >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "04-governance.md must not require Redis for VNC token tracking"
fi

rg -q "POST /api/v1/tickets/\\{id\\}/cancel" docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4-checklist.md must use API-prefixed cancellation endpoint"

rg -q "no active token revocation API" docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4-checklist.md must document V1 no-active-revocation scope"

rg -q 'StatusURL:.*"/api/v1/vms/batch/"' docs/design/examples/usecase/batch_approval.go \
  || fail "batch_approval example must return canonical status_url path"

if rg -n "VMStatusDeleted" docs/design/examples/domain/vm.go >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "docs/design/examples/domain/vm.go must not include persisted DELETED state"
fi

# Operations docs are production-facing and must keep deployment, monitoring,
# database, admin handoff, and live E2E validation wired together.
rg -q "\[production-deployment\.md\]\(\./production-deployment\.md\)" docs/operations/README.md \
  || fail "docs/operations/README.md must link to production-deployment.md"

rg -q "\[live-e2e-validation\.md\]\(\./live-e2e-validation\.md\)" docs/operations/README.md \
  || fail "docs/operations/README.md must link to live-e2e-validation.md"

rg -q "deploy-prod\.sh" docs/operations/production-deployment.md \
  || fail "production-deployment.md must document deploy-prod.sh"

rg -q "docker-compose\.monitoring\.yml" docs/operations/production-deployment.md \
  || fail "production-deployment.md must document the monitoring Compose overlay"

rg -q "database-operations\.md" docs/operations/production-deployment.md \
  || fail "production-deployment.md must reference database operations"

rg -q "platform-admin-sop\.md" docs/operations/production-deployment.md \
  || fail "production-deployment.md must reference platform admin SOP"

rg -q "live-e2e-validation\.md" docs/operations/production-deployment.md \
  || fail "production-deployment.md must reference live E2E validation"

rg -q "scripts/run_e2e_live\.sh" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document scripts/run_e2e_live.sh"

rg -q -- "--preflight-only" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document --preflight-only mode"

rg -q -- "--preflight-only" docs/operations/production-deployment.md \
  || fail "production-deployment.md must include live E2E preflight-only validation"

rg -q -- "--preflight-only" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must expose --preflight-only mode"

rg -q -- "--evidence-file" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must expose --evidence-file mode"

rg -q -- "--evidence-file" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document --evidence-file"

rg -q "live-e2e\.evidence\.json" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must emit live-e2e.evidence.json by default"

rg -q "live-e2e\.evidence\.json" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must define live-e2e.evidence.json"

rg -q "live-e2e\.evidence\.json" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must include live-e2e.evidence.json pass criteria"

rg -q "live E2E evidence manifest" docs/operations/production-deployment.md \
  || fail "production-deployment.md must require live E2E evidence manifest capture"

check_file_exists "docs/operations/production-deployment-evidence.example.json"
check_file_exists "scripts/check_production_evidence.sh"
check_file_exists "scripts/collect_production_evidence.sh"

rg -q "^ci-production-evidence-schema:" Makefile \
  || fail "Makefile must expose ci-production-evidence-schema"

rg -q "^production-evidence-collect:" Makefile \
  || fail "Makefile must expose production-evidence-collect"

rg -q "PRODUCTION_EVIDENCE_COLLECT_ARGS" Makefile \
  || fail "Makefile production-evidence-collect must allow passing collector arguments"

rg -q -- "--require-production-ready" scripts/collect_production_evidence.sh \
  || fail "production evidence collector must support strict production-ready validation"

rg -q "PRODUCTION_EVIDENCE_REQUIRE_READY" docs/operations/production-deployment.md \
  || fail "production-deployment.md must document strict production evidence collection"

rg -q "production-evidence-collect" docs/operations/production-deployment.md \
  || fail "production-deployment.md must document production evidence collection"

rg -q "ci-production-evidence-schema" docs/operations/production-deployment.md \
  || fail "production-deployment.md must document production evidence validation"

rg -q "production-deployment-evidence.example.json" docs/operations/README.md \
  || fail "operations README must link to the production deployment evidence manifest template"

rg -q "require-live-e2e-pass" scripts/check_production_evidence.sh \
  || fail "production evidence validator must support full live E2E pass validation"

rg -q -- "--self-test" scripts/check_production_evidence.sh \
  || fail "production evidence validator must expose --self-test"

rg -q "check_production_evidence\.sh --self-test" Makefile \
  || fail "ci-production-evidence-schema must run the production evidence validator self-test"

rg -q "check_live_e2e_evidence_manifest\.sh" scripts/check_production_evidence.sh \
  || fail "production evidence validator must delegate strict live E2E evidence validation to ADR-0058 validator"

rg -q '"live_e2e_full"' docs/operations/production-deployment-evidence.example.json \
  || fail "production deployment evidence example must include live_e2e_full check"

rg -q "PLAYWRIGHT_JSON_OUTPUT_FILE" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must configure Playwright JSON output"

rg -q "PLAYWRIGHT_JSON_OUTPUT_FILE" web/playwright.config.ts \
  || fail "playwright.config.ts must wire Playwright JSON output"

rg -q "PW_E2E_RUN_ID" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must pass a per-run Playwright E2E run id"

rg -q "check_live_e2e_evidence_manifest\.sh" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document the evidence manifest validation gate"

rg -q "check_live_e2e_evidence_manifest\.sh" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document the evidence manifest validation gate"

rg -q "check_live_e2e_evidence_manifest\.sh" docs/design/ci/README.md \
  || fail "ci README must document check_live_e2e_evidence_manifest.sh"

rg -q "find_latest_live_e2e_full_evidence\.mjs" docs/design/ci/README.md \
  || fail "ci README must document find_latest_live_e2e_full_evidence.mjs"

rg -q -- "--self-test" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document evidence checker self-test mode"

rg -q -- "--self-test" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must expose --self-test"

rg -q "live-e2e-evidence-full\.failed-early\.json" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate full early-failure fixture"

rg -q "live-e2e-evidence-flaky\.invalid\.json" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must self-test flaky negative fixture"

rg -q "live-e2e-evidence-cluster-probe\.invalid\.json" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must self-test cluster-probe negative fixture"

rg -q "live-e2e-evidence-cluster-probe-skipped\.invalid\.json" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must self-test skipped cluster-probe negative fixture"

rg -q "live-e2e-evidence-cluster-probe\.invalid\.json" docs/design/ci/README.md \
  || fail "ci README must document cluster-probe negative fixture"

rg -q "live-e2e-evidence-cluster-probe-skipped\.invalid\.json" docs/design/ci/README.md \
  || fail "ci README must document skipped cluster-probe negative fixture"

rg -q "api_server_reachable" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate live cluster API probe evidence"

rg -q "kubevirt_api_available" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate KubeVirt API discovery evidence"

rg -q "kubectl .*api-versions" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must record KubeVirt API discovery evidence"

rg -q "kubectl .*version --output=json" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must record Kubernetes API server probe evidence"

rg -q "for required_cmd in .*kubectl" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh readiness must require kubectl"

rg -q "E2E_PREFLIGHT_CLUSTER_PROBE:-1" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh must enable live cluster probes by default"

rg -q "clusterProbeSkipped" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh evidence manifest must record skipped cluster probe policy"

rg -q "auth whoami" scripts/run_e2e_live.sh \
  || fail "run_e2e_live.sh readiness must prove authenticated kubeconfig context"

rg -q "policy_gates\.cluster_probe" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document policy_gates.cluster_probe"

rg -q "policy_gates', 'cluster_probe" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate policy_gates.cluster_probe"

rg -q "cluster\.authenticated_context" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document authenticated cluster context evidence"

rg -q "cluster', 'authenticated_context" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate authenticated cluster context evidence"

rg -q "E2E_PREFLIGHT_CLUSTER_PROBE=0" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document cluster-probe debug override"

rg -q "kubectl" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document kubectl as a live E2E input"

rg -q "Failure Evidence" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document failure evidence semantics"

rg -q "result_file\.phase" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document failed-run phase evidence"

rg -q "requireOptionalResultZero\\(errors, manifestPath, manifest, 'flaky'\\)" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate runner flaky summary"

rg -q "requireOptionalStatsZero\\(errors, manifestPath, manifest, 'flaky'\\)" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate Playwright flaky stats"

rg -q "^ci-live-e2e-latest-evidence:" Makefile \
  || fail "Makefile must expose ci-live-e2e-latest-evidence"

rg -q "find_latest_live_e2e_full_evidence\.mjs" Makefile \
  || fail "ci-live-e2e-latest-evidence must select the latest mode=full manifest"

rg -q -- "--self-test" docs/design/ci/scripts/find_latest_live_e2e_full_evidence.mjs \
  || fail "find_latest_live_e2e_full_evidence.mjs must expose --self-test"

rg -q "GH-026" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track Prometheus monitoring gate hardening"

rg -q "GH-030" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track live E2E latest-full evidence hardening"

rg -q "GH-031" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track project completion readiness split"

rg -q "PROMTOOL_REQUIRED" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must document promtool fail-closed governance"

rg -q "find_latest_live_e2e_full_evidence\.mjs" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must document latest full live E2E evidence selector"

rg -q "make ci-monitoring-assets" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must document monitoring asset validation"

rg -q "GH-032" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track Prometheus config validation hardening"

rg -q "GH-033" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track Prometheus Operator rule parity hardening"

rg -q "GH-034" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track Prometheus alert runbook link hardening"

rg -q "GH-035" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track Grafana dashboard PromQL validation hardening"

rg -q "GH-036" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track River queue observability hardening"

rg -q "GH-037" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track HTTP request correlation logging hardening"

rg -q "GH-038" docs/design/ci/GATE_HARDENING_CHECKLIST.md \
  || fail "GATE_HARDENING_CHECKLIST.md must track River worker correlation logging hardening"

rg -q "ADR-0054" docs/design/observability/river-queue-metrics-baseline.md \
  || fail "river-queue-metrics-baseline.md must reference ADR-0054"

rg -q "shepherd_river_ready_jobs" docs/design/observability/river-queue-metrics-baseline.md \
  || fail "river-queue-metrics-baseline.md must document shepherd_river_ready_jobs"

rg -q "shepherd_river_queue_stats_scrape_success" docs/design/observability/river-queue-metrics-baseline.md \
  || fail "river-queue-metrics-baseline.md must document River queue stats scrape success"

rg -q "shepherd_river_jobs_by_state" internal/observability/river_queue_stats.go \
  || fail "river_queue_stats.go must expose shepherd_river_jobs_by_state"

rg -q "shepherd_river_queue_stats_scrape_success" internal/observability/river_queue_stats.go \
  || fail "river_queue_stats.go must expose shepherd_river_queue_stats_scrape_success"

rg -q "TestRiverQueueStatsCollectorEmitsQueueHealthMetrics" internal/observability/river_queue_stats_test.go \
  || fail "river_queue_stats_test.go must cover successful River queue metrics collection"

rg -q "TestRiverQueueStatsCollectorReportsScrapeFailure" internal/observability/river_queue_stats_test.go \
  || fail "river_queue_stats_test.go must cover River queue scrape failure"

rg -q "WithRiverQueueStats" internal/app/bootstrap.go \
  || fail "bootstrap.go must wire River queue metrics"

rg -q "river_metrics_enabled" config/config.yaml.example \
  || fail "config.yaml.example must expose observability.river_metrics_enabled"

rg -q "business_metrics_enabled" config/config.yaml.example \
  || fail "config.yaml.example must expose observability.business_metrics_enabled"

rg -q "shepherd_business_metrics_scrape_success" internal/observability/business_metrics.go \
  || fail "business_metrics.go must expose shepherd_business_metrics_scrape_success"

rg -q "OBSERVABILITY_RIVER_METRICS_ENABLED" deploy/prod/.env.prod.example \
  || fail ".env.prod.example must expose OBSERVABILITY_RIVER_METRICS_ENABLED"

rg -q "OBSERVABILITY_BUSINESS_METRICS_ENABLED" deploy/prod/.env.prod.example \
  || fail ".env.prod.example must expose OBSERVABILITY_BUSINESS_METRICS_ENABLED"

rg -q "OBSERVABILITY_RIVER_METRICS_ENABLED" deploy/prod/docker-compose.prod.yml \
  || fail "docker-compose.prod.yml must pass OBSERVABILITY_RIVER_METRICS_ENABLED"

rg -q "OBSERVABILITY_BUSINESS_METRICS_ENABLED" deploy/prod/docker-compose.prod.yml \
  || fail "docker-compose.prod.yml must pass OBSERVABILITY_BUSINESS_METRICS_ENABLED"

rg -q "ADR-0057" docs/design/observability/request-correlation-logging-baseline.md \
  || fail "request-correlation-logging-baseline.md must reference ADR-0057"

rg -q "X-Shepherd-Trace-ID" docs/design/observability/request-correlation-logging-baseline.md \
  || fail "request-correlation-logging-baseline.md must document X-Shepherd-Trace-ID"

rg -q "HTTPRequestLogMiddleware" internal/observability/http_request_log.go \
  || fail "http_request_log.go must expose HTTPRequestLogMiddleware"

rg -q "X-Shepherd-Trace-ID" internal/observability/http_request_log.go \
  || fail "http_request_log.go must define X-Shepherd-Trace-ID"

for field in request_id trace_id span_id method route status duration_ms; do
  rg -q "zap\\.[A-Za-z0-9]+\\(\"${field}\"" internal/observability/http_request_log.go \
    || fail "http_request_log.go must log bounded field ${field}"
done

if rg -n 'zap\.[A-Za-z0-9]+\("(path|raw_path|url|query|header|body|user|role|session|ticket|vm|namespace|cluster|system|service|provider|error)"' internal/observability/http_request_log.go; then
  fail "http_request_log.go must not log ADR-0057 forbidden fields"
fi

if rg -n '\.(RawPath|RawQuery)|RequestURI|Query\(\)|GetHeader\([^)]*(Authorization|Cookie|User|Ticket|VM|Namespace|Cluster)' internal/observability/http_request_log.go; then
  fail "http_request_log.go must not read raw URL query or sensitive headers for logging"
fi

rg -q "requireAllowedHTTPRequestLogFields" internal/observability/http_request_log_test.go \
  || fail "http_request_log_test.go must assert the ADR-0057 field allowlist"

rg -q "HTTPRequestLogMiddleware" internal/app/router.go \
  || fail "router.go must wire HTTPRequestLogMiddleware"

rg -q "TestHTTPRequestLogMiddlewareLogsCorrelationFields" internal/observability/http_request_log_test.go \
  || fail "http_request_log_test.go must cover correlation fields"

rg -q "TestHTTPRequestLogMiddlewareSkipsSuccessfulOperationalEndpoints" internal/observability/http_request_log_test.go \
  || fail "http_request_log_test.go must cover operational endpoint skip policy"

rg -q "TestHTTPRequestLogMiddlewareLogsFailedOperationalEndpoints" internal/observability/http_request_log_test.go \
  || fail "http_request_log_test.go must cover failed operational endpoints"

rg -q "TestNewRouterExposesTraceIDHeaderFromTracingMiddleware" internal/app/router_test.go \
  || fail "router_test.go must prove tracing middleware feeds X-Shepherd-Trace-ID through newRouter"

rg -q "ADR-0057" docs/design/observability/river-worker-correlation-logging-baseline.md \
  || fail "river-worker-correlation-logging-baseline.md must reference ADR-0057"

rg -q "river_job_completed" docs/design/observability/river-worker-correlation-logging-baseline.md \
  || fail "river-worker-correlation-logging-baseline.md must document river_job_completed"

rg -q "RiverWorkerLogMiddleware" internal/observability/river_worker_log.go \
  || fail "river_worker_log.go must expose RiverWorkerLogMiddleware"

rg -q "shepherd_trace" internal/observability/river_worker_log.go \
  || fail "river_worker_log.go must use Shepherd-owned trace metadata"

rg -q "NewRiverWorkerLogMiddleware" internal/infrastructure/database.go \
  || fail "database.go must wire River worker log middleware"

for field in job_id job_kind queue attempt max_attempts duration_ms result trace_id span_id error_type; do
  rg -q "zap\\.[A-Za-z0-9]+\\(\"${field}\"" internal/observability/river_worker_log.go \
    || fail "river_worker_log.go must log bounded field ${field}"
done

if rg -n 'zap\.[A-Za-z0-9]+\("(args|encoded_args|metadata|tags|payload|path|raw_path|url|query|header|body|user|role|session|ticket|vm|namespace|cluster|system|service|provider|error)"' internal/observability/river_worker_log.go; then
  fail "river_worker_log.go must not log ADR-0057 forbidden fields"
fi

if rg -n '\.(EncodedArgs|Tags)|zap\.Error\(' internal/observability/river_worker_log.go; then
  fail "river_worker_log.go must not read raw job args/tags or log raw errors"
fi

rg -q "requireAllowedRiverWorkerLogFields" internal/observability/river_worker_log_test.go \
  || fail "river_worker_log_test.go must assert the ADR-0057 field allowlist"

rg -q "TestRiverWorkerLogMiddlewareInjectsTraceMetadata" internal/observability/river_worker_log_test.go \
  || fail "river_worker_log_test.go must cover trace metadata injection"

rg -q "TestRiverWorkerLogMiddlewareLogsCompletionFields" internal/observability/river_worker_log_test.go \
  || fail "river_worker_log_test.go must cover completion fields"

rg -q "TestRiverWorkerLogMiddlewareClassifiesNonSuccessResults" internal/observability/river_worker_log_test.go \
  || fail "river_worker_log_test.go must cover non-success result classification"

rg -q "TestRiverWorkerLogMiddlewareDoesNotLogRawErrorOrArgs" internal/observability/river_worker_log_test.go \
  || fail "river_worker_log_test.go must cover raw error and args suppression"

rg -q "TestBuildRiverMiddlewareIncludesWorkerLogMiddleware" internal/infrastructure/database_test.go \
  || fail "database_test.go must prove River client middleware wiring"

rg -q "ADR-0056" docs/design/observability/prometheus-config-validation-baseline.md \
  || fail "prometheus-config-validation-baseline.md must reference ADR-0056"

rg -q "ADR-0056" docs/design/observability/prometheus-operator-rule-parity-baseline.md \
  || fail "prometheus-operator-rule-parity-baseline.md must reference ADR-0056"

rg -q "PrometheusRule\.spec\.groups" docs/design/observability/prometheus-operator-rule-parity-baseline.md \
  || fail "prometheus-operator-rule-parity-baseline.md must document PrometheusRule.spec.groups parity"

rg -q "ADR-0055" docs/design/observability/alert-runbook-link-baseline.md \
  || fail "alert-runbook-link-baseline.md must reference ADR-0055"

rg -q "runbook_url" docs/design/observability/alert-runbook-link-baseline.md \
  || fail "alert-runbook-link-baseline.md must document runbook_url validation"

rg -q "ADR-0055" docs/design/observability/grafana-dashboard-promql-baseline.md \
  || fail "grafana-dashboard-promql-baseline.md must reference ADR-0055"

rg -q "promtool check rules" docs/design/observability/grafana-dashboard-promql-baseline.md \
  || fail "grafana-dashboard-promql-baseline.md must document promtool check rules"

rg -q "promtool check config" docs/design/observability/prometheus-config-validation-baseline.md \
  || fail "prometheus-config-validation-baseline.md must document promtool check config"

rg -q "check config --lint=duplicate-rules" docs/design/ci/scripts/check_prometheus_config.sh \
  || fail "check_prometheus_config.sh must run promtool check config"

rg -q "shepherd-recording-rules.yml#\\$\\{ROOT_DIR\\}" docs/design/ci/scripts/check_prometheus_config.sh \
  || fail "check_prometheus_config.sh must render recording rule path to a local repository path"

rg -q "^ci-prometheus-config:" Makefile \
  || fail "Makefile must expose ci-prometheus-config"

rg -q "\\$\\(MAKE\\) ci-prometheus-config" Makefile \
  || fail "ci-prometheus-rules must include ci-prometheus-config"

rg -q "^ci-prometheus-operator-rule-parity:" Makefile \
  || fail "Makefile must expose ci-prometheus-operator-rule-parity"

rg -q "\\$\\(MAKE\\) ci-prometheus-operator-rule-parity" Makefile \
  || fail "ci-prometheus-rules must include ci-prometheus-operator-rule-parity"

rg -q "diff -u" docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh \
  || fail "check_prometheus_operator_rule_parity.sh must compare extracted Operator rules to native rules"

rg -q "check rules" docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh \
  || fail "check_prometheus_operator_rule_parity.sh must run promtool check rules on extracted Operator rules"

rg -q "^ci-prometheus-alert-runbooks:" Makefile \
  || fail "Makefile must expose ci-prometheus-alert-runbooks"

rg -q "\\$\\(MAKE\\) ci-prometheus-alert-runbooks" Makefile \
  || fail "ci-prometheus-rules must include ci-prometheus-alert-runbooks"

rg -q "markdown_anchors" docs/design/ci/scripts/check_prometheus_alert_runbooks.sh \
  || fail "check_prometheus_alert_runbooks.sh must validate markdown anchors"

rg -q "^ci-grafana-dashboard-promql:" Makefile \
  || fail "Makefile must expose ci-grafana-dashboard-promql"

rg -q "\\$\\(MAKE\\) ci-grafana-dashboard-promql" Makefile \
  || fail "ci-monitoring-assets must include ci-grafana-dashboard-promql"

rg -q "shepherd\.grafana\.dashboard" docs/design/ci/scripts/check_grafana_dashboard_promql.sh \
  || fail "check_grafana_dashboard_promql.sh must render a Grafana dashboard rule group"

rg -q "check rules" docs/design/ci/scripts/check_grafana_dashboard_promql.sh \
  || fail "check_grafana_dashboard_promql.sh must run promtool check rules"

rg -q "^project-completion-readiness:" Makefile \
  || fail "Makefile must expose project-completion-readiness"

rg -q "\\$\\(MAKE\\) ci-monitoring-assets" Makefile \
  || fail "project-completion-readiness must require monitoring asset validation"

rg -q "\\$\\(MAKE\\) ci-live-e2e-evidence" Makefile \
  || fail "project-completion-readiness must require live E2E evidence schema validation"

if sed -n '/^project-completion-readiness:/,/^$/p' Makefile | rg -q "ci-live-e2e-latest-evidence"; then
  fail "project-completion-readiness must not require latest full live E2E evidence validation"
fi

rg -q "project-completion-readiness" docs/design/ci/README.md \
  || fail "ci README must document project-completion-readiness"

rg -q "project-completion-readiness" docs/design/ci/MASTER_FLOW_STRICT_TEST_FLOW.md \
  || fail "MASTER_FLOW_STRICT_TEST_FLOW.md must document project-completion-readiness"

rg -q "static master-flow completion" docs/design/ci/MASTER_FLOW_STRICT_TEST_FLOW.md \
  || fail "MASTER_FLOW_STRICT_TEST_FLOW.md must distinguish static completion from project completion"

if rg -q "run_e2e_live\\.sh|ci-live-e2e-latest-evidence|ENABLE_LIVE_E2E|live-e2e-evidence" .github/workflows/ci.yml; then
  fail "ci.yml must not run or upload live E2E evidence in required CI"
fi

rg -q "manual release evidence" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document latest-manifest validation as manual release evidence"

rg -q "mode=full" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document latest full-manifest selection"

rg -q "cluster\.api_server_reachable" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document Kubernetes API probe evidence"

rg -q "cluster\.authenticated_context" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document authenticated cluster context pass criteria"

rg -q "cluster\.kubevirt_api_available" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document KubeVirt API discovery pass criteria"

rg -q "KubeVirt API discovery succeeded" docs/adr/ADR-0058-live-e2e-evidence-bundle-baseline.md \
  || fail "ADR-0058 must require KubeVirt API discovery evidence"

rg -q "kubeconfig authenticates" docs/adr/ADR-0058-live-e2e-evidence-bundle-baseline.md \
  || fail "ADR-0058 must require authenticated kubeconfig evidence"

rg -q "preflight manifests are ignored" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document that latest full validation ignores preflight manifests"

rg -q "required GitHub CI" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document that live E2E is outside required GitHub CI"

rg -q "manual live E2E evidence bundle" docs/operations/production-deployment.md \
  || fail "production-deployment.md must capture manual live E2E evidence bundle path"

rg -q "PROMTOOL_REQUIRED" docs/design/observability/rule-tests-baseline.md \
  || fail "rule-tests-baseline.md must document CI promtool-required mode"

rg -q "PROMTOOL_REQUIRED" docs/adr/ADR-0055-prometheus-rules-and-grafana-dashboard-baseline.md \
  || fail "ADR-0055 must require CI promtool enforcement"

rg -q "PROMTOOL_REQUIRED" docs/design/ci/scripts/promtool_lib.sh \
  || fail "promtool_lib.sh must support PROMTOOL_REQUIRED"

rg -q "promtool --version" .github/workflows/ci.yml \
  || fail "ci.yml must verify promtool is installed before governance"

rg -q "sudo apt-get update && sudo apt-get install -y .*prometheus" .github/workflows/ci.yml \
  || fail "ci.yml must install the prometheus package for promtool"

rg -q "PROMTOOL_REQUIRED:[[:space:]]*\"1\"" .github/workflows/ci.yml \
  || fail "ci.yml must run governance with PROMTOOL_REQUIRED=1"

rg -q "^live-e2e-readiness:" Makefile \
  || fail "Makefile must expose live-e2e-readiness"

rg -q "^live-e2e-status:" Makefile \
  || fail "Makefile must expose live-e2e-status"

rg -q "^ci-live-e2e-evidence:" Makefile \
  || fail "Makefile must expose ci-live-e2e-evidence"

rg -q "make live-e2e-readiness" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document make live-e2e-readiness"

rg -q "make live-e2e-status" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document make live-e2e-status"

rg -q "E2E_KUBECONFIG_B64" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document E2E_KUBECONFIG_B64"

rg -q "check_live_e2e_no_mock\.sh" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document the no-mock live E2E gate"

rg -q "check_master_flow_test_matrix\.go" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document the master-flow test matrix gate"

rg -q -- "--no-db-wrapper" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document --no-db-wrapper mode"

rg -q -- "--status" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document status polling"

# Local path/anchor link integrity must be deterministic and blocking.
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
mkdir -p "${GOCACHE}"
go run docs/design/ci/scripts/check_markdown_links.go

# Prometheus config validation accepted by ADR-0056 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_config.sh

# Prometheus recording rules accepted by ADR-0055 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_recording_rules.sh

# Prometheus alert rules accepted by ADR-0055 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_alert_rules.sh

# Prometheus alert runbook links accepted by ADR-0055 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_alert_runbooks.sh

# Prometheus rule tests accepted by ADR-0055 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_rule_tests.sh

# Prometheus Operator rule parity accepted by ADR-0056 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_operator_rule_parity.sh

# Prometheus Operator packaging accepted by ADR-0056 must stay structurally valid.
bash docs/design/ci/scripts/check_prometheus_operator_assets.sh

# Docker Compose monitoring packaging accepted by ADR-0056 must stay structurally valid.
bash docs/design/ci/scripts/check_monitoring_compose_assets.sh

# Grafana dashboard assets accepted by ADR-0055 must stay structurally valid.
bash docs/design/ci/scripts/check_grafana_dashboards.sh

# Grafana dashboard panel PromQL accepted by ADR-0055 must stay syntactically valid.
bash docs/design/ci/scripts/check_grafana_dashboard_promql.sh

# ADR-0058 live E2E evidence manifest fixtures must stay structurally valid.
bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh
bash docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh --self-test

# Master-flow traceability (ADR-0032)
go run docs/design/ci/scripts/check_master_flow_traceability.go

enforce_traceability_manifest_update() {
  local event_path="${GITHUB_EVENT_PATH:-}"
  local event_name="${GITHUB_EVENT_NAME:-}"

  local base_sha=""
  local head_sha=""
  local diff_mode="ci"
  if [[ -n "${event_path}" && -f "${event_path}" ]]; then
    if ! command -v python3 >/dev/null 2>&1; then
      fail "python3 is required for traceability diff enforcement in CI"
    fi
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      fail "git repository is required for traceability diff enforcement in CI"
    fi

    read -r base_sha head_sha < <(
      python3 - "$event_name" "$event_path" <<'PY'
import json
import sys

event_name = sys.argv[1]
event_path = sys.argv[2]

with open(event_path, "r", encoding="utf-8") as f:
    data = json.load(f)

base = None
head = None
if event_name in ("pull_request", "pull_request_target"):
    pr = data.get("pull_request") or {}
    base = (pr.get("base") or {}).get("sha")
    head = (pr.get("head") or {}).get("sha")
elif event_name == "push":
    base = data.get("before")
    head = data.get("after")

if not base or not head:
    sys.exit(1)

print(base, head)
PY
    ) || fail "Cannot determine base/head commit for traceability diff enforcement. Ensure checkout fetch-depth is 0."
  else
    diff_mode="local"
    if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
      return 0
    fi
    if ! git show-ref --verify --quiet refs/remotes/origin/main; then
      fail "Local traceability diff enforcement requires refs/remotes/origin/main. Run 'git fetch origin main' before local ci-checks."
    fi

    base_sha="$(git merge-base HEAD refs/remotes/origin/main)" \
      || fail "Cannot determine merge-base against origin/main for local traceability diff enforcement."
    head_sha="$(git rev-parse HEAD)" \
      || fail "Cannot determine HEAD for local traceability diff enforcement."
  fi

  local changed_files=""
  if [[ "${diff_mode}" == "ci" ]]; then
    changed_files="$(git diff --name-only "${base_sha}...${head_sha}" 2>/dev/null)" \
      || fail "git diff failed for ${base_sha}...${head_sha}. Ensure checkout fetch-depth is 0."
  else
    local tracked_changes=""
    local untracked_changes=""
    tracked_changes="$(git diff --name-only "${base_sha}" 2>/dev/null)" \
      || fail "git diff failed for local traceability enforcement against ${base_sha}."
    untracked_changes="$(git ls-files --others --exclude-standard)"
    changed_files="$(printf '%s\n%s\n' "${tracked_changes}" "${untracked_changes}" | awk 'NF' | sort -u)"
  fi

  if [[ -z "${changed_files}" ]]; then
    return 0
  fi

  # If canonical docs changed, require traceability manifest update in the same PR.
  # A producer pipe plus `rg -q` is nondeterministic under pipefail for large
  # change sets: rg exits on the first match and printf can then fail with SIGPIPE.
  if rg -q '^(docs/design/interaction-flows/master-flow\.md|docs/design/phases/|docs/design/checklist/|docs/design/examples/)' <<<"${changed_files}"; then
    if ! rg -q '^docs/design/traceability/master-flow\.json$' <<<"${changed_files}"; then
      fail "Traceability manifest must be updated when master-flow/phases/checklists/examples/ADRs change: docs/design/traceability/master-flow.json"
    fi
  fi
}

enforce_traceability_manifest_update

echo "[design-doc-governance] OK"
