#!/usr/bin/env bash
# Blocks new public-source fixture leaks that are too project-specific for gitleaks.

set -euo pipefail

BASE_REF="${PUBLIC_HYGIENE_BASE_REF:-origin/main}"
violations=()

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

  while IFS= read -r email; do
    [ -n "$email" ] || continue
    local domain="${email##*@}"
    domain="$(printf '%s' "$domain" | tr '[:upper:]' '[:lower:]')"
    if ! allowed_email_domain "$domain"; then
      violations+=("${file}:${line_no}: non-reserved email fixture '${email}' must use example.com/org/net or a documented project contact domain")
    fi
  done < <(printf '%s\n' "$line" | grep -Eio '[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}' || true)

  if printf '%s\n' "$line" | grep -Eiq '(^|[[:space:]`"'\''(<])(\.\./)*private/([[:alnum:]_.-]*enterprise|[[:alnum:]_.-]*internal|[[:alnum:]_.-]*repo)(/|[[:space:]`"'\''>)]|$)'; then
    violations+=("${file}:${line_no}: private repository path markers must not be introduced into public source")
  fi
}

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
