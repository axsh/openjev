#!/usr/bin/env bash
# Common tool discovery logic for tt prompt wrapper scripts.
# Source this file, then use $TOOL variable.

_resolve_tt() {
    if [ -n "${TT_TOOL:-}" ]; then
        echo "$TT_TOOL"
        return 0
    fi
    if command -v tt &>/dev/null; then
        echo "tt"
        return 0
    fi
    # Check project-local bin/
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local project_root
    project_root="$(cd "$script_dir/../.." && pwd)"
    local local_bin="$project_root/bin/tt"
    if [ -x "$local_bin" ]; then
        echo "$local_bin"
        return 0
    fi
    echo "Skipping coding agent settings update: tt tool not found." >&2
    echo "Set TT_TOOL env var, add tt to PATH, or place it in bin/" >&2
    return 1
}

TOOL="$(_resolve_tt)" || exit 1
