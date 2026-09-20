#!/usr/bin/env bash
# scripts/prompt/update.sh -- tt prompt update wrapper
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_resolve_tool.sh"

exec "$TOOL" prompt update "$@"
