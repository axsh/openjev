#!/bin/bash
set -euo pipefail

# ============================================================
# integration_test.sh — Integration Test Runner
#
# Runs Go integration tests located under the tests/ directory.
# Requires go.mod to exist in tests/ to proceed.
#
# Usage:
#   ./scripts/process/integration_test.sh [OPTIONS]
#
# Options:
#   --specify <Filter>   Run only tests matching the filter
#                        (passed to 'go test -run')
#   --help               Show this help message
#
# Exit Codes:
#   0 = All tests passed (or no tests to run)
#   1 = Test failure
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
Usage: ./scripts/process/integration_test.sh [OPTIONS]

Runs Go integration tests in the tests/ directory.
Requires tests/go.mod to exist.

Options:
  --specify <Filter>   Run only tests matching the filter
                       (passed to 'go test -run')
  --help               Show this help message

Go tests are executed with -tags integration and -timeout 45m.
The timeout is longer than the Go default because the decision load matrix can exceed 10 minutes.

Exit Codes:
  0 = All tests passed (or no tests to run)
  1 = Test failure

Examples:
  # Run all integration tests
  ./scripts/process/integration_test.sh

  # Run a specific test by name
  ./scripts/process/integration_test.sh --specify "TestNewFeature"
EOF
}

# --- Argument Parsing ---
SPECIFY=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --specify)
            if [[ -z "${2:-}" ]]; then
                fail "--specify requires a value"
                exit 1
            fi
            SPECIFY="$2"
            shift 2
            ;;
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

# ============================================================
# Main
# ============================================================
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║     Integration Test Pipeline            ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════╝${NC}"
    echo ""

    cd "$PROJECT_ROOT"

    local start_time=$SECONDS

    # Check tests/ directory
    if [[ ! -d "tests" ]]; then
        warn "tests/ directory not found — no integration tests to run."
        exit 0
    fi

    # Check go.mod in tests/
    if [[ ! -f "tests/go.mod" ]]; then
        warn "tests/go.mod not found — no integration tests to run."
        exit 0
    fi

    # Run Go integration tests
    step "Running Go integration tests"

    cd "$PROJECT_ROOT/tests"

    local go_test_args=("-v" "-count=1" "-timeout" "45m" "-tags" "integration")
    if [[ -n "$SPECIFY" ]]; then
        go_test_args+=("-run" "$SPECIFY")
        info "Test filter: $SPECIFY"
    fi

    if go test "${go_test_args[@]}" ./...; then
        local elapsed=$(( SECONDS - start_time ))
        echo ""
        echo -e "${BOLD}─────────────────────────────────────────────${NC}"
        success "All integration tests passed! (${elapsed}s)"
        exit 0
    else
        local elapsed=$(( SECONDS - start_time ))
        echo ""
        echo -e "${BOLD}─────────────────────────────────────────────${NC}"
        fail "Integration tests FAILED. (${elapsed}s)"
        exit 1
    fi
}

main
