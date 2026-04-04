#!/usr/bin/env bash
# cli-contract.sh — Verify CLI flags, exit codes, and output format
#
# Tests the CLI contract to prevent breaking changes.
#
# Usage:
#   ./scripts/cli-contract.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

if [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    GORGON="$ROOT_DIR/bin/gorgon.exe"
else
    GORGON="$ROOT_DIR/bin/gorgon"
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

if [[ ! -x "$GORGON" ]]; then
    info "Building gorgon…"
    mkdir -p "$ROOT_DIR/bin"
    go build -o "$GORGON" "$ROOT_DIR/cmd/gorgon"
fi

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — CLI Contract Tests${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

ERRORS=0
PASS_COUNT=0
TEST_NUM=0

run_test() {
    local name="$1"
    local expected_exit="$2"
    local check_output="${3:-}"
    shift 3

    TEST_NUM=$((TEST_NUM + 1))
    info "Test $TEST_NUM: $name"

    set +e
    OUTPUT=$("$@" 2>&1)
    ACTUAL_EXIT=$?
    set -e

    # Check exit code
    if [[ "$ACTUAL_EXIT" -ne "$expected_exit" ]]; then
        fail "  Expected exit $expected_exit, got $ACTUAL_EXIT"
        echo "  Output: $(echo "$OUTPUT" | head -3)"
        ERRORS=$((ERRORS + 1))
        return
    fi

    # Check output content if specified
    if [[ -n "$check_output" ]]; then
        if [[ "$OUTPUT" != *"$check_output"* ]]; then
            fail "  Output missing expected string: '$check_output'"
            echo "  Output: $(echo "$OUTPUT" | head -5)"
            ERRORS=$((ERRORS + 1))
            return
        fi
    fi

    ok "$name"
    PASS_COUNT=$((PASS_COUNT + 1))
}

# ── Help & Usage ──────────────────────────────────────────────────────────
run_test "--help exits 0" 0 "" "$GORGON" --help
run_test "no args shows usage" 1 "Usage:" "$GORGON"

# ── Print AST ─────────────────────────────────────────────────────────────
run_test "-print-ast on valid file" 0 "File" \
    "$GORGON" -print-ast "$ROOT_DIR/examples/mutations/arithmetic_flip/arithmetic_flip.go"

run_test "-print-ast on another file" 0 "AST" \
    "$GORGON" -print-ast "$ROOT_DIR/pkg/mutator/mutator.go"

# ── Operators ─────────────────────────────────────────────────────────────
run_test "single operator" 0 "" \
    "$GORGON" -operators arithmetic_flip "$ROOT_DIR/examples/mutations/arithmetic_flip"

run_test "multiple operators" 0 "" \
    "$GORGON" -operators arithmetic_flip,condition_negation "$ROOT_DIR/examples/mutations"

run_test "category operator" 0 "" \
    "$GORGON" -operators logical "$ROOT_DIR/examples/mutations"

run_test "unknown operator exits 1" 1 "Unknown operator" \
    "$GORGON" -operators nonexistent_operator "$ROOT_DIR/examples/mutations/arithmetic_flip"

# ── Concurrency ───────────────────────────────────────────────────────────
run_test "-concurrent=all" 0 "" \
    "$GORGON" -concurrent=all "$ROOT_DIR/examples/mutations/arithmetic_flip"

run_test "-concurrent=half" 0 "" \
    "$GORGON" -concurrent=half "$ROOT_DIR/examples/mutations/arithmetic_flip"

run_test "-concurrent=2" 0 "" \
    "$GORGON" -concurrent=2 "$ROOT_DIR/examples/mutations/arithmetic_flip"

# ── Invalid paths ─────────────────────────────────────────────────────────
run_test "nonexistent path exits 1" 1 "" \
    "$GORGON" "$ROOT_DIR/nonexistent/path"

# ── Output format ─────────────────────────────────────────────────────────
run_test "output contains mutation score" 0 "Mutation Score" \
    "$GORGON" "$ROOT_DIR/examples/mutations/arithmetic_flip"

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $ERRORS -eq 0 ]]; then
    echo -e "  ${GREEN}All $PASS_COUNT CLI contract tests passed.${NC}"
else
    echo -e "  ${RED}$ERRORS CLI contract test(s) failed, $PASS_COUNT passed.${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERRORS
