#!/usr/bin/env bash
# check_no_chinese_chars.sh
#
# Enforces repository-wide "English-only" text policy for code/docs,
# except approved Chinese i18n resources.

set -euo pipefail

if ! command -v rg >/dev/null 2>&1; then
  echo "ERROR: ripgrep (rg) is required for CJK allowlist scan."
  exit 127
fi

readonly ALLOWED_TRANSLATION="docs/i18n/zh-CN/design/interaction-flows/master-flow.md"
readonly ALLOWED_WEB_LOCALE_DIR="web/src/i18n/locales/zh-CN/**"
readonly CJK_PATTERN='[\x{3400}-\x{4DBF}\x{4E00}-\x{9FFF}\x{F900}-\x{FAFF}]'

# Exclude git metadata, local working notes, and common third-party/generated trees.
# Keep this list minimal to avoid masking real violations.
cmd=(
  rg -n --pcre2 "$CJK_PATTERN" .
  --glob '!.git/**'
  --glob '!ai-code/**'
  --glob '!node_modules/**'
  --glob '!vendor/**'
  --glob '!dist/**'
  --glob '!coverage/**'
  --glob "!${ALLOWED_TRANSLATION}"
  --glob "!${ALLOWED_WEB_LOCALE_DIR}"
)

set +e
output="$(${cmd[@]} 2>/dev/null)"
status=$?
set -e

if [ "$status" -eq 0 ]; then
  echo "ERROR: Chinese characters detected outside approved i18n allowlist:"
  echo "  ${ALLOWED_TRANSLATION}"
  echo "  ${ALLOWED_WEB_LOCALE_DIR}"
  echo ""
  echo "$output"
  exit 1
fi

if [ "$status" -eq 1 ]; then
  echo "OK: no Chinese characters found outside approved i18n allowlist"
  exit 0
fi

echo "ERROR: failed to run CJK scan (rg exit code: ${status})"
exit "$status"
