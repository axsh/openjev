#!/bin/bash
set -euo pipefail

# ============================================================
# get_latest.sh — Get the latest file modification time under specified directories
#
# Usage:
#   get_latest.sh <dir> [<dir>...]
#
# Output (stdout):
#   <epoch_seconds> <filepath>
#
# Exit Codes:
#   0 = Success
#   1 = No files found / insufficient arguments
#
# Examples:
#   # Single directory
#   ./scripts/utils/get_latest.sh services/roslyn-server
#   # => 1740648000 services/roslyn-server/Program.cs
#
#   # Multiple directories
#   ./scripts/utils/get_latest.sh services/roslyn-server docker/roslyn-server
#
#   # Get epoch only
#   EPOCH=$(./scripts/utils/get_latest.sh services/roslyn-server | cut -d' ' -f1)
#
#   # Display in human-readable format
#   ./scripts/utils/get_latest.sh services/roslyn-server | \
#     awk '{ system("date -d @" $1 " \"+%Y-%m-%d %H:%M:%S\""); print "  " $2 }'
# ============================================================

show_help() {
    cat << 'EOF'
Usage: get_latest.sh <dir> [<dir>...]

Recursively searches the specified directories and outputs
the modification time (epoch seconds) and path of the newest file to stdout.

Output format:
  <epoch_seconds> <filepath>

Exit codes:
  0 = Success
  1 = No files found or invalid arguments

Options:
  --help    Show this help message
EOF
}

if [[ "${1:-}" == "--help" ]] || [[ "$#" -lt 1 ]]; then
    show_help
    exit 1
fi

LATEST_EPOCH=0
LATEST_FILE=""

for DIR in "$@"; do
    if [[ ! -d "$DIR" ]]; then
        echo "Warning: directory not found: $DIR" >&2
        continue
    fi

    while IFS= read -r file; do
        # Get file modification epoch (try GNU stat, then BSD stat)
        FILE_EPOCH=$(stat -c %Y "$file" 2>/dev/null) || \
        FILE_EPOCH=$(stat -f %m "$file" 2>/dev/null) || continue

        if [[ "$FILE_EPOCH" -gt "$LATEST_EPOCH" ]]; then
            LATEST_EPOCH="$FILE_EPOCH"
            LATEST_FILE="$file"
        fi
    done < <(find "$DIR" -type f -not -path '*/\.*' -not -name '*.swp' -not -name '*~')
done

if [[ "$LATEST_EPOCH" -eq 0 ]]; then
    echo "No files found in specified directories" >&2
    exit 1
fi

echo "$LATEST_EPOCH $LATEST_FILE"
