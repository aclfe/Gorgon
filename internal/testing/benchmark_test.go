package testing_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	stdtesting "testing"
	"time"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/pkg/mutator"
)

const (
	defaultConcurrency = 4
	percentageScale    = 100
)

// BenchmarkFullMutationPipeline measures the complete end-to-end process
func BenchmarkFullMutationPipeline(bnch *stdtesting.B) {
	const small7MutantsPath = "../../examples/mutations/arithmetic_flip"
	const medium11MutantsPath = "../../examples"

	sizes := []struct {
		name string
		path string
	}{
		{"Small_7mutants", small7MutantsPath},
		{"Medium_11mutants", medium11MutantsPath},
	}

	for _, sz := range sizes {
		bnch.Run(sz.name, func(bnch *stdtesting.B) {
			absPath, err := filepath.Abs(sz.path)
			if err != nil {
				bnch.Fatal(err)
			}
			sites, operators := loadTestSites(bnch, absPath)

			bnch.ResetTimer()
			for i := 0; i < bnch.N; i++ {
				mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, defaultConcurrency)
				if err != nil {
					bnch.Fatal(err)
				}
				if len(mutants) == 0 {
					bnch.Fatal("no mutants generated")
				}
			}
		})
	}
}

const defaultConcurrent = 4

// BenchmarkSchemataGeneration measures ONLY the schemata generation (no test execution)
//
//nolint:gocognit
func BenchmarkSchemataGeneration(bnch *stdtesting.B) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		bnch.Fatal(err)
	}
	sites, operators := loadTestSites(bnch, absPath)

	bnch.ResetTimer()
	for i := 0; i < bnch.N; i++ {
		bnch.StopTimer()
		tempDir := bnch.TempDir()
		bnch.StartTimer()

		if err := testing.CopyDir(absPath, tempDir); err != nil {
			bnch.Fatal(err)
		}
		if err := testing.RewriteImports(tempDir); err != nil {
			bnch.Fatal(err)
		}
		if err := testing.MakeSelfContained(tempDir); err != nil {
			bnch.Fatal(err)
		}

		// applying zhe schema!!
		var mutants []testing.Mutant
		id := 1
		for _, site := range sites {
			for _, op := range operators {
				if op.CanApply(site.Node) {
					mutants = append(mutants, testing.Mutant{ID: id, Site: site, Operator: op})
					id++
				}
			}
		}

		fileToMutants := make(map[string][]*testing.Mutant)
		for i := range mutants {
			m := &mutants[i]
			relPath := "arithmetic_flip.go"
			tempFile := filepath.Join(tempDir, relPath)
			fileToMutants[tempFile] = append(fileToMutants[tempFile], m)
		}

		for tempFile, fileMutants := range fileToMutants {
			if err := testing.ApplySchemataToFile(tempFile, fileMutants); err != nil {
				bnch.Fatal(err)
			}
		}

		if err := testing.InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
			bnch.Fatal(err)
		}
	}
}

func BenchmarkTestExecution(bnch *stdtesting.B) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		bnch.Fatal(err)
	}
	sites, operators := loadTestSites(bnch, absPath)

	tempDir, mutants := prepareOnce(bnch, absPath, sites, operators)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			bnch.Error("Failed to remove temp directory:", err)
		}
	}()

	testBinary := filepath.Join(tempDir, "examples/mutations/arithmetic_flip/package.test")

	bnch.ResetTimer()
	for i := 0; i < bnch.N; i++ {
		for _, m := range mutants {
			//nolint:gosec // Running test binary is required for benchmarking
			cmd := exec.Command(testBinary, "-test.timeout=10s")
			cmd.Dir = filepath.Dir(testBinary)
			cmd.Env = append(os.Environ(), fmt.Sprintf("GORGON_MUTANT_ID=%d", m.ID))
			//nolint:errcheck
			_, _ = cmd.CombinedOutput() // expected failure for killed mutants
		}
	}
}

// BenchmarkParallelTestExecution measures parallel test execution
func BenchmarkParallelTestExecution(bnch *stdtesting.B) {
	concurrencies := []int{1, 2, defaultConcurrency, 8}

	for _, conc := range concurrencies {
		bnch.Run(fmt.Sprintf("Concurrent_%d", conc), func(bnch *stdtesting.B) {
			absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
			if err != nil {
				bnch.Fatal(err)
			}
			sites, operators := loadTestSites(bnch, absPath)

			bnch.ResetTimer()
			for i := 0; i < bnch.N; i++ {
				_, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, conc)
				if err != nil {
					bnch.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPerMutantCost measures the marginal cost per additional mutant
func BenchmarkPerMutantCost(bnch *stdtesting.B) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		bnch.Fatal(err)
	}
	sites, operators := loadTestSites(bnch, absPath)
	tempDir, mutants := prepareOnce(bnch, absPath, sites, operators)
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			bnch.Error("Failed to remove temp directory:", err)
		}
	}()

	if len(mutants) == 0 {
		bnch.Fatal("no mutants")
	}

	testBinary := filepath.Join(tempDir, "examples/mutations/arithmetic_flip/package.test")

	bnch.ResetTimer()
	for i := 0; i < bnch.N; i++ {
		m := mutants[0]
		//nolint:gosec // Running test binary is required for benchmarking
		cmd := exec.Command(testBinary, "-test.timeout=10s")
		cmd.Dir = filepath.Dir(testBinary)
		cmd.Env = append(os.Environ(), fmt.Sprintf("GORGON_MUTANT_ID=%d", m.ID))
		//nolint:errcheck
		_, _ = cmd.CombinedOutput() // expected failure
	}
}

//nolint:gocognit,funlen
func BenchmarkPhaseBreakdown(bnch *stdtesting.B) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		bnch.Fatal(err)
	}
	sites, operators := loadTestSites(bnch, absPath)

	var (
		copyTime     time.Duration
		rewriteTime  time.Duration
		schemataTime time.Duration
		buildTime    time.Duration
		testTime     time.Duration
	)

	for i := 0; i < bnch.N; i++ {
		tempDir := bnch.TempDir()

		// Phase 1: Copy
		start := time.Now()
		if err := testing.CopyDir(absPath, tempDir); err != nil {
			bnch.Fatal(err)
		}
		copyTime += time.Since(start)

		// Phase 2: Rewrite imports
		start = time.Now()
		if err := testing.RewriteImports(tempDir); err != nil {
			bnch.Fatal(err)
		}
		if err := testing.MakeSelfContained(tempDir); err != nil {
			bnch.Fatal(err)
		}
		rewriteTime += time.Since(start)

		// Phase 3: Apply schemata
		start = time.Now()
		var mutants []testing.Mutant
		id := 1
		for _, site := range sites {
			for _, op := range operators {
				if op.CanApply(site.Node) {
					mutants = append(mutants, testing.Mutant{ID: id, Site: site, Operator: op})
					id++
				}
			}
		}
		fileToMutants := make(map[string][]*testing.Mutant)
		for i := range mutants {
			m := &mutants[i]
			fileToMutants[filepath.Join(tempDir, "arithmetic_flip.go")] = append(fileToMutants[filepath.Join(tempDir, "arithmetic_flip.go")], m)
		}
		for tempFile, fileMutants := range fileToMutants {
			if err := testing.ApplySchemataToFile(tempFile, fileMutants); err != nil {
				bnch.Fatal(err)
			}
		}
		if err := testing.InjectSchemataHelpers(tempDir, fileToMutants); err != nil {
			bnch.Fatal(err)
		}
		schemataTime += time.Since(start)

		start = time.Now()
		cmd := exec.Command("go", "build", "./...")
		cmd.Dir = tempDir
		if _, err := cmd.CombinedOutput(); err != nil {
			bnch.Fatal(err)
		}
		buildTime += time.Since(start)

		start = time.Now()
		cmd = exec.Command("go", "test", "-timeout=10s", "-count=1", "./...")
		cmd.Dir = tempDir
		cmd.Env = append(os.Environ(), "GORGON_MUTANT_ID=1")
		_, _ = cmd.CombinedOutput() // expected failure
		testTime += time.Since(start)
	}

	bnch.ReportMetric(float64(copyTime.Milliseconds())/float64(bnch.N), "ms/copy")
	bnch.ReportMetric(float64(rewriteTime.Milliseconds())/float64(bnch.N), "ms/rewrite")
	bnch.ReportMetric(float64(schemataTime.Milliseconds())/float64(bnch.N), "ms/schemata")
	bnch.ReportMetric(float64(buildTime.Milliseconds())/float64(bnch.N), "ms/build")
	bnch.ReportMetric(float64(testTime.Milliseconds())/float64(bnch.N), "ms/test")
}

// BenchmarkMutationDetectionRate measures how effectively tests kill mutants
func BenchmarkMutationDetectionRate(bnch *stdtesting.B) {
	absPath, err := filepath.Abs("../../examples/mutations/arithmetic_flip")
	if err != nil {
		bnch.Fatal(err)
	}
	sites, operators := loadTestSites(bnch, absPath)

	var totalMutants, killedMutants int

	bnch.ResetTimer()
	for i := 0; i < bnch.N; i++ {
		mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, absPath, defaultConcurrency)
		if err != nil {
			bnch.Fatal(err)
		}

		totalMutants += len(mutants)
		for _, m := range mutants {
			if m.Status == "killed" {
				killedMutants++
			}
		}
	}

	detectionRate := float64(killedMutants) / float64(totalMutants) * percentageScale
	bnch.ReportMetric(detectionRate, "kill_rate_%")
	bnch.ReportMetric(float64(totalMutants)/float64(bnch.N), "mutants/op")
}

//nolint:thelper
func loadTestSites(t stdtesting.TB, basePath string) ([]engine.Site, []mutator.Operator) {
	t.Helper()
	var sites []engine.Site
	//nolint:staticcheck
	if err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" || filepath.Base(path) == "gorgon_schemata.go" {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			if be, ok := n.(*ast.BinaryExpr); ok {
				sites = append(sites, engine.Site{
					File: fset.File(be.OpPos),
					Pos:  be.OpPos,
					End:  be.End(),
					Node: be,
				})
			}
			return true
		})
		return nil
	}); err != nil {
	}

	operators := []mutator.Operator{
		mutator.ArithmeticFlip{},
		mutator.ConditionNegation{},
	}

	return sites, operators
}

//nolint:thelper
func prepareOnce(bnch *stdtesting.B, basePath string, sites []engine.Site, operators []mutator.Operator) (string, []testing.Mutant) {
	bnch.Helper()
	mutants, err := testing.GenerateAndRunSchemata(context.Background(), sites, operators, basePath, 1)
	if err != nil {
		bnch.Fatal(err)
	}
	if len(mutants) == 0 {
		bnch.Fatal("no mutants")
	}
	return mutants[0].TempDir, mutants
}
