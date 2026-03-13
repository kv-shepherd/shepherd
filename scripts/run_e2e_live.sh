#!/usr/bin/env bash

set -euo pipefail

LOG_TS_FORMAT="${LOG_TS_FORMAT:-%Y-%m-%dT%H:%M:%S%z}"
ts_now() {
  date +"${LOG_TS_FORMAT}"
}

log_info() {
  printf '%s INFO: %s\n' "$(ts_now)" "$*"
}

log_warn() {
  printf '%s WARN: %s\n' "$(ts_now)" "$*"
}

log_error() {
  printf '%s ERROR: %s\n' "$(ts_now)" "$*" >&2
}

json_string() {
  node -p 'JSON.stringify(process.argv[1])' "$1"
}

login_api_token() {
  local username="$1"
  local password="$2"
  local response=""

  response="$(
    curl -fsS "${API_BASE_URL}/api/v1/auth/login" \
      -H "Content-Type: application/json" \
      -d "{\"username\":$(json_string "${username}"),\"password\":$(json_string "${password}")}"
  )" || return 1

  printf '%s' "${response}" | node -e '
    let raw = "";
    process.stdin.on("data", (chunk) => { raw += chunk; });
    process.stdin.on("end", () => {
      const parsed = JSON.parse(raw);
      if (typeof parsed.token !== "string" || parsed.token.trim() === "") {
        process.exit(1);
      }
      process.stdout.write(parsed.token.trim());
    });
  '
}

acquire_cleanup_token() {
  local token=""

  if token="$(login_api_token "${E2E_ADMIN_USERNAME}" "${E2E_ADMIN_PASSWORD}" 2>/dev/null)"; then
    printf '%s' "${token}"
    return 0
  fi

  if [[ -n "${E2E_NEW_PASSWORD:-}" ]]; then
    if token="$(login_api_token "${E2E_ADMIN_USERNAME}" "${E2E_NEW_PASSWORD}" 2>/dev/null)"; then
      printf '%s' "${token}"
      return 0
    fi
  fi

  return 1
}

api_get() {
  local token="$1"
  local path="$2"
  api_request_json GET "${token}" "${path}"
}

api_post_json() {
  local token="$1"
  local path="$2"
  local body="$3"
  api_request_json POST "${token}" "${path}" "${body}"
}

api_patch_json() {
  local token="$1"
  local path="$2"
  local body="$3"
  api_request_json PATCH "${token}" "${path}" "${body}"
}

api_put_json() {
  local token="$1"
  local path="$2"
  local body="$3"
  api_request_json PUT "${token}" "${path}" "${body}"
}

api_request_json() {
  local method="$1"
  local token="$2"
  local path="$3"
  local body="${4:-}"
  local response_file=""
  local http_code=""

  response_file="$(mktemp)"
  if [[ "${method}" == "GET" ]]; then
    http_code="$(
      curl -sS -o "${response_file}" -w '%{http_code}' "${API_BASE_URL}${path}" \
        -H "Authorization: Bearer ${token}"
    )" || {
      rm -f "${response_file}"
      return 1
    }
  else
    http_code="$(
      curl -sS -o "${response_file}" -w '%{http_code}' "${API_BASE_URL}${path}" \
        -X "${method}" \
        -H "Authorization: Bearer ${token}" \
        -H "Content-Type: application/json" \
        -d "${body}"
    )" || {
      rm -f "${response_file}"
      return 1
    }
  fi

  if [[ "${http_code}" =~ ^[0-9]+$ ]] && (( http_code >= 400 )); then
    log_error "API ${method} ${path} failed with HTTP ${http_code}"
    if [[ -s "${response_file}" ]]; then
      sed 's/^/response body: /' "${response_file}" >&2
    fi
    rm -f "${response_file}"
    return 22
  fi

  cat "${response_file}"
  rm -f "${response_file}"
}

extract_json_id() {
  local response="$1"
  printf '%s' "${response}" | node -e '
    let raw = "";
    process.stdin.on("data", (chunk) => { raw += chunk; });
    process.stdin.on("end", () => {
      const parsed = JSON.parse(raw);
      const id = typeof parsed.id === "string" ? parsed.id.trim() : "";
      process.stdout.write(id);
    });
  '
}

find_item_id_by_name() {
  local response="$1"
  local target_name="$2"
  printf '%s' "${response}" | node -e '
    const target = process.argv[1].trim().toLowerCase();
    let raw = "";
    process.stdin.on("data", (chunk) => { raw += chunk; });
    process.stdin.on("end", () => {
      const parsed = JSON.parse(raw);
      const items = Array.isArray(parsed.items) ? parsed.items : [];
      for (const item of items) {
        const name = typeof item.name === "string" ? item.name.trim().toLowerCase() : "";
        if (name !== target) {
          continue;
        }
        const id = typeof item.id === "string" ? item.id.trim() : "";
        process.stdout.write(id);
        return;
      }
      process.stdout.write("");
    });
  ' "${target_name}"
}

strip_name_from_payload() {
  local payload="$1"
  printf '%s' "${payload}" | node -e '
    let raw = "";
    process.stdin.on("data", (chunk) => { raw += chunk; });
    process.stdin.on("end", () => {
      const parsed = JSON.parse(raw);
      delete parsed.name;
      process.stdout.write(JSON.stringify(parsed));
    });
  '
}

build_namespace_payload() {
  node -e '
    const payload = {
      name: process.argv[1],
      environment: "test",
      description: "Live E2E namespace"
    };
    process.stdout.write(JSON.stringify(payload));
  ' "${E2E_NAMESPACE}"
}

build_system_payload() {
  node -e '
    const payload = {
      name: process.argv[1],
      description: "Live E2E system"
    };
    process.stdout.write(JSON.stringify(payload));
  ' "${E2E_SYSTEM:-e2e-system}"
}

build_service_payload() {
  node -e '
    const payload = {
      name: process.argv[1],
      description: "Live E2E service"
    };
    process.stdout.write(JSON.stringify(payload));
  ' "${E2E_SERVICE:-e2e-service}"
}

build_template_payload() {
  node - <<'EOF' "${E2E_TEMPLATE:-e2e-template}" "${E2E_NAMESPACE:-e2e-live}"
const [name, namespace] = process.argv.slice(2);
const payload = {
  name,
  display_name: "Ubuntu 22.04 (E2E)",
  description: `Live E2E Ubuntu template for namespace ${namespace}`,
  catalog_scope: "all",
  source_type: "cdi_image_import",
  image_url: "docker://quay.io/containerdisks/ubuntu:22.04",
  cloud_init: [
    "#cloud-config",
    "hostname: e2e-template",
    "manage_etc_hosts: true",
    "users:",
    "  - default",
    "package_update: false"
  ].join("\n"),
  os_family: "linux",
  os_version: "22.04",
  enabled: true
};
process.stdout.write(JSON.stringify(payload));
EOF
}

build_cluster_policy_payload() {
  node - <<'EOF' "${E2E_NAMESPACE:-e2e-live}"
const namespace = process.argv[2].trim();
const allowedCloneNamespaces = ["golden-images"];
if (namespace !== "" && namespace !== "golden-images") {
  allowedCloneNamespaces.push(namespace);
}
const payload = {
  allow_cpu_overcommit: true,
  allow_memory_overcommit: true,
  allow_dedicated_cpu: true,
  allow_gpu: true,
  allow_sriov: true,
  allow_hugepages: true,
  allowed_hugepages_sizes: ["2Mi", "1Gi"],
  allow_cdi_clone: true,
  allowed_clone_source_namespaces: allowedCloneNamespaces,
  allowed_storage_classes: []
};
process.stdout.write(JSON.stringify(payload));
EOF
}

live_instance_size_payloads() {
  node - <<'EOF' "${E2E_SIZE:-e2e-small}"
const defaultSizeName = process.argv[2];
const fixtures = [
  {
    name: defaultSizeName,
    display_name: "E2E Small",
    description: "Small baseline overcommit profile for live E2E",
    catalog_scope: "all",
    cpu_cores: 1,
    cpu_request: 0.5,
    memory_gi: 2,
    memory_request_gi: 1,
    disk_gb: 60,
    sort_order: 10,
    enabled: true
  },
  {
    name: "e2e-overcommit",
    display_name: "E2E Overcommit",
    description: "CPU and memory overcommit example for live E2E",
    catalog_scope: "all",
    cpu_cores: 4,
    cpu_request: 2,
    memory_gi: 8,
    memory_request_gi: 6,
    disk_gb: 80,
    sort_order: 20,
    enabled: true
  },
  {
    name: "e2e-dedicated",
    display_name: "E2E Dedicated CPU",
    description: "Dedicated CPU example for live E2E",
    catalog_scope: "all",
    cpu_cores: 4,
    cpu_request: 4,
    memory_gi: 8,
    memory_request_gi: 8,
    disk_gb: 80,
    dedicated_cpu: true,
    spec_overrides: {
      spec: {
        template: {
          spec: {
            domain: {
              cpu: {
                dedicatedCpuPlacement: true
              }
            }
          }
        }
      }
    },
    sort_order: 30,
    enabled: true
  },
  {
    name: "e2e-gpu",
    display_name: "E2E GPU",
    description: "GPU workload example for live E2E",
    catalog_scope: "all",
    cpu_cores: 8,
    memory_gi: 16,
    disk_gb: 120,
    requires_gpu: true,
    spec_overrides: {
      spec: {
        template: {
          spec: {
            domain: {
              devices: {
                gpus: [{ name: "gpu0", deviceName: "nvidia.com/A10" }]
              }
            }
          }
        }
      }
    },
    sort_order: 40,
    enabled: true
  },
  {
    name: "e2e-hugepages",
    display_name: "E2E Hugepages",
    description: "Hugepages workload example for live E2E",
    catalog_scope: "all",
    cpu_cores: 4,
    memory_gi: 8,
    disk_gb: 80,
    requires_hugepages: true,
    hugepages_size: "2Mi",
    spec_overrides: {
      spec: {
        template: {
          spec: {
            domain: {
              memory: {
                hugepages: {
                  pageSize: "2Mi"
                }
              }
            }
          }
        }
      }
    },
    sort_order: 50,
    enabled: true
  },
  {
    name: "e2e-sriov",
    display_name: "E2E SR-IOV",
    description: "SR-IOV workload example for live E2E",
    catalog_scope: "all",
    cpu_cores: 4,
    memory_gi: 8,
    disk_gb: 80,
    requires_sriov: true,
    spec_overrides: {
      spec: {
        template: {
          spec: {
            domain: {
              devices: {
                interfaces: [{ name: "sriov-net-1" }]
              }
            }
          }
        }
      }
    },
    sort_order: 60,
    enabled: true
  }
];
for (const fixture of fixtures) {
  process.stdout.write(JSON.stringify(fixture) + "\n");
}
EOF
}

ensure_namespace_api() {
  local token="$1"
  local existing=""
  local response=""

  if ! response="$(api_get "${token}" "/api/v1/admin/namespaces?page=1&per_page=100")"; then
    return 1
  fi
  existing="$(find_item_id_by_name "${response}" "${E2E_NAMESPACE}")"
  if [[ -n "${existing}" ]]; then
    printf '%s' "${existing}"
    return 0
  fi

  if ! response="$(api_post_json "${token}" "/api/v1/admin/namespaces" "$(build_namespace_payload)")"; then
    return 1
  fi
  extract_json_id "${response}"
}

ensure_system_api() {
  local token="$1"
  local system_name="${E2E_SYSTEM:-e2e-system}"
  local existing=""
  local response=""

  if ! response="$(api_get "${token}" "/api/v1/systems?page=1&per_page=100")"; then
    return 1
  fi
  existing="$(find_item_id_by_name "${response}" "${system_name}")"
  if [[ -n "${existing}" ]]; then
    printf '%s' "${existing}"
    return 0
  fi

  if ! response="$(api_post_json "${token}" "/api/v1/systems" "$(build_system_payload)")"; then
    return 1
  fi
  extract_json_id "${response}"
}

ensure_service_api() {
  local token="$1"
  local system_id="$2"
  local service_name="${E2E_SERVICE:-e2e-service}"
  local existing=""
  local response=""

  if ! response="$(api_get "${token}" "/api/v1/systems/${system_id}/services?page=1&per_page=100")"; then
    return 1
  fi
  existing="$(find_item_id_by_name "${response}" "${service_name}")"
  if [[ -n "${existing}" ]]; then
    printf '%s' "${existing}"
    return 0
  fi

  if ! response="$(api_post_json "${token}" "/api/v1/systems/${system_id}/services" "$(build_service_payload)")"; then
    return 1
  fi
  extract_json_id "${response}"
}

ensure_cluster_api() {
  local token="$1"
  local cluster_name="${E2E_CLUSTER:-e2e-cluster}"
  local existing=""
  local response=""
  local payload=""

  if ! response="$(api_get "${token}" "/api/v1/admin/clusters?page=1&per_page=100")"; then
    return 1
  fi
  existing="$(find_item_id_by_name "${response}" "${cluster_name}")"
  if [[ -n "${existing}" ]]; then
    printf '%s' "${existing}"
    return 0
  fi

  payload="$(node -e '
    const payload = {
      name: process.argv[1],
      display_name: "E2E Cluster",
      environment: "test",
      kubeconfig: process.argv[2]
    };
    process.stdout.write(JSON.stringify(payload));
  ' "${cluster_name}" "${E2E_KUBECONFIG_B64}")"
  if ! response="$(api_post_json "${token}" "/api/v1/admin/clusters" "${payload}")"; then
    return 1
  fi
  extract_json_id "${response}"
}

upsert_cluster_policy_api() {
  local token="$1"
  local cluster_id="$2"
  api_put_json "${token}" "/api/v1/admin/clusters/${cluster_id}/policy" "$(build_cluster_policy_payload)" >/dev/null
}

upsert_template_api() {
  local token="$1"
  local template_name="${E2E_TEMPLATE:-e2e-template}"
  local existing_id=""
  local response=""
  local payload=""

  if ! response="$(api_get "${token}" "/api/v1/admin/templates?page=1&per_page=100")"; then
    return 1
  fi
  existing_id="$(find_item_id_by_name "${response}" "${template_name}")"
  payload="$(build_template_payload)"
  if [[ -n "${existing_id}" ]]; then
    api_patch_json "${token}" "/api/v1/admin/templates/${existing_id}" "$(strip_name_from_payload "${payload}")" >/dev/null
    return 0
  fi

  api_post_json "${token}" "/api/v1/admin/templates" "${payload}" >/dev/null
}

upsert_instance_size_api() {
  local token="$1"
  local payload="$2"
  local name=""
  local existing_id=""
  local response=""

  name="$(printf '%s' "${payload}" | node -e '
    let raw = "";
    process.stdin.on("data", (chunk) => { raw += chunk; });
    process.stdin.on("end", () => {
      const parsed = JSON.parse(raw);
      process.stdout.write(typeof parsed.name === "string" ? parsed.name.trim() : "");
    });
  ')"
  if ! response="$(api_get "${token}" "/api/v1/admin/instance-sizes")"; then
    return 1
  fi
  existing_id="$(find_item_id_by_name "${response}" "${name}")"
  if [[ -n "${existing_id}" ]]; then
    api_patch_json "${token}" "/api/v1/admin/instance-sizes/${existing_id}" "${payload}" >/dev/null
    return 0
  fi

  api_post_json "${token}" "/api/v1/admin/instance-sizes" "${payload}" >/dev/null
}

seed_live_api_managed_fixtures() {
  local token="$1"
  local namespace_id=""
  local system_id=""
  local service_id=""
  local cluster_id=""
  local payload=""

  log_info "seeding namespace fixture (${E2E_NAMESPACE})"
  namespace_id="$(ensure_namespace_api "${token}")"
  log_info "seeding system fixture (${E2E_SYSTEM:-e2e-system})"
  system_id="$(ensure_system_api "${token}")"
  log_info "seeding service fixture (${E2E_SERVICE:-e2e-service})"
  service_id="$(ensure_service_api "${token}" "${system_id}")"
  log_info "seeding cluster fixture (${E2E_CLUSTER:-e2e-cluster})"
  cluster_id="$(ensure_cluster_api "${token}")"
  log_info "upserting cluster policy for cluster ${cluster_id}"
  upsert_cluster_policy_api "${token}" "${cluster_id}"
  log_info "upserting template fixture (${E2E_TEMPLATE:-e2e-template})"
  upsert_template_api "${token}"

  while IFS= read -r payload; do
    [[ -z "${payload}" ]] && continue
    log_info "upserting instance size fixture ($(printf '%s' "${payload}" | node -e 'let raw = ""; process.stdin.on("data", (chunk) => { raw += chunk; }); process.stdin.on("end", () => { const parsed = JSON.parse(raw); process.stdout.write(typeof parsed.name === "string" ? parsed.name : ""); });'))"
    upsert_instance_size_api "${token}" "${payload}"
  done < <(live_instance_size_payloads)

  log_info "seeded API-managed live fixtures (namespace=${namespace_id} system=${system_id} service=${service_id} cluster=${cluster_id})"
}

list_namespace_vms() {
  local token="$1"
  local namespace="$2"
  local page=1
  local per_page=100
  local response=""
  local total_pages=1

  while (( page <= total_pages )); do
    response="$(
      curl -fsS -G "${API_BASE_URL}/api/v1/vms" \
        --data-urlencode "page=${page}" \
        --data-urlencode "per_page=${per_page}" \
        --data-urlencode "namespace=${namespace}" \
        -H "Authorization: Bearer ${token}"
    )" || return 1

    printf '%s' "${response}" | node -e '
      let raw = "";
      process.stdin.on("data", (chunk) => { raw += chunk; });
      process.stdin.on("end", () => {
        const parsed = JSON.parse(raw);
        for (const item of parsed.items ?? []) {
          const id = typeof item.id === "string" ? item.id.trim() : "";
          if (id === "") {
            continue;
          }
          const name = typeof item.name === "string" ? item.name.trim() : "";
          const status = typeof item.status === "string" ? item.status.trim() : "";
          process.stdout.write(`${id}\t${name}\t${status}\n`);
        }
      });
    '

    total_pages="$(printf '%s' "${response}" | node -e '
      let raw = "";
      process.stdin.on("data", (chunk) => { raw += chunk; });
      process.stdin.on("end", () => {
        const parsed = JSON.parse(raw);
        const totalPages = Number(parsed.pagination?.total_pages ?? 1);
        process.stdout.write(String(Number.isFinite(totalPages) && totalPages > 0 ? totalPages : 1));
      });
    ')"
    page=$((page + 1))
  done
}

wait_for_vm_status() {
  local token="$1"
  local vm_id="$2"
  local expected_status="$3"
  local response=""
  local current_status=""

  for _ in $(seq 1 40); do
    response="$(curl -fsS "${API_BASE_URL}/api/v1/vms/${vm_id}" -H "Authorization: Bearer ${token}" 2>/dev/null || true)"
    current_status="$(printf '%s' "${response}" | node -e '
      let raw = "";
      process.stdin.on("data", (chunk) => { raw += chunk; });
      process.stdin.on("end", () => {
        if (raw.trim() === "") {
          process.stdout.write("");
          return;
        }
        try {
          const parsed = JSON.parse(raw);
          process.stdout.write(typeof parsed.status === "string" ? parsed.status.trim().toUpperCase() : "");
        } catch {
          process.stdout.write("");
        }
      });
    ')"
    if [[ "${current_status}" == "${expected_status}" ]]; then
      return 0
    fi
    sleep 2
  done

  return 1
}

wait_for_vm_absent() {
  local token="$1"
  local vm_id="$2"

  for _ in $(seq 1 60); do
    if ! curl -fsS "${API_BASE_URL}/api/v1/vms/${vm_id}" -H "Authorization: Bearer ${token}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  return 1
}

cleanup_namespace_vms() {
  local enabled="${E2E_CLEANUP_NAMESPACE_VMS:-1}"
  local namespace="${E2E_NAMESPACE:-}"
  local token=""
  local cleanup_failed=0
  local vm_entries=""
  local delete_response=""
  local ticket_id=""

  if [[ "${enabled}" == "0" ]]; then
    log_warn "skipping namespace VM cleanup (E2E_CLEANUP_NAMESPACE_VMS=0)"
    return 0
  fi
  if [[ -z "${namespace}" ]]; then
    log_warn "skipping namespace VM cleanup because E2E_NAMESPACE is empty"
    return 0
  fi

  if ! token="$(acquire_cleanup_token)"; then
    log_warn "failed to acquire API token for namespace VM cleanup"
    return 1
  fi

  log_info "cleaning up VMs in namespace ${namespace}"

  if ! vm_entries="$(list_namespace_vms "${token}" "${namespace}")"; then
    log_warn "failed to list VMs for namespace cleanup"
    return 1
  fi

  while IFS=$'\t' read -r vm_id vm_name vm_status; do
    [[ -z "${vm_id}" ]] && continue
    vm_status="$(printf '%s' "${vm_status}" | tr '[:lower:]' '[:upper:]')"

    if [[ "${vm_status}" == "RUNNING" || "${vm_status}" == "STARTING" || "${vm_status}" == "UNKNOWN" ]]; then
      if curl -fsS "${API_BASE_URL}/api/v1/vms/${vm_id}/stop" \
        -X POST \
        -H "Authorization: Bearer ${token}" >/dev/null 2>&1; then
        if ! wait_for_vm_status "${token}" "${vm_id}" "STOPPED"; then
          log_warn "timed out waiting for VM ${vm_id} to stop before cleanup delete"
          cleanup_failed=1
          continue
        fi
      else
        log_warn "failed to request stop for VM ${vm_id} during cleanup"
        cleanup_failed=1
        continue
      fi
    fi

    delete_response="$(
      curl -fsS "${API_BASE_URL}/api/v1/vms/${vm_id}?confirm=true" \
        -X DELETE \
        -H "Authorization: Bearer ${token}" 2>/dev/null || true
    )"
    ticket_id="$(printf '%s' "${delete_response}" | node -e '
      let raw = "";
      process.stdin.on("data", (chunk) => { raw += chunk; });
      process.stdin.on("end", () => {
        if (raw.trim() === "") {
          process.stdout.write("");
          return;
        }
        try {
          const parsed = JSON.parse(raw);
          const ticketID = typeof parsed.ticket_id === "string" ? parsed.ticket_id.trim() : "";
          process.stdout.write(ticketID);
        } catch {
          process.stdout.write("");
        }
      });
    ')"
    if [[ -z "${ticket_id}" ]]; then
      log_warn "failed to create delete approval for VM ${vm_id} during cleanup"
      cleanup_failed=1
      continue
    fi

    if ! curl -fsS "${API_BASE_URL}/api/v1/approvals/${ticket_id}/approve" \
      -X POST \
      -H "Authorization: Bearer ${token}" \
      -H "Content-Type: application/json" \
      -d '{}' >/dev/null 2>&1; then
      log_warn "failed to approve delete ticket ${ticket_id} for VM ${vm_id}"
      cleanup_failed=1
      continue
    fi

    if ! wait_for_vm_absent "${token}" "${vm_id}"; then
      log_warn "timed out waiting for VM ${vm_id} to disappear after cleanup approval"
      cleanup_failed=1
    fi
  done <<< "${vm_entries}"

  return "${cleanup_failed}"
}

run_live_e2e_preflight_checks() {
  local skip="${E2E_SKIP_PREFLIGHT_GATES:-0}"
  if [[ "${skip}" == "1" ]]; then
    log_warn "skipping live-e2e preflight gates (E2E_SKIP_PREFLIGHT_GATES=1)"
    return 0
  fi

  log_info "running master-flow test matrix gate (includes live_step_markers checks)"
  go run docs/design/ci/scripts/check_master_flow_test_matrix.go

  log_info "running live-e2e no-mock policy gate"
  bash docs/design/ci/scripts/check_live_e2e_no_mock.sh
}

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

NO_DB_WRAPPER=0
BACKGROUND=0
FOREGROUND=0
STATUS_ONLY=0
BG_LOG_FILE=""
BG_PID_FILE=""
BG_RESULT_FILE=""
BG_OUTPUT_DIR="${ROOT_DIR}/.run/live-e2e"
BG_STATE_FILE=""
PASSTHRU_ARGS=()

usage() {
  cat <<'EOF'
Usage:
  scripts/run_e2e_live.sh [options] [-- playwright-args...]

Options:
  (default)              Start detached in background, run with a fresh ephemeral PG18 container, write files into .run/live-e2e/
  --foreground           Run in current shell (useful for CI or debugging)
  --no-db-wrapper        Use existing DATABASE_URL instead of auto-starting Docker PostgreSQL
  --background           Force detached background mode
  --status               Read run status only (no log content), for low-token polling
  --output-dir <path>    Background run output root (default: .run/live-e2e/, subfolders: YYYYMMDD/HHMM)
  --log-file <path>      Background mode log file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.log)
  --pid-file <path>      Background mode pid file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.pid)
  --result-file <path>   Background mode result file path (default: <output-dir>/YYYYMMDD/HHMM/live-e2e.result)
  --state-file <path>    Status metadata file (default: <output-dir>/latest.env)
  -h, --help             Show this help

Environment:
  E2E_SKIP_PREFLIGHT_GATES=1
                         Skip preflight gates (master-flow test matrix + live-e2e no-mock policy)
  E2E_BACKEND_CRITICAL_GUARD=0
                         Disable backend critical-log guard (default: enabled)
  E2E_BACKEND_STRICT_GUARD=1
                         Enable strict backend-log patterns (default: disabled to reduce false positives)
  E2E_BACKEND_ERROR_ALLOWLIST_REGEX=<regex>
                         Ignore matching backend-log lines in guard checks
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-db-wrapper)
      NO_DB_WRAPPER=1
      shift
      ;;
    --background)
      BACKGROUND=1
      shift
      ;;
    --foreground)
      FOREGROUND=1
      shift
      ;;
    --status)
      STATUS_ONLY=1
      shift
      ;;
    --output-dir)
      BG_OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --log-file)
      BG_LOG_FILE="${2:-}"
      shift 2
      ;;
    --pid-file)
      BG_PID_FILE="${2:-}"
      shift 2
      ;;
    --result-file)
      BG_RESULT_FILE="${2:-}"
      shift 2
      ;;
    --state-file)
      BG_STATE_FILE="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      PASSTHRU_ARGS+=("$@")
      break
      ;;
    *)
      PASSTHRU_ARGS+=("$1")
      shift
      ;;
  esac
done

BG_STATE_FILE="${BG_STATE_FILE:-${BG_OUTPUT_DIR}/latest.env}"

extract_result_summary() {
  local log_file="$1"
  local line failed flaky passed total pass_rate

  line="$(rg -N "Playwright Run Summary::" "${log_file}" 2>/dev/null | tail -n 1 || true)"
  if [[ -z "${line}" ]]; then
    return 1
  fi

  failed="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ failed' | head -n 1 | awk '{print $1}')"
  flaky="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ flaky' | head -n 1 | awk '{print $1}')"
  passed="$(printf '%s\n' "${line}" | rg -oN '[0-9]+ passed' | head -n 1 | awk '{print $1}')"
  failed="${failed:-0}"
  flaky="${flaky:-0}"
  passed="${passed:-0}"
  total=$((failed + flaky + passed))
  if (( total > 0 )); then
    pass_rate="$(awk -v p="${passed}" -v t="${total}" 'BEGIN { printf "%.2f", (p*100)/t }')"
  else
    pass_rate="0.00"
  fi

  echo "failed=${failed} flaky=${flaky} passed=${passed} total=${total} pass_rate=${pass_rate}%"
}

check_backend_critical_errors() {
  local log_file="$1"
  local allow_regex="${E2E_BACKEND_ERROR_ALLOWLIST_REGEX:-}"
  # Strict backend pattern gate is opt-in to avoid CI flakiness from
  # environment-dependent warnings unrelated to user-visible regressions.
  local strict_guard="${E2E_BACKEND_STRICT_GUARD:-0}"
  local findings=""
  local pattern matched

  if [[ ! -f "${log_file}" ]]; then
    log_warn "backend log file not found for critical-error guard: ${log_file}"
    return 0
  fi

  # High-signal backend failures that should fail live E2E even when Playwright UI
  # assertions happen to pass.
  local -a critical_patterns=(
    "OpenAPI response validation failed"
    "OpenAPI validator setup failed"
    "failed to send APPROVAL_(PENDING|COMPLETED|REJECTED) notification"
    "violates foreign key constraint \\\"notifications_users_notifications\\\""
    "panic:"
    "fatal error:"
  )

  # Strict guard (master-flow aligned): detect latent backend behavior drift that
  # often slips past page-level assertions.
  local -a strict_patterns=(
    "Cluster health check failed"
    "jobexecutor.JobExecutor: Job errored"
    "no approvers found for notification"
  )

  for pattern in "${critical_patterns[@]}"; do
    matched="$(rg -n --no-heading -e "${pattern}" "${log_file}" || true)"
    if [[ -n "${matched}" ]]; then
      findings+="${matched}"$'\n'
    fi
  done

  if [[ "${strict_guard}" != "0" ]]; then
    for pattern in "${strict_patterns[@]}"; do
      matched="$(rg -n --no-heading -e "${pattern}" "${log_file}" || true)"
      if [[ -n "${matched}" ]]; then
        findings+="${matched}"$'\n'
      fi
    done
  fi

  if [[ -z "${findings}" ]]; then
    return 0
  fi

  # De-duplicate and drop empty lines.
  findings="$(printf '%s' "${findings}" | awk 'NF && !seen[$0]++')"

  if [[ -n "${allow_regex}" ]]; then
    findings="$(printf '%s\n' "${findings}" | rg -v -e "${allow_regex}" || true)"
  fi

  if [[ -n "${findings//[[:space:]]/}" ]]; then
    log_error "critical backend errors detected in ${log_file}"
    printf '%s\n' "${findings}" | sed 's/^/  /' >&2
    log_error "set E2E_BACKEND_ERROR_ALLOWLIST_REGEX to suppress known non-blocking signatures"
    log_error "set E2E_BACKEND_STRICT_GUARD=0 to disable strict backend pattern gate"
    return 1
  fi

  return 0
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    if ss -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:|\\.)${port}$"; then
      return 0
    fi
  fi

  if command -v lsof >/dev/null 2>&1; then
    if lsof -nP -iTCP:"${port}" -sTCP:LISTEN >/dev/null 2>&1; then
      return 0
    fi
  fi

  if command -v netstat >/dev/null 2>&1; then
    if netstat -ltn 2>/dev/null | awk '{print $4}' | grep -Eq "(^|:|\\.)${port}$"; then
      return 0
    fi
  fi

  return 1
}

port_listener_pids() {
  local port="$1"
  local out=""

  if command -v ss >/dev/null 2>&1; then
    out="$(ss -ltnp "sport = :${port}" 2>/dev/null | sed -n 's/.*pid=\([0-9]\+\).*/\1/p' | sort -u | tr '\n' ' ')"
  elif command -v lsof >/dev/null 2>&1; then
    out="$(lsof -nP -iTCP:"${port}" -sTCP:LISTEN -t 2>/dev/null | sort -u | tr '\n' ' ')"
  fi

  echo "${out}"
}

cleanup_next_web_port() {
  local port="${1:-}"
  local pid cmd cwd ppid pcmd
  if [[ -z "${port}" ]]; then
    return 0
  fi

  for pid in $(port_listener_pids "${port}"); do
    [[ -z "${pid}" ]] && continue
    if ! ps -p "${pid}" -o pid= >/dev/null 2>&1; then
      continue
    fi
    cmd="$(ps -p "${pid}" -o cmd= 2>/dev/null || true)"
    cwd="$(readlink -f "/proc/${pid}/cwd" 2>/dev/null || true)"
    ppid="$(ps -p "${pid}" -o ppid= 2>/dev/null | tr -d ' ' || true)"
    pcmd=""
    if [[ -n "${ppid}" ]] && ps -p "${ppid}" -o pid= >/dev/null 2>&1; then
      pcmd="$(ps -p "${ppid}" -o cmd= 2>/dev/null || true)"
    fi

    # Only kill Next.js listeners started from this repo's web workspace (or their matching parent).
    if [[ "${cwd}" != "${ROOT_DIR}/web" ]] && [[ "${pcmd}" != *"--port ${port}"* ]]; then
      continue
    fi

    if [[ "${cmd}" == *"next-server"* ]] || [[ "${pcmd}" == *"next "* ]] || [[ "${pcmd}" == *"node "*".bin/next"* ]]; then
      log_warn "cleaning residual Next.js listener on port ${port} (pid=${pid})"
      kill "${pid}" >/dev/null 2>&1 || true
      if [[ -n "${ppid}" ]] && [[ "${pcmd}" == *"--port ${port}"* ]] && ([[ "${pcmd}" == *"next "* ]] || [[ "${pcmd}" == *".bin/next"* ]]); then
        kill "${ppid}" >/dev/null 2>&1 || true
      fi
    fi
  done

  for _ in $(seq 1 20); do
    if ! port_in_use "${port}"; then
      break
    fi
    sleep 0.2
  done
}

cleanup_residual_pg_on_port() {
  local target_port="$1"
  local cid image network_mode host_ports args
  local to_remove=()

  if ! command -v docker >/dev/null 2>&1; then
    return 0
  fi

  while IFS= read -r cid; do
    [[ -z "${cid}" ]] && continue
    image="$(docker inspect -f '{{.Config.Image}}' "${cid}" 2>/dev/null || true)"
    if [[ "${image}" != postgres* ]]; then
      continue
    fi

    host_ports="$(docker inspect -f '{{range $k, $v := .NetworkSettings.Ports}}{{if $v}}{{(index $v 0).HostPort}} {{end}}{{end}}' "${cid}" 2>/dev/null || true)"
    network_mode="$(docker inspect -f '{{.HostConfig.NetworkMode}}' "${cid}" 2>/dev/null || true)"
    args="$(docker inspect -f '{{range .Args}}{{.}} {{end}}' "${cid}" 2>/dev/null || true)"

    if [[ " ${host_ports} " == *" ${target_port} "* ]]; then
      to_remove+=("${cid}")
      continue
    fi

    if [[ "${network_mode}" == "host" ]] && [[ " ${args} " == *" -p ${target_port} "* ]]; then
      to_remove+=("${cid}")
      continue
    fi
  done < <(docker ps -aq 2>/dev/null || true)

  if (( ${#to_remove[@]} > 0 )); then
    log_info "removing residual PostgreSQL containers on port ${target_port}: ${to_remove[*]}"
    docker rm -f "${to_remove[@]}" >/dev/null 2>&1 || true
  fi
}

# Remove any residual shepherd E2E containers left by a previous aborted run.
# Matches containers whose name starts with shepherd-e2e-* or shepherd-test-*.
# Called unconditionally before the DB wrapper so stale containers cannot
# occupy the test port and cause a confusing "port in use" error.
cleanup_residual_e2e_containers() {
  if ! command -v docker > /dev/null 2>&1; then
    return 0
  fi

  local cid name
  local to_remove=()

  while IFS= read -r cid; do
    [[ -z "${cid}" ]] && continue
    name="$(docker inspect -f '{{.Name}}' "${cid}" 2>/dev/null | sed 's|^/||' || true)"
    if [[ "${name}" == shepherd-e2e-* || "${name}" == shepherd-test-* ]]; then
      to_remove+=("${cid}")
    fi
  done < <(docker ps -aq 2>/dev/null || true)

  if (( ${#to_remove[@]} > 0 )); then
    log_info "removing ${#to_remove[@]} residual shepherd E2E container(s): ${to_remove[*]}"
    docker rm -f "${to_remove[@]}" > /dev/null 2>&1 || true
  fi
}

if [[ "${BACKGROUND}" -eq 1 && "${FOREGROUND}" -eq 1 ]]; then
  log_error "--background and --foreground cannot be used together"
  exit 1
fi

if [[ "${STATUS_ONLY}" -eq 0 && "${BACKGROUND}" -eq 0 && "${FOREGROUND}" -eq 0 ]]; then
  BACKGROUND=1
fi

if [[ "$STATUS_ONLY" -eq 1 ]]; then
  if [[ -z "${BG_LOG_FILE}" || -z "${BG_PID_FILE}" ]]; then
    if [[ -f "${BG_STATE_FILE}" ]]; then
      # shellcheck disable=SC1090
      source "${BG_STATE_FILE}"
    fi
  fi

  if [[ -z "${BG_LOG_FILE}" || -z "${BG_PID_FILE}" ]]; then
    log_error "status mode needs --log-file and --pid-file, or a valid --state-file"
    exit 1
  fi

  pid="N/A"
  running="no"
  if [[ -f "${BG_PID_FILE}" ]]; then
    pid="$(cat "${BG_PID_FILE}" 2>/dev/null || echo N/A)"
  fi
  if [[ "${pid}" != "N/A" ]] && kill -0 "${pid}" >/dev/null 2>&1; then
    running="yes"
  fi

  log_size_bytes=0
  log_mtime="N/A"
  if [[ -f "${BG_LOG_FILE}" ]]; then
    log_size_bytes="$(wc -c <"${BG_LOG_FILE}")"
    log_mtime="$(stat -c '%y' "${BG_LOG_FILE}" | cut -d'.' -f1)"
  fi

  echo "STATUS: running=${running} pid=${pid} log=${BG_LOG_FILE} size_bytes=${log_size_bytes} updated_at=${log_mtime}"

  if [[ "${running}" == "no" ]]; then
    if [[ -z "${BG_RESULT_FILE}" ]]; then
      if [[ -f "${BG_STATE_FILE}" ]]; then
        # shellcheck disable=SC1090
        source "${BG_STATE_FILE}"
      fi
    fi
    if [[ -n "${BG_RESULT_FILE}" && -f "${BG_RESULT_FILE}" ]]; then
      echo "RESULT_FILE: ${BG_RESULT_FILE}"
      cat "${BG_RESULT_FILE}"
    elif [[ -f "${BG_LOG_FILE}" ]] && result_line="$(extract_result_summary "${BG_LOG_FILE}")"; then
      echo "RESULT: ${result_line}"
    else
      echo "RESULT: summary line not found in log"
    fi
  fi
  exit 0
fi

if [[ "$BACKGROUND" -eq 1 ]]; then
  run_day="$(date +%Y%m%d)"
  run_hm="$(date +%H%M)"
  run_dir_base="${BG_OUTPUT_DIR}/${run_day}/${run_hm}"
  run_dir="${run_dir_base}"
  suffix=1
  while [[ -e "${run_dir}" ]]; do
    run_dir="${run_dir_base}-${suffix}"
    suffix=$((suffix + 1))
  done

  mkdir -p "${BG_OUTPUT_DIR}"
  mkdir -p "${run_dir}"
  BG_LOG_FILE="${BG_LOG_FILE:-${run_dir}/live-e2e.log}"
  BG_PID_FILE="${BG_PID_FILE:-${run_dir}/live-e2e.pid}"
  BG_RESULT_FILE="${BG_RESULT_FILE:-${run_dir}/live-e2e.result}"
  BG_STATE_FILE="${BG_STATE_FILE:-${BG_OUTPUT_DIR}/latest.env}"
  mkdir -p "$(dirname "${BG_LOG_FILE}")"
  mkdir -p "$(dirname "${BG_PID_FILE}")"
  mkdir -p "$(dirname "${BG_RESULT_FILE}")"
  mkdir -p "$(dirname "${BG_STATE_FILE}")"

  CMD=(bash "${ROOT_DIR}/scripts/run_e2e_live.sh")
  CMD+=(--foreground)
  if [[ "$NO_DB_WRAPPER" -eq 1 ]]; then
    CMD+=(--no-db-wrapper)
  fi
  if [[ "${#PASSTHRU_ARGS[@]}" -gt 0 ]]; then
    CMD+=(-- "${PASSTHRU_ARGS[@]}")
  fi

  RUN_RESULT_FILE="${BG_RESULT_FILE}" RUN_LOG_FILE="${BG_LOG_FILE}" nohup "${CMD[@]}" >"${BG_LOG_FILE}" 2>&1 &
  bg_pid=$!
  echo "${bg_pid}" >"${BG_PID_FILE}"
  cat >"${BG_STATE_FILE}" <<EOF
BG_LOG_FILE=${BG_LOG_FILE}
BG_PID_FILE=${BG_PID_FILE}
BG_RESULT_FILE=${BG_RESULT_FILE}
BG_RUN_DIR=${run_dir}
EOF
  log_info "started background live e2e run"
  log_info "pid=${bg_pid}"
  log_info "output root: ${BG_OUTPUT_DIR}"
  log_info "run dir: ${run_dir}"
  log_info "pid file: ${BG_PID_FILE}"
  log_info "log file: ${BG_LOG_FILE}"
  log_info "result file: ${BG_RESULT_FILE}"
  log_info "reminder: poll status every 5 minutes (not log content) until completion"
  log_info "command: bash scripts/run_e2e_live.sh --status --state-file ${BG_STATE_FILE}"
  exit 0
fi

set -- "${PASSTHRU_ARGS[@]}"

if [[ "$NO_DB_WRAPPER" -eq 0 ]]; then
  # Live E2E default: always run against an isolated PostgreSQL test container.
  PG_IMAGE="${PG_IMAGE:-postgres:18}"
  E2E_PG_PORT="${E2E_PG_PORT:-55432}"
  # First, sweep ALL shepherd E2E/test containers (catches stale DB containers
  # from any previous aborted run regardless of port configuration).
  cleanup_residual_e2e_containers
  # Then do the port-specific check as a safety net.
  cleanup_residual_pg_on_port "${E2E_PG_PORT}"
  if port_in_use "${E2E_PG_PORT}"; then
    log_error "port ${E2E_PG_PORT} is still in use after residual cleanup; stop the process and retry"
    exit 1
  fi
  exec env PG_PORT="${E2E_PG_PORT}" ./scripts/run_with_docker_pg.sh --image "${PG_IMAGE}" -- bash ./scripts/run_e2e_live.sh --foreground --no-db-wrapper "$@"
fi

if [[ -z "${DATABASE_URL:-}" ]]; then
  log_error "DATABASE_URL is required when --no-db-wrapper is set"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  log_error "curl command not found"
  exit 1
fi

if ! command -v node >/dev/null 2>&1; then
  log_error "node command not found"
  exit 1
fi

pick_free_port() {
  local candidate
  for _ in $(seq 1 80); do
    candidate=$((RANDOM % 10000 + 18080))
    if ! port_in_use "$candidate"; then
      echo "$candidate"
      return 0
    fi
  done
  return 1
}

if [[ -n "${SERVER_PORT:-}" ]]; then
  SERVER_PORT="$SERVER_PORT"
elif [[ -n "${E2E_BACKEND_PORT:-}" ]]; then
  SERVER_PORT="$E2E_BACKEND_PORT"
else
  SERVER_PORT="$(pick_free_port || true)"
  if [[ -z "$SERVER_PORT" ]]; then
    log_error "unable to allocate free backend port"
    exit 1
  fi
fi

API_BASE_URL="${API_BASE_URL:-http://127.0.0.1:${SERVER_PORT}}"
INTERNAL_API_URL="${INTERNAL_API_URL:-http://127.0.0.1:${SERVER_PORT}}"
# Resolve the run directory: prefer the parent process's BG_RUN_DIR (set when launched
# via --background), fall back to an isolated timestamped directory so that direct
# --foreground / --no-db-wrapper invocations also land in .run/ and never collide.
E2E_RUN_DIR="${BG_RUN_DIR:-${BG_OUTPUT_DIR}/$(date +%Y%m%d)/$(date +%H%M)-$$}"
mkdir -p "${E2E_RUN_DIR}"
SERVER_LOG="${E2E_SERVER_LOG:-${E2E_RUN_DIR}/shepherd-e2e-server.log}"
SERVER_BIN="${E2E_SERVER_BIN:-${E2E_RUN_DIR}/shepherd-e2e-server-bin}"
# Use a per-run Next.js dist directory to avoid lock contention on
# web/.next-e2e/dev/lock when another dev server is alive or a stale lock exists.
E2E_RUN_ID="$(basename "${E2E_RUN_DIR}")"
NEXT_DIST_DIR="${NEXT_DIST_DIR:-.next-e2e/${E2E_RUN_ID}}"
# Keep Next.js tsconfig auto-mutations away from repository files during live E2E.
NEXT_TSCONFIG_PATH="${NEXT_TSCONFIG_PATH:-.next-e2e/tsconfig.e2e.json}"
WEB_NEXT_TSCONFIG_PATH="${ROOT_DIR}/web/${NEXT_TSCONFIG_PATH}"
mkdir -p "$(dirname "${WEB_NEXT_TSCONFIG_PATH}")"
cat > "${WEB_NEXT_TSCONFIG_PATH}" <<'EOF'
{
  "extends": "../tsconfig.json"
}
EOF
log_info "run directory : ${E2E_RUN_DIR}"
log_info "backend log   : ${SERVER_LOG}"
log_info "next dist dir : ${NEXT_DIST_DIR}"
log_info "next tsconfig : ${NEXT_TSCONFIG_PATH}"
# Use same-origin API path by default to avoid browser CORS between Playwright web port
# and backend random port. Next.js rewrite (INTERNAL_API_URL) forwards /api/v1 to backend.
# Keep env override support for explicit direct-base testing when needed.
NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-/api/v1}"
if [[ -n "${PW_WEB_PORT:-}" ]]; then
  PW_WEB_PORT="$PW_WEB_PORT"
else
  PW_WEB_PORT="$(pick_free_port || true)"
  if [[ -z "$PW_WEB_PORT" ]]; then
    log_error "unable to allocate free Playwright web port"
    exit 1
  fi
fi
PW_BASE_URL="${PW_BASE_URL:-http://127.0.0.1:${PW_WEB_PORT}}"

export SERVER_PORT
export INTERNAL_API_URL
export NEXT_PUBLIC_API_URL
export NEXT_DIST_DIR
export NEXT_TSCONFIG_PATH
export PW_WEB_PORT
export PW_BASE_URL
# Expose run directory to Playwright config (used for webServer stdout/stderr logs).
export E2E_RUN_DIR
export DATABASE_AUTO_MIGRATE="${DATABASE_AUTO_MIGRATE:-true}"
export SECURITY_SESSION_SECRET="${SECURITY_SESSION_SECRET:-0123456789abcdef0123456789abcdef}"
export SECURITY_ENCRYPTION_KEY="${SECURITY_ENCRYPTION_KEY:-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef}"
# Strict live e2e runs on random Playwright web ports; allow all origins in this
# test harness to prevent CORS false negatives unrelated to product behavior.
export SERVER_UNSAFE_ALLOW_ALL_ORIGINS="${SERVER_UNSAFE_ALLOW_ALL_ORIGINS:-true}"
# Unified live E2E auth defaults (master-flow first-login reality):
# default account is admin/admin, and first-login password change target is admin123.
DEFAULT_E2E_USERNAME="${E2E_USERNAME:-${E2E_ADMIN_USERNAME:-admin}}"
DEFAULT_E2E_PASSWORD="${E2E_PASSWORD:-${E2E_ADMIN_PASSWORD:-admin}}"

export E2E_USERNAME="${DEFAULT_E2E_USERNAME}"
export E2E_PASSWORD="${DEFAULT_E2E_PASSWORD}"
export E2E_NEW_PASSWORD="${E2E_NEW_PASSWORD:-admin123}"
export E2E_CLUSTER="${E2E_CLUSTER:-e2e-cluster}"
export E2E_SYSTEM="${E2E_SYSTEM:-e2e-system}"
export E2E_SERVICE="${E2E_SERVICE:-e2e-service}"
export E2E_TEMPLATE="${E2E_TEMPLATE:-e2e-template}"
export E2E_SIZE="${E2E_SIZE:-e2e-small}"
export E2E_NAMESPACE="${E2E_NAMESPACE:-e2e-live}"
export E2E_VM_RUNNING_ID="${E2E_VM_RUNNING_ID:-vm-e2e-running}"
export E2E_VM_STOPPED_ID="${E2E_VM_STOPPED_ID:-vm-e2e-stopped}"
export E2E_VM_RUNNING_NAME="${E2E_VM_RUNNING_NAME:-vm-live}"
export E2E_VM_STOPPED_NAME="${E2E_VM_STOPPED_NAME:-vm-stopped}"

if [[ -z "${E2E_KUBECONFIG_B64:-}" ]]; then
  DEFAULT_KUBECONFIG_FILE="${ROOT_DIR}/k8s-admin.yaml"
  if [[ -f "${DEFAULT_KUBECONFIG_FILE}" ]]; then
    if E2E_KUBECONFIG_B64="$(base64 -w0 "${DEFAULT_KUBECONFIG_FILE}" 2>/dev/null)"; then
      export E2E_KUBECONFIG_B64
      log_info "using default live kubeconfig file: ${DEFAULT_KUBECONFIG_FILE}"
    elif E2E_KUBECONFIG_B64="$(base64 <"${DEFAULT_KUBECONFIG_FILE}" 2>/dev/null | tr -d '\n')"; then
      export E2E_KUBECONFIG_B64
      log_info "using default live kubeconfig file: ${DEFAULT_KUBECONFIG_FILE}"
    fi
  fi
fi

if [[ -z "${E2E_KUBECONFIG_B64:-}" ]]; then
  log_error "live E2E requires a real kubeconfig (set E2E_KUBECONFIG_B64 or provide ${ROOT_DIR}/k8s-admin.yaml)"
  exit 1
fi

# Keep e2e-seed aligned with the same account to avoid user/data drift.
export E2E_ADMIN_USERNAME="${E2E_ADMIN_USERNAME:-${E2E_USERNAME}}"
export E2E_ADMIN_PASSWORD="${E2E_ADMIN_PASSWORD:-${E2E_PASSWORD}}"

SERVER_PID=""
cleanup() {
  local exit_code=$?
  local cleanup_vm_exit=0
  set +e
  cleanup_namespace_vms
  cleanup_vm_exit=$?
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
  cleanup_next_web_port "${PW_WEB_PORT:-}"
  if [[ "${cleanup_vm_exit}" -ne 0 ]]; then
    log_warn "namespace VM cleanup completed with warnings"
  fi
  return "${exit_code}"
}
trap cleanup EXIT INT TERM

if port_in_use "$SERVER_PORT"; then
  log_error "backend port ${SERVER_PORT} is already in use"
  exit 1
fi

log_info "building backend server binary"
go build -o "$SERVER_BIN" ./cmd/server

log_info "starting backend server on :${SERVER_PORT}"
"$SERVER_BIN" >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!

log_info "waiting for backend readiness (${API_BASE_URL}/api/v1/health/live)"
READY=0
for _ in $(seq 1 120); do
  if ! kill -0 "$SERVER_PID" >/dev/null 2>&1; then
    log_error "backend server process exited before readiness"
    log_info "tailing server log ($SERVER_LOG)"
    tail -n 120 "$SERVER_LOG" || true
    exit 1
  fi
  if curl -fsS "${API_BASE_URL}/api/v1/health/live" >/dev/null; then
    READY=1
    break
  fi
  sleep 1
done

if [[ "$READY" -ne 1 ]]; then
  log_error "backend server did not become ready"
  log_info "tailing server log ($SERVER_LOG)"
  tail -n 120 "$SERVER_LOG" || true
  exit 1
fi

log_info "seeding baseline data"
go run ./cmd/seed

log_info "seeding API-managed live fixtures"
SEED_API_TOKEN="$(login_api_token "${E2E_ADMIN_USERNAME}" "${E2E_ADMIN_PASSWORD}")"
seed_live_api_managed_fixtures "${SEED_API_TOKEN}"

log_info "seeding extended low-level fixtures"
E2E_SKIP_API_MANAGED_FIXTURES=1 go run ./cmd/e2e-seed

log_info "running live-e2e preflight gates"
run_live_e2e_preflight_checks

log_info "running live Playwright E2E suite (no mock routes)"
DEFAULT_PW_PROJECT="${E2E_PLAYWRIGHT_PROJECT:-live-chromium}"
HAS_PROJECT_ARG=0
for arg in "$@"; do
  if [[ "${arg}" == "--project" || "${arg}" == --project=* ]]; then
    HAS_PROJECT_ARG=1
    break
  fi
done
PLAYWRIGHT_ARGS=("$@")
if [[ "${HAS_PROJECT_ARG}" -eq 0 ]]; then
  PLAYWRIGHT_ARGS+=("--project=${DEFAULT_PW_PROJECT}")
fi

set +e
CI=1 npm --prefix web run test:e2e:all -- "${PLAYWRIGHT_ARGS[@]}"
RUN_EXIT_CODE=$?
set -e

BACKEND_GUARD_EXIT=0
if [[ "${E2E_BACKEND_CRITICAL_GUARD:-1}" != "0" ]]; then
  if ! check_backend_critical_errors "${SERVER_LOG}"; then
    BACKEND_GUARD_EXIT=1
  fi
fi

FINAL_EXIT_CODE="${RUN_EXIT_CODE}"
if [[ "${BACKEND_GUARD_EXIT}" -ne 0 ]]; then
  FINAL_EXIT_CODE=1
fi

if [[ -n "${RUN_RESULT_FILE:-}" ]]; then
  {
    echo "exit_code=${FINAL_EXIT_CODE}"
    echo "playwright_exit_code=${RUN_EXIT_CODE}"
    echo "backend_guard_exit_code=${BACKEND_GUARD_EXIT}"
    if [[ -n "${RUN_LOG_FILE:-}" ]] && result_line="$(extract_result_summary "${RUN_LOG_FILE}")"; then
      echo "${result_line}"
    else
      echo "summary=unavailable"
    fi
  } >"${RUN_RESULT_FILE}"
fi

exit "${FINAL_EXIT_CODE}"
