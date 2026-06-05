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

# Checklist governance statement
rg -q "Global Single Standard" docs/design/checklist/README.md \
  || fail "checklist/README.md must declare CHECKLIST.md as global single standard"

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

rg -q "policy_gates\.cluster_probe" docs/design/ci/live-e2e-evidence-baseline.md \
  || fail "live-e2e-evidence-baseline.md must document policy_gates.cluster_probe"

rg -q "policy_gates', 'cluster_probe" docs/design/ci/scripts/check_live_e2e_evidence_manifest.sh \
  || fail "check_live_e2e_evidence_manifest.sh must validate policy_gates.cluster_probe"

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

rg -q "cluster\.kubevirt_api_available" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document KubeVirt API discovery pass criteria"

rg -q "KubeVirt API discovery succeeded" docs/adr/ADR-0058-live-e2e-evidence-bundle-baseline.md \
  || fail "ADR-0058 must require KubeVirt API discovery evidence"

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

rg -q "^ci-live-e2e-evidence:" Makefile \
  || fail "Makefile must expose ci-live-e2e-evidence"

rg -q "make live-e2e-readiness" docs/operations/live-e2e-validation.md \
  || fail "live-e2e-validation.md must document make live-e2e-readiness"

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
  if printf '%s\n' "${changed_files}" | rg -q '^(docs/design/interaction-flows/master-flow\.md|docs/design/phases/|docs/design/checklist/|docs/design/examples/)'; then
    if ! printf '%s\n' "${changed_files}" | rg -q '^docs/design/traceability/master-flow\.json$'; then
      fail "Traceability manifest must be updated when master-flow/phases/checklists/examples/ADRs change: docs/design/traceability/master-flow.json"
    fi
  fi
}

enforce_traceability_manifest_update

echo "[design-doc-governance] OK"
