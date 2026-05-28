#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"
source docs/design/ci/scripts/promtool_lib.sh

fail() {
  echo "[prometheus-config] ERROR: $1" >&2
  exit 1
}

config_file="${1:-deploy/monitoring/prometheus/prometheus.yml}"
recording_rules="deploy/monitoring/prometheus/shepherd-recording-rules.yml"
alert_rules="deploy/monitoring/prometheus/shepherd-alerts.yml"
rendered_config=""
trap '[[ -n "${rendered_config}" ]] && rm -f "${rendered_config}"' EXIT

[[ -f "${config_file}" ]] || fail "config file missing: ${config_file}"
[[ -f "${recording_rules}" ]] || fail "recording rules missing: ${recording_rules}"
[[ -f "${alert_rules}" ]] || fail "alert rules missing: ${alert_rules}"

rg -q '^global:$' "${config_file}" \
  || fail "${config_file}: missing global config"
rg -q '^[[:space:]]+scrape_interval: 30s$' "${config_file}" \
  || fail "${config_file}: scrape_interval must be 30s"
rg -q '^[[:space:]]+evaluation_interval: 30s$' "${config_file}" \
  || fail "${config_file}: evaluation_interval must be 30s"
rg -q '^rule_files:$' "${config_file}" \
  || fail "${config_file}: missing rule_files"
rg -q '^[[:space:]]+- /etc/prometheus/rules/shepherd-recording-rules\.yml$' "${config_file}" \
  || fail "${config_file}: must load recording rules from container rule path"
rg -q '^[[:space:]]+- /etc/prometheus/rules/shepherd-alerts\.yml$' "${config_file}" \
  || fail "${config_file}: must load alert rules from container rule path"
recording_line="$(rg -n 'shepherd-recording-rules\.yml' "${config_file}" | cut -d: -f1 | head -n1)"
alerts_line="$(rg -n 'shepherd-alerts\.yml' "${config_file}" | cut -d: -f1 | head -n1)"
[[ -n "${recording_line}" && -n "${alerts_line}" ]] \
  || fail "${config_file}: cannot determine rule file order"
(( recording_line < alerts_line )) \
  || fail "${config_file}: recording rules must load before alert rules"
rg -q '^scrape_configs:$' "${config_file}" \
  || fail "${config_file}: missing scrape_configs"
rg -q '^[[:space:]]+- job_name: shepherd$' "${config_file}" \
  || fail "${config_file}: missing shepherd scrape job"
rg -q '^[[:space:]]+metrics_path: /metrics$' "${config_file}" \
  || fail "${config_file}: metrics_path must be /metrics"
rg -q '^[[:space:]]+scheme: http$' "${config_file}" \
  || fail "${config_file}: scheme must be http"
rg -q '^[[:space:]]+- server:8080$' "${config_file}" \
  || fail "${config_file}: scrape target must be server:8080"

if promtool_bin="$(resolve_promtool)"; then
  rendered_config="$(mktemp)"
  sed \
    -e "s#/etc/prometheus/rules/shepherd-recording-rules.yml#${ROOT_DIR}/${recording_rules}#g" \
    -e "s#/etc/prometheus/rules/shepherd-alerts.yml#${ROOT_DIR}/${alert_rules}#g" \
    "${config_file}" >"${rendered_config}"
  "${promtool_bin}" check config --lint=duplicate-rules "${rendered_config}"
elif [[ $? -eq 2 ]]; then
  echo "[prometheus-config] promtool not found; running structural checks only for ${config_file}"
else
  fail "promtool discovery failed"
fi

echo "[prometheus-config] OK: ${config_file} checked"
