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

rg -q "POST /api/v1/approvals/\\{id\\}/cancel" docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4-checklist.md must use API-prefixed cancellation endpoint"

rg -q "no active token revocation API" docs/design/checklist/phase-4-checklist.md \
  || fail "phase-4-checklist.md must document V1 no-active-revocation scope"

rg -q 'StatusURL:.*"/api/v1/vms/batch/"' docs/design/examples/usecase/batch_approval.go \
  || fail "batch_approval example must return canonical status_url path"

if rg -n "VMStatusDeleted" docs/design/examples/domain/vm.go >"${legacy_refs_file}"; then
  cat "${legacy_refs_file}" >&2
  fail "docs/design/examples/domain/vm.go must not include persisted DELETED state"
fi

# Local path/anchor link integrity must be deterministic and blocking.
export GOCACHE="${GOCACHE:-/tmp/go-build-cache}"
mkdir -p "${GOCACHE}"
go run docs/design/ci/scripts/check_markdown_links.go

# Master-flow traceability (ADR-0032)
go run docs/design/ci/scripts/check_master_flow_traceability.go

enforce_traceability_manifest_update() {
  local event_path="${GITHUB_EVENT_PATH:-}"
  local event_name="${GITHUB_EVENT_NAME:-}"

  if [[ -z "${event_path}" || ! -f "${event_path}" ]]; then
    return 0
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    fail "python3 is required for traceability diff enforcement in CI"
  fi
  if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    fail "git repository is required for traceability diff enforcement in CI"
  fi

  local base_sha=""
  local head_sha=""
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

  local changed_files=""
  changed_files="$(git diff --name-only "${base_sha}...${head_sha}" 2>/dev/null)" \
    || fail "git diff failed for ${base_sha}...${head_sha}. Ensure checkout fetch-depth is 0."

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
