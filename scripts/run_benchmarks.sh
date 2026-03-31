#!/bin/bash
# Benchmark runner script for Gorgon mutation testing tool
# Outputs comprehensive benchmark results to a text file

set -e

# Configuration
OUTPUT_DIR="./benchmarks"
OUTPUT_FILE="${OUTPUT_DIR}/benchmark_results_$(date +%Y%m%d_%H%M%S).txt"
BENCH_TIME="1s"
BENCH_MEM=false
BENCH_CPU=""

# Colors for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--time)
            BENCH_TIME="$2"
            shift 2
            ;;
        -m|--mem)
            BENCH_MEM=true
            shift
            ;;
        -c|--cpu)
            BENCH_CPU="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  -t, --time TIME    Benchmark time per test (default: 1s)"
            echo "  -m, --mem          Include memory allocation benchmarks"
            echo "  -c, --cpu CPUS     Number of CPUs to use"
            echo "  -o, --output FILE  Output file path"
            echo "  -h, --help         Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Create output directory
mkdir -p "$OUTPUT_DIR"

# Function to print colored output
print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

print_section() {
    echo -e "${GREEN}--- $1 ---${NC}"
}

print_info() {
    echo -e "${YELLOW}$1${NC}"
}

# Start benchmark output file
{
    echo "=============================================="
    echo "Gorgon Mutation Testing Tool - Benchmark Results"
    echo "=============================================="
    echo ""
    echo "Date: $(date)"
    echo "Go Version: $(go version)"
    echo "OS: $(uname -s) $(uname -m)"
    echo "CPU Cores: $(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 'unknown')"
    echo "Benchmark Time: ${BENCH_TIME}"
    echo "Memory Benchmarks: ${BENCH_MEM}"
    if [ -n "$BENCH_CPU" ]; then
        echo "CPU Limit: ${BENCH_CPU}"
    fi
    echo ""
    echo "=============================================="
} > "$OUTPUT_FILE"

print_header "Starting Gorgon Benchmarks"
print_info "Output file: $OUTPUT_FILE"
print_info "Benchmark time per test: $BENCH_TIME"

# Engine Benchmarks
print_section "Engine Benchmarks"
{
    echo ""
    echo "=============================================="
    echo "ENGINE BENCHMARKS"
    echo "=============================================="
    echo ""
} >> "$OUTPUT_FILE"

print_info "Running engine benchmarks..."
if ! go test -bench=BenchmarkEngine -benchtime="$BENCH_TIME" -run=^$ ./internal/engine >> "$OUTPUT_FILE" 2>&1; then
    print_info "Note: Some engine benchmarks may have failed"
fi

# Mutator Benchmarks
print_section "Mutator Benchmarks"
{
    echo ""
    echo "=============================================="
    echo "MUTATOR BENCHMARKS"
    echo "=============================================="
    echo ""
} >> "$OUTPUT_FILE"

print_info "Running mutator benchmarks..."
if ! go test -bench=BenchmarkMutator -benchtime="$BENCH_TIME" -run=^$ ./pkg/mutator >> "$OUTPUT_FILE" 2>&1; then
    print_info "Note: Some mutator benchmarks may have failed"
fi

# Schemata Benchmarks
print_section "Schemata Benchmarks"
{
    echo ""
    echo "=============================================="
    echo "SCHEMATA BENCHMARKS"
    echo "=============================================="
    echo ""
} >> "$OUTPUT_FILE"

print_info "Running schemata benchmarks..."
if ! go test -bench=BenchmarkSchemata -benchtime="$BENCH_TIME" -run=^$ ./internal/testing >> "$OUTPUT_FILE" 2>&1; then
    print_info "Note: Some schemata benchmarks may have failed"
fi

# Reporter Benchmarks
print_section "Reporter Benchmarks"
{
    echo ""
    echo "=============================================="
    echo "REPORTER BENCHMARKS"
    echo "=============================================="
    echo ""
} >> "$OUTPUT_FILE"

