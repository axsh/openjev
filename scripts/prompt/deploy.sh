#!/usr/bin/env bash
# scripts/prompt/deploy.sh -- tt prompt deploy wrapper
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_resolve_tool.sh"

exec "$TOOL" prompt deploy "$@"
