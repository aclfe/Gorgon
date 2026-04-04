#!/usr/bin/env bash
# setup.sh — One-time developer environment setup for Gorgon
#
# Installs all required tools, verifies Go version, and prepares the workspace.
# Safe to run multiple times (idempotent).
#
# Usage:
#   ./scripts/setup.sh          # install everything
#   ./scripts/setup.sh --dry    # check what's missing without installing

set -euo pipefail

DRY_RUN=false
if [[ "${1:-}" == "--dry" ]]; then
    DRY_RUN=true
fi

# ── Colors ────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()    { echo -e "${RED}[FAIL]${NC}  $*"; }

# ── Helpers ───────────────────────────────────────────────────────────────
install_go_tool() {
    local tool="$1"
    local path="$2"
    local version="${3:-latest}"

    if command -v "$tool" &>/dev/null; then
        ok "$tool already installed ($( $tool --version 2>/dev/null | head -1 || echo 'version unknown' ))"
        return 0
    fi

    if $DRY_RUN; then
        warn "$tool is missing — would install: go install ${path}@${version}"
        return 1
    fi

    info "Installing $tool…"
    if go install "${path}@${version}"; then
        ok "$tool installed"
    else
        fail "Failed to install $tool"
        return 1
    fi
}

# ── Header ────────────────────────────────────────────────────────────────
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Developer Environment Setup${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

ERRORS=0

# ── 1. Go version ─────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
    fail "Go is not installed. Install from https://go.dev/dl/"
    ERRORS=$((ERRORS + 1))
else
    GO_VERSION=$(go version | grep -oP '\d+\.\d+(\.\d+)?' | head -1)
    # Require Go 1.24+
    GO_MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
    GO_MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
    if [[ "$GO_MAJOR" -ge 1 && "$GO_MINOR" -ge 24 ]]; then
        ok "Go $GO_VERSION"
    else
        fail "Go 1.24+ required (found $GO_VERSION)"
        ERRORS=$((ERRORS + 1))
    fi
fi

# ── 2. Go modules ─────────────────────────────────────────────────────────
info "Tidying Go modules…"
go mod tidy
go mod download
ok "Go modules ready"

# ── 3. Required tools ─────────────────────────────────────────────────────
echo ""
info "Checking required tools…"
echo ""

# Linter
install_go_tool "golangci-lint" \
    "github.com/golangci/golangci-lint/v2/cmd/golangci-lint" \
    "latest" || ERRORS=$((ERRORS + 1))

# Dead code detector
install_go_tool "deadcode" \
    "golang.org/x/tools/cmd/deadcode" \
    "latest" || ERRORS=$((ERRORS + 1))

# Coverage reporter
install_go_tool "octocov" \
    "github.com/k1LoW/octocov" \
    "latest" || ERRORS=$((ERRORS + 1))

# Static analysis
install_go_tool "staticcheck" \
    "honnef.co/go/tools/cmd/staticcheck" \
    "latest" || ERRORS=$((ERRORS + 1))

# Vulnerability checker
install_go_tool "govulncheck" \
    "golang.org/x/vuln/cmd/govulncheck" \
    "latest" || ERRORS=$((ERRORS + 1))

# ── 4. Build ──────────────────────────────────────────────────────────────
echo ""
info "Building gorgon binary…"
if $DRY_RUN; then
    warn "Would run: go build -o bin/gorgon ./cmd/gorgon"
else
    mkdir -p bin
    go build -o bin/gorgon ./cmd/gorgon
    ok "Binary built → bin/gorgon"
fi

# ── 5. Quick smoke test ──────────────────────────────────────────────────
if ! $DRY_RUN; then
    echo ""
    info "Running smoke test…"
    if bin/gorgon -print-ast examples/mutations/arithmetic_flip/arithmetic_flip.go &>/dev/null; then
        ok "Smoke test passed"
    else
        fail "Smoke test failed — something is broken"
        ERRORS=$((ERRORS + 1))
    fi
fi

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $ERRORS -eq 0 ]]; then
    echo -e "  ${GREEN}Setup complete. Run 'make check' to validate everything.${NC}"
else
    echo -e "  ${RED}$ERRORS issue(s) detected. Review output above.${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERRORS
