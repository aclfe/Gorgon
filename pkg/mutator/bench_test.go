// Package mutator_test provides comprehensive benchmarks for mutation operators.
package mutator_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/assignment_operator"
	"github.com/aclfe/gorgon/pkg/mutator/boundary_value"
	"github.com/aclfe/gorgon/pkg/mutator/defer_removal"
	"github.com/aclfe/gorgon/pkg/mutator/inc_dec_flip"
	"github.com/aclfe/gorgon/pkg/mutator/logical_operator"
	"github.com/aclfe/gorgon/pkg/mutator/loop_body_removal"
	"github.com/aclfe/gorgon/pkg/mutator/loop_break_first"
	"github.com/aclfe/gorgon/pkg/mutator/loop_break_removal"
	"github.com/aclfe/gorgon/pkg/mutator/sign_toggle"
	"github.com/aclfe/gorgon/pkg/mutator/switch_mutations"
	"github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
	_ "github.com/aclfe/gorgon/pkg/mutator/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/variable_replacement"
)

// =============================================================================
// Test Code Samples for Benchmarks
// =============================================================================

const (
	arithmeticCode = `package test
func Add(a, b int) int { return a + b }
func Sub(a, b int) int { return a - b }
func Mul(a, b int) int { return a * b }
func Div(a, b int) int { return a / b }
`
	logicalCode = `package test
func And(a, b bool) bool { return a && b }
func Or(a, b bool) bool { return a || b }
func Complex(a, b, c bool) bool { return (a && b) || c }
`
	conditionCode = `package test
func Eq(a, b int) bool { return a == b }
func Neq(a, b int) bool { return a != b }
func Lt(a, b int) bool { return a < b }
func Lte(a, b int) bool { return a <= b }
func Gt(a, b int) bool { return a > b }
func Gte(a, b int) bool { return a >= b }
`
	boundaryCode = `package test
func Check(a, b int) bool {
	if a < b { return true }
	if a > b { return false }
	return a <= b
}
`
	assignmentCode = `package test
func Test() {
	var x int
	x = 5
	x += 10
	x -= 5
	x *= 2
	x /= 2
}
`
	returnCode = `package test
func GetNum() int { return 42 }
func GetStr() string { return "hello" }
func GetBool() bool { return true }
func GetError() error { return fmt.Errorf("error") }
`
	loopCode = `package test
func LoopTest() {
	for i := 0; i < 10; i++ {
		if i == 5 { break }
	}
	for _, v := range []int{1,2,3} {
		_ = v
	}
}
`
	incDecCode = `package test
func IncDec() {
	var x int
	x++
	x--
}
`
	signCode = `package test
func Sign() {
	var x int
	x = -5
	x = +10
}
`
	switchCode = `package test
func SwitchTest(x int) {
	switch x {
	case 1: return 1
	case 2: return 2
	default: return 0
	}
}
`
	deferCode = `package test
func DeferTest() {
	defer cleanup()
	defer fn()
}
func cleanup() {}
func fn() {}
`
)

// =============================================================================
// Helper Functions
// =============================================================================

func parseCode(b *testing.B, code string) *ast.File {
	b.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}
	return f
}

func findNodes(b *testing.B, f *ast.File, nodeType string) []ast.Node {
	b.Helper()
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n != nil && getNodeTypeName(n) == nodeType {
			nodes = append(nodes, n)
		}
		return true
	})
	return nodes
}

func getNodeTypeName(n ast.Node) string {
	if n == nil {
		return ""
	}
	switch n.(type) {
	case *ast.BinaryExpr:
		return "BinaryExpr"
	case *ast.ReturnStmt:
		return "ReturnStmt"
	case *ast.AssignStmt:
		return "AssignStmt"
	case *ast.ForStmt:
		return "ForStmt"
	case *ast.RangeStmt:
		return "RangeStmt"
	case *ast.IncDecStmt:
		return "IncDecStmt"
	case *ast.UnaryExpr:
		return "UnaryExpr"
	case *ast.SwitchStmt:
		return "SwitchStmt"
	case *ast.CaseClause:
		return "CaseClause"
	case *ast.DeferStmt:
		return "DeferStmt"
	case *ast.IfStmt:
		return "IfStmt"
	case *ast.CallExpr:
		return "CallExpr"
	case *ast.Ident:
		return "Ident"
	case *ast.BasicLit:
		return "BasicLit"
	default:
		return "Other"
	}
}

// =============================================================================
// Arithmetic Operator Benchmarks
// =============================================================================

