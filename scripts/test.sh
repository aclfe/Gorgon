#!/usr/bin/env bash
# test.sh — Run all Go unit tests with race detector
#
# Usage:
#   ./scripts/test.sh              # run all tests
#   ./scripts/test.sh --count 3    # run N times (catch flaky tests)
#   ./scripts/test.sh --verbose    # verbose output

set -euo pipefail

COUNT=1
VERBOSE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --count) COUNT="$2"; shift 2 ;;
        --verbose|-v) VERBOSE=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--count N] [--verbose]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[PASS]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; }

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Unit Tests${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

FLAGS="-race -count=$COUNT"
if $VERBOSE; then
    FLAGS="$FLAGS -v"
fi

info "Running tests ($COUNT run(s), race detector enabled)…"
echo ""

if go test $FLAGS ./...; then
    echo ""
    ok "All tests passed"
    exit 0
else
    echo ""
    fail "Tests failed"
    exit 1
fi
