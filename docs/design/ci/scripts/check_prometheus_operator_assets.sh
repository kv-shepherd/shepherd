#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

fail() {
  echo "[prometheus-operator-assets] ERROR: $1" >&2
  exit 1
}

service_monitor="deploy/monitoring/prometheus-operator/shepherd-service-monitor.yml"
prometheus_rule="deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml"

expected_records=(
  shepherd:http_requests:rate5m
  shepherd:http_requests_5xx:rate5m
  shepherd:http_5xx:ratio5m
  shepherd:http_request_duration_seconds:p95_5m
  shepherd:openapi_validation_failures:rate5m
  shepherd:river_ready_jobs:sum
  shepherd:river_recent_discarded_jobs:sum
)

expected_alerts=(
  ShepherdMetricsTargetDown
  ShepherdHighHTTP5xxRate
  ShepherdHighHTTPP95Latency
  ShepherdOpenAPIValidationFailures
  ShepherdPostgresStatsScrapeFailed
  ShepherdRiverDeadTupleRatioHigh
  ShepherdRiverQueueStatsScrapeFailed
  ShepherdRiverQueueBacklogAgeHigh
  ShepherdRiverJobsDiscarded
)

[[ -f "${service_monitor}" ]] || fail "missing ${service_monitor}"
[[ -f "${prometheus_rule}" ]] || fail "missing ${prometheus_rule}"

rg -q '^apiVersion: monitoring\.coreos\.com/v1$' "${service_monitor}" \
  || fail "${service_monitor}: unexpected apiVersion"
rg -q '^kind: ServiceMonitor$' "${service_monitor}" \
  || fail "${service_monitor}: unexpected kind"
rg -q '^[[:space:]]+name: shepherd$' "${service_monitor}" \
  || fail "${service_monitor}: metadata.name must be shepherd"
rg -q '^[[:space:]]+app\.kubernetes\.io/name: shepherd$' "${service_monitor}" \
  || fail "${service_monitor}: missing app.kubernetes.io/name selector label"
rg -q '^[[:space:]]+matchLabels:$' "${service_monitor}" \
  || fail "${service_monitor}: missing selector.matchLabels"
rg -q '^[[:space:]]+- port: http$' "${service_monitor}" \
  || fail "${service_monitor}: endpoint port must be named http"
rg -q '^[[:space:]]+path: /metrics$' "${service_monitor}" \
  || fail "${service_monitor}: endpoint path must be /metrics"
rg -q '^[[:space:]]+interval: 30s$' "${service_monitor}" \
  || fail "${service_monitor}: endpoint interval must be 30s"
rg -q '^[[:space:]]+scheme: http$' "${service_monitor}" \
  || fail "${service_monitor}: endpoint scheme must be http"

rg -q '^apiVersion: monitoring\.coreos\.com/v1$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: unexpected apiVersion"
rg -q '^kind: PrometheusRule$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: unexpected kind"
rg -q '^[[:space:]]+name: shepherd-baseline$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: metadata.name must be shepherd-baseline"
rg -q '^[[:space:]]+role: alert-rules$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: missing role alert-rules label"
rg -q '^[[:space:]]+- name: shepherd\.recording$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: missing shepherd.recording group"
rg -q '^[[:space:]]+- name: shepherd\.baseline$' "${prometheus_rule}" \
  || fail "${prometheus_rule}: missing shepherd.baseline group"

for record in "${expected_records[@]}"; do
  count="$(rg -c "^[[:space:]]*- record: ${record}$" "${prometheus_rule}" || true)"
  [[ "${count}" == "1" ]] || fail "${prometheus_rule}: expected exactly one ${record}, found ${count}"
done

for alert in "${expected_alerts[@]}"; do
  count="$(rg -c "^[[:space:]]*- alert: ${alert}$" "${prometheus_rule}" || true)"
  [[ "${count}" == "1" ]] || fail "${prometheus_rule}: expected exactly one ${alert}, found ${count}"
done

if rg -n '^[[:space:]]+(user|session|ticket|vm|cluster|query|header):' "${prometheus_rule}" "${service_monitor}"; then
  fail "operator assets must not add sensitive or high-cardinality labels"
fi

echo "[prometheus-operator-assets] OK"
