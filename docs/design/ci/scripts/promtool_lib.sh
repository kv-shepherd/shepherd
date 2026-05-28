#!/usr/bin/env bash

resolve_promtool() {
  if [[ -n "${PROMTOOL:-}" ]]; then
    if [[ "${PROMTOOL}" != /* ]]; then
      echo "[promtool] ERROR: PROMTOOL must be an absolute path: ${PROMTOOL}" >&2
      return 1
    fi
    if [[ ! -x "${PROMTOOL}" ]]; then
      echo "[promtool] ERROR: PROMTOOL is set but not executable: ${PROMTOOL}" >&2
      return 1
    fi
    printf '%s\n' "${PROMTOOL}"
    return 0
  fi

  if command -v promtool >/dev/null 2>&1; then
    command -v promtool
    return 0
  fi

  if [[ "${PROMTOOL_REQUIRED:-0}" == "1" ]]; then
    echo "[promtool] ERROR: promtool is required (PROMTOOL_REQUIRED=1) but was not found on PATH" >&2
    return 1
  fi

  return 2
}
