package regression

// import (
// 	"context"
// 	"path/filepath"
// 	"strings"
// 	"testing"

// 	gorgontesting "github.com/aclfe/gorgon/internal/core"
// 	"github.com/aclfe/gorgon/internal/engine"
// 	"github.com/aclfe/gorgon/pkg/config"
// 	"github.com/aclfe/gorgon/pkg/mutator"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/concurrency"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_panic_recover"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/error_handling"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/function_call_removal"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_body_removal"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_first"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_removal"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/math_operators"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/negate_condition"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/reference_returns"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/switch_mutations"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
// 	_ "github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
// 	"github.com/aclfe/gorgon/tests/testutil"
// )

// // compilation errors during schema generation and execution.
// // This catches: undefined vars, unused vars, type errors, invalid operations, etc.
// func TestMutationPipelineCompiles(t *testing.T) {
// 	testCases := []struct {
// 		name      string
// 		path      string
// 		operators []string
// 	}{
// 		{"arithmetic_flip_all", "../../examples/mutations/arithmetic_flip", []string{
// 			"arithmetic_flip", "assignment_operator", "binary_math", "inc_dec_flip", "sign_toggle",
// 		}},
// 		{"boundary_value_all", "../../examples/mutations/boundary_value", []string{
// 			"boundary_value", "condition_negation", "negate_condition",
// 		}},
// 		{"logical_all", "../../examples/mutations/logical_operator", []string{
// 			"logical_operator", "condition_negation", "negate_condition",
// 		}},
// 		{"conditional_all", "../../examples/mutations/conditional_expression", []string{
// 			"if_condition_true", "if_condition_false", "for_condition_true", "for_condition_false",
// 		}},
// 		{"loop_all", "../../examples/mutations/loop_body_removal", []string{
// 			"loop_body_removal", "loop_break_first", "loop_break_removal",
// 		}},
// 		{"switch_all", "../../examples/mutations/switch_mutations", []string{
// 			"swap_case_bodies", "switch_remove_default",
// 		}},
// 		{"reference_all", "../../examples/mutations/reference_returns", []string{
// 			"pointer_returns", "slice_returns", "map_returns", "channel_returns", "interface_returns",
// 		}},
// 		{"zero_value_all", "../../examples/mutations/zero_value_return", []string{
// 			"zero_value_return_numeric", "zero_value_return_string", "zero_value_return_bool", "zero_value_return_error",
// 		}},
// 		{"literal_all", "../../examples/mutations/constant_replacement", []string{
// 			"constant_replacement", "variable_replacement",
// 		}},
// 		{"function_body_all", "../../examples/mutations/empty_body", []string{
// 			"empty_body", "defer_removal", "early_return_removal",
// 		}},
// 		{"error_handling_all", "../../examples/mutations/error_handling", []string{
// 			"error_check_removal", "error_return_nil",
// 		}},
// 		{"function_call_all", "../../examples/mutations/function_call_removal", []string{
// 			"function_call_removal",
// 		}},
// 		{"goroutine_all", "../../examples/mutations/goroutine_removal", []string{
// 			"goroutine_removal",
// 		}},
// 	}

// 	for _, tc := range testCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			absPath, err := filepath.Abs(tc.path)
// 			if err != nil {
// 				t.Fatal(err)
// 			}

// 			var operators []mutator.Operator
// 			for _, opName := range tc.operators {
// 				op, ok := mutator.Get(opName)
// 				if !ok {
// 					t.Skipf("Operator %s not found, skipping", opName)
// 				}
// 				operators = append(operators, op)
// 			}

// 			if len(operators) == 0 {
// 				t.Skip("No operators available")
// 			}

// 			eng := engine.NewEngine(false)
// 			eng.SetOperators(operators)
// 			if err := eng.Traverse(absPath, nil); err != nil {
// 				t.Fatalf("Traverse failed: %v", err)
// 			}

// 			sites := eng.Sites()
// 			if len(sites) == 0 {
// 				t.Skipf("No mutation sites found in %s", tc.path)
// 			}

// 			_, err = gorgontesting.GenerateAndRunSchemata(
// 				context.Background(),
// 				sites,
// 				operators,
// 				operators,
// 				absPath,
// 				absPath,
// 				nil,
// 				nil,
// 				2,
// 				nil,
// 				nil,
// 				nil,
// 				testutil.Logger(),
// 				false,
// 				true,
// 				config.ExternalSuitesConfig{},
// 				config.Default(),
// 			)

// 			if err != nil {
// 				errStr := err.Error()

// 				var errorType string
// 				switch {
// 				case strings.Contains(errStr, "undefined:"):
// 					errorType = "UNDEFINED VARIABLE in generated schema"
// 				case strings.Contains(errStr, "declared and not used"):
// 					errorType = "UNUSED VARIABLE in generated schema"
// 				case strings.Contains(errStr, "cannot use"):
// 					errorType = "TYPE MISMATCH in generated schema"
// 				case strings.Contains(errStr, "invalid operation"):
// 					errorType = "INVALID OPERATION in generated schema"
// 				case strings.Contains(errStr, "too many errors"):
// 					errorType = "MULTIPLE COMPILATION ERRORS in generated schema"
// 				default:
// 					errorType = "PIPELINE ERROR"
// 				}

// 				t.Fatalf("%s for operators %v:\n%v", errorType, tc.operators, err)
// 			}
// 		})
// 	}
// }
