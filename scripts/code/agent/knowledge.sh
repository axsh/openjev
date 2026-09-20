#!/usr/bin/env bash
# scripts/code/agent/knowledge.sh -- tt agent knowledge wrapper
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../_resolve_tool.sh"

if [[ $# -lt 1 ]]; then
  echo "Usage: knowledge.sh <add|append|list|split|merge|rename|move> [OPTIONS]" >&2
  exit 1
fi

SUBCMD="$1"
shift

TT_ARGS=()
case "$SUBCMD" in
  add)
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --category-path)  TT_ARGS+=(--category-path "$2");  shift 2 ;;
        --title)          TT_ARGS+=(--title "$2");          shift 2 ;;
        --description)    TT_ARGS+=(--description "$2");    shift 2 ;;
        --content-file)   TT_ARGS+=(--content-file "$2");   shift 2 ;;
        --source-events)  TT_ARGS+=(--source-events "$2");  shift 2 ;;
        *) echo "[ERROR] Unknown argument for add: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge add "${TT_ARGS[@]}"
    ;;
  append)
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --category-path)  TT_ARGS+=(--category-path "$2");  shift 2 ;;
        --title)          TT_ARGS+=(--title "$2");          shift 2 ;;
        --content-file)   TT_ARGS+=(--content-file "$2");   shift 2 ;;
        --source-events)  TT_ARGS+=(--source-events "$2");  shift 2 ;;
        *) echo "[ERROR] Unknown argument for append: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge append "${TT_ARGS[@]}"
    ;;
  list)
    exec "$TOOL" agent knowledge list
    ;;
  split)
    CATEGORY_PATH="$1"; shift
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --into) TT_ARGS+=(--into "$2"); shift 2 ;;
        --plan) TT_ARGS+=(--plan "$2"); shift 2 ;;
        *) echo "[ERROR] Unknown argument for split: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge split "$CATEGORY_PATH" "${TT_ARGS[@]}"
    ;;
  merge)
    CATS=()
    while [[ $# -gt 0 && "$1" != --* ]]; do CATS+=("$1"); shift; done
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --into)  TT_ARGS+=(--into "$2");  shift 2 ;;
        --plan)  TT_ARGS+=(--plan "$2");  shift 2 ;;
        *) echo "[ERROR] Unknown argument for merge: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge merge "${CATS[@]}" "${TT_ARGS[@]}"
    ;;
  rename)
    OLD_PATH="$1"; NEW_PATH="$2"; shift 2
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --title) TT_ARGS+=(--title "$2"); shift 2 ;;
        *) echo "[ERROR] Unknown argument for rename: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge rename "$OLD_PATH" "$NEW_PATH" "${TT_ARGS[@]}"
    ;;
  move)
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --from) TT_ARGS+=(--from "$2"); shift 2 ;;
        --to)   TT_ARGS+=(--to "$2");   shift 2 ;;
        *) echo "[ERROR] Unknown argument for move: $1" >&2; exit 1 ;;
      esac
    done
    exec "$TOOL" agent knowledge move "${TT_ARGS[@]}"
    ;;
  *)
    echo "[ERROR] Unknown subcommand: $SUBCMD. Use 'add', 'append', 'list', 'split', 'merge', 'rename', or 'move'." >&2
    exit 1
    ;;
esac
