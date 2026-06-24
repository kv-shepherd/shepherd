#!/usr/bin/env bash
# Blocks new public-source fixture leaks that are too project-specific for gitleaks.

set -euo pipefail

BASE_REF="${PUBLIC_HYGIENE_BASE_REF:-origin/main}"
violations=()
email_re='[[:alnum:]._%+-]+@[[:alnum:].-]+[.][[:alpha:]]{2,}'
private_path_re="(^|[[:space:]\`\"'(<])([.][.]/)*private/([[:alnum:]_.-]*enterprise|[[:alnum:]_.-]*internal|[[:alnum:]_.-]*repo)(/|[[:space:]\`\"'>)]|$)"

skip_path() {
  case "$1" in
    ai-code/*|.git/*|node_modules/*|web/node_modules/*|coverage/*|web/coverage/*)
      return 0
      ;;
  esac
  return 1
}

allowed_email_domain() {
  local domain="$1"
  case "$domain" in
    example.com|example.org|example.net|kv-shepherd.io|users.noreply.github.com|*.users.noreply.github.com)
      return 0
      ;;
  esac
  return 1
}

check_line() {
  local file="$1"
  local line_no="$2"
  local line="$3"
  local rest="$line"

  while [[ $rest =~ $email_re ]]; do
    local email="${BASH_REMATCH[0]}"
    local domain="${email##*@}"
    domain="${domain,,}"
    if ! allowed_email_domain "$domain"; then
      violations+=("${file}:${line_no}: non-reserved email fixture '${email}' must use example.com/org/net or a documented project contact domain")
    fi
    rest="${rest#*"${email}"}"
  done

  if [[ ${line,,} =~ $private_path_re ]]; then
    violations+=("${file}:${line_no}: private repository path markers must not be introduced into public source")
  fi
}

run_self_test() {
  violations=()
  local disallowed_email="alice@corp"
  disallowed_email+=".invalid"
  local private_marker="../pri"
  private_marker+="vate/internal/repo"

  check_line "fixture.txt" 1 "reserved addresses alice@example.com OPS@KV-SHEPHERD.IO are allowed"
  if [ "${#violations[@]}" -ne 0 ]; then
    printf 'FAIL: public hygiene self-test expected reserved addresses to pass, got %d issue(s)\n' "${#violations[@]}"
    printf ' - %s\n' "${violations[@]}"
    exit 1
  fi

  check_line "fixture.txt" 2 "reject direct user fixture ${disallowed_email}"
  check_line "fixture.txt" 3 "reject path ${private_marker}"
  if [ "${#violations[@]}" -ne 2 ]; then
    printf 'FAIL: public hygiene self-test expected 2 issue(s), got %d\n' "${#violations[@]}"
    printf ' - %s\n' "${violations[@]}"
    exit 1
  fi

  echo "OK: public hygiene self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  run_self_test
  exit 0
fi

scan_diff_stream() {
  local file=""
  local new_line=0
  while IFS= read -r line; do
    case "$line" in
      "+++ b/"*)
        file="${line#+++ b/}"
        new_line=0
        ;;
      "@@ "*)
        if [[ "$line" =~ \+([0-9]+) ]]; then
          new_line="${BASH_REMATCH[1]}"
        else
          new_line=0
        fi
        ;;
      "+++"*)
        ;;
      "+"*)
        if [ -n "$file" ] && ! skip_path "$file"; then
          check_line "$file" "$new_line" "${line#+}"
        fi
        if [ "$new_line" -gt 0 ]; then
          new_line=$((new_line + 1))
        fi
        ;;
      "-"*)
        ;;
      *)
        if [ "$new_line" -gt 0 ]; then
          new_line=$((new_line + 1))
        fi
        ;;
    esac
  done
}

if git rev-parse --verify "$BASE_REF" >/dev/null 2>&1; then
  scan_diff_stream < <(git diff --unified=0 --no-ext-diff "$BASE_REF"...HEAD -- . ':!ai-code/**' ':!node_modules/**' ':!web/node_modules/**')
fi

scan_diff_stream < <(git diff --cached --unified=0 --no-ext-diff -- . ':!ai-code/**' ':!node_modules/**' ':!web/node_modules/**')
scan_diff_stream < <(git diff --unified=0 --no-ext-diff -- . ':!ai-code/**' ':!node_modules/**' ':!web/node_modules/**')

while IFS= read -r file; do
  [ -n "$file" ] || continue
  skip_path "$file" && continue
  [ -f "$file" ] || continue
  grep -Iq . "$file" || continue
  line_no=0
  while IFS= read -r line || [ -n "$line" ]; do
    line_no=$((line_no + 1))
    check_line "$file" "$line_no" "$line"
  done < "$file"
done < <(git ls-files -o --exclude-standard)

if [ "${#violations[@]}" -gt 0 ]; then
  printf 'FAIL: public hygiene scan found %d issue(s):\n' "${#violations[@]}"
  printf ' - %s\n' "${violations[@]}"
  exit 1
fi

echo "OK: public hygiene scan passed"
