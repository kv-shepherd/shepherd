#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

fail() {
  echo "[grafana-dashboards] ERROR: $1" >&2
  exit 1
}

dashboard_file="${1:-deploy/monitoring/grafana/dashboards/shepherd-overview.json}"
provisioning_file="${2:-deploy/monitoring/grafana/provisioning/dashboards/shepherd.yml}"

[[ -f "${dashboard_file}" ]] || fail "dashboard file missing: ${dashboard_file}"
[[ -f "${provisioning_file}" ]] || fail "provisioning file missing: ${provisioning_file}"

python3 - "${dashboard_file}" <<'PY'
import json
import re
import sys
from pathlib import Path

path = Path(sys.argv[1])
dashboard = json.loads(path.read_text(encoding="utf-8"))

def fail(message: str) -> None:
    print(f"[grafana-dashboards] ERROR: {path}: {message}", file=sys.stderr)
    sys.exit(1)

if dashboard.get("uid") != "shepherd-overview":
    fail("uid must be shepherd-overview")
if dashboard.get("title") != "Shepherd Overview":
    fail("title must be Shepherd Overview")
if dashboard.get("schemaVersion", 0) < 17:
    fail("schemaVersion must be present and >= 17")
if dashboard.get("version") != 1:
    fail("version must be 1")
if dashboard.get("refresh") != "30s":
    fail("refresh must be 30s")
if dashboard.get("time", {}).get("from") != "now-6h" or dashboard.get("time", {}).get("to") != "now":
    fail("time range must be now-6h to now")

tags = set(dashboard.get("tags") or [])
for tag in ("shepherd", "observability"):
    if tag not in tags:
        fail(f"missing tag {tag}")

variables = dashboard.get("templating", {}).get("list") or []
variable_names = {variable.get("name") for variable in variables}
if variable_names != {"datasource"}:
    fail(f"variables must contain only datasource, got {sorted(variable_names)}")
datasource = variables[0]
if datasource.get("type") != "datasource" or datasource.get("query") != "prometheus":
    fail("datasource variable must select prometheus datasources")

expected_panels = {
    "Metrics target availability",
    "HTTP request rate",
    "HTTP 5xx ratio",
    "HTTP p95 latency",
    "OpenAPI validation failures",
    "PostgreSQL stats scrape success",
    "PostgreSQL table dead tuple ratio",
    "River dead tuple ratio",
    "River queue stats scrape success",
    "River ready jobs",
    "River oldest ready job age",
    "River recent terminal jobs",
}

panels = dashboard.get("panels") or []
titles = [panel.get("title") for panel in panels]
if set(titles) != expected_panels:
    fail(f"panel titles mismatch: got {sorted(titles)}")
if len(titles) != len(set(titles)):
    fail("panel titles must be unique")

allowed_metrics = {
    "shepherd:http_requests:rate5m",
    "shepherd:http_5xx:ratio5m",
    "shepherd:http_request_duration_seconds:p95_5m",
    "shepherd:openapi_validation_failures:rate5m",
    "shepherd_postgres_table_stats_scrape_success",
    "shepherd_postgres_table_dead_tuple_ratio",
    "shepherd_river_dead_tuple_ratio",
    "shepherd_river_queue_stats_scrape_success",
    "shepherd:river_ready_jobs:sum",
    "shepherd_river_oldest_ready_job_age_seconds",
    "shepherd_river_recent_terminal_jobs",
}
required_metrics = set(allowed_metrics)
observed_metrics = set()
for panel in panels:
    grid = panel.get("gridPos") or {}
    for key in ("h", "w", "x", "y"):
        if not isinstance(grid.get(key), int):
            fail(f"panel {panel.get('title')} missing integer gridPos.{key}")
    targets = panel.get("targets") or []
    if not targets:
        fail(f"panel {panel.get('title')} missing targets")
    for target in targets:
        ds = target.get("datasource") or {}
        if ds.get("type") != "prometheus" or ds.get("uid") != "${datasource}":
            fail(f"panel {panel.get('title')} target must use prometheus ${{datasource}}")
        expr = target.get("expr")
        if not isinstance(expr, str) or not expr.strip():
            fail(f"panel {panel.get('title')} target missing expr")
        for metric in re.findall(r"shepherd[:_][A-Za-z0-9_:]+", expr):
            if metric not in allowed_metrics:
                fail(f"unsupported metric reference {metric}")
            observed_metrics.add(metric)
        for forbidden in ("user", "role", "session", "ticket", "vm", "namespace", "cluster", "path", "query", "header"):
            if re.search(rf"[$_{{(,]\s*{forbidden}\b", expr):
                fail(f"expr must not group/filter by forbidden label {forbidden}: {expr}")

missing = sorted(required_metrics - observed_metrics)
if missing:
    fail(f"required metric references missing: {missing}")
PY

rg -q '^apiVersion: 1$' "${provisioning_file}" || fail "${provisioning_file}: missing apiVersion"
rg -q '^[[:space:]]+- name: shepherd$' "${provisioning_file}" || fail "${provisioning_file}: missing shepherd provider"
rg -q '^[[:space:]]+folder: Shepherd$' "${provisioning_file}" || fail "${provisioning_file}: missing Shepherd folder"
rg -q '^[[:space:]]+type: file$' "${provisioning_file}" || fail "${provisioning_file}: provider must use file type"
rg -q '^[[:space:]]+allowUiUpdates: false$' "${provisioning_file}" || fail "${provisioning_file}: UI updates must be disabled"
rg -q '^[[:space:]]+path: /var/lib/grafana/dashboards/shepherd$' "${provisioning_file}" || fail "${provisioning_file}: unexpected dashboard path"

echo "[grafana-dashboards] OK: dashboard and provisioning files checked"