print_info "Running reporter benchmarks..."
if ! go test -bench=BenchmarkReporter -benchtime="$BENCH_TIME" -run=^$ ./internal/reporter >> "$OUTPUT_FILE" 2>&1; then
    print_info "Note: Some reporter benchmarks may have failed"
fi

# Full Pipeline Benchmarks
print_section "Full Pipeline Benchmarks"
{
    echo ""
    echo "=============================================="
    echo "FULL PIPELINE BENCHMARKS"
    echo "=============================================="
    echo ""
} >> "$OUTPUT_FILE"

print_info "Running full pipeline benchmarks..."
if ! go test -bench=BenchmarkPipeline -benchtime="$BENCH_TIME" -run=^$ ./internal/benchmark >> "$OUTPUT_FILE" 2>&1; then
    print_info "Note: Some pipeline benchmarks may have failed"
fi

# Memory allocation benchmarks if requested
if [ "$BENCH_MEM" = true ]; then
    print_section "Memory Allocation Benchmarks"
    {
        echo ""
        echo "=============================================="
        echo "MEMORY ALLOCATION BENCHMARKS"
        echo "=============================================="
        echo ""
    } >> "$OUTPUT_FILE"

    print_info "Running memory allocation benchmarks..."
    {
        echo "Engine Memory:"
        go test -bench=BenchmarkEngine.*Alloc -benchmem -benchtime="$BENCH_TIME" -run=^$ ./internal/engine 2>/dev/null || true
        echo ""
        echo "Mutator Memory:"
        go test -bench=BenchmarkMutator.*Alloc -benchmem -benchtime="$BENCH_TIME" -run=^$ ./pkg/mutator 2>/dev/null || true
        echo ""
        echo "Pipeline Memory:"
        go test -bench=BenchmarkPipeline.*Alloc -benchmem -benchtime="$BENCH_TIME" -run=^$ ./internal/benchmark 2>/dev/null || true
    } >> "$OUTPUT_FILE"
fi

# Summary section
{
    echo ""
    echo "=============================================="
    echo "BENCHMARK SUMMARY"
    echo "=============================================="
    echo ""
    echo "For detailed analysis, review the full output above."
    echo ""
    echo "Key metrics to compare:"
    echo "  - ns/op: Nanoseconds per operation (lower is better)"
    echo "  - B/op: Bytes allocated per operation (lower is better)"
    echo "  - allocs/op: Allocations per operation (lower is better)"
    echo "  - mutants/sec: Mutation testing throughput (higher is better)"
    echo "  - kill_rate_%: Mutation detection rate (context dependent)"
    echo ""
    echo "=============================================="
    echo "End of Benchmark Results"
    echo "=============================================="
} >> "$OUTPUT_FILE"

print_header "Benchmarks Complete!"
print_info "Results saved to: $OUTPUT_FILE"
print_info ""
print_info "To view results:"
print_info "  cat $OUTPUT_FILE"
print_info ""
print_info "To compare with previous runs:"
print_info "  diff <(cat $OUTPUT_FILE) <(cat ${OUTPUT_DIR}/benchmark_results_*.txt | tail -1)"

# Also output a quick summary to stdout
{
    echo ""
    echo "Quick Summary (top results from each category):"
    echo ""
}

# Extract some key metrics for quick viewing
echo "Engine:"
grep -E "BenchmarkEngine_(TraverseSmall|SiteDetectionSmall|PrintTreeSmall)" "$OUTPUT_FILE" | tail -3 || echo "  (no results)"
echo ""
echo "Mutator:"
grep -E "BenchmarkMutator_(ArithmeticFlip|LogicalOperator|AllOperators).*Full" "$OUTPUT_FILE" | tail -3 || echo "  (no results)"
echo ""
echo "Pipeline:"
grep -E "BenchmarkPipeline_(FullSmall|ConcurrencyScaling|MutationDetection)" "$OUTPUT_FILE" | tail -3 || echo "  (no results)"

echo ""
echo "Full results available in: $OUTPUT_FILE"
