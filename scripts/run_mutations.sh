#!/usr/bin/env bash
# run_mutations.sh — Run mutations on all example folders with output capture
#
# Usage:
#   ./scripts/run_mutations.sh              # run all examples
#   ./scripts/run_mutations.sh --ops all    # use specific operators
#   ./scripts/run_mutations.sh --json       # output JSON summary
#   ./scripts/run_mutations.sh --concurrent half

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
GORGON="$ROOT_DIR/bin/gorgon"
EXAMPLES_DIR="$ROOT_DIR/examples/mutations"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC}  $*"; }
ok()    { echo -e "${GREEN}[PASS]${NC}  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }

OPERATORS="all"
CONCURRENT="all"
JSON_OUTPUT=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --ops|-o) OPERATORS="$2"; shift 2 ;;
        --concurrent|-c) CONCURRENT="$2"; shift 2 ;;
        --json|-j) JSON_OUTPUT=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--ops OPS] [--concurrent N] [--json]"
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

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${BLUE}  Gorgon — Mutation Runner${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

TOTAL_MUTANTS=0
TOTAL_KILLED=0
TOTAL_SURVIVED=0
DIR_COUNT=0
ERROR_COUNT=0

if $JSON_OUTPUT; then
    echo "{"
    echo "  \"date\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\","
    echo "  \"operators\": \"$OPERATORS\","
    echo "  \"concurrent\": \"$CONCURRENT\","
    echo "  \"directories\": ["
fi

FIRST_JSON=true

for dir in "$EXAMPLES_DIR"/*/; do
    [[ -d "$dir" ]] || continue
    DIRNAME=$(basename "$dir")
    DIR_COUNT=$((DIR_COUNT + 1))

    info "[$DIR_COUNT] $DIRNAME"

    set +e
    OUTPUT=$(timeout 60 "$GORGON" -operators "$OPERATORS" -concurrent "$CONCURRENT" "$dir" 2>&1)
    EXIT_CODE=$?
    set -e

    if [[ $EXIT_CODE -eq 124 ]]; then
        fail "  Timeout (60s)"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    elif [[ $EXIT_CODE -gt 1 ]]; then
        fail "  Error (exit $EXIT_CODE)"
        ERROR_COUNT=$((ERROR_COUNT + 1))
    else
        MUTANTS=$(echo "$OUTPUT" | grep -i "Total:" | grep -oP '\d+' | tail -1 || echo "0")
        KILLED=$(echo "$OUTPUT" | grep -i "Killed:" | grep -oP '\d+' | head -1 || echo "0")
        SURVIVED=$(echo "$OUTPUT" | grep -i "Survived:" | grep -oP '\d+' | head -1 || echo "0")
        SCORE=$(echo "$OUTPUT" | grep -i "Mutation Score" | grep -oP '[\d.]+' | head -1 || echo "0")

        TOTAL_MUTANTS=$((TOTAL_MUTANTS + MUTANTS))
        TOTAL_KILLED=$((TOTAL_KILLED + KILLED))
        TOTAL_SURVIVED=$((TOTAL_SURVIVED + SURVIVED))

        if [[ "$MUTANTS" -gt 0 ]]; then
            ok "  $MUTANTS mutants | Killed: $KILLED | Survived: $SURVIVED | Score: ${SCORE}%"
        else
            info "  No mutants generated"
        fi

        if $JSON_OUTPUT; then
            if ! $FIRST_JSON; then echo ","; fi
            FIRST_JSON=false
            echo "    {"
            echo "      \"name\": \"$DIRNAME\","
            echo "      \"mutants\": $MUTANTS,"
            echo "      \"killed\": $KILLED,"
            echo "      \"survived\": $SURVIVED,"
            echo "      \"score\": $SCORE"
            echo -n "    }"
        fi
    fi
    echo ""
done

if $JSON_OUTPUT; then
    echo ""
    echo "  ],"
    echo "  \"total\": {"
    echo "    \"mutants\": $TOTAL_MUTANTS,"
    echo "    \"killed\": $TOTAL_KILLED,"
    echo "    \"survived\": $TOTAL_SURVIVED"
    echo "  }"
    echo "}"
fi

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  ${BLUE}Summary: $DIR_COUNT directories | $TOTAL_MUTANTS mutants | $TOTAL_KILLED killed | $TOTAL_SURVIVED survived | $ERROR_COUNT errors${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

exit $ERROR_COUNT
