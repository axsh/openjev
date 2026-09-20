#!/usr/bin/env bash
# Common tool discovery logic for tt wrapper scripts.
# Source this file, then use $TOOL variable.
# If tt is not found, exits with 0 (graceful skip) and prints a skip message.

_resolve_tt() {
    if [ -n "${TT_TOOL:-}" ]; then
        echo "$TT_TOOL"
        return 0
    fi
    if command -v tt &>/dev/null; then
        echo "tt"
        return 0
    fi
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local project_root
    project_root="$(cd "$script_dir/../.." && pwd)"
    local local_bin="$project_root/bin/tt"
    if [ -x "$local_bin" ]; then
        echo "$local_bin"
        return 0
    fi
    local local_bin_exe="$project_root/bin/tt.exe"
    if [ -x "$local_bin_exe" ]; then
        echo "$local_bin_exe"
        return 0
    fi
    echo "[SKIP] tt tool is not installed. This is expected in some environments." >&2
    echo "[SKIP] Do NOT attempt to install tt. Simply skip this step and continue." >&2
    exit 0
}

TOOL="$(_resolve_tt)"
