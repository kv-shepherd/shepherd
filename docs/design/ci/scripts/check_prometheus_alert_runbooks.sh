#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"

fail() {
  echo "[prometheus-alert-runbooks] ERROR: $1" >&2
  exit 1
}

alert_file="${1:-deploy/monitoring/prometheus/shepherd-alerts.yml}"

[[ -f "${alert_file}" ]] || fail "alert rule file missing: ${alert_file}"

python3 - "${alert_file}" <<'PY'
import re
import sys
from pathlib import Path

try:
    import yaml
except Exception as exc:
    print(f"[prometheus-alert-runbooks] ERROR: PyYAML is required: {exc}", file=sys.stderr)
    sys.exit(1)

alert_file = Path(sys.argv[1])
expected_alerts = {
    "ShepherdMetricsTargetDown",
    "ShepherdHighHTTP5xxRate",
    "ShepherdHighHTTPP95Latency",
    "ShepherdOpenAPIValidationFailures",
    "ShepherdPostgresStatsScrapeFailed",
    "ShepherdRiverDeadTupleRatioHigh",
    "ShepherdRiverQueueStatsScrapeFailed",
    "ShepherdRiverQueueBacklogAgeHigh",
    "ShepherdRiverJobsDiscarded",
    "ShepherdBusinessMetricsScrapeFailed",
    "ShepherdApprovalPendingTooLong",
    "ShepherdApprovalFailuresPresent",
    "ShepherdBatchApprovalPendingTooLong",
    "ShepherdBatchApprovalFailuresPresent",
    "ShepherdApprovalFailureAuditActionsRecent",
}


def fail(message: str) -> None:
    print(f"[prometheus-alert-runbooks] ERROR: {message}", file=sys.stderr)
    sys.exit(1)


def slugify_heading(heading: str) -> str:
    value = heading.strip().lower()
    value = re.sub(r"[^\w\s-]", "", value)
    value = re.sub(r"\s+", "-", value)
    return value


def markdown_anchors(path: Path) -> set[str]:
    anchors: set[str] = set()
    for line in path.read_text(encoding="utf-8").splitlines():
        match = re.match(r"^(#{1,6})\s+(.+?)\s*$", line)
        if match:
            anchors.add(slugify_heading(match.group(2)))
    return anchors


try:
    parsed = yaml.safe_load(alert_file.read_text(encoding="utf-8"))
except Exception as exc:
    fail(f"{alert_file}: failed to parse YAML: {exc}")

groups = parsed.get("groups") if isinstance(parsed, dict) else None
if not isinstance(groups, list):
    fail(f"{alert_file}: groups must be a list")

seen: dict[str, str] = {}
for group in groups:
    if not isinstance(group, dict):
        continue
    for rule in group.get("rules") or []:
        if not isinstance(rule, dict) or "alert" not in rule:
            continue
        alert = rule.get("alert")
        if alert not in expected_alerts:
            continue
        if alert in seen:
            fail(f"{alert_file}: duplicate alert {alert}")
        annotations = rule.get("annotations")
        if not isinstance(annotations, dict):
            fail(f"{alert_file}: {alert}: annotations must be an object")
        runbook_url = annotations.get("runbook_url")
        if not isinstance(runbook_url, str) or not runbook_url.strip():
            fail(f"{alert_file}: {alert}: runbook_url annotation is required")
        seen[alert] = runbook_url.strip()

missing = sorted(expected_alerts - set(seen))
if missing:
    fail(f"{alert_file}: missing baseline alert(s): {', '.join(missing)}")

anchor_cache: dict[Path, set[str]] = {}
for alert, runbook_url in sorted(seen.items()):
    if "://" in runbook_url or runbook_url.startswith("/"):
        fail(f"{alert_file}: {alert}: runbook_url must be a repository-local markdown anchor: {runbook_url}")
    if "#" not in runbook_url:
        fail(f"{alert_file}: {alert}: runbook_url must include an anchor fragment: {runbook_url}")
    raw_path, raw_anchor = runbook_url.split("#", 1)
    if not raw_path.endswith(".md"):
        fail(f"{alert_file}: {alert}: runbook_url must target a markdown file: {runbook_url}")
    if not raw_path.startswith("docs/design/observability/"):
        fail(f"{alert_file}: {alert}: runbook_url must stay under docs/design/observability/: {runbook_url}")
    if not raw_anchor:
        fail(f"{alert_file}: {alert}: runbook_url anchor must not be empty: {runbook_url}")

    target = Path(raw_path)
    if not target.is_file():
        fail(f"{alert_file}: {alert}: runbook target does not exist: {raw_path}")
    if target not in anchor_cache:
        anchor_cache[target] = markdown_anchors(target)
    if raw_anchor not in anchor_cache[target]:
        fail(f"{alert_file}: {alert}: runbook anchor #{raw_anchor} not found in {raw_path}")

print(f"[prometheus-alert-runbooks] OK: {len(seen)} alert runbook link(s) checked")
PY
