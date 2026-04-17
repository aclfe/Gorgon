// Package reporter_test provides comprehensive benchmarks for the reporter package.
package reporter_test

import (
	"bytes"
	"fmt"
	"go/token"
	"io"
	"os"
	"strings"
	"testing"

	mtest "github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/boundary_value"
	"github.com/aclfe/gorgon/pkg/mutator/operators/logical_operator"
	"github.com/aclfe/gorgon/pkg/mutator/operators/zero_value_return"
)

// =============================================================================
// Test Data Generation
// =============================================================================

func generateMutants(count int) []mtest.Mutant {
	mutants := make([]mtest.Mutant, count)
	statuses := []string{"killed", "survived", "error"}

	for i := 0; i < count; i++ {
		status := statuses[i%3]
		mutants[i] = mtest.Mutant{
			ID:       i + 1,
			Status:   status,
			Operator: mutator.ArithmeticFlip{},
			Site: engine.Site{
				File:   &token.File{},
				Line:   (i % 100) + 1,
				Column: (i % 50) + 1,
			},
		}
	}

	// Set some survived mutants for realistic output
	for i := range mutants {
		if i%3 == 1 {
			mutants[i].Status = "survived"
			// Create a mock file for the site
			f := token.NewFileSet().AddFile("test.go", -1, 1000)
			mutants[i].Site.File = f
		}
	}

	return mutants
}

func generateRealisticMutants() []mtest.Mutant {
	// Generate a realistic distribution of mutants
	mutants := make([]mtest.Mutant, 0, 50)

	operators := []mutator.Operator{
		mutator.ArithmeticFlip{},
		mutator.ConditionNegation{},
		logical_operator.LogicalOperator{},
		boundary_value.BoundaryValue{},
		zero_value_return.ZeroValueReturnNumeric{},
	}

	statuses := []string{"killed", "killed", "killed", "survived", "survived", "error"}

	id := 1
	for i := 0; i < 10; i++ {
		for _, op := range operators {
			status := statuses[(i*len(operators)+id)%len(statuses)]
			f := token.NewFileSet().AddFile(fmt.Sprintf("file%d.go", i%5), -1, 1000)
			mutants = append(mutants, mtest.Mutant{
				ID:       id,
				Status:   status,
				Operator: op,
				Site: engine.Site{
					File:   f,
					Line:   (i * 10) + 1,
					Column: (id * 5) + 1,
				},
			})
			id++
		}
	}

	return mutants
}

// =============================================================================
// Basic Reporting Benchmarks
// =============================================================================

func BenchmarkReporter_SmallMutantSet(b *testing.B) {
	mutants := generateMutants(10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Capture output to avoid console spam
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_MediumMutantSet(b *testing.B) {
	mutants := generateMutants(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_LargeMutantSet(b *testing.B) {
	mutants := generateMutants(200)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

// =============================================================================
// Realistic Scenario Benchmarks
// =============================================================================

func BenchmarkReporter_RealisticDistribution(b *testing.B) {
	mutants := generateRealisticMutants()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

// =============================================================================
// Score Calculation Benchmarks
// =============================================================================

func BenchmarkReporter_ScoreCalculation(b *testing.B) {
	mutants := generateMutants(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		total := len(mutants)
		killed := 0
		survived := 0
		errors := 0

		for _, mutant := range mutants {
			switch mutant.Status {
			case "killed":
				killed++
			case "survived":
				survived++
			case "error":
				errors++
			}
		}

		_ = float64(killed) / float64(total) * 100
	}
}

func BenchmarkReporter_StatusCounting(b *testing.B) {
	mutants := generateMutants(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		killed := 0
		survived := 0
		errors := 0

		for _, mutant := range mutants {
			switch mutant.Status {
			case "killed":
				killed++
			case "survived":
				survived++
			case "error":
				errors++
			}
		}

		_ = killed + survived + errors
	}
}

// =============================================================================
// Output Formatting Benchmarks
// =============================================================================

func BenchmarkReporter_TabWriterFormatting(b *testing.B) {
	mutants := generateMutants(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		total := len(mutants)
		killed := 0
		survived := 0
		errors := 0

		for _, mutant := range mutants {
			switch mutant.Status {
			case "killed":
				killed++
			case "survived":
				survived++
			case "error":
				errors++
			}
		}

		score := float64(killed) / float64(total) * 100

		fmt.Fprintf(&buf, "Mutation Score\tKilled\tSurvived\tErrors\tTotal\n")
		fmt.Fprintf(&buf, "%.2f%%\t%d\t%d\t%d\t%d\n", score, killed, survived, errors, total)
	}
}

func BenchmarkReporter_SurvivedMutantFormatting(b *testing.B) {
	mutants := generateMutants(50)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		for _, mutant := range mutants {
			if mutant.Status == "survived" {
				fmt.Fprintf(&buf, "- %s in %s:%d:%d (Operator: %s)\n",
					mutant.Status,
					"test.go",
					mutant.Site.Line,
					mutant.Site.Column,
					mutant.Operator.Name())
			}
		}
	}
}

// =============================================================================
// Visual Column Calculation Benchmarks (internal function)
// =============================================================================

func BenchmarkReporter_CalculateVisualColumn(b *testing.B) {
	// Generate sample file content with tabs and spaces
	content := strings.Repeat("func Test() {\n\tif x > 5 {\n\t\treturn true\n\t}\n\treturn false\n}\n", 20)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: calculateVisualColumn is unexported, using a simple alternative
		_ = calculateVisualColumn([]byte(content), 5, 10)
	}
}

func BenchmarkReporter_CalculateVisualColumn_LargeFile(b *testing.B) {
	// Generate larger file content
	content := strings.Repeat("func Test() {\n\tif x > 5 {\n\t\treturn true\n\t}\n\treturn false\n}\n", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calculateVisualColumn([]byte(content), 50, 25)
	}
}

// Internal copy for benchmarking (since original is unexported)
func calculateVisualColumn(content []byte, line, col int) int {
	start := 0
	currentLine := 1
	for i, b := range content {
		if currentLine == line {
			start = i
			break
		}
		if b == '\n' {
			currentLine++
		}
	}

	visualCol := 1
	for i := 0; i < col-1; i++ {
		if start+i >= len(content) {
			break
		}
		if content[start+i] == '\t' {
			visualCol += 4 - (visualCol-1)%4
		} else {
			visualCol++
		}
	}
	return visualCol
}

// =============================================================================
// Memory Allocation Benchmarks
// =============================================================================

func BenchmarkReporter_Allocations_Small(b *testing.B) {
	mutants := generateMutants(10)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_Allocations_Medium(b *testing.B) {
	mutants := generateMutants(50)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_Allocations_Large(b *testing.B) {
	mutants := generateMutants(200)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

// =============================================================================
// Edge Case Benchmarks
// =============================================================================

func BenchmarkReporter_EmptyMutantSet(b *testing.B) {
	mutants := []mtest.Mutant{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_AllKilled(b *testing.B) {
	mutants := generateMutants(50)
	for i := range mutants {
		mutants[i].Status = "killed"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_AllSurvived(b *testing.B) {
	mutants := generateMutants(50)
	for i := range mutants {
		mutants[i].Status = "survived"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}

func BenchmarkReporter_AllErrors(b *testing.B) {
	mutants := generateMutants(50)
	for i := range mutants {
		mutants[i].Status = "error"
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := reporter.Report(mutants, 0, false, false, "", "")
		if err != nil {
			b.Fatalf("Report failed: %v", err)
		}

		w.Close()
		_, _ = io.Copy(io.Discard, r)
		os.Stdout = old
	}
}
