#!/usr/bin/env bash

set -euo pipefail

# Strict test-first delta guard:
# - If backend runtime Go files change, same-package *_test.go must also change.
# - If frontend runtime files under web/src change, at least one frontend test file must change.
#
# Baseline: merge-base(HEAD, origin/main). Falls back to HEAD~1 when base is unavailable.

ROOT_DIR="$(git rev-parse --show-toplevel)"
cd "$ROOT_DIR"

ALLOWLIST_FILE="docs/design/ci/allowlists/test_delta_guard_exempt.txt"
BASE_REF="${TEST_GUARD_BASE_REF:-origin/main}"
INCLUDE_WORKTREE="${TEST_GUARD_INCLUDE_WORKTREE:-1}"

if ! git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
  git fetch --no-tags --depth=200 origin main >/dev/null 2>&1 || true
fi

if git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
  BASE_COMMIT="$(git merge-base HEAD "$BASE_REF")"
elif git rev-parse --verify HEAD~1 >/dev/null 2>&1; then
  BASE_COMMIT="HEAD~1"
else
  echo "WARN: unable to determine base commit for test delta guard; skipping"
  exit 0
fi

declare -A CHANGED_SET=()

while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  CHANGED_SET["$file"]=1
done < <(git diff --name-only "${BASE_COMMIT}"...HEAD)

if [[ "$INCLUDE_WORKTREE" == "1" ]]; then
  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    CHANGED_SET["$file"]=1
  done < <(git diff --name-only)
  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    CHANGED_SET["$file"]=1
  done < <(git diff --cached --name-only)
  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    CHANGED_SET["$file"]=1
  done < <(git ls-files --others --exclude-standard)
fi

CHANGED_FILES=()
for file in "${!CHANGED_SET[@]}"; do
  CHANGED_FILES+=("$file")
done
IFS=$'\n' CHANGED_FILES=($(printf '%s\n' "${CHANGED_FILES[@]}" | sort))
unset IFS

if [[ "${#CHANGED_FILES[@]}" -eq 0 ]]; then
  echo "OK: no changed files in diff range"
  exit 0
fi

declare -A BACKEND_CHANGED_TEST_DIRS=()
declare -A EXEMPT_PREFIXES=()
FRONTEND_TEST_CHANGED=0

if [[ -f "$ALLOWLIST_FILE" ]]; then
  while IFS= read -r line; do
    trimmed="$(echo "$line" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//')"
    [[ -z "$trimmed" || "$trimmed" == \#* ]] && continue
    EXEMPT_PREFIXES["$trimmed"]=1
  done <"$ALLOWLIST_FILE"
fi

is_exempt_runtime_path() {
  local path="$1"
  for prefix in "${!EXEMPT_PREFIXES[@]}"; do
    if [[ "$path" == "$prefix"* ]]; then
      return 0
    fi
  done
  return 1
}

is_backend_runtime_go() {
  local path="$1"
  [[ "$path" =~ ^(internal|cmd)/.*\.go$ ]] || return 1
  [[ "$path" =~ _test\.go$ ]] && return 1
  [[ "$path" == internal/api/generated/* ]] && return 1
  [[ "$path" == internal/repository/sqlc/* ]] && return 1
  [[ "$path" == ent/* ]] && return 1
  return 0
}

is_frontend_runtime() {
  local path="$1"
  [[ "$path" =~ ^web/src/.*\.(ts|tsx)$ ]] || return 1
  [[ "$path" =~ \.test\.ts$ || "$path" =~ \.test\.tsx$ ]] && return 1
  [[ "$path" == web/src/types/* ]] && return 1
  [[ "$path" == web/src/i18n/locales/* ]] && return 1
  return 0
}

BACKEND_RUNTIME_CHANGED=()
FRONTEND_RUNTIME_CHANGED=()

for file in "${CHANGED_FILES[@]}"; do
  if [[ "$file" =~ ^(internal|cmd)/.*_test\.go$ ]]; then
    BACKEND_CHANGED_TEST_DIRS["$(dirname "$file")"]=1
  fi
  if [[ "$file" =~ ^web/src/.*\.test\.(ts|tsx)$ || "$file" =~ ^web/tests/ ]]; then
    FRONTEND_TEST_CHANGED=1
  fi
done

for file in "${CHANGED_FILES[@]}"; do
  if is_backend_runtime_go "$file"; then
    if is_exempt_runtime_path "$file"; then
      continue
    fi
    BACKEND_RUNTIME_CHANGED+=("$file")
  fi
  if is_frontend_runtime "$file"; then
    if is_exempt_runtime_path "$file"; then
      continue
    fi
    FRONTEND_RUNTIME_CHANGED+=("$file")
  fi
done

VIOLATIONS=()

if [[ "${#BACKEND_RUNTIME_CHANGED[@]}" -gt 0 ]]; then
  for file in "${BACKEND_RUNTIME_CHANGED[@]}"; do
    dir="$(dirname "$file")"
    if [[ -z "${BACKEND_CHANGED_TEST_DIRS[$dir]:-}" ]]; then
      VIOLATIONS+=("backend runtime file changed without same-package test change: $file (expected test change under $dir/*_test.go)")
    fi
  done
fi

if [[ "${#FRONTEND_RUNTIME_CHANGED[@]}" -gt 0 && "$FRONTEND_TEST_CHANGED" -ne 1 ]]; then
  VIOLATIONS+=("frontend runtime files changed but no frontend test files changed (web/src/**/*.test.ts(x) or web/tests/**)")
fi

if [[ "${#VIOLATIONS[@]}" -gt 0 ]]; then
  echo "FAIL: strict test-first delta guard failed"
  echo "Base commit: $BASE_COMMIT"
  for v in "${VIOLATIONS[@]}"; do
    echo " - $v"
  done
  echo
  echo "Use allowlist only for justified edge cases: $ALLOWLIST_FILE"
  exit 1
fi

echo "OK: strict test-first delta guard passed"
echo "Base commit: $BASE_COMMIT"
