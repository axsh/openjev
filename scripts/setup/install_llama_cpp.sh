#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
CONFIG="$ROOT/settings/decision-test.yaml"

show_help() {
    cat << 'EOF'
Usage: ./scripts/setup/install_llama_cpp.sh

Download the pinned llama.cpp Windows CUDA build and the matching CUDA runtime
into the directory named by settings/decision-test.yaml.

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

BUILD="$(yaml_get "$CONFIG" llama_build)"
ZIP="$(yaml_get "$CONFIG" llama_zip)"
CUDART="$(yaml_get "$CONFIG" cudart_zip)"
BINARY="$(yaml_get "$CONFIG" llama_binary)"
DEST="$ROOT/third_party/llama.cpp/$BUILD"

if [[ -f "$ROOT/$BINARY" ]]; then
    echo "llama-server already present at $BINARY"
    exit 0
fi

mkdir -p "$DEST"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

gh release download "$BUILD" --repo ggml-org/llama.cpp --dir "$TMP" --pattern "$ZIP" --pattern "$CUDART"
unzip -o "$TMP/$ZIP" -d "$DEST"
unzip -o "$TMP/$CUDART" -d "$DEST"

if [[ ! -f "$ROOT/$BINARY" ]]; then
    found="$(find "$DEST" -name 'llama-server.exe' -print -quit)"
    if [[ -z "$found" ]]; then
        echo "llama-server.exe was not found after extract" >&2
        exit 1
    fi
    cp "$found" "$ROOT/$BINARY"
    cp "$(dirname "$found")"/*.dll "$DEST" 2>/dev/null || true
fi

echo "Installed llama-server to $BINARY"
