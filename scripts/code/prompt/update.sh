#!/usr/bin/env bash
# scripts/code/prompt/update.sh -- tt prompt update wrapper
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../_resolve_tool.sh"

TT_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --force)   TT_ARGS+=(--force);   shift ;;
    --verbose) TT_ARGS+=(--verbose); shift ;;
    --dry-run) TT_ARGS+=(--dry-run); shift ;;
    *)
      echo "[ERROR] Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

exec "$TOOL" prompt update "${TT_ARGS[@]}"
