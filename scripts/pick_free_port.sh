#!/usr/bin/env bash

set -euo pipefail

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

for _ in $(seq 1 80); do
  candidate=$((RANDOM % 10000 + 18080))
  if ! port_in_use "${candidate}"; then
    echo "${candidate}"
    exit 0
  fi
done

echo "unable to allocate free port" >&2
exit 1
