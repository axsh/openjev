#!/usr/bin/env bash
# scripts/code/agent/intake.sh -- tt agent intake list/show wrapper
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../_resolve_tool.sh"

if [[ $# -lt 1 ]]; then
  echo "Usage: intake.sh <list|show|processed> [OPTIONS]" >&2
  exit 1
fi

SUBCMD="$1"
shift

TT_ARGS=()
case "$SUBCMD" in
  list)
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --status)  TT_ARGS+=(--status "$2");  shift 2 ;;
        --agent)   TT_ARGS+=(--agent "$2");   shift 2 ;;
        --branch)  TT_ARGS+=(--branch "$2");  shift 2 ;;
        --query)   TT_ARGS+=(--query "$2");   shift 2 ;;
        --from)    TT_ARGS+=(--from "$2");    shift 2 ;;
        --to)      TT_ARGS+=(--to "$2");      shift 2 ;;
        --format)  TT_ARGS+=(--format "$2");  shift 2 ;;
        --limit)   TT_ARGS+=(--limit "$2");   shift 2 ;;
        *)
          echo "[ERROR] Unknown argument for list: $1" >&2
          exit 1
          ;;
      esac
    done
    exec "$TOOL" agent intake list "${TT_ARGS[@]}"
    ;;
  show)
    if [[ $# -lt 1 ]]; then
      echo "Usage: intake.sh show <event-id>" >&2
      exit 1
    fi
    EVENT_ID="$1"
    exec "$TOOL" agent intake show "$EVENT_ID"
    ;;
  processed)
    if [[ $# -lt 1 ]]; then
      echo "Usage: intake.sh processed <event-id>" >&2
      exit 1
    fi
    EVENT_ID="$1"
    exec "$TOOL" agent intake processed "$EVENT_ID"
    ;;
  *)
    echo "[ERROR] Unknown subcommand: $SUBCMD. Use 'list', 'show', or 'processed'." >&2
    exit 1
    ;;
esac
