#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="$ROOT/settings/decision-test.yaml"

show_help() {
    cat << 'EOF'
Usage: ./scripts/setup/run_llama_server.sh

Start llama-server with the pinned model. Thinking is disabled per request,
not with a server-wide reasoning flag. This process stays in the foreground.

Options:
  --help    Show this help message
EOF
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
    show_help
    exit 0
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

exec "$BINARY" -m "$MODEL" -c 2048 -b 512 -ngl 99 -np 1 --jinja --host "$HOST" --port "$PORT"
