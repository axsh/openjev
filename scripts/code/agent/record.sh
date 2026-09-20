#!/usr/bin/env bash
# scripts/code/agent/record.sh -- tt agent record wrapper
# Maps Coding Agent options to tt agent record options explicitly.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/../_resolve_tool.sh"

TT_ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --agent)            TT_ARGS+=(--agent "$2");            shift 2 ;;
    --summary)          TT_ARGS+=(--summary "$2");          shift 2 ;;
    --summary-file)     TT_ARGS+=(--summary-file "$2");     shift 2 ;;
    --note)             TT_ARGS+=(--note "$2");             shift 2 ;;
    --notes-file)       TT_ARGS+=(--notes-file "$2");       shift 2 ;;
    --changed-path)     TT_ARGS+=(--changed-path "$2");     shift 2 ;;
    --changed-paths-from-git) TT_ARGS+=(--changed-paths-from-git); shift ;;
    # Existing flags
    --architecture-impact)    TT_ARGS+=(--architecture-impact);    shift ;;
    --memory-related)         TT_ARGS+=(--memory-related);         shift ;;
    --prompt-related)         TT_ARGS+=(--prompt-related);         shift ;;
    --agent-behavior-related) TT_ARGS+=(--agent-behavior-related); shift ;;
    --requires-immediate-action) TT_ARGS+=(--requires-immediate-action); shift ;;
    # Far-knowledge flags (R1)
    --design-pattern)         TT_ARGS+=(--design-pattern);         shift ;;
    --convention)             TT_ARGS+=(--convention);             shift ;;
    --lesson-learned)         TT_ARGS+=(--lesson-learned);         shift ;;
    --preference)             TT_ARGS+=(--preference);             shift ;;
    # Metadata
    --client-request-id) TT_ARGS+=(--client-request-id "$2"); shift 2 ;;
    --dry-run)          TT_ARGS+=(--dry-run);               shift ;;
    --print-payload)    TT_ARGS+=(--print-payload);         shift ;;
    *)
      echo "[ERROR] Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

exec "$TOOL" agent record "${TT_ARGS[@]}"
