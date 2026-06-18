#!/usr/bin/env bash
# Validate and optionally apply PostgreSQL/River production table maintenance
# settings from docs/operations/database-operations.md.

set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/check_postgres_ops.sh [--apply-autovacuum] [--database-url URL]

Environment:
  DATABASE_URL    PostgreSQL connection URI when --database-url is omitted.

Options:
  --apply-autovacuum  Apply the required table autovacuum reloptions before checking.
  --database-url URL  PostgreSQL connection URI. The value is never echoed.
  -h, --help          Show this help.

The default mode is read-only. It checks that autovacuum is enabled and that
river_job, audit_logs, and domain_events have the required table reloptions.
EOF
}

apply_autovacuum=false
database_url="${DATABASE_URL:-}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply-autovacuum)
      apply_autovacuum=true
      shift
      ;;
    --database-url)
      if [[ $# -lt 2 || -z "${2:-}" ]]; then
        echo "ERROR: --database-url requires a non-empty value" >&2
        exit 2
      fi
      database_url="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "ERROR: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [[ -z "${database_url}" ]]; then
  echo "ERROR: DATABASE_URL or --database-url is required" >&2
  exit 2
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "ERROR: psql is required for PostgreSQL operations validation" >&2
  exit 2
fi

psql_cmd=(psql -X -v ON_ERROR_STOP=1 -d "${database_url}")

run_psql() {
  "${psql_cmd[@]}" "$@"
}

run_psql_tsv() {
  "${psql_cmd[@]}" -A -t -F $'\t' "$@"
}

if [[ "${apply_autovacuum}" == "true" ]]; then
  echo "Applying required PostgreSQL/River autovacuum reloptions..."
  run_psql -q <<'SQL'
ALTER TABLE river_job SET (
  autovacuum_vacuum_scale_factor = 0.01,
  autovacuum_vacuum_threshold = 1000,
  autovacuum_analyze_scale_factor = 0.01,
  autovacuum_analyze_threshold = 500
);

ALTER TABLE audit_logs SET (
  autovacuum_vacuum_scale_factor = 0.02,
  autovacuum_vacuum_threshold = 5000
);

ALTER TABLE domain_events SET (
  autovacuum_vacuum_scale_factor = 0.02,
  autovacuum_vacuum_threshold = 2000
);
SQL
fi

autovacuum_setting="$(run_psql_tsv -c "SHOW autovacuum;")"
if [[ "${autovacuum_setting}" != "on" ]]; then
  echo "FAIL: PostgreSQL autovacuum is ${autovacuum_setting}; expected on" >&2
  exit 1
fi

echo "OK: PostgreSQL autovacuum is enabled"

failures=0
reloptions_rows="$(mktemp)"
cleanup() {
  rm -f "${reloptions_rows}"
}
trap cleanup EXIT

run_psql_tsv >"${reloptions_rows}" <<'SQL'
WITH expected(table_name, option_name, expected_value) AS (
  VALUES
    ('river_job', 'autovacuum_vacuum_scale_factor', '0.01'),
    ('river_job', 'autovacuum_vacuum_threshold', '1000'),
    ('river_job', 'autovacuum_analyze_scale_factor', '0.01'),
    ('river_job', 'autovacuum_analyze_threshold', '500'),
    ('audit_logs', 'autovacuum_vacuum_scale_factor', '0.02'),
    ('audit_logs', 'autovacuum_vacuum_threshold', '5000'),
    ('domain_events', 'autovacuum_vacuum_scale_factor', '0.02'),
    ('domain_events', 'autovacuum_vacuum_threshold', '2000')
),
actual AS (
  SELECT
    e.table_name,
    e.option_name,
    e.expected_value,
    o.option_value AS actual_value,
    CASE
      WHEN c.oid IS NULL THEN 'missing_table'
      WHEN o.option_value = e.expected_value THEN 'ok'
      ELSE 'mismatch'
    END AS status
  FROM expected e
  LEFT JOIN pg_class c ON c.oid = to_regclass(e.table_name)
  LEFT JOIN LATERAL pg_options_to_table(c.reloptions) AS o(option_name, option_value)
    ON o.option_name = e.option_name
)
SELECT table_name, option_name, expected_value, COALESCE(actual_value, '<unset>'), status
FROM actual
ORDER BY table_name, option_name;
SQL

while IFS=$'\t' read -r table_name option_name expected_value actual_value status; do
  [[ -n "${table_name}" ]] || continue
  case "${status}" in
    ok)
      printf 'OK: %s %s=%s\n' "${table_name}" "${option_name}" "${actual_value}"
      ;;
    missing_table)
      printf 'FAIL: table %s is missing from the configured database/search_path\n' "${table_name}" >&2
      failures=$((failures + 1))
      ;;
    *)
      printf 'FAIL: %s %s expected %s, got %s\n' \
        "${table_name}" "${option_name}" "${expected_value}" "${actual_value:-<unset>}" >&2
      failures=$((failures + 1))
      ;;
  esac
done <"${reloptions_rows}"

echo
echo "PostgreSQL/River table health:"
run_psql <<'SQL'
SELECT
  relname AS table_name,
  n_dead_tup,
  n_live_tup,
  round(100.0 * n_dead_tup / nullif(n_live_tup + n_dead_tup, 0), 2) AS dead_ratio_percent,
  last_autovacuum,
  last_autoanalyze
FROM pg_stat_user_tables
WHERE relname IN ('river_job', 'audit_logs', 'domain_events')
ORDER BY dead_ratio_percent DESC NULLS LAST, relname;
SQL

if [[ "${failures}" -gt 0 ]]; then
  echo "FAIL: PostgreSQL/River operations validation found ${failures} issue(s)" >&2
  exit 1
fi

echo "OK: PostgreSQL/River operations validation passed"
