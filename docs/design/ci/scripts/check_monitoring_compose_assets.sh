#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

fail() {
  echo "[monitoring-compose-assets] ERROR: $1" >&2
  exit 1
}

compose_file="deploy/prod/docker-compose.monitoring.yml"
prometheus_config="deploy/monitoring/prometheus/prometheus.yml"
grafana_datasource="deploy/monitoring/grafana/provisioning/datasources/prometheus.yml"
grafana_dashboard="deploy/monitoring/grafana/dashboards/shepherd-overview.json"
tempo_config="deploy/monitoring/tempo/tempo.yml"
otel_config="deploy/monitoring/otel-collector/otel-collector.yml"
prod_compose="deploy/prod/docker-compose.prod.yml"
prod_nginx="deploy/prod/nginx/prod.conf"
env_example="deploy/prod/.env.prod.example"
dev_compose="deploy/dev/docker-compose.yml"
dev_start="deploy/dev/start-dev.sh"
dev_nginx="deploy/dev/nginx/default.conf.template"
rendered_config=""
trap '[[ -n "${rendered_config}" ]] && rm -f "${rendered_config}"' EXIT

for file in "${compose_file}" "${prometheus_config}" "${grafana_datasource}" "${grafana_dashboard}" "${tempo_config}" "${otel_config}" "${prod_compose}" "${prod_nginx}" "${env_example}" "${dev_compose}" "${dev_start}" "${dev_nginx}"; do
  [[ -f "${file}" ]] || fail "missing ${file}"
done

rg -q '^[[:space:]]+prometheus:$' "${compose_file}" \
  || fail "${compose_file}: missing prometheus service"
if rg -q '^[[:space:]]+grafana:$' "${compose_file}"; then
  fail "${compose_file}: Grafana must not be part of the default monitoring overlay"
fi
rg -q '^[[:space:]]+tempo:$' "${compose_file}" \
  || fail "${compose_file}: missing tempo service"
rg -q '^[[:space:]]+otel-collector:$' "${compose_file}" \
  || fail "${compose_file}: missing otel-collector service"
rg -q 'image: \$\{PROMETHEUS_IMAGE:\?PROMETHEUS_IMAGE is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: prometheus image must be deployment-owned"
rg -q 'image: \$\{TEMPO_IMAGE:\?TEMPO_IMAGE is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: tempo image must be deployment-owned"
rg -q 'image: \$\{OTEL_COLLECTOR_IMAGE:\?OTEL_COLLECTOR_IMAGE is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: OpenTelemetry Collector image must be deployment-owned"
rg -q '\.\./monitoring/prometheus/prometheus\.yml:/etc/prometheus/prometheus\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: prometheus config must be mounted read-only"
rg -q '\.\./monitoring/prometheus/shepherd-recording-rules\.yml:/etc/prometheus/rules/shepherd-recording-rules\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: recording rules must be mounted read-only"
rg -q '\.\./monitoring/prometheus/shepherd-alerts\.yml:/etc/prometheus/rules/shepherd-alerts\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: alert rules must be mounted read-only"
rg -q '\.\./monitoring/tempo/tempo\.yml:/etc/tempo/tempo\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: Tempo config must be mounted read-only"
rg -q '\.\./monitoring/otel-collector/otel-collector\.yml:/etc/otelcol-contrib/otel-collector\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: OpenTelemetry Collector config must be mounted read-only"
rg -q '^[[:space:]]+nginx:$' "${compose_file}" \
  || fail "${compose_file}: must extend nginx service for tracing startup ordering"
if rg -q 'location /grafana/|proxy_pass http://grafana:3000|X-Forwarded-Prefix /grafana' "${prod_nginx}"; then
  fail "${prod_nginx}: must not expose Grafana in the default nginx config"
fi

rg -q '^global:$' "${prometheus_config}" \
  || fail "${prometheus_config}: missing global config"
rg -q '^[[:space:]]+scrape_interval: 30s$' "${prometheus_config}" \
  || fail "${prometheus_config}: scrape_interval must be 30s"
rg -q '^[[:space:]]+evaluation_interval: 30s$' "${prometheus_config}" \
  || fail "${prometheus_config}: evaluation_interval must be 30s"
rg -q '^[[:space:]]+- /etc/prometheus/rules/shepherd-recording-rules\.yml$' "${prometheus_config}" \
  || fail "${prometheus_config}: must load recording rules"
rg -q '^[[:space:]]+- /etc/prometheus/rules/shepherd-alerts\.yml$' "${prometheus_config}" \
  || fail "${prometheus_config}: must load alert rules"
recording_line="$(rg -n 'shepherd-recording-rules\.yml' "${prometheus_config}" | cut -d: -f1 | head -n1)"
alerts_line="$(rg -n 'shepherd-alerts\.yml' "${prometheus_config}" | cut -d: -f1 | head -n1)"
(( recording_line < alerts_line )) \
  || fail "${prometheus_config}: recording rules must load before alert rules"
