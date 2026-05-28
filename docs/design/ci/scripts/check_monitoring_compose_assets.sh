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
prod_compose="deploy/prod/docker-compose.prod.yml"
env_example="deploy/prod/.env.prod.example"
rendered_config=""
trap '[[ -n "${rendered_config}" ]] && rm -f "${rendered_config}"' EXIT

for file in "${compose_file}" "${prometheus_config}" "${grafana_datasource}" "${prod_compose}" "${env_example}"; do
  [[ -f "${file}" ]] || fail "missing ${file}"
done

rg -q '^[[:space:]]+prometheus:$' "${compose_file}" \
  || fail "${compose_file}: missing prometheus service"
rg -q '^[[:space:]]+grafana:$' "${compose_file}" \
  || fail "${compose_file}: missing grafana service"
rg -q 'image: \$\{PROMETHEUS_IMAGE:\?PROMETHEUS_IMAGE is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: prometheus image must be deployment-owned"
rg -q 'image: \$\{GRAFANA_IMAGE:\?GRAFANA_IMAGE is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: grafana image must be deployment-owned"
rg -q 'GF_SECURITY_ADMIN_PASSWORD: \$\{GRAFANA_ADMIN_PASSWORD:\?GRAFANA_ADMIN_PASSWORD is required when monitoring overlay is enabled\}' "${compose_file}" \
  || fail "${compose_file}: grafana admin password must be required"
rg -q '\.\./monitoring/prometheus/prometheus\.yml:/etc/prometheus/prometheus\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: prometheus config must be mounted read-only"
rg -q '\.\./monitoring/prometheus/shepherd-recording-rules\.yml:/etc/prometheus/rules/shepherd-recording-rules\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: recording rules must be mounted read-only"
rg -q '\.\./monitoring/prometheus/shepherd-alerts\.yml:/etc/prometheus/rules/shepherd-alerts\.yml:ro' "${compose_file}" \
  || fail "${compose_file}: alert rules must be mounted read-only"
rg -q '\.\./monitoring/grafana/provisioning/datasources:/etc/grafana/provisioning/datasources:ro' "${compose_file}" \
  || fail "${compose_file}: grafana datasource provisioning must be mounted"
rg -q '\.\./monitoring/grafana/provisioning/dashboards:/etc/grafana/provisioning/dashboards:ro' "${compose_file}" \
  || fail "${compose_file}: grafana dashboard provisioning must be mounted"
rg -q '\.\./monitoring/grafana/dashboards:/var/lib/grafana/dashboards/shepherd:ro' "${compose_file}" \
  || fail "${compose_file}: grafana dashboard directory must be mounted"

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

for key in \
  OBSERVABILITY_METRICS_ENABLED \
  OBSERVABILITY_METRICS_PATH \
  OBSERVABILITY_DATABASE_METRICS_ENABLED \
  OBSERVABILITY_DATABASE_METRICS_TIMEOUT \
  OBSERVABILITY_TRACING_ENABLED \
  OBSERVABILITY_TRACING_EXPORTER \
  OTEL_EXPORTER_OTLP_ENDPOINT \
  OTEL_EXPORTER_OTLP_TRACES_ENDPOINT \
  PROMETHEUS_IMAGE \
  GRAFANA_IMAGE \
  GRAFANA_ADMIN_PASSWORD; do
  rg -q "${key}" "${prod_compose}" "${env_example}" \
    || fail "expected ${key} in production compose/env example"
done

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  rendered_config="$(mktemp)"
  SERVER_IMAGE=example/shepherd-server:test \
    WEB_IMAGE=example/shepherd-web:test \
    DATABASE_URL='postgres://shepherd:example-postgres-password@db:5432/shepherd_db?sslmode=disable' \
    POSTGRES_PASSWORD=example-postgres-password \
    PROMETHEUS_IMAGE=prom/prometheus:test \
    GRAFANA_IMAGE=grafana/grafana:test \
    GRAFANA_ADMIN_PASSWORD=example-admin-password \
    docker compose \
      -f "${prod_compose}" \
      -f "${compose_file}" \
      config >"${rendered_config}" \
    || fail "docker compose config failed for production monitoring overlay"

  rg -q '^[[:space:]]+prometheus:$' "${rendered_config}" \
    || fail "rendered compose config missing prometheus service"
  rg -q '^[[:space:]]+grafana:$' "${rendered_config}" \
    || fail "rendered compose config missing grafana service"
else
  echo "[monitoring-compose-assets] docker compose not found; skipping rendered compose config check"
fi

echo "[monitoring-compose-assets] OK"
