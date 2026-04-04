#!/usr/bin/env bash
# performance-regression.sh — Track execution time against baseline
#
# Runs a fixed mutation workload, measures duration, compares against
# a stored baseline. Fails if performance regresses beyond threshold.
#
# Usage:
#   ./scripts/performance-regression.sh              # check against baseline
#   ./scripts/performance-regression.sh --baseline   # update baseline (main branch only)
#   ./scripts/performance-regression.sh --threshold 20  # set % threshold

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
GORGON="$ROOT_DIR/bin/gorgon"
BASELINE_FILE="$ROOT_DIR/benchmarks/performance-baseline.txt"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

UPDATE_BASELINE=false
THRESHOLD=20

while [[ $# -gt 0 ]]; do
    case "$1" in
        --baseline|-b) UPDATE_BASELINE=true; shift ;;
        --threshold|-t) THRESHOLD="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: $0 [--baseline] [--threshold N]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

if [[ ! -x "$GORGON" ]]; then
    info "Building gorgon…"
    mkdir -p "$ROOT_DIR/bin"
    go build -o "$GORGON" "$ROOT_DIR/cmd/gorgon"
fi

mkdir -p "$ROOT_DIR/benchmarks"

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Performance Regression Check${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# ── Fixed workload ────────────────────────────────────────────────────────
TARGET="$ROOT_DIR/examples/mutations"
OPERATORS="all"

info "Workload: $TARGET"
info "Operators: $OPERATORS"
info "Threshold: ${THRESHOLD}%"
echo ""

# ── Measure ───────────────────────────────────────────────────────────────
info "Running benchmark…"
START_NS=$(date +%s%N)
"$GORGON" -operators "$OPERATORS" "$TARGET" &>/dev/null || true
END_NS=$(date +%s%N)
ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))

info "Elapsed: ${ELAPSED_MS}ms"
echo ""

# ── Baseline ──────────────────────────────────────────────────────────────
if $UPDATE_BASELINE; then
    echo "$ELAPSED_MS" > "$BASELINE_FILE"
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$BASELINE_FILE"
    ok "Baseline updated: ${ELAPSED_MS}ms"
    echo "  File: $BASELINE_FILE"
    exit 0
fi

if [[ ! -f "$BASELINE_FILE" ]]; then
    warn "No baseline found. Creating one…"
    echo "$ELAPSED_MS" > "$BASELINE_FILE"
    echo "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$BASELINE_FILE"
    ok "Baseline created: ${ELAPSED_MS}ms"
    exit 0
fi

BASELINE_MS=$(head -1 "$BASELINE_FILE")
BASELINE_DATE=$(sed -n '2p' "$BASELINE_FILE")

info "Baseline: ${BASELINE_MS}ms (from $BASELINE_DATE)"

# ── Compare ───────────────────────────────────────────────────────────────
if [[ $ELAPSED_MS -gt $(( BASELINE_MS * (100 + THRESHOLD) / 100 )) ]]; then
    REGRESSION=$(( (ELAPSED_MS - BASELINE_MS) * 100 / BASELINE_MS ))
    fail "Performance regressed by ${REGRESSION}% (${BASELINE_MS}ms → ${ELAPSED_MS}ms)"
    echo ""
    echo "  To update baseline (after confirming improvement is expected):"
    echo "    $0 --baseline"
    exit 1
else
    ok "Performance within threshold (${BASELINE_MS}ms → ${ELAPSED_MS}ms)"
    exit 0
fi
