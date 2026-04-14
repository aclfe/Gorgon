package testing_test

import (
	"context"
	"path/filepath"
	stdtesting "testing"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
)




type expectedMutations struct {
	folder   string
	operator string
	total    int
	killed   int
	survived int
}

var expectedResults = []expectedMutations{
	{folder: "arithmetic_flip", operator: "arithmetic_flip", total: 6, killed: 3, survived: 3},
	{folder: "negate_condition", operator: "negate_condition", total: 2, killed: 2, survived: 0},
	{folder: "assignment_operator", operator: "assignment_operator", total: 4, killed: 4, survived: 0},
	{folder: "empty_body", operator: "empty_body", total: 4, killed: 0, survived: 4},
	{folder: "logical_operator", operator: "logical_operator", total: 4, killed: 4, survived: 0},
	{folder: "condition_negation", operator: "condition_negation", total: 4, killed: 2, survived: 2},
	{folder: "boundary_value", operator: "boundary_value", total: 6, killed: 5, survived: 1},
	{folder: "zero_value_return", operator: "zero_value_return_numeric", total: 1, killed: 1, survived: 0},
	{folder: "zero_value_return", operator: "zero_value_return_string", total: 1, killed: 1, survived: 0},
	{folder: "sign_toggle", operator: "sign_toggle", total: 4, killed: 4, survived: 0},
	{folder: "constant_replacement", operator: "constant_replacement", total: 6, killed: 5, survived: 1},
	{folder: "variable_replacement", operator: "variable_replacement", total: 18, killed: 15, survived: 3},
	{folder: "binary_math", operator: "binary_math", total: 5, killed: 5, survived: 0},
	{folder: "inc_dec_flip", operator: "inc_dec_flip", total: 2, killed: 2, survived: 0},
	{folder: "switch_mutations/switch_remove_default", operator: "switch_remove_default", total: 3, killed: 2, survived: 1},
	{folder: "switch_mutations/swap_case_bodies", operator: "swap_case_bodies", total: 11, killed: 11, survived: 0},
	{folder: "reference_returns/pointer_returns", operator: "pointer_returns", total: 2, killed: 1, survived: 1},
	{folder: "reference_returns/slice_returns", operator: "slice_returns", total: 3, killed: 2, survived: 1},
	{folder: "reference_returns/map_returns", operator: "map_returns", total: 3, killed: 2, survived: 1},
	{folder: "reference_returns/interface_returns", operator: "interface_returns", total: 2, killed: 2, survived: 0},
	{folder: "reference_returns/channel_returns", operator: "channel_returns", total: 2, killed: 1, survived: 1},
	{folder: "conditional_expression/if_condition_true", operator: "if_condition_true", total: 3, killed: 1, survived: 2},
	{folder: "conditional_expression/if_condition_false", operator: "if_condition_false", total: 3, killed: 2, survived: 1},
	{folder: "conditional_expression/for_condition_true", operator: "for_condition_true", total: 3, killed: 2, survived: 1},
	{folder: "conditional_expression/for_condition_false", operator: "for_condition_false", total: 2, killed: 2, survived: 0},
}

func TestMutationCounts(tst *stdtesting.T) {
	
	

	for _, expected := range expectedResults {
		tst.Run(expected.folder+"/"+expected.operator, func(t *stdtesting.T) {
			absPath, err := filepath.Abs("../../examples/mutations/" + expected.folder)
			if err != nil {
				t.Fatal(err)
			}

			op, ok := mutator.Get(expected.operator)
			if !ok {
				t.Fatalf("Unknown operator: %s", expected.operator)
			}

			eng := engine.NewEngine(false)
			eng.SetOperators([]mutator.Operator{op})
			if err := eng.Traverse(absPath, nil); err != nil {
				t.Fatalf("Traverse failed: %v", err)
			}

			sites := eng.Sites()
			operators := []mutator.Operator{op}

			mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, 2, nil, nil, nil, false, false)
			if err != nil {
				t.Fatalf("GenerateAndRunSchemata failed: %v", err)
			}

			if len(mutants) != expected.total {
				t.Errorf("Expected %d mutants, got %d", expected.total, len(mutants))
			}

			killed := 0
			survived := 0
			for _, m := range mutants {
				switch m.Status {
				case "killed":
					killed++
				case "survived":
					survived++
				}
			}

			if killed != expected.killed {
				t.Errorf("Expected %d killed, got %d", expected.killed, killed)
			}
			if survived != expected.survived {
				t.Errorf("Expected %d survived, got %d", expected.survived, survived)
			}
		})
	}
}

func TestAllOperatorsCombined(tst *stdtesting.T) {
	
	tst.Skip("Slow integration test - run explicitly if needed")
	
	absPath, err := filepath.Abs("../../examples/mutations")
	if err != nil {
		tst.Fatal(err)
	}

	operators := mutator.List()
	eng := engine.NewEngine(false)
	eng.SetOperators(operators)
	if err := eng.Traverse(absPath, nil); err != nil {
		tst.Fatalf("Traverse failed: %v", err)
	}

	sites := eng.Sites()

	mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, 2, nil, nil, nil, false, false)
	if err != nil {
		tst.Fatalf("GenerateAndRunSchemata failed: %v", err)
	}

	tst.Logf("Total mutants: %d", len(mutants))

	for _, m := range mutants {
		if m.Status == "survived" {
			tst.Logf("Survived: %s:%d (%s)", m.Site.File.Name(), m.Site.Line, m.Operator.Name())
		}
	}
}

func TestOperatorDetection(tst *stdtesting.T) {
	allOps := mutator.List()
	if len(allOps) == 0 {
		tst.Fatal("No operators registered")
	}

	names := make(map[string]bool)
	for _, op := range allOps {
		name := op.Name()
		if names[name] {
			tst.Errorf("Duplicate operator name: %s", name)
		}
		names[name] = true
		tst.Logf("Registered operator: %s", name)
	}

	expectedOperators := []string{
		"arithmetic_flip",
		"condition_negation",
		"boundary_value",
		"logical_operator",
		"negate_condition",
		"assignment_operator",
		"empty_body",
		"zero_value_return_numeric",
		"zero_value_return_string",
		"zero_value_return_bool",
		"zero_value_return_error",
		"sign_toggle",
		"binary_math",
		"inc_dec_flip",
		"switch_remove_default",
		"swap_case_bodies",
		"pointer_returns",
		"slice_returns",
		"map_returns",
		"channel_returns",
		"interface_returns",
		"if_condition_true",
		"if_condition_false",
		"for_condition_true",
		"for_condition_false",
		"constant_replacement",
		"variable_replacement",
		"early_return_removal",
		"loop_body_removal",
		"loop_break_first",
		"loop_break_removal",
		"defer_removal",
	}

	for _, expected := range expectedOperators {
		if !names[expected] {
			tst.Errorf("Missing expected operator: %s", expected)
		}
	}
}