func BenchmarkMutator_ArithmeticFlip_CanApply(b *testing.B) {
	f := parseCode(b, arithmeticCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ArithmeticFlip{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_ArithmeticFlip_Mutate(b *testing.B) {
	f := parseCode(b, arithmeticCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ArithmeticFlip{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

func BenchmarkMutator_ArithmeticFlip_FullCycle(b *testing.B) {
	f := parseCode(b, arithmeticCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ArithmeticFlip{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			if op.CanApply(node) {
				_ = op.Mutate(node)
			}
		}
	}
}

// =============================================================================
// Logical Operator Benchmarks
// =============================================================================

func BenchmarkMutator_LogicalOperator_CanApply(b *testing.B) {
	f := parseCode(b, logicalCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := logical_operator.LogicalOperator{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_LogicalOperator_Mutate(b *testing.B) {
	f := parseCode(b, logicalCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := logical_operator.LogicalOperator{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Condition Negation Benchmarks
// =============================================================================

func BenchmarkMutator_ConditionNegation_CanApply(b *testing.B) {
	f := parseCode(b, conditionCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ConditionNegation{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_ConditionNegation_Mutate(b *testing.B) {
	f := parseCode(b, conditionCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ConditionNegation{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Boundary Value Benchmarks
// =============================================================================

func BenchmarkMutator_BoundaryValue_CanApply(b *testing.B) {
	f := parseCode(b, boundaryCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := boundary_value.BoundaryValue{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_BoundaryValue_Mutate(b *testing.B) {
	f := parseCode(b, boundaryCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := boundary_value.BoundaryValue{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Assignment Operator Benchmarks
// =============================================================================

func BenchmarkMutator_AssignmentOperator_CanApply(b *testing.B) {
	f := parseCode(b, assignmentCode)
	nodes := findNodes(b, f, "AssignStmt")
	op := assignment_operator.AssignmentOperator{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_AssignmentOperator_Mutate(b *testing.B) {
	f := parseCode(b, assignmentCode)
	nodes := findNodes(b, f, "AssignStmt")
	op := assignment_operator.AssignmentOperator{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Zero Value Return Benchmarks
// =============================================================================

func BenchmarkMutator_ZeroValueReturnNumeric_CanApply(b *testing.B) {
	f := parseCode(b, returnCode)
	nodes := findNodes(b, f, "ReturnStmt")
	op := zero_value_return.ZeroValueReturnNumeric{}
	ctx := mutator.Context{ReturnType: "int"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApplyWithContext(node, ctx)
		}
	}
}

func BenchmarkMutator_ZeroValueReturnNumeric_Mutate(b *testing.B) {
	f := parseCode(b, returnCode)
	nodes := findNodes(b, f, "ReturnStmt")
	op := zero_value_return.ZeroValueReturnNumeric{}
	ctx := mutator.Context{ReturnType: "int"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.MutateWithContext(node, ctx)
		}
	}
}

func BenchmarkMutator_ZeroValueReturnString_CanApply(b *testing.B) {
	f := parseCode(b, returnCode)
	nodes := findNodes(b, f, "ReturnStmt")
	op := zero_value_return.ZeroValueReturnString{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApplyWithContext(node, mutator.Context{})
		}
	}
}

func BenchmarkMutator_ZeroValueReturnBool_CanApply(b *testing.B) {
	f := parseCode(b, returnCode)
	nodes := findNodes(b, f, "ReturnStmt")
	op := zero_value_return.ZeroValueReturnBool{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApplyWithContext(node, mutator.Context{})
		}
	}
}

// =============================================================================
// Loop Mutation Benchmarks
// =============================================================================

func BenchmarkMutator_LoopBodyRemoval_CanApply(b *testing.B) {
	f := parseCode(b, loopCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.ForStmt); ok {
			nodes = append(nodes, n)
		}
		if _, ok := n.(*ast.RangeStmt); ok {
			nodes = append(nodes, n)
		}
		return true
	})
	op := loop_body_removal.LoopBodyRemoval{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_LoopBreakFirst_CanApply(b *testing.B) {
	f := parseCode(b, loopCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.ForStmt); ok {
			nodes = append(nodes, n)
		}
		return true
	})
	op := loop_break_first.LoopBreakFirst{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_LoopBreakRemoval_CanApply(b *testing.B) {
	f := parseCode(b, loopCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.BranchStmt); ok {
			nodes = append(nodes, n)
		}
		return true
	})
	op := loop_break_removal.LoopBreakRemoval{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

// =============================================================================
// Increment/Decrement Benchmarks
// =============================================================================

func BenchmarkMutator_IncDecFlip_CanApply(b *testing.B) {
	f := parseCode(b, incDecCode)
	nodes := findNodes(b, f, "IncDecStmt")
	op := inc_dec_flip.IncDecFlip{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_IncDecFlip_Mutate(b *testing.B) {
	f := parseCode(b, incDecCode)
	nodes := findNodes(b, f, "IncDecStmt")
	op := inc_dec_flip.IncDecFlip{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Sign Toggle Benchmarks
// =============================================================================

func BenchmarkMutator_SignToggle_CanApply(b *testing.B) {
	f := parseCode(b, signCode)
	nodes := findNodes(b, f, "UnaryExpr")
	op := sign_toggle.SignToggle{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

func BenchmarkMutator_SignToggle_Mutate(b *testing.B) {
	f := parseCode(b, signCode)
	nodes := findNodes(b, f, "UnaryExpr")
	op := sign_toggle.SignToggle{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.Mutate(node)
		}
	}
}

// =============================================================================
// Switch Mutation Benchmarks
// =============================================================================

func BenchmarkMutator_SwitchRemoveDefault_CanApply(b *testing.B) {
	f := parseCode(b, switchCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.CaseClause); ok {
			nodes = append(nodes, n)
		}
		return true
	})
	op := switch_mutations.SwitchRemoveDefault{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApply(node)
		}
	}
}

// =============================================================================
// Defer Removal Benchmarks
// =============================================================================

func BenchmarkMutator_DeferRemoval_CanApply(b *testing.B) {
	f := parseCode(b, deferCode)
	nodes := findNodes(b, f, "DeferStmt")
	op := defer_removal.DeferRemoval{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.CanApplyWithContext(node, mutator.Context{})
		}
	}
}

func BenchmarkMutator_DeferRemoval_Mutate(b *testing.B) {
	f := parseCode(b, deferCode)
	nodes := findNodes(b, f, "DeferStmt")
	op := defer_removal.DeferRemoval{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			_ = op.MutateWithContext(node, mutator.Context{})
		}
	}
}

// =============================================================================
// All Operators Combined Benchmarks
// =============================================================================

const allOperatorsCode = `package test
func Add(a, b int) int { return a + b }
func And(a, b bool) bool { return a && b }
func Eq(a, b int) bool { return a == b }
func GetNum() int { return 42 }
`

func BenchmarkMutator_AllOperators_CanApply(b *testing.B) {
	f := parseCode(b, allOperatorsCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n != nil {
			nodes = append(nodes, n)
		}
		return true
	})
	ops := mutator.List()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			for _, op := range ops {
				if cop, ok := op.(mutator.ContextualOperator); ok {
					_ = cop.CanApplyWithContext(node, mutator.Context{})
				} else {
					_ = op.CanApply(node)
				}
			}
		}
	}
}

func BenchmarkMutator_AllOperators_FullCycle(b *testing.B) {
	f := parseCode(b, allOperatorsCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n != nil {
			nodes = append(nodes, n)
		}
		return true
	})
	ops := mutator.List()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			ctx := mutator.Context{ReturnType: "int"}
			for _, op := range ops {
				if cop, ok := op.(mutator.ContextualOperator); ok {
					if cop.CanApplyWithContext(node, ctx) {
						_ = cop.MutateWithContext(node, ctx)
					}
				} else {
					if op.CanApply(node) {
						_ = op.Mutate(node)
					}
				}
			}
		}
	}
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

func BenchmarkMutator_ArithmeticFlip_Allocations(b *testing.B) {
	f := parseCode(b, arithmeticCode)
	nodes := findNodes(b, f, "BinaryExpr")
	op := mutator.ArithmeticFlip{}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			if op.CanApply(node) {
				_ = op.Mutate(node)
			}
		}
	}
}

func BenchmarkMutator_AllOperators_Allocations(b *testing.B) {
	f := parseCode(b, allOperatorsCode)
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n != nil {
			nodes = append(nodes, n)
		}
		return true
	})
	ops := mutator.List()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range nodes {
			for _, op := range ops {
				if cop, ok := op.(mutator.ContextualOperator); ok {
					_ = cop.CanApplyWithContext(node, mutator.Context{})
				} else {
					_ = op.CanApply(node)
				}
			}
		}
	}
}

// =============================================================================
// Operator Registry Benchmarks
// =============================================================================

func BenchmarkMutator_Registry_List(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mutator.List()
	}
}

func BenchmarkMutator_Registry_All(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = mutator.All()
	}
}

func BenchmarkMutator_Registry_Get(b *testing.B) {
	opNames := []string{
		"arithmetic_flip",
		"condition_negation",
		"logical_operator",
		"boundary_value",
		"zero_value_return_numeric",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range opNames {
			_, _ = mutator.Get(name)
		}
	}
}

func BenchmarkMutator_Registry_GetCategory(b *testing.B) {
	categories := []string{
		"arithmetic",
		"logical",
		"binary",
		"zero_value_return",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, cat := range categories {
			_, _ = mutator.GetCategory(cat)
		}
	}
}
