#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="$ROOT/settings/decision-test.yaml"

show_help() {
    cat << 'EOF'
Usage: ./scripts/setup/download_model.sh

Download the pinned MiniCPM5-2B GGUF into the path named by
settings/decision-test.yaml. Existing non-empty files are kept.

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
    awk -v k="$2" '$1 == k":" { $1=""; sub(/^ /, ""); print; exit }' "$1"
}

URL="$(yaml_get "$CONFIG" model_url)"
REL="$(yaml_get "$CONFIG" model_path)"
DEST="$ROOT/$REL"

if [[ -f "$DEST" && -s "$DEST" ]]; then
    echo "Model already present at $REL"
    exit 0
fi

mkdir -p "$(dirname "$DEST")"
curl -L --fail --retry 3 -o "$DEST.partial" "$URL"
if [[ ! -s "$DEST.partial" ]]; then
    echo "Downloaded model is empty" >&2
    exit 1
fi
mv "$DEST.partial" "$DEST"
echo "Downloaded model to $REL"
