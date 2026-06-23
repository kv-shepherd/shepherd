#!/usr/bin/env bash
# check_go_coverage_threshold.sh — enforce handwritten Go coverage threshold.
#
# The raw ci-go-test profile intentionally covers ./ent/... and generated API
# code so the artifact remains complete. The threshold gate filters generated
# and template-only code before using go tool cover for the pass/fail number.

set -euo pipefail

profile="coverage.out"
min_coverage="60.0"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --profile)
      profile="${2:-}"
      shift 2
      ;;
    --min)
      min_coverage="${2:-}"
      shift 2
      ;;
    *)
      echo "usage: $0 [--profile coverage.out] [--min 60.0]" >&2
      exit 2
      ;;
  esac
done

if [ ! -f "$profile" ]; then
  echo "FAIL: coverage profile not found: $profile" >&2
  exit 1
fi

excluded_prefixes=(
  "kv-shepherd.io/shepherd/ent/"
  "kv-shepherd.io/shepherd/internal/api/generated/"
  "kv-shepherd.io/shepherd/internal/api/specembed/"
  "kv-shepherd.io/shepherd/plugins/authprovider/example/"
  "kv-shepherd.io/shepherd/plugins/authprovider/template/"
)

prefix_arg=$(IFS='|'; echo "${excluded_prefixes[*]}")
filtered_profile=$(mktemp)
stats_file=$(mktemp)
trap 'rm -f "$filtered_profile" "$stats_file"' EXIT

awk -v prefixes="$prefix_arg" -v filtered="$filtered_profile" '
BEGIN {
  split(prefixes, excluded, "|")
}
NR == 1 {
  print $0 > filtered
  next
}
{
  path = $1
  sub(/:.*/, "", path)
  statements = $2 + 0
  is_excluded = 0
  for (i in excluded) {
    if (index(path, excluded[i]) == 1) {
      is_excluded = 1
      break
    }
  }
  if (is_excluded) {
    excluded_files[path] = 1
    excluded_total += statements
    if ($3 + 0 > 0) {
      excluded_covered += statements
    }
    next
  }
  print $0 > filtered
  included_files[path] = 1
  included_total += statements
  if ($3 + 0 > 0) {
    included_covered += statements
  }
}
END {
  for (path in included_files) {
    included_file_count++
  }
  for (path in excluded_files) {
    excluded_file_count++
  }
  printf("included_total=%d\n", included_total)
  printf("included_covered=%d\n", included_covered)
  printf("included_files=%d\n", included_file_count)
  printf("excluded_total=%d\n", excluded_total)
  printf("excluded_covered=%d\n", excluded_covered)
  printf("excluded_files=%d\n", excluded_file_count)
}
' "$profile" > "$stats_file"

# shellcheck disable=SC1090
. "$stats_file"

if [ "${included_total:-0}" -eq 0 ]; then
  echo "FAIL: no included Go statements found in $profile" >&2
  exit 1
fi

coverage_output=$(go tool cover -func="$filtered_profile")
included_pct=$(printf '%s\n' "$coverage_output" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
if [ -z "$included_pct" ]; then
  echo "FAIL: unable to read total coverage from go tool cover output" >&2
  printf '%s\n' "$coverage_output" >&2
  exit 1
fi

excluded_pct=$(awk -v covered="${excluded_covered:-0}" -v total="${excluded_total:-0}" 'BEGIN {
  if (total == 0) {
    printf "100.0"
  } else {
    printf "%.1f", covered * 100 / total
  }
}')

printf 'go coverage: included %.1f%% (%d/%d statements across %d file(s)); excluded %.1f%% (%d/%d statements across %d generated/template file(s))\n' \
  "$included_pct" \
  "${included_covered:-0}" \
  "${included_total:-0}" \
  "${included_files:-0}" \
  "$excluded_pct" \
  "${excluded_covered:-0}" \
  "${excluded_total:-0}" \
  "${excluded_files:-0}"

if ! awk -v got="$included_pct" -v min="$min_coverage" 'BEGIN { exit(got + 0 >= min + 0 ? 0 : 1) }'; then
  printf 'FAIL: included Go coverage %.1f%% is below threshold %.1f%%\n' "$included_pct" "$min_coverage" >&2
  echo "Excluded generated/template prefixes:" >&2
  for prefix in "${excluded_prefixes[@]}"; do
    echo "  - $prefix" >&2
  done
  exit 1
fi

printf 'OK: included Go coverage %.1f%% meets threshold %.1f%%\n' "$included_pct" "$min_coverage"
