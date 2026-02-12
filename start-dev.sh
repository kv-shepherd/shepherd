#!/bin/bash
set -euo pipefail
exec "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/deploy/dev/start-dev.sh" "$@"
