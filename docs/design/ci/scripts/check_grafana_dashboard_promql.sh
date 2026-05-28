#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../../../.." && pwd)"
cd "${ROOT_DIR}"

# shellcheck source=docs/design/ci/scripts/promtool_lib.sh
source "${SCRIPT_DIR}/promtool_lib.sh"

fail() {
  echo "[grafana-dashboard-promql] ERROR: $1" >&2
  exit 1
}

dashboard_file="${1:-deploy/monitoring/grafana/dashboards/shepherd-overview.json}"
temp_rules=""
trap '[[ -n "${temp_rules}" ]] && rm -f "${temp_rules}"' EXIT

[[ -f "${dashboard_file}" ]] || fail "dashboard file missing: ${dashboard_file}"

temp_rules="$(mktemp)"

python3 - "${dashboard_file}" "${temp_rules}" <<'PY'
import json
import re
import sys
from pathlib import Path

dashboard_path = Path(sys.argv[1])
rules_path = Path(sys.argv[2])


def fail(message: str) -> None:
    print(f"[grafana-dashboard-promql] ERROR: {dashboard_path}: {message}", file=sys.stderr)
    sys.exit(1)


def sanitize(value: object, fallback: str) -> str:
    raw = str(value if value not in (None, "") else fallback)
    name = re.sub(r"[^A-Za-z0-9_]", "_", raw)
    name = re.sub(r"_+", "_", name).strip("_").lower()
    return name or fallback


try:
    dashboard = json.loads(dashboard_path.read_text(encoding="utf-8"))
except json.JSONDecodeError as exc:
    fail(f"invalid JSON: {exc}")

panels = dashboard.get("panels")
if not isinstance(panels, list) or not panels:
    fail("dashboard must contain at least one panel")

rules: list[tuple[str, str]] = []
seen_records: set[str] = set()
for panel_index, panel in enumerate(panels, start=1):
    if not isinstance(panel, dict):
        fail(f"panel #{panel_index} must be an object")

    panel_id = sanitize(panel.get("id"), f"panel_{panel_index}")
    panel_title = panel.get("title") or f"panel #{panel_index}"
    targets = panel.get("targets")
    if not isinstance(targets, list) or not targets:
        fail(f"panel {panel_title!r} must contain at least one target")

    for target_index, target in enumerate(targets, start=1):
        if not isinstance(target, dict):
            fail(f"panel {panel_title!r} target #{target_index} must be an object")
        expr = target.get("expr")
        if not isinstance(expr, str) or not expr.strip():
            fail(f"panel {panel_title!r} target #{target_index} missing expr")

        ref_id = sanitize(target.get("refId"), f"target_{target_index}")
        base_record = f"shepherd:grafana_dashboard_panel_{panel_id}_{ref_id}:query"
        record = base_record
        duplicate_index = 2
        while record in seen_records:
            record = f"{base_record}_{duplicate_index}"
            duplicate_index += 1
        seen_records.add(record)
        rules.append((record, expr.strip()))

if not rules:
    fail("dashboard must contain at least one Prometheus target expression")

lines = [
    "groups:",
    "  - name: shepherd.grafana.dashboard",
    "    rules:",
]
for record, expr in rules:
    lines.append(f"      - record: {record}")
    lines.append("        expr: |")
    for expr_line in expr.splitlines():
        lines.append(f"          {expr_line}")

rules_path.write_text("\n".join(lines) + "\n", encoding="utf-8")
print(f"[grafana-dashboard-promql] generated {len(rules)} panel query rule(s)")
PY

promtool_status=0
promtool_bin="$(resolve_promtool)" || promtool_status=$?
case "${promtool_status}" in
  0)
    "${promtool_bin}" check rules "${temp_rules}"
    ;;
  2)
    echo "[grafana-dashboard-promql] WARN: promtool not found; skipped PromQL parser validation"
    ;;
  *)
    exit "${promtool_status}"
    ;;
esac

echo "[grafana-dashboard-promql] OK: dashboard PromQL checked"
