#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
cd "${ROOT_DIR}"
source docs/design/ci/scripts/promtool_lib.sh

fail() {
  echo "[prometheus-operator-rule-parity] ERROR: $1" >&2
  exit 1
}

recording_rules="${1:-deploy/monitoring/prometheus/shepherd-recording-rules.yml}"
alert_rules="${2:-deploy/monitoring/prometheus/shepherd-alerts.yml}"
operator_rule="${3:-deploy/monitoring/prometheus-operator/shepherd-prometheus-rule.yml}"
expected_rules=""
extracted_rules=""
trap '[[ -n "${expected_rules}" ]] && rm -f "${expected_rules}"; [[ -n "${extracted_rules}" ]] && rm -f "${extracted_rules}"' EXIT

for file in "${recording_rules}" "${alert_rules}" "${operator_rule}"; do
  [[ -f "${file}" ]] || fail "missing ${file}"
done

expected_rules="$(mktemp)"
extracted_rules="$(mktemp)"

{
  echo "groups:"
  first_rule_file=1
  for file in "${recording_rules}" "${alert_rules}"; do
    if [[ "${first_rule_file}" -eq 0 ]]; then
      echo
    fi
    first_rule_file=0
    awk '
      NR == 1 {
        if ($0 != "groups:") {
          printf "%s: first line must be groups:\n", FILENAME > "/dev/stderr"
          exit 1
        }
        next
      }
      { print }
    ' "${file}"
  done
} >"${expected_rules}" || fail "failed to build combined native rule groups"

awk '
  BEGIN {
    in_spec = 0
    in_groups = 0
    seen_groups = 0
  }
  /^spec:[[:space:]]*$/ {
    in_spec = 1
    next
  }
  in_spec && /^  groups:[[:space:]]*$/ {
    print "groups:"
    in_groups = 1
    seen_groups = 1
    next
  }
  in_groups {
    if ($0 ~ /^[^[:space:]]/ && $0 !~ /^$/) {
      exit
    }
    sub(/^  /, "")
    print
  }
  END {
    if (seen_groups != 1) {
      exit 2
    }
  }
' "${operator_rule}" >"${extracted_rules}" || fail "failed to extract spec.groups from ${operator_rule}"

if ! diff -u "${expected_rules}" "${extracted_rules}"; then
  fail "${operator_rule}: spec.groups must match native recording and alert rule files"
fi

if promtool_bin="$(resolve_promtool)"; then
  "${promtool_bin}" check rules "${extracted_rules}"
elif [[ $? -eq 2 ]]; then
  echo "[prometheus-operator-rule-parity] promtool not found; parity comparison ran without extracted-rule syntax validation"
else
  fail "promtool discovery failed"
fi

echo "[prometheus-operator-rule-parity] OK"
