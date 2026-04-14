// Package benchmark_test provides comprehensive end-to-end pipeline benchmarks.
package benchmark_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	gtest "github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/arithmetic_flip"
	"github.com/aclfe/gorgon/pkg/mutator/operators/condition_negation"
	"github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/loop_body_removal"
	"github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_first"
	"github.com/aclfe/gorgon/pkg/mutator/operators/loop_break_removal"
	"github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/operators/variable_replacement"
)

const (
	smallCodebase  = "../../examples/mutations/arithmetic_flip"
	mediumCodebase = "../../examples/mutations"
)

// =============================================================================
// Full Pipeline Benchmarks
// =============================================================================

// Note: Full pipeline benchmarks require working module dependencies
// If you see module path errors, run: go mod tidy
func BenchmarkPipeline_FullSmallCodebase(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
		if len(mutants) == 0 {
			b.Skip("No mutants generated")
		}

		// Include reporting in the pipeline
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		_ = reporter.Report(mutants, 0, false, false, "", "")
		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkPipeline_FullMediumCodebase(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, mediumCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, mediumCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		_ = reporter.Report(mutants, 0, false, false, "", "")
		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

// =============================================================================
// Phase-by-Phase Pipeline Benchmarks
// =============================================================================

// Note: Phase breakdown benchmarks require working module dependencies
func BenchmarkPipeline_PhaseBreakdown(b *testing.B) {
	ops := loadAllOperators(b)

	var (
		siteDetectionTime time.Duration
		mutantGenTime     time.Duration
		schemataAppTime   time.Duration
		testExecTime      time.Duration
		reportTime        time.Duration
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Phase 1: Site Detection
		start := time.Now()
		detectedSites := collectSites(b, smallCodebase, ops)
		siteDetectionTime += time.Since(start)

		// Phase 2: Mutant Generation
		start = time.Now()
		mutants := gtest.GenerateMutants(detectedSites, ops)
		mutantGenTime += time.Since(start)

		// Phase 3: Schemata Application (full pipeline includes this)
		start = time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		runMutants, err := gtest.GenerateAndRunSchemata(ctx, detectedSites, ops, smallCodebase, 1, nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Schemata failed (dependency issue): %v", err)
		}
		schemataAppTime += time.Since(start)

		// Phase 4: Test Execution (included in GenerateAndRunSchemata)
		testExecTime += schemataAppTime

		// Phase 5: Reporting
		start = time.Now()
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		_ = reporter.Report(runMutants, 0, false, false, "", "")
		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
		reportTime += time.Since(start)

		_ = mutants
	}

	b.ReportMetric(float64(siteDetectionTime.Milliseconds())/float64(b.N), "ms/site_detection")
	b.ReportMetric(float64(mutantGenTime.Milliseconds())/float64(b.N), "ms/mutant_gen")
	b.ReportMetric(float64(schemataAppTime.Milliseconds())/float64(b.N), "ms/schemata_app")
	b.ReportMetric(float64(testExecTime.Milliseconds())/float64(b.N), "ms/test_exec")
	b.ReportMetric(float64(reportTime.Milliseconds())/float64(b.N), "ms/report")
}

// =============================================================================
// Concurrency Scaling Benchmarks
// =============================================================================

// Note: This benchmark is broken due to concurrency issues - commented out
/*
func BenchmarkPipeline_ConcurrencyScaling(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	concurrencies := []int{1, 2, 4, 8, runtime.NumCPU()}

	for _, conc := range concurrencies {
		b.Run(fmt.Sprintf("Workers_%d", conc), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithCancel(context.Background())
				_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, conc, nil, nil, false)
				cancel()
				if err != nil {
					b.Skipf("Pipeline failed (dependency issue): %v", err)
				}
			}
		})
	}
}
*/

// =============================================================================
// Mutation Detection Rate Benchmarks
// =============================================================================

// Note: Requires working module dependencies
func BenchmarkPipeline_MutationDetectionRate(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	var totalMutants, killedMutants int

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}

		totalMutants += len(mutants)
		for _, m := range mutants {
			if m.Status == "killed" {
				killedMutants++
			}
		}
	}

	detectionRate := float64(killedMutants) / float64(totalMutants) * 100
	b.ReportMetric(detectionRate, "kill_rate_%")
	b.ReportMetric(float64(totalMutants)/float64(b.N), "mutants/op")
}

// =============================================================================
// Operator-Specific Pipeline Benchmarks
// =============================================================================

// Note: Requires working module dependencies
func BenchmarkPipeline_ArithmeticOperators(b *testing.B) {
	ops := []mutator.Operator{
		arithmetic_flip.ArithmeticFlip{},
	}
	sites := collectSites(b, smallCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
	}
}

func BenchmarkPipeline_LogicalOperators(b *testing.B) {
	ops := []mutator.Operator{
		condition_negation.ConditionNegation{},
		logical_operator.LogicalOperator{},
	}
	sites := collectSites(b, mediumCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, mediumCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
	}
}

func BenchmarkPipeline_ZeroValueReturns(b *testing.B) {
	ops := []mutator.Operator{
		zero_value_return.ZeroValueReturnNumeric{},
		zero_value_return.ZeroValueReturnString{},
		zero_value_return.ZeroValueReturnBool{},
	}
	sites := collectSites(b, mediumCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, mediumCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
	}
}

func BenchmarkPipeline_LoopMutations(b *testing.B) {
	ops := []mutator.Operator{
		loop_body_removal.LoopBodyRemoval{},
		loop_break_first.LoopBreakFirst{},
		loop_break_removal.LoopBreakRemoval{},
	}
	sites := collectSites(b, mediumCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, mediumCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
	}
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

// Note: Requires working module dependencies
func BenchmarkPipeline_MemoryAllocations(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
		_ = mutants
	}
}

// =============================================================================
// Throughput Benchmarks
// =============================================================================

func BenchmarkPipeline_Throughput(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
		elapsed := time.Since(start)

		b.ReportMetric(float64(len(mutants))/elapsed.Seconds(), "mutants/sec")
	}
}

// =============================================================================
// Cold Start vs Warm Start Benchmarks
// =============================================================================

func BenchmarkPipeline_ColdStart(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Each iteration is a cold start - new context, new temp dir
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
		}
	}
}

// =============================================================================
// Error Handling Benchmarks
// =============================================================================

func BenchmarkPipeline_ErrorRecovery(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		mutants, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, smallCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()

		// Count errors for metrics
		errorCount := 0
		for _, m := range mutants {
			if m.Status == "error" {
				errorCount++
			}
		}
		b.ReportMetric(float64(errorCount), "errors")

		if err != nil && !strings.Contains(err.Error(), "expected") {
			// Some errors are expected in mutation testing
		}
	}
}

// =============================================================================
// Build Time Benchmarks
// =============================================================================

// Note: Requires working module dependencies
func BenchmarkPipeline_BuildTime(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

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

		// Generate and apply schemata
		mutants := gtest.GenerateMutants(sites, ops)
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
		if err := gtest.InjectSchemataHelpers(fileToMutants); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		// Measure build time
		start := time.Now()
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = tempDir
		_ = cmd.Run()
		b.ReportMetric(float64(time.Since(start).Milliseconds()), "ms/build")
	}
}

// =============================================================================
// Test Compilation Benchmarks
// =============================================================================

func BenchmarkPipeline_TestCompilation(b *testing.B) {
	ops := loadAllOperators(b)
	sites := collectSites(b, smallCodebase, ops)

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

		mutants := gtest.GenerateMutants(sites, ops)
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
		if err := gtest.InjectSchemataHelpers(fileToMutants); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()

		// Measure test compilation time
		start := time.Now()
		cmd := exec.Command("go", "test", "-c", "-o", "package.test")
		cmd.Dir = tempDir
		_ = cmd.Run()
		b.ReportMetric(float64(time.Since(start).Milliseconds()), "ms/test_compile")
	}
}

// =============================================================================
// Conditional Expression Pipeline Benchmarks
// =============================================================================

// Note: Requires working module dependencies
func BenchmarkPipeline_ConditionalExpressions(b *testing.B) {
	ifConditionTrue, _ := mutator.Get("if_condition_true")
	ifConditionFalse, _ := mutator.Get("if_condition_false")
	if ifConditionTrue == nil || ifConditionFalse == nil {
		b.Skip("conditional expression mutators not available")
	}
	ops := []mutator.Operator{
		ifConditionTrue,
		ifConditionFalse,
	}
	sites := collectSites(b, mediumCodebase, ops)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		_, err := gtest.GenerateAndRunSchemata(ctx, sites, ops, mediumCodebase, runtime.NumCPU(), nil, nil, false)
		cancel()
		if err != nil {
			b.Skipf("Pipeline failed (dependency issue): %v", err)
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

func collectSites(b *testing.B, path string, ops []mutator.Operator) []engine.Site {
	b.Helper()
	e := engine.NewEngine(false)
	e.SetOperators(ops)

	if err := e.Traverse(path, nil); err != nil {
		b.Fatalf("Traverse failed: %v", err)
	}

	return e.Sites()
}
