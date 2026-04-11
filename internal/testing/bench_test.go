
package testing_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	gtest "github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
)

func mustGetMutator(name string) mutator.Operator {
	op, ok := mutator.Get(name)
	if !ok {
		panic("mutator not found: " + name)
	}
	return op
}

const (
	smallCodebase  = "../../examples/mutations/arithmetic_flip"
	mediumCodebase = "../../examples/mutations"
)

// =============================================================================
// Schemata Generation Benchmarks
// =============================================================================

func BenchmarkSchemata_GenerateMutants(b *testing.B) {
	sites, operators := loadTestSitesBench(b, smallCodebase)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range sites {
			for _, op := range operators {
				apply := false
				if cop, ok := op.(mutator.ContextualOperator); ok {
					ctx := mutator.Context{ReturnType: site.ReturnType}
					apply = cop.CanApplyWithContext(site.Node, ctx)
				} else {
					apply = op.CanApply(site.Node)
				}
				if apply {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}
		if len(mutants) == 0 {
			b.Fatal("No mutants generated")
		}
	}
}

func BenchmarkSchemata_GenerateMutantsMedium(b *testing.B) {
	sites, operators := loadTestSitesBench(b, mediumCodebase)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range sites {
			for _, op := range operators {
				apply := false
				if cop, ok := op.(mutator.ContextualOperator); ok {
					ctx := mutator.Context{ReturnType: site.ReturnType}
					apply = cop.CanApplyWithContext(site.Node, ctx)
				} else {
					apply = op.CanApply(site.Node)
				}
				if apply {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}
	}
}

// =============================================================================
// Schemata Application Benchmarks
// =============================================================================

func BenchmarkSchemata_ApplyToFile(b *testing.B) {
	sites, operators := loadTestSitesBench(b, smallCodebase)

	// Generate mutants
	var mutants []gtest.Mutant
	mutantID := 1
	for _, site := range sites {
		for _, op := range operators {
			if op.CanApply(site.Node) {
				mutants = append(mutants, gtest.Mutant{
					ID:       mutantID,
					Site:     site,
					Operator: op,
				})
				mutantID++
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := b.TempDir()
		if err := gtest.CopyDir(smallCodebase, tempDir); err != nil {
			b.Fatal(err)
		}
		targetFile := filepath.Join(tempDir, "arithmetic_flip.go")
		b.StartTimer()

		fileMutants := make([]*gtest.Mutant, len(mutants))
		for j := range mutants {
			fileMutants[j] = &mutants[j]
		}

		if err := gtest.ApplySchemataToFile(targetFile, fileMutants); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Schemata Handler Benchmarks
// =============================================================================

func BenchmarkSchemata_HandlerLookup(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, smallCodebase+"/arithmetic_flip.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

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
			_ = schemata_nodes.GetHandler(node)
		}
	}
}

func BenchmarkSchemata_BinaryExprHandler(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, smallCodebase+"/arithmetic_flip.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	var binaryNodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.BinaryExpr); ok {
			binaryNodes = append(binaryNodes, n)
		}
		return true
	})

	mutants := []schemata_nodes.MutantForSite{
		{ID: 1, Op: mutator.ArithmeticFlip{}},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range binaryNodes {
			handler := schemata_nodes.GetHandler(node)
			if handler != nil {
				_ = handler(node, mutants, "int", f)
			}
		}
	}
}

func BenchmarkSchemata_ReturnStmtHandler(b *testing.B) {
	code := `package test
func GetNum() int { return 42 }
func GetStr() string { return "hello" }
func GetBool() bool { return true }
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	var returnNodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.ReturnStmt); ok {
			returnNodes = append(returnNodes, n)
		}
		return true
	})

	mutants := []schemata_nodes.MutantForSite{
		{ID: 1, Op: zero_value_return.ZeroValueReturnNumeric{}, ReturnType: "int"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range returnNodes {
			handler := schemata_nodes.GetHandler(node)
			if handler != nil {
				_ = handler(node, mutants, "int", f)
			}
		}
	}
}

func BenchmarkSchemata_IfStmtHandler(b *testing.B) {
	code := `package test
func Test() {
	if x > 5 {
		println("big")
	}
	if y < 10 {
		println("small")
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "test.go", code, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	var ifNodes []ast.Node
	ast.Inspect(f, func(n ast.Node) bool {
		if _, ok := n.(*ast.IfStmt); ok {
			ifNodes = append(ifNodes, n)
		}
		return true
	})

	mutants := []schemata_nodes.MutantForSite{
		{ID: 1, Op: mustGetMutator("if_condition_true")},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, node := range ifNodes {
			handler := schemata_nodes.GetHandler(node)
			if handler != nil {
				_ = handler(node, mutants, "", f)
			}
		}
	}
}

// =============================================================================
// File Copy and Setup Benchmarks
// =============================================================================

func BenchmarkSchemata_CopyDirectory(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := b.TempDir()
		b.StartTimer()

		if err := gtest.CopyDir(smallCodebase, tempDir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSchemata_MakeSelfContained(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := b.TempDir()
		if err := gtest.CopyDir(smallCodebase, tempDir); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		if err := gtest.MakeSelfContained(tempDir); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Full Schemata Pipeline Benchmarks
// =============================================================================

func BenchmarkSchemata_FullPipelineSmall(b *testing.B) {
	sites, operators := loadTestSitesBench(b, smallCodebase)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := b.TempDir()
		if err := gtest.CopyDir(smallCodebase, tempDir); err != nil {
			b.Fatal(err)
		}
		if err := gtest.RewriteImports(tempDir); err != nil {
			b.Fatal(err)
		}
		if err := gtest.MakeSelfContained(tempDir); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		// Generate mutants
		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range sites {
			for _, op := range operators {
				if op.CanApply(site.Node) {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}

		// Group by file
		fileToMutants := make(map[string][]*gtest.Mutant)
		for j := range mutants {
			m := &mutants[j]
			relPath := "arithmetic_flip.go"
			tempFile := filepath.Join(tempDir, relPath)
			fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
		}

		// Apply schemata
		for tempFile, fileMutants := range fileToMutants {
			if err := gtest.ApplySchemataToFile(tempFile, fileMutants); err != nil {
				b.Fatal(err)
			}
		}

		// Inject helpers
		if err := gtest.InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
			b.Fatal(err)
		}
	}
}

// =============================================================================
// Phase Breakdown Benchmarks
// =============================================================================

func BenchmarkSchemata_PhaseBreakdown(b *testing.B) {
	sites, operators := loadTestSitesBench(b, smallCodebase)

	var (
		copyTime     time.Duration
		rewriteTime  time.Duration
		schemataTime time.Duration
		injectTime   time.Duration
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		tempDir := b.TempDir()
		b.StartTimer()

		// Phase 1: Copy
		start := time.Now()
		if err := gtest.CopyDir(smallCodebase, tempDir); err != nil {
			b.Fatal(err)
		}
		copyTime += time.Since(start)

		// Phase 2: Rewrite imports
		start = time.Now()
		if err := gtest.RewriteImports(tempDir); err != nil {
			b.Fatal(err)
		}
		if err := gtest.MakeSelfContained(tempDir); err != nil {
			b.Fatal(err)
		}
		rewriteTime += time.Since(start)

		// Phase 3: Generate and apply schemata
		start = time.Now()
		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range sites {
			for _, op := range operators {
				if op.CanApply(site.Node) {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}

		fileToMutants := make(map[string][]*gtest.Mutant)
		for j := range mutants {
			m := &mutants[j]
			tempFile := filepath.Join(tempDir, "arithmetic_flip.go")
			fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
		}

		for tempFile, fileMutants := range fileToMutants {
			if err := gtest.ApplySchemataToFile(tempFile, fileMutants); err != nil {
				b.Fatal(err)
			}
		}
		schemataTime += time.Since(start)

		// Phase 4: Inject helpers
		start = time.Now()
		if err := gtest.InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
			b.Fatal(err)
		}
		injectTime += time.Since(start)
	}

	b.ReportMetric(float64(copyTime.Milliseconds())/float64(b.N), "ms/copy")
	b.ReportMetric(float64(rewriteTime.Milliseconds())/float64(b.N), "ms/rewrite")
	b.ReportMetric(float64(schemataTime.Milliseconds())/float64(b.N), "ms/schemata")
	b.ReportMetric(float64(injectTime.Milliseconds())/float64(b.N), "ms/inject")
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

func BenchmarkSchemata_MutantGenerationAllocs(b *testing.B) {
	sites, operators := loadTestSitesBench(b, smallCodebase)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var mutants []gtest.Mutant
		mutantID := 1
		for _, site := range sites {
			for _, op := range operators {
				if op.CanApply(site.Node) {
					mutants = append(mutants, gtest.Mutant{
						ID:       mutantID,
						Site:     site,
						Operator: op,
					})
					mutantID++
				}
			}
		}
	}
}

// =============================================================================
// Helper Functions
// =============================================================================

func loadTestSitesBench(t testing.TB, basePath string) ([]engine.Site, []mutator.Operator) {
	t.Helper()

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		t.Fatal(err)
	}

	var sites []engine.Site
	fset := token.NewFileSet()

	err = filepath.Walk(absPath, func(path string, info os.FileInfo, err error) error {
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

		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return true
			}

			pos := fset.Position(n.Pos())
			sites = append(sites, engine.Site{
				File:   fset.File(n.Pos()),
				Line:   pos.Line,
				Column: pos.Column,
				Node:   n,
			})
			return true
		})

		return nil
	})

	if err != nil {
		t.Fatalf("Walk failed: %v", err)
	}

	operators := mutator.List()
	if len(operators) == 0 {
		t.Fatal("No operators registered")
	}

	return sites, operators
}
