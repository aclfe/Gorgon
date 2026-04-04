#!/usr/bin/env bash
# run_benchmarks.sh — Benchmark runner for Gorgon
#
# Usage:
#   ./scripts/run_benchmarks.sh              # run all benchmarks
#   ./scripts/run_benchmarks.sh --time 5s    # set bench time
#   ./scripts/run_benchmarks.sh --mem        # include memory benchmarks
#   ./scripts/run_benchmarks.sh --compare    # compare against previous baseline

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
OUTPUT_DIR="$ROOT_DIR/benchmarks"
BENCH_TIME="3s"
BENCH_MEM=false
COMPARE=false

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }

while [[ $# -gt 0 ]]; do
    case "$1" in
        --time|-t) BENCH_TIME="$2"; shift 2 ;;
        --mem|-m) BENCH_MEM=true; shift ;;
        --compare|-c) COMPARE=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--time DURATION] [--mem] [--compare]"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

mkdir -p "$OUTPUT_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
OUTPUT_FILE="$OUTPUT_DIR/benchmark_results_${TIMESTAMP}.txt"
LATEST_LINK="$OUTPUT_DIR/latest.txt"

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Benchmarks${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
info "Bench time: $BENCH_TIME"
info "Output: $OUTPUT_FILE"
echo ""

# ── Header ────────────────────────────────────────────────────────────────
{
    echo "=============================================="
    echo "Gorgon Mutation Testing Tool — Benchmark Results"
    echo "=============================================="
    echo ""
    echo "Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "Go Version: $(go version)"
    echo "OS: $(uname -s) $(uname -m)"
    echo "CPU Cores: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown')"
    echo "Benchmark Time: $BENCH_TIME"
    echo ""
} > "$OUTPUT_FILE"

# ── Engine Benchmarks ─────────────────────────────────────────────────────
info "Engine benchmarks…"
{
    echo "=== ENGINE ==="
    go test -bench=. -benchtime="$BENCH_TIME" -benchmem -run=^$ ./internal/engine 2>&1 || true
    echo ""
} >> "$OUTPUT_FILE"

# ── Mutator Benchmarks ────────────────────────────────────────────────────
info "Mutator benchmarks…"
{
    echo "=== MUTATOR ==="
    go test -bench=. -benchtime="$BENCH_TIME" -benchmem -run=^$ ./pkg/mutator 2>&1 || true
    echo ""
} >> "$OUTPUT_FILE"

# ── Schemata Benchmarks ───────────────────────────────────────────────────
info "Schemata benchmarks…"
{
    echo "=== SCHEMATA ==="
    go test -bench=. -benchtime="$BENCH_TIME" -benchmem -run=^$ ./internal/testing 2>&1 || true
    echo ""
} >> "$OUTPUT_FILE"

# ── Reporter Benchmarks ───────────────────────────────────────────────────
info "Reporter benchmarks…"
{
    echo "=== REPORTER ==="
    go test -bench=. -benchtime="$BENCH_TIME" -benchmem -run=^$ ./internal/reporter 2>&1 || true
    echo ""
} >> "$OUTPUT_FILE"

# ── Memory benchmarks (optional) ─────────────────────────────────────────
if $BENCH_MEM; then
    info "Memory benchmarks…"
    {
        echo "=== MEMORY PROFILES ==="
        go test -bench=. -benchmem -memprofile="$OUTPUT_DIR/mem.out" -cpuprofile="$OUTPUT_DIR/cpu.out" -benchtime="$BENCH_TIME" -run=^$ ./internal/engine 2>&1 || true
        echo ""
    } >> "$OUTPUT_FILE"
fi

# ── Compare (optional) ────────────────────────────────────────────────────
if $COMPARE && [[ -f "$LATEST_LINK" ]]; then
    info "Comparing against previous baseline…"
    PREV=$(readlink -f "$LATEST_LINK")
    {
        echo "=== COMPARISON (benchstat) ==="
        if command -v benchstat &>/dev/null; then
            benchstat "$PREV" "$OUTPUT_FILE" 2>&1 || echo "benchstat comparison failed"
        else
            echo "benchstat not installed. Run: go install golang.org/x/perf/cmd/benchstat@latest"
            diff "$PREV" "$OUTPUT_FILE" 2>&1 || true
        fi
        echo ""
    } >> "$OUTPUT_FILE"
fi

# Update latest link
ln -sf "$OUTPUT_FILE" "$LATEST_LINK"

# ── Quick summary to stdout ───────────────────────────────────────────────
echo ""
info "Key results:"
echo ""
grep -E "^Benchmark" "$OUTPUT_FILE" | awk '{printf "  %-50s %12s %8s\n", $1, $2, $3}' || true
echo ""
info "Full results: $OUTPUT_FILE"
info "Latest: $LATEST_LINK"

if $BENCH_MEM; then
    info "Memory profile: $OUTPUT_DIR/mem.out"
    info "CPU profile: $OUTPUT_DIR/cpu.out"
    info "View: go tool pprof -http=:8080 $OUTPUT_DIR/mem.out"
fi
