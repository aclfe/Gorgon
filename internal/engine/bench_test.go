// Package engine_test provides comprehensive benchmarks for the engine package.
package engine_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/assignment_operator"
	"github.com/aclfe/gorgon/pkg/mutator/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/constant_replacement"
	"github.com/aclfe/gorgon/pkg/mutator/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/inc_dec_flip"
	"github.com/aclfe/gorgon/pkg/mutator/logical_operator"
	"github.com/aclfe/gorgon/pkg/mutator/loop_body_removal"
	"github.com/aclfe/gorgon/pkg/mutator/loop_break_first"
	"github.com/aclfe/gorgon/pkg/mutator/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/variable_replacement"
	"github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
)

const (
	smallExamplePath    = "../../examples/mutations/arithmetic_flip"
	mediumExamplePath   = "../../examples/mutations"
	largeExamplePath    = "../../examples"
	benchmarkIterations = 1
)

// =============================================================================
// AST Traversal Benchmarks
// =============================================================================

func BenchmarkEngine_TraverseSmallDirectory(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(smallExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

func BenchmarkEngine_TraverseMediumDirectory(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(mediumExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

func BenchmarkEngine_TraverseLargeDirectory(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(largeExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

// =============================================================================
// Site Detection Benchmarks
// =============================================================================

func BenchmarkEngine_SiteDetectionSmall(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(smallExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
		sites := e.Sites()
		if len(sites) == 0 {
			b.Fatal("Expected sites to be detected")
		}
	}
}

func BenchmarkEngine_SiteDetectionMedium(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(mediumExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
		sites := e.Sites()
		if len(sites) == 0 {
			b.Fatal("Expected sites to be detected")
		}
	}
}

func BenchmarkEngine_SiteDetectionLarge(b *testing.B) {
	ops := loadAllOperators(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(largeExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
		sites := e.Sites()
		if len(sites) == 0 {
			b.Fatal("Expected sites to be detected")
		}
	}
}

// =============================================================================
// AST Printing Benchmarks
// =============================================================================

func BenchmarkEngine_PrintTreeSmall(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, smallExamplePath+"/arithmetic_flip.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	engine.PrintEnabled.Store(true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.PrintTree(io.Discard, fset, f); err != nil {
			b.Fatalf("PrintTree failed: %v", err)
		}
	}
}

func BenchmarkEngine_PrintTreeMedium(b *testing.B) {
	// Parse multiple files for medium benchmark
	fset := token.NewFileSet()
	files := parseDirectory(b, fset, mediumExamplePath)

	engine.PrintEnabled.Store(true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, f := range files {
			if err := engine.PrintTree(io.Discard, fset, f); err != nil {
				b.Fatalf("PrintTree failed: %v", err)
			}
		}
	}
}

// =============================================================================
// Context Building Benchmarks
// =============================================================================

func BenchmarkEngine_BuildContext(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, smallExamplePath+"/arithmetic_flip.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	var targetNode ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.BinaryExpr); ok {
			targetNode = n
			return false
		}
		return true
	})

	if targetNode == nil {
		b.Fatal("No BinaryExpr found for benchmark")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Access unexported buildContext via traversal
		e := engine.NewEngine(false)
		_ = e.Traverse(smallExamplePath, nil)
	}
}

// =============================================================================
// Operator Application Benchmarks
// =============================================================================

func BenchmarkEngine_OperatorApplication(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, smallExamplePath+"/arithmetic_flip.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	ops := loadAllOperators(b)

	// Collect all nodes
	var nodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if n != nil {
			nodes = append(nodes, n)
		}
		return true
	})

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
// Single File vs Directory Benchmarks
// =============================================================================

func BenchmarkEngine_SingleFileTraversal(b *testing.B) {
	singleFile := filepath.Join(smallExamplePath, "arithmetic_flip.go")
	ops := loadAllOperators(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(singleFile, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

func BenchmarkEngine_MultiFileTraversal(b *testing.B) {
	ops := loadAllOperators(b)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(mediumExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

// =============================================================================
// Node Type Specific Benchmarks
// =============================================================================

func BenchmarkEngine_BinaryExprDetection(b *testing.B) {
	ops := []mutator.Operator{
		mutator.ArithmeticFlip{},
		mutator.ConditionNegation{},
		logical_operator.LogicalOperator{},
		boundary_value.BoundaryValue{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(smallExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

func BenchmarkEngine_ReturnStmtDetection(b *testing.B) {
	ops := []mutator.Operator{
		zero_value_return.ZeroValueReturnNumeric{},
		zero_value_return.ZeroValueReturnString{},
		zero_value_return.ZeroValueReturnBool{},
		zero_value_return.ZeroValueReturnError{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(mediumExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

func BenchmarkEngine_StatementDetection(b *testing.B) {
	ops := []mutator.Operator{
		defer_removal.DeferRemoval{},
		loop_body_removal.LoopBodyRemoval{},
		loop_break_first.LoopBreakFirst{},
		loop_break_removal.LoopBreakRemoval{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(mediumExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

func BenchmarkEngine_TraverseAllocations(b *testing.B) {
	ops := loadAllOperators(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		e.SetOperators(ops)
		if err := e.Traverse(smallExamplePath, nil); err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func loadAllOperators(b *testing.B) []mutator.Operator {
	b.Helper()
	ops := mutator.List()
	if len(ops) == 0 {
		b.Fatal("No operators registered")
	}
	return ops
}

func parseDirectory(b *testing.B, fset *token.FileSet, dir string) []*ast.File {
	b.Helper()
	var files []*ast.File

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		files = append(files, f)
		return nil
	})

	if err != nil {
		b.Fatalf("Walk failed: %v", err)
	}

	return files
}
