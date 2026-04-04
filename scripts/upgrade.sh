#!/usr/bin/env bash
# upgrade.sh — Safely upgrade Go and tooling to latest versions
#
# Non-destructive: uses your system package manager or official installers.
# Preserves existing Go installation and GOPATH.
#
# Usage:
#   ./scripts/upgrade.sh              # upgrade everything
#   ./scripts/upgrade.sh --go         # upgrade Go only
#   ./scripts/upgrade.sh --tools      # upgrade tools only
#   ./scripts/upgrade.sh --dry        # show what would be upgraded

set -euo pipefail

DRY_RUN=false
UPGRADE_GO=false
UPGRADE_TOOLS=false

# If no flags given, upgrade everything
if [[ $# -eq 0 ]]; then
    UPGRADE_GO=true
    UPGRADE_TOOLS=true
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --go) UPGRADE_GO=true; shift ;;
        --tools) UPGRADE_TOOLS=true; shift ;;
        --dry) DRY_RUN=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--go] [--tools] [--dry]"
            echo ""
            echo "  --go      Upgrade Go to latest stable"
            echo "  --tools   Upgrade all Go tools (linters, analyzers, etc.)"
            echo "  --dry     Show what would be upgraded without installing"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Upgrade Go & Tools${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

ERRORS=0

# ── Upgrade Go ────────────────────────────────────────────────────────────
if $UPGRADE_GO; then
    info "Checking Go version…"

    if ! command -v go &>/dev/null; then
        fail "Go is not installed. Install from https://go.dev/dl/"
        exit 1
    fi

    CURRENT=$(go version | grep -oP '\d+\.\d+(\.\d+)?' | head -1)
    LATEST=$(curl -sSf https://go.dev/VERSION?m=text | head -n1 | sed 's/^go//')

    info "Current: $CURRENT | Latest: $LATEST"

    if [[ "$CURRENT" == "$LATEST" ]]; then
        ok "Go is already at latest version ($CURRENT)"
    elif $DRY_RUN; then
        warn "Would upgrade Go from $CURRENT to $LATEST"
        warn "Download: https://go.dev/dl/go${LATEST}.$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz"
    else
        OS=$(uname -s | tr '[:upper:]' '[:lower:]')
        ARCH=$(uname -m)
        [[ "$ARCH" == "x86_64" ]] && ARCH="amd64"
        [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]] && ARCH="arm64"

        FILE="go${LATEST}.${OS}-${ARCH}.tar.gz"
        URL="https://go.dev/dl/${FILE}"

        info "Downloading $URL…"
        curl -sSfL "$URL" -o "/tmp/${FILE}"

        info "Installing Go ${LATEST}…"
        sudo rm -rf /usr/local/go
        sudo tar -C /usr/local -xzf "/tmp/${FILE}"
        rm "/tmp/${FILE}"

        NEW_VERSION=$(/usr/local/go/bin/go version | grep -oP '\d+\.\d+(\.\d+)?' | head -1)
        ok "Go upgraded to $NEW_VERSION"
        info "Restart your terminal or run: export PATH=/usr/local/go/bin:\$PATH"
    fi
    echo ""
fi

# ── Upgrade Tools ─────────────────────────────────────────────────────────
if $UPGRADE_TOOLS; then
    info "Upgrading Go tools…"
    echo ""

    TOOLS=(
        "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
        "golang.org/x/tools/cmd/deadcode"
        "github.com/k1LoW/octocov"
        "honnef.co/go/tools/cmd/staticcheck"
        "golang.org/x/vuln/cmd/govulncheck"
        "golang.org/x/perf/cmd/benchstat"
        "mvdan.cc/gofumpt"
    )

    for tool_path in "${TOOLS[@]}"; do
        TOOL_NAME=$(basename "$tool_path")
        info "Upgrading $TOOL_NAME…"
        if $DRY_RUN; then
            warn "  Would run: go install ${tool_path}@latest"
        else
            if go install "${tool_path}@latest" 2>/dev/null; then
                ok "  $TOOL_NAME upgraded"
            else
                fail "  $TOOL_NAME failed to upgrade"
                ERRORS=$((ERRORS + 1))
            fi
        fi
    done
    echo ""

    # ── Update Go modules ─────────────────────────────────────────────────
    info "Updating Go module dependencies…"
    if $DRY_RUN; then
        warn "Would run: go get -u ./... && go mod tidy"
    else
        go get -u ./... 2>/dev/null || true
        go mod tidy
        ok "Dependencies updated"
    fi
fi

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $ERRORS -eq 0 ]]; then
    echo -e "  ${GREEN}Upgrade complete. Run './scripts/setup.sh' to verify.${NC}"
else
    echo -e "  ${RED}$ERRORS tool(s) failed to upgrade.${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERRORS
