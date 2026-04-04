#!/usr/bin/env bash
# internal-regression.sh — Per-operator regression tracking for Gorgon's own codebase
#
# Runs each mutation operator individually against pkg/, internal/, and cmd/gorgon/.
# Compares results (total, killed, survived, score, survived sites) against
# committed snapshots. Detects regressions when operators change unexpectedly.
#
# When you add a new operator:
#   1. Run the script — it will warn about the missing baseline
#   2. Review the output, then run with --update to commit the baseline
#
# When you modify an existing operator:
#   1. Run the script — it will show a diff of what changed
#   2. If intentional, run with --update to accept the new baseline
#
# Usage:
#   ./scripts/internal-regression.sh              # verify all operators
#   ./scripts/internal-regression.sh --update     # regenerate baselines
#   ./scripts/internal-regression.sh --op NAME    # check single operator
#   ./scripts/internal-regression.sh --timeout 60 # set timeout per operator

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SNAPSHOT_DIR="$ROOT_DIR/test/internal-snapshots"
GORGON="$ROOT_DIR/bin/gorgon"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }

UPDATE=false
SINGLE_OP=""
TIMEOUT=300

while [[ $# -gt 0 ]]; do
    case "$1" in
        --update|-u) UPDATE=true; shift ;;
        --op) SINGLE_OP="$2"; shift 2 ;;
        --timeout|-t) TIMEOUT="$2"; shift 2 ;;
        --help|-h)
            echo "Usage: $0 [--update] [--op NAME] [--timeout SECONDS]"
            echo ""
            echo "  --update    Regenerate all snapshot baselines"
            echo "  --op NAME   Check only the named operator"
            echo "  --timeout   Max seconds per operator (default: 300)"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# ── Discover operators from categoryMap ──────────────────────────────────
discover_operators() {
    local mutator_file="$ROOT_DIR/pkg/mutator/mutator.go"
    if [[ ! -f "$mutator_file" ]]; then
        echo "ERROR: Cannot find pkg/mutator/mutator.go" >&2
        exit 1
    fi

    # Extract operator names from categoryMap — each quoted string inside the map
    # that isn't a category key (category keys are the ones before {)
    # We parse lines like: "arithmetic_flip", inside the categoryMap block
    grep -oP '"[a-z_]+"(?=\s*[,}])' "$mutator_file" | \
        tr -d '"' | \
        sort -u | \
        grep -vE '^(arithmetic|logical|boundary|assignment|function_body|reference_returns|switch_mutations|zero_value_return|binary|literal|early_return|loop|statement|conditional_expression)$'
}

# ── Ensure binary exists ─────────────────────────────────────────────────
if [[ ! -x "$GORGON" ]]; then
    info "Building gorgon…"
    mkdir -p "$ROOT_DIR/bin"
    go build -o "$GORGON" "$ROOT_DIR/cmd/gorgon"
fi

mkdir -p "$SNAPSHOT_DIR"

OPERATORS=($(discover_operators))
TARGETS=(
    "$ROOT_DIR/pkg"
    "$ROOT_DIR/internal"
    "$ROOT_DIR/cmd/gorgon"
)

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if $UPDATE; then
    echo -e "${BLUE}  Gorgon — Update Internal Regression Baselines${NC}"
else
    echo -e "${BLUE}  Gorgon — Internal Regression Check${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
info "Discovered ${#OPERATORS[@]} operators"
info "Targets: pkg/ internal/ cmd/gorgon/"
info "Timeout per operator: ${TIMEOUT}s"
echo ""

# ── Run one operator against all targets, merge results ──────────────────
run_operator() {
    local op="$1"
    local tmpdir="$2"
    local total=0
    local killed=0
    local survived=0
    local errors=0

    # Collect all survived mutant lines across all targets
    local all_sites=""

    for target in "${TARGETS[@]}"; do
        if [[ ! -d "$target" && ! -f "$target" ]]; then
            continue
        fi

        set +e
        OUTPUT=$(timeout "$TIMEOUT" "$GORGON" -operators "$op" "$target" 2>&1)
        EXIT_CODE=$?
        set -e

        if [[ $EXIT_CODE -eq 124 ]]; then
            warn "  Timeout on $target after ${TIMEOUT}s"
            continue
        fi

        # Parse the data line (second line of output: "75.00%  9  3  0  12")
        # Format: score%  killed  survived  errors  total
        local data_line
        data_line=$(echo "$OUTPUT" | sed -n '2p')

        if [[ -z "$data_line" ]]; then
            continue
        fi

        local op_killed op_survived op_errors op_total
        op_killed=$(echo "$data_line" | awk '{print $2}')
        op_survived=$(echo "$data_line" | awk '{print $3}')
        op_errors=$(echo "$data_line" | awk '{print $4}')
        op_total=$(echo "$data_line" | awk '{print $5}')

        # Defaults if parsing fails
        op_killed=${op_killed:-0}
        op_survived=${op_survived:-0}
        op_errors=${op_errors:-0}
        op_total=${op_total:-0}

        total=$((total + op_total))
        killed=$((killed + op_killed))
        survived=$((survived + op_survived))
        errors=$((errors + op_errors))

        # Collect survived mutant sites
        local sites
        sites=$(echo "$OUTPUT" | grep -E '^- survived in ' | sort || true)
        if [[ -n "$sites" ]]; then
            if [[ -n "$all_sites" ]]; then
                all_sites="$all_sites"$'\n'"$sites"
            else
                all_sites="$sites"
            fi
        fi
    done

    # Calculate score
    local score="0.00"
    if [[ $total -gt 0 ]]; then
        score=$(awk "BEGIN {printf \"%.2f\", ($killed / $total) * 100}")
    fi

    # Write merged result to tmpdir
    cat > "$tmpdir/result.txt" <<EOF
Total: $total
Killed: $killed
Survived: $survived
Errors: $errors
Score: ${score}%
Sites:
$all_sites
EOF
}

# ── Compare result against snapshot ──────────────────────────────────────
check_operator() {
    local op="$1"
    local snapshot="$SNAPSHOT_DIR/${op}.snap"
    local result="$2"

    if [[ ! -f "$snapshot" ]]; then
        if $UPDATE; then
            cp "$result" "$snapshot"
            local total
            total=$(head -1 "$result" | awk '{print $2}')
            ok "$op — baseline created ($total mutants)"
        else
            warn "$op — no baseline exists (new operator)"
            echo "  Run with --update to create baseline"
        fi
        return 0
    fi

    if $UPDATE; then
        cp "$result" "$snapshot"
        local total
        total=$(head -1 "$result" | awk '{print $2}')
        ok "$op — baseline updated ($total mutants)"
        return 0
    fi

    local expected
    expected=$(cat "$snapshot")
    local actual
    actual=$(cat "$result")

    if [[ "$expected" == "$actual" ]]; then
        local total
        total=$(head -1 "$result" | awk '{print $2}')
        ok "$op ($total mutants, unchanged)"
        return 0
    fi

    # Show what changed
    local diff_output
    diff_output=$(diff <(echo "$expected") <(echo "$actual") 2>&1 || true)

    fail "$op — regression detected!"
    echo "$diff_output" | head -30 | sed 's/^/    /'
    return 1
}

# ── Run checks ───────────────────────────────────────────────────────────
ERRORS=0
PASS_COUNT=0
WARN_COUNT=0

TMP_BASE=$(mktemp -d)
trap 'rm -rf "$TMP_BASE"' EXIT

for op in "${OPERATORS[@]}"; do
    if [[ -n "$SINGLE_OP" && "$op" != "$SINGLE_OP" ]]; then
        continue
    fi

    info "Checking: $op"

    op_tmpdir="$TMP_BASE/$op"
    mkdir -p "$op_tmpdir"

    run_operator "$op" "$op_tmpdir"

    if check_operator "$op" "$op_tmpdir/result.txt"; then
        if [[ ! -f "$SNAPSHOT_DIR/${op}.snap" ]]; then
            WARN_COUNT=$((WARN_COUNT + 1))
        else
            PASS_COUNT=$((PASS_COUNT + 1))
        fi
    else
        ERRORS=$((ERRORS + 1))
    fi

    echo ""
done

# ── Summary ───────────────────────────────────────────────────────────────
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
if [[ $ERRORS -eq 0 ]]; then
    msg="${GREEN}All $PASS_COUNT operators passed"
    if [[ $WARN_COUNT -gt 0 ]]; then
        msg="$msg, $WARN_COUNT new operator(s) need baselining"
    fi
    echo -e "  ${msg}.${NC}"
else
    echo -e "  ${RED}$ERRORS operator(s) regressed, $PASS_COUNT passed, $WARN_COUNT new.${NC}"
fi
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERRORS
