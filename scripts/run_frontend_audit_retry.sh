#!/usr/bin/env bash
set -euo pipefail

attempts="${NPM_AUDIT_ATTEMPTS:-3}"
delay_seconds="${NPM_AUDIT_RETRY_DELAY_SECONDS:-2}"

tmp_log="$(mktemp)"
cleanup() {
  rm -f "$tmp_log"
}
trap cleanup EXIT

run_audit() {
  npm_config_fetch_retries="${NPM_CONFIG_FETCH_RETRIES:-5}" \
  npm_config_fetch_retry_factor="${NPM_CONFIG_FETCH_RETRY_FACTOR:-2}" \
  npm_config_fetch_retry_mintimeout="${NPM_CONFIG_FETCH_RETRY_MINTIMEOUT:-1000}" \
  npm_config_fetch_retry_maxtimeout="${NPM_CONFIG_FETCH_RETRY_MAXTIMEOUT:-10000}" \
  npm audit --prefix web --audit-level=high
}

is_transient_failure() {
  grep -Eiq \
    'audit endpoint returned an error|Client network socket disconnected|ECONNRESET|ETIMEDOUT|EAI_AGAIN|fetch failed|network socket disconnected|socket hang up' \
    "$tmp_log"
}

for attempt in $(seq 1 "$attempts"); do
  if run_audit 2>&1 | tee "$tmp_log"; then
    exit 0
  fi

  if [[ "$attempt" -lt "$attempts" ]] && is_transient_failure; then
    echo "Transient npm audit failure detected; retrying ($attempt/$attempts)..." >&2
    sleep "$delay_seconds"
    continue
  fi

  exit 1
done
