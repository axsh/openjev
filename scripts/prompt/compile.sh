#!/usr/bin/env bash
# scripts/prompt/compile.sh -- tt prompt compile wrapper
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_resolve_tool.sh"

exec "$TOOL" prompt compile "$@"
