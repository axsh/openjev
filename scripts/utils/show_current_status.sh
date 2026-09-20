#!/bin/bash
set -e

# scripts/utils/show_current_status.sh

# 1. Determine Current Phase
# Look for directories in prompts/phases matching pattern [0-9][0-9][0-9]-*
# Sort to find the one with the largest number.
PHASE_DIR=$(find prompts/phases -maxdepth 1 -type d -name "[0-9][0-9][0-9]-*" | sort | tail -n 1)

if [ -z "$PHASE_DIR" ]; then
  # Fallback if no phase directory is found. Verify this behavior with user if critical.
  # For now, we assume at least 000-initial exists or return error/null?
  # Returning empty or specific error might be better, but let's default to "000-initial" or exit.
  # Given the user context, there's likely always a phase.
  echo "Error: No phase directory found in prompts/phases" >&2
  exit 1
fi
PHASE_NAME=$(basename "$PHASE_DIR")

# 2. Determine Current Branch
# Use git rev-parse to get the current branch name.
BRANCH_NAME=$(git rev-parse --abbrev-ref HEAD)
# If detached head or error, default to main or handle error.
if [ -z "$BRANCH_NAME" ]; then
    BRANCH_NAME="main"
fi

BRANCH_DIR="prompts/phases/${PHASE_NAME}/branches/${BRANCH_NAME}"

# Helper function to get the next available ID
get_next_id() {
  local target_dir="$1"
  
  # Ensure the directory exists, otherwise next ID is 000 (and mkdir needed by caller/workflow)
  if [ ! -d "$target_dir" ]; then
    echo "000"
    return
  fi

  # Find files matching [0-9][0-9][0-9]-*.md
  # Extract the leading 3 digits
  # Sort numerically
  # Take the last one (max)
  local last_id=$(find "$target_dir" -maxdepth 1 -name "[0-9][0-9][0-9]-*.md" | \
    sed 's/.*\/\([0-9]\{3\}\)-.*/\1/' | \
    sort -n | tail -n 1)

  if [ -z "$last_id" ]; then
    echo "000"
  else
    # Use 10# to force base-10 interpretation to avoid octal issues (e.g. 008)
    local next_val=$((10#$last_id + 1))
    printf "%03d" "$next_val"
  fi
}

# 3. Legacy fields (backward compatibility for ideas / plans)
NEXT_IDEA_ID=$(get_next_id "${BRANCH_DIR}/ideas")
NEXT_PLAN_ID=$(get_next_id "${BRANCH_DIR}/plans")

# 4. Dynamic fields: one next_<dirname>_id per subdirectory under the branch
DYNAMIC_FIELDS=""
if [ -d "$BRANCH_DIR" ]; then
  while IFS= read -r dir; do
    [ -z "$dir" ] && continue
    name=$(basename "$dir")
    next_id=$(get_next_id "$dir")
    DYNAMIC_FIELDS="${DYNAMIC_FIELDS},
  \"next_${name}_id\": \"${next_id}\""
  done < <(find "$BRANCH_DIR" -maxdepth 1 -mindepth 1 -type d | sort)
fi

# Output JSON
cat <<EOF
{
  "phase": "${PHASE_NAME}",
  "branch": "${BRANCH_NAME}",
  "next_idea_id": "${NEXT_IDEA_ID}",
  "next_plan_id": "${NEXT_PLAN_ID}"${DYNAMIC_FIELDS}
}
EOF