rg -q '^[[:space:]]+- job_name: shepherd$' "${prometheus_config}" \
  || fail "${prometheus_config}: missing shepherd scrape job"
rg -q '^[[:space:]]+metrics_path: /metrics$' "${prometheus_config}" \
  || fail "${prometheus_config}: metrics_path must be /metrics"
rg -q '^[[:space:]]+scheme: http$' "${prometheus_config}" \
  || fail "${prometheus_config}: scheme must be http"
rg -q '^[[:space:]]+- server:8080$' "${prometheus_config}" \
  || fail "${prometheus_config}: scrape target must be server:8080"

rg -q '^apiVersion: 1$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: missing apiVersion"
rg -q '^[[:space:]]+- name: Prometheus$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource name must be Prometheus"
rg -q '^[[:space:]]+type: prometheus$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource type must be prometheus"
rg -q '^[[:space:]]+uid: prometheus$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource uid must be prometheus"
rg -q '^[[:space:]]+access: proxy$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource access must be proxy"
rg -q '^[[:space:]]+url: http://prometheus:9090$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource URL must target prometheus service"
rg -q '^[[:space:]]+isDefault: true$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource must be default"
rg -q '^[[:space:]]+editable: false$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource must not be UI editable"
rg -q '^[[:space:]]+- name: Tempo$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: datasource name must include Tempo"
rg -q '^[[:space:]]+type: tempo$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: Tempo datasource type must be tempo"
rg -q '^[[:space:]]+uid: tempo$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: Tempo datasource uid must be tempo"
rg -q '^[[:space:]]+url: http://tempo:3200$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: Tempo datasource URL must target tempo service"
rg -q '^[[:space:]]+datasourceUid: prometheus$' "${grafana_datasource}" \
  || fail "${grafana_datasource}: Tempo service map must use Prometheus datasource"
rg -F -q '"title": "Firing alerts"' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: must show current firing alert count"
rg -F -q '"title": "Firing alert details"' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: must show current firing alert details"
rg -F -q 'sum(ALERTS{alertstate=\"firing\", service=\"shepherd\"}) or vector(0)' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: firing alert count must default to zero"
rg -F -q 'shepherd:openapi_validation_failures:rate5m' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: OpenAPI validation panel must use recording rule"
rg -F -q 'shepherd:river_ready_jobs:sum' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: River backlog panel must use recording rule"
rg -F -q '"title": "Pending approvals"' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: must show pending approval backlog"
rg -F -q '"title": "Approval failure audit actions"' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: must show audit-derived approval failure actions"
rg -F -q 'shepherd:business_approval_pending:sum' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: approval backlog must use recording rule"
rg -F -q 'shepherd:business_approval_failure_audit_actions:sum' "${grafana_dashboard}" \
  || fail "${grafana_dashboard}: approval failure audit actions must be shown"

for key in \
  OBSERVABILITY_METRICS_ENABLED \
  OBSERVABILITY_METRICS_PATH \
  OBSERVABILITY_DATABASE_METRICS_ENABLED \
  OBSERVABILITY_DATABASE_METRICS_TIMEOUT \
  OBSERVABILITY_RIVER_METRICS_ENABLED \
  OBSERVABILITY_RIVER_METRICS_TIMEOUT \
  OBSERVABILITY_BUSINESS_METRICS_ENABLED \
  OBSERVABILITY_BUSINESS_METRICS_TIMEOUT \
  OBSERVABILITY_TRACING_ENABLED \
  OBSERVABILITY_TRACING_SERVICE_NAME \
  OBSERVABILITY_TRACING_EXPORTER \
  OBSERVABILITY_TRACING_SAMPLE_RATIO \
  OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT \
  OTEL_EXPORTER_OTLP_ENDPOINT \
  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT \
  TEMPO_IMAGE \
  OTEL_COLLECTOR_IMAGE \
  PROMETHEUS_IMAGE; do
  rg -q "${key}" "${prod_compose}" "${env_example}" \
    || fail "expected ${key} in production compose/env example"
done

for key in \
  OBSERVABILITY_METRICS_ENABLED \
  OBSERVABILITY_METRICS_PATH \
  OBSERVABILITY_DATABASE_METRICS_ENABLED \
  OBSERVABILITY_DATABASE_METRICS_TIMEOUT \
  OBSERVABILITY_RIVER_METRICS_ENABLED \
  OBSERVABILITY_RIVER_METRICS_TIMEOUT \
  OBSERVABILITY_BUSINESS_METRICS_ENABLED \
  OBSERVABILITY_BUSINESS_METRICS_TIMEOUT \
  OBSERVABILITY_TRACING_ENABLED \
  OBSERVABILITY_TRACING_SERVICE_NAME \
  OBSERVABILITY_TRACING_EXPORTER \
  OBSERVABILITY_TRACING_SAMPLE_RATIO \
  OBSERVABILITY_TRACING_SHUTDOWN_TIMEOUT \
  OTEL_EXPORTER_OTLP_ENDPOINT \
  OTEL_EXPORTER_OTLP_HEADERS \
  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT \
  OTEL_EXPORTER_OTLP_TRACES_HEADERS; do
  rg -q "^[[:space:]]+${key}:" "${dev_compose}" \
    || fail "${dev_compose}: missing server environment ${key}"
done
rg -q 'verify_backend_metrics' "${dev_start}" \
  || fail "${dev_start}: must verify the backend metrics endpoint during local startup"
rg -q 'shepherd_http_requests_total' "${dev_start}" \
  || fail "${dev_start}: metrics smoke check must verify Shepherd HTTP series"
rg -q 'shepherd_business_metrics_scrape_success' "${dev_start}" \
  || fail "${dev_start}: metrics smoke check must verify Shepherd business series"
if rg -q 'verify_grafana_ready|/grafana/api/health|Grafana direct|Grafana embedded' "${dev_start}"; then
  fail "${dev_start}: must not start or verify Grafana by default"
fi
rg -q '^[[:space:]]+prometheus:$' "${dev_compose}" \
  || fail "${dev_compose}: must include built-in Prometheus for local observability"
if rg -q '^[[:space:]]+grafana:$' "${dev_compose}"; then
  fail "${dev_compose}: must not include built-in Grafana by default"
fi
rg -q '^[[:space:]]+tempo:$' "${dev_compose}" \
  || fail "${dev_compose}: must include built-in Tempo for local tracing"
rg -q '^[[:space:]]+otel-collector:$' "${dev_compose}" \
  || fail "${dev_compose}: must include built-in OpenTelemetry Collector for local tracing"
if rg -q 'location /grafana/|proxy_pass http://grafana:3000|X-Forwarded-Prefix /grafana' "${dev_nginx}"; then
  fail "${dev_nginx}: local ingress must not expose Grafana by default"
fi

rg -q '^[[:space:]]+endpoint: 0\.0\.0\.0:4318$' "${otel_config}" \
  || fail "${otel_config}: OTLP HTTP receiver must listen on 4318"
rg -q '^[[:space:]]+endpoint: tempo:4317$' "${otel_config}" \
  || fail "${otel_config}: Collector must export traces to Tempo"
rg -q '^[[:space:]]+memory_limiter:$' "${otel_config}" \
  || fail "${otel_config}: Collector trace pipeline must include memory_limiter processor"
rg -q '^[[:space:]]+batch:$' "${otel_config}" \
  || fail "${otel_config}: Collector trace pipeline must include batch processor"
rg -q '^[[:space:]]+- memory_limiter$' "${otel_config}" \
  || fail "${otel_config}: Collector traces pipeline must run memory_limiter before batch"
memory_limiter_line="$(rg -n '^[[:space:]]+- memory_limiter$' "${otel_config}" | cut -d: -f1 | head -n1)"
batch_line="$(rg -n '^[[:space:]]+- batch$' "${otel_config}" | cut -d: -f1 | tail -n1)"
(( memory_limiter_line < batch_line )) \
  || fail "${otel_config}: Collector traces pipeline must run memory_limiter before batch"
rg -q '^[[:space:]]+backend: local$' "${tempo_config}" \
  || fail "${tempo_config}: Tempo must use local storage in Compose"
rg -q '^[[:space:]]+http_listen_port: 3200$' "${tempo_config}" \
  || fail "${tempo_config}: Tempo must expose HTTP API on 3200"

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  rendered_config="$(mktemp)"
  SERVER_IMAGE=example/shepherd-server:test \
    WEB_IMAGE=example/shepherd-web:test \
    DATABASE_URL='postgres://shepherd:example-postgres-password@db:5432/shepherd_db?sslmode=disable' \
    POSTGRES_PASSWORD=example-postgres-password \
    PROMETHEUS_IMAGE=prom/prometheus:test \
    TEMPO_IMAGE=grafana/tempo:test \
    OTEL_COLLECTOR_IMAGE=otel/opentelemetry-collector-contrib:test \
    docker compose \
      -f "${prod_compose}" \
      -f "${compose_file}" \
      config >"${rendered_config}" \
    || fail "docker compose config failed for production monitoring overlay"

  rg -q '^[[:space:]]+prometheus:$' "${rendered_config}" \
    || fail "rendered compose config missing prometheus service"
  if rg -q '^[[:space:]]+grafana:$' "${rendered_config}"; then
    fail "rendered compose config must not include grafana service by default"
  fi
  rg -q '^[[:space:]]+tempo:$' "${rendered_config}" \
    || fail "rendered compose config missing tempo service"
  rg -q '^[[:space:]]+otel-collector:$' "${rendered_config}" \
    || fail "rendered compose config missing otel-collector service"
else
  echo "[monitoring-compose-assets] docker compose not found; skipping rendered compose config check"
fi

echo "[monitoring-compose-assets] OK"
