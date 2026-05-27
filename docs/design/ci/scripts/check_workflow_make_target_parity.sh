#!/usr/bin/env bash
# Compat wrapper — delegates to the Go policy checker.
# This script is retained so that Makefile:195 continues to work unchanged.
# It will be removed in PR-D once the Makefile calls the Go checker directly.
set -euo pipefail
exec go run docs/design/ci/scripts/check_workflow_make_parity.go "$@"
