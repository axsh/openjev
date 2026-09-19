#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="$ROOT/settings/decision-test.yaml"

show_help() {
    cat << 'EOF'
Usage: ./scripts/setup/run_llama_server.sh [--parallel N]

Start llama-server with the pinned model. Thinking is disabled per request,
not with a server-wide reasoning flag. This process stays in the foreground.
--parallel sets the slot count. The default is 1.

Options:
  --parallel N  Number of server slots (default: 1)
  --help        Show this help message
EOF
}

PARALLEL=1
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    show_help
    exit 0
fi
if [[ "${1:-}" == "--parallel" ]]; then
    if [[ -z "${2:-}" || ! "$2" =~ ^[1-9][0-9]*$ ]]; then
        echo "--parallel requires an integer >= 1" >&2
        exit 1
    fi
    PARALLEL="$2"
    shift 2
fi
if [[ $# -gt 0 ]]; then
    echo "Unknown option: $1" >&2
    show_help
    exit 1
fi

yaml_get() {
    awk -v k="$2" '$1 == k":" { print $2; exit }' "$1"
}

BINARY="$ROOT/$(yaml_get "$CONFIG" llama_binary)"
MODEL="$ROOT/$(yaml_get "$CONFIG" model_path)"
HOST="$(yaml_get "$CONFIG" llama_host)"
PORT="$(yaml_get "$CONFIG" llama_port)"

exec "$BINARY" -m "$MODEL" -c 2048 -b 512 -ngl 99 -np "$PARALLEL" --jinja --host "$HOST" --port "$PORT"
