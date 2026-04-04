#!/usr/bin/env bash
# ci-check.sh — CI validation script with segmented and full modes
#
# Usage:
#   ./scripts/ci-check.sh              # run everything (full mode)
#   ./scripts/ci-check.sh --mode full  # run everything
#   ./scripts/ci-check.sh --mode lint  # run only lint
#   ./scripts/ci-check.sh --mode test  # run only tests
#   ./scripts/ci-check.sh --mode build # run only build
#   ./scripts/ci-check.sh --mode tidy  # run only tidy + fmt + vet + vulncheck
#   ./scripts/ci-check.sh --skip lint  # skip a specific check (full mode)

set -euo pipefail

# ── Colors ────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[SKIP]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

# ── Parse args ────────────────────────────────────────────────────────────
MODE="full"
SKIP_CHECKS=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --mode)
            MODE="$2"
            shift 2
            ;;
        --skip)
            SKIP_CHECKS="$2"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [--mode MODE] [--skip CHECK]"
            echo ""
            echo "Modes: full, lint, test, build, tidy"
            echo "  full   — run everything (tidy, fmt, vet, vulncheck, test, coverage, deadcode, lint)"
            echo "  lint   — run only golangci-lint"
            echo "  test   — run only unit tests"
            echo "  build  — run only cross-platform build"
            echo "  tidy   — run tidy, fmt, vet, vulncheck (no tests, no lint)"
            echo ""
            echo "Checks (for --skip in full mode): tidy fmt vet vulncheck test coverage deadcode lint"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

should_run() {
    local check="$1"
    if [[ -n "$SKIP_CHECKS" ]]; then
        IFS=',' read -ra SKIP_ARR <<< "$SKIP_CHECKS"
        for s in "${SKIP_ARR[@]}"; do
            if [[ "$s" == "$check" ]]; then
                return 1
            fi
        done
    fi
    return 0
}

# ── Header ────────────────────────────────────────────────────────────────
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  Gorgon — CI Validation [mode: $MODE]"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

ERRORS=0
PASS_COUNT=0

run_check() {
    local name="$1"
    shift

    if ! should_run "$name"; then
        warn "$name (skipped)"
        return 0
    fi

    info "Running: $name"
    if "$@"; then
        ok "$name"
        PASS_COUNT=$((PASS_COUNT + 1))
    else
        fail "$name"
        ERRORS=$((ERRORS + 1))
    fi
    echo ""
}

# ── Mode: tidy ────────────────────────────────────────────────────────────
run_tidy() {
    run_check "tidy" bash -c '
        set -e
        go mod tidy
        go mod download 2>/dev/null || true
        if [[ -n "$(git diff -- go.mod go.sum 2>/dev/null)" ]]; then
            echo "go.mod/go.sum are not tidy — run: go mod tidy"
            exit 1
        fi
    '

    run_check "fmt" bash -c '
        set -e
        OUTPUT=$(go run mvdan.cc/gofumpt@latest -d . 2>&1 | head -50)
        echo "$OUTPUT"
        if [[ -n "$OUTPUT" ]]; then
            echo "Code is not formatted — run: go run mvdan.cc/gofumpt@latest -w ."
            exit 1
        fi
    '

    run_check "vet" bash -c '
        go vet ./...
    '

    run_check "vulncheck" bash -c '
        if ! command -v govulncheck &>/dev/null; then
            go install golang.org/x/vuln/cmd/govulncheck@latest
        fi
        govulncheck ./...
    '
}

# ── Mode: test ────────────────────────────────────────────────────────────
run_test() {
    run_check "test" bash -c '
        go clean -testcache
        PKGS=$(go list ./... | grep -v "github.com/aclfe/gorgon/cmd/gorgon$" | grep -v "github.com/aclfe/gorgon/examples/")
        go test -race $PKGS
    '
}

# ── Mode: build ───────────────────────────────────────────────────────────
run_build() {
    run_check "build" bash -c '
        mkdir -p bin
        GOOS=linux GOARCH=amd64 go build -o /dev/null ./cmd/gorgon
        GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/gorgon
        GOOS=darwin GOARCH=amd64 go build -o /dev/null ./cmd/gorgon
        GOOS=darwin GOARCH=arm64 go build -o /dev/null ./cmd/gorgon
    '
}

# ── Mode: lint ────────────────────────────────────────────────────────────
run_lint() {
    run_check "lint" bash -c '
        if ! command -v golangci-lint &>/dev/null; then
            echo "golangci-lint not found — run: ./scripts/setup.sh"
            exit 1
        fi
        golangci-lint run --timeout=5m ./...
    '
}

# ── Dispatch by mode ──────────────────────────────────────────────────────
case "$MODE" in
    full)
        run_tidy
        run_test
        run_check "coverage" bash -c '
            go clean -testcache
            PKGS=$(go list ./... | grep -v "github.com/aclfe/gorgon/cmd/gorgon$" | grep -v "github.com/aclfe/gorgon/examples/")
            COVER_PKGS=$(echo "$PKGS" | tr "\n" "," | sed "s/,$//")
            go test -coverpkg="$COVER_PKGS" -coverprofile=coverage.out $PKGS
            COVERAGE=$(go tool cover -func=coverage.out | grep total | awk "{print \$3}" | tr -d "%")
            COVERAGE_INT=$(echo "$COVERAGE" | cut -d. -f1)
            echo "Coverage: ${COVERAGE}%"
            if [[ "$COVERAGE_INT" -lt 70 ]]; then
                echo "Coverage below 70% threshold"
                exit 1
            fi
        '
        run_check "deadcode" bash -c '
            if ! command -v deadcode &>/dev/null; then
                go install golang.org/x/tools/cmd/deadcode@latest
            fi
            deadcode -test ./...
        '
        run_lint
        ;;
    tidy)
        run_tidy
        ;;
    test)
        run_test
        ;;
    build)
        run_build
        ;;
    lint)
        run_lint
        ;;
    *)
        echo "Unknown mode: $MODE"
        echo "Valid modes: full, tidy, test, build, lint"
        exit 1
        ;;
esac

# ── Summary ───────────────────────────────────────────────────────────────
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $ERRORS -eq 0 ]]; then
    echo -e "  ${GREEN}All $PASS_COUNT checks passed. PR away.${NC}"
else
    echo -e "  ${RED}$ERRORS check(s) failed, $PASS_COUNT passed.${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERRORS
