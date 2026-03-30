#!/bin/bash

GORGON="./bin/gorgon"
EXAMPLES="examples"
PASS=0
FAIL=0

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[1;33m'
nc='\033[0m'

run_test() {
    local name="$1"
    local cmd="$2"
    local expected_fail="${3:-false}"
    echo -e "${yellow}Testing: $name${nc}"
    echo "Command: $cmd"
    if eval "$cmd" > /tmp/gorgon_test.log 2>&1; then
        echo -e "${green}✓ PASS${nc}"
        ((PASS++))
    elif [ "$expected_fail" = "true" ]; then
        echo -e "${yellow}✓ EXPECTED FAIL${nc}"
        ((PASS++))
    else
        echo -e "${red}✗ FAIL${nc}"
        cat /tmp/gorgon_test.log
        ((FAIL++))
    fi
    echo "---"
    return 0
}

echo "=== Gorgon Test Suite ==="
echo ""

run_test "Single file (no go.mod)" "$GORGON $EXAMPLES/mutations/arithmetic_flip/arithmetic_flip.go" "true"
run_test "Single directory" "$GORGON $EXAMPLES/mutations/arithmetic_flip"
run_test "All examples" "$GORGON $EXAMPLES"

run_test "arithmetic_flip" "$GORGON -operators arithmetic_flip $EXAMPLES"
run_test "condition_negation" "$GORGON -operators condition_negation $EXAMPLES"
run_test "zero_value_return" "$GORGON -operators zero_value_return $EXAMPLES"
run_test "pointer_returns" "$GORGON -operators pointer_returns $EXAMPLES"
run_test "slice_returns" "$GORGON -operators slice_returns $EXAMPLES"
run_test "map_returns" "$GORGON -operators map_returns $EXAMPLES"
run_test "channel_returns" "$GORGON -operators channel_returns $EXAMPLES"
run_test "interface_returns" "$GORGON -operators interface_returns $EXAMPLES"

run_test "all operators" "$GORGON -operators all $EXAMPLES"
run_test "combo operators" "$GORGON -operators arithmetic_flip,condition_negation $EXAMPLES"
run_test "print AST" "$GORGON -print-ast $EXAMPLES/mutations/arithmetic_flip"

echo ""
echo "=== Summary ==="
echo -e "Passed: ${green}$PASS${nc}"
echo -e "Failed: ${red}$FAIL${nc}"

echo ""
echo "=== Survivors ==="
$GORGON $EXAMPLES 2>&1 | grep -A20 "Survived"
