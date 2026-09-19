#!/bin/bash
set -euo pipefail

# ============================================================
# build.sh — Full Build & Unit Test Runner
#
# Builds the entire project and runs unit tests.
# Integration tests (under tests/) are excluded;
# use integration_test.sh for those.
#
# Usage:
#   ./scripts/process/build.sh [OPTIONS]
#
# Options:
#   --backend-only   Run only the Go backend build & tests
#   --help           Show this help message
#
# Exit Codes:
#   0 = All builds and tests passed
#   1 = Build or test failure
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color
BOLD='\033[1m'

# --- Helpers ---
info()    { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[PASS]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail()    { echo -e "${RED}[FAIL]${NC} $*"; }
step()    { echo -e "${CYAN}${BOLD}===> $*${NC}"; }

show_help() {
    cat << 'EOF'
Usage: ./scripts/process/build.sh [OPTIONS]

Builds the entire project and runs unit tests.
Integration tests (under tests/) are excluded.

Options:
  --help           Show this help message

Exit Codes:
  0 = All builds and tests passed
  1 = Build or test failure

Examples:
  # Full build
  ./scripts/process/build.sh

EOF
}

# --- Argument Parsing ---

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            show_help
            exit 0
            ;;
        *)
            fail "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# --- Track overall result ---
FAILED=false

# ============================================================
# Go Build & Unit Test
# ============================================================
build_go() {
    step "Go: Build & Unit Test"

    cd "$PROJECT_ROOT"

    # Ensure bin/ directory exists
    mkdir -p "$PROJECT_ROOT/bin"

    # Enumerate features/{name}/ directories containing go.mod
    local found_any=false
    for feature_dir in features/*/; do
        # Skip if glob didn't match (no features/ directories)
        [[ -d "$feature_dir" ]] || continue

        # Only process directories that contain go.mod (Go projects)
        if [[ ! -f "$feature_dir/go.mod" ]]; then
            info "Skipping $feature_dir — no go.mod found."
            continue
        fi

        found_any=true
        local feature_name
        feature_name=$(basename "$feature_dir")

        step "Feature: $feature_name"
        cd "$PROJECT_ROOT/$feature_dir"

        # --- Unit Tests ---
        info "Running Go unit tests for $feature_name (excluding tests/ directory)..."

        UNIT_PKGS=$(go list ./... | grep -v '/tests/' | grep -v '/tests$' || true)

        if [[ -z "$UNIT_PKGS" ]]; then
            warn "No Go unit test packages found for $feature_name."
        elif echo "$UNIT_PKGS" | xargs go test -v -count=1; then
            success "Unit tests passed for $feature_name."
        else
            fail "Unit tests failed for $feature_name."
            FAILED=true
            return 1
        fi

        # --- Build ---
        # go build -o <file> ./... fails when the module has more than one
        # package. Compile the single main package instead.
        info "Building $feature_name..."
        local main_pkgs=()
        while IFS= read -r pkg; do
            [[ -n "$pkg" ]] && main_pkgs+=("$pkg")
        done < <(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)
        if [[ ${#main_pkgs[@]} -ne 1 ]]; then
            fail "Feature $feature_name must contain exactly one main package (found ${#main_pkgs[@]})."
            FAILED=true
            return 1
        fi
        out="$PROJECT_ROOT/bin/$feature_name"
        case "$(uname -s)" in
            MINGW*|MSYS*|CYGWIN*) out="${out}.exe" ;;
        esac
        if go build -o "$out" "${main_pkgs[0]}"; then
            success "Build succeeded for $feature_name → bin/$(basename "$out")"
        else
            fail "Build failed for $feature_name."
            FAILED=true
            return 1
        fi

        cd "$PROJECT_ROOT"
    done

    if [[ "$found_any" == "false" ]]; then
        warn "No Go projects found under features/*/."
        warn "Expected structure: features/{name}/go.mod"
        return 0
    fi
}

# ============================================================
# Main
# ============================================================
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║     Build & Unit Test Pipeline           ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
    echo ""

    local start_time=$SECONDS

    build_go

    local elapsed=$(( SECONDS - start_time ))
    echo ""
    echo -e "${BOLD}─────────────────────────────────────────────${NC}"

    if [[ "$FAILED" == "true" ]]; then
        fail "Build pipeline FAILED (${elapsed}s)"
        echo -e "${RED}Fix the errors above before running integration tests.${NC}"
        exit 1
    else
        success "Build pipeline PASSED (${elapsed}s)"
        echo -e "${GREEN}Ready for integration tests: ./scripts/process/integration_test.sh${NC}"
        exit 0
    fi
}

main
