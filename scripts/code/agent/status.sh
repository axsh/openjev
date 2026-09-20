#!/usr/bin/env bash
# scripts/code/agent/status.sh -- tt agent status wrapper
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../_resolve_tool.sh"

TT_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --format) TT_ARGS+=(--format "$2"); shift 2 ;;
    *)
      echo "[ERROR] Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

exec "$TOOL" agent status "${TT_ARGS[@]}"
