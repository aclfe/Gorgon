package testing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// testExecutor handles compilation and execution of mutant tests for a package.
type testExecutor struct {
	tempDir    string
	testBinary string
	pkgDir     string
	tests      []string
	timeout    time.Duration
	baseEnv    []string // Cached environment to avoid repeated os.Environ() calls
}

func newTestExecutor(tempDir, pkgDir string, tests []string) *testExecutor {
	return &testExecutor{
		tempDir: tempDir,
		pkgDir:  pkgDir,
		tests:   tests,
		baseEnv: os.Environ(),
	}
}

// compileWithDebug compiles the test binary and logs unique errors if debug is true.
func (e *testExecutor) compileWithDebug(ctx context.Context, debug bool) error {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", e.testBinary, relPkg)
	cmd.Dir = e.tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		if debug {
			errs := uniqueErrors(string(out))
			fmt.Fprintf(os.Stderr, "  Compilation failed (%d unique errors)\n", len(errs))
		}
		return fmt.Errorf("test compilation failed for %s:\n%s", relPkg, out)
	}
	return nil
}

// measureBaseline runs the test binary once to determine appropriate timeout.
func (e *testExecutor) measureBaseline(ctx context.Context) time.Duration {
	start := time.Now()
	_ = exec.CommandContext(ctx, e.testBinary, testArgs("5s", e.tests)...).Run()
	duration := time.Since(start)
	if duration < minBaselineDuration*time.Millisecond {
		duration = minBaselineDuration * time.Millisecond
	}
	return duration
}

// timeoutFor calculates appropriate test timeout.
func (e *testExecutor) timeoutFor(baseline time.Duration) (string, time.Duration) {
	timeout := time.Duration(float64(baseline) * timeoutMultiplier)
	if timeout > maxTimeout*time.Second {
		timeout = maxTimeout * time.Second
	}
	e.timeout = timeout
	return fmt.Sprintf("%.0fs", timeout.Seconds()), timeout
}

// runMutant executes a single mutant test.
// A per-mutant timeout is required because some mutations (e.g. for_condition_true)
// create infinite loops; without this, exec.CommandContext would never kill the process.
func (e *testExecutor) runMutant(ctx context.Context, mutantID int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout+2*time.Second)
	defer cancel()

	args := testArgs(fmt.Sprintf("%.0fs", e.timeout.Seconds()), e.tests)
	cmd := exec.CommandContext(ctx, e.testBinary, args...)
	cmd.Dir = e.pkgDir

	// Reuse cached baseEnv with pre-allocated slice to avoid GC pressure
	mutantEnv := make([]string, len(e.baseEnv)+1)
	copy(mutantEnv, e.baseEnv)
	mutantEnv[len(e.baseEnv)] = "GORGON_MUTANT_ID=" + strconv.Itoa(mutantID)
	cmd.Env = mutantEnv

	if out, err := cmd.CombinedOutput(); err != nil {
		return "killed", fmt.Errorf("%s", out)
	}
	return "survived", nil
}

func (e *testExecutor) relPath() string {
	rel, _ := filepath.Rel(e.tempDir, e.pkgDir)
	if rel == "." {
		return ""
	}
	return "./" + filepath.ToSlash(rel)
}

// compileAndRunPackages compiles test binaries and runs mutants concurrently.
// Compilation and test execution overlap: as soon as a package compiles,
// its tests start running while other packages are still compiling.
// Each compile goroutine directly dispatches its own test goroutines,
// avoiding a single-goroutine processor bottleneck.
func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, concurrent int, tests []string) ([]mutantResult, error) {

	type compileResult struct {
		pkgDir string
		err    error
	}

	resultsChan := make(chan mutantResult, sumMutantIDs(pkgToMutantIDs))
	testGroup, testCtx := errgroup.WithContext(ctx)
	testGroup.SetLimit(concurrent)

	var compErrsMu sync.Mutex
	var compErrors = make(map[string]error)

	// Compile packages concurrently — each goroutine dispatches its own tests
	var compileGroup, compileCtx = errgroup.WithContext(ctx)
	compileGroup.SetLimit(concurrent)

	for pkgDir, mutantIDs := range pkgToMutantIDs {
		pkgDir := pkgDir
		mutantIDs := mutantIDs
		compileGroup.Go(func() error {
			executor := newTestExecutor(tempDir, pkgDir, tests)
			err := executor.compileWithDebug(compileCtx, false)
			if err != nil {
				compErrsMu.Lock()
				compErrors[pkgDir] = err
				compErrsMu.Unlock()
				return nil // Don't cancel siblings on compile error
			}

			// Measure baseline and dispatch tests directly from this goroutine
			baseline := executor.measureBaseline(testCtx)
			_, _ = executor.timeoutFor(baseline)

			for _, mutantID := range mutantIDs {
				mutantID := mutantID
				testGroup.Go(func() error {
					status, err := executor.runMutant(testCtx, mutantID)
					resultsChan <- mutantResult{id: mutantID, status: status, err: err}
					return nil
				})
			}
			return nil
		})
	}

	// Wait for all compilations
	_ = compileGroup.Wait()

	// Wait for all tests to complete
	if err := testGroup.Wait(); err != nil {
		return nil, fmt.Errorf("test execution failed: %w", err)
	}
	close(resultsChan)

	var allResults []mutantResult
	for result := range resultsChan {
		allResults = append(allResults, result)
	}

	if len(compErrors) > 0 {
		var errs []string
		for pkgDir, err := range compErrors {
			errs = append(errs, fmt.Sprintf("%s: %v", pkgDir, err))
		}
		return nil, fmt.Errorf("compilation failures: %s", strings.Join(errs, "; "))
	}

	return allResults, nil
}

// runStandalonePackage handles mutation testing for a single package without go.mod.
// Copies files to temp, applies schemata, compiles, runs mutants, collects results.
func runStandalonePackage(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, debug bool) error {
	// Create temp workspace
	tempDir, err := os.MkdirTemp("", "gorgon-standalone-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Determine package name
	pkgName := detectPackageName(pkgDir)

	// Create go.mod
	goMod := fmt.Sprintf("module %s\n\ngo %s\n", pkgName, goVersion)
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Copy Go files
	if err := copyDir(pkgDir, tempDir); err != nil {
		return err
	}

	// Apply schemata
	tempFileToMutants := mapFilesToMutants(pkgMutants, tempDir)
	for tempFile, mutants := range tempFileToMutants {
		if err := ApplySchemataToFile(tempFile, mutants); err != nil {
			return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}
	if err := InjectSchemataHelpers(tempDir, tempFileToMutants); err != nil {
		return err
	}

	// Compile
	executor := newTestExecutor(tempDir, tempDir, tests)
	if err := executor.compileWithDebug(context.Background(), debug); err != nil {
		for _, m := range pkgMutants {
			m.Status = "error"
			m.Error = err
			m.TempDir = tempDir
		}
		return nil
	}

	// Measure baseline and run mutants concurrently
	baseline := executor.measureBaseline(context.Background())
	_, _ = executor.timeoutFor(baseline)

	resultsChan := make(chan mutantResult, len(pkgMutants))
	ids := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		ids[i] = m.ID
	}

	// Run all mutant tests for this package concurrently
	executor.runMutantsConcurrent(context.Background(), ids, concurrent, resultsChan)

	// Collect results using O(1) map lookup instead of linear search
	idToMutant := make(map[int]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		idToMutant[m.ID] = m
	}
	for result := range resultsChan {
		if m, ok := idToMutant[result.id]; ok {
			m.Status = result.status
			m.Error = result.err
			m.TempDir = tempDir
		}
	}

	return nil
}

// runMutantsConcurrent executes multiple mutants concurrently, closing results channel when done.
// Uses sync.WaitGroup instead of errgroup so that one mutant error does NOT cascade-cancel
// all remaining goroutines — every mutant gets a fair chance to run and report its true status.
func (e *testExecutor) runMutantsConcurrent(ctx context.Context, mutantIDs []int, concurrent int, results chan mutantResult) {
	defer close(results)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrent)

	for _, mutantID := range mutantIDs {
		mutantID := mutantID
		sem <- struct{}{} // Acquire semaphore
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }() // Release semaphore

			select {
			case <-ctx.Done():
				results <- mutantResult{id: mutantID, status: "error", err: ctx.Err()}
				return
			default:
			}

			status, err := e.runMutant(ctx, mutantID)
			results <- mutantResult{id: mutantID, status: status, err: err}
		}()
	}

	wg.Wait()
}

// Utility functions

func testArgs(timeout string, tests []string) []string {
	args := []string{"-test.timeout=" + timeout}
	if len(tests) > 0 {
		args = append(args, "-test.run="+strings.Join(tests, "|"))
	}
	return args
}

func uniqueErrors(output string) []string {
	return UniqueErrorLines(output, "")
}

func sumMutantIDs(m map[string][]int) int {
	total := 0
	for _, ids := range m {
		total += len(ids)
	}
	return total
}

func detectPackageName(pkgDir string) string {
	entries, _ := os.ReadDir(pkgDir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			pkgName := extractFilePath(filepath.Join(pkgDir, entry.Name()))
			if pkgName != "" {
				return pkgName
			}
		}
	}
	return filepath.Base(pkgDir)
}

func mapFilesToMutants(pkgMutants []*Mutant, tempDir string) map[string][]*Mutant {
	result := make(map[string][]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		tempFile := filepath.Join(tempDir, filepath.Base(m.Site.File.Name()))
		result[tempFile] = append(result[tempFile], m)
	}
	return result
}

type mutantResult struct {
	id     int
	status string
	err    error
}
