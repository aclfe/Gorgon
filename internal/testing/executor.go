package testing

import (
	"context"
	"fmt"
	"go/ast"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)


type testExecutor struct {
	tempDir    string
	testBinary string
	pkgDir     string
	tests      []string
	timeout    time.Duration
	baseEnv    []string 
}

func newTestExecutor(tempDir, pkgDir string, tests []string) *testExecutor {
	return &testExecutor{
		tempDir: tempDir,
		pkgDir:  pkgDir,
		tests:   tests,
		baseEnv: os.Environ(),
	}
}


func (e *testExecutor) compileWithDebug(ctx context.Context, debug bool) error {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", e.testBinary, relPkg)
	cmd.Dir = e.tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		
		if e.tryLazyCleanup(debug) {
			
			cmd = exec.CommandContext(ctx, "go", "test", "-c", "-o", e.testBinary, relPkg)
			cmd.Dir = e.tempDir
			if retryOut, retryErr := cmd.CombinedOutput(); retryErr == nil {
				_ = out 
				return nil 
			} else {
				_ = retryOut
			}
		}
		
		if debug {
			errs := uniqueErrors(string(out))
			fmt.Fprintf(os.Stderr, "  Compilation failed (%d unique errors)\n", len(errs))
		}
		return fmt.Errorf("test compilation failed for %s:\n%s", relPkg, out)
	}
	return nil
}

func (e *testExecutor) tryLazyCleanup(debug bool) bool {
	
	entries, _ := os.ReadDir(e.pkgDir)
	success := true
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		filePath := filepath.Join(e.pkgDir, name)
		if err := CleanupUnusedImportsAndLoopVars(filePath); err != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "  Lazy cleanup failed on %s: %v\n", name, err)
			}
			success = false
		}
	}
	return success
}

func (e *testExecutor) measureBaseline(ctx context.Context) time.Duration {
	var durations []time.Duration
	maxAttempts := 3
	failureCount := 0

	for i := 0; i < maxAttempts && len(durations) < 3; i++ {
		start := time.Now()
		cmd := exec.CommandContext(ctx, e.testBinary, testArgs("5s", e.tests)...)
		cmd.Dir = e.tempDir
		err := cmd.Run()

		if err == nil {
			duration := time.Since(start)
			durations = append(durations, duration)
		} else {
			failureCount++
		}
	}

	if len(durations) == 0 {
		return minBaselineDuration * time.Millisecond
	}

	sort.Slice(durations, func(i, j int) bool {
		return durations[i] < durations[j]
	})
	median := durations[len(durations)/2]

	if median < minBaselineDuration*time.Millisecond {
		median = minBaselineDuration * time.Millisecond
	}
	return median
}

func (e *testExecutor) timeoutFor(baseline time.Duration) (string, time.Duration) {
	timeout := time.Duration(float64(baseline) * timeoutMultiplier)
	if timeout > maxTimeout*time.Second {
		timeout = maxTimeout * time.Second
	}
	e.timeout = timeout
	return fmt.Sprintf("%.0fs", timeout.Seconds()), timeout
}

func (e *testExecutor) runMutant(ctx context.Context, mutantID int) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout+2*time.Second)
	defer cancel()

	args := testArgs(fmt.Sprintf("%.0fs", e.timeout.Seconds()), e.tests)
	cmd := exec.CommandContext(ctx, e.testBinary, args...)
	cmd.Dir = e.pkgDir


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

func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, concurrent int, tests []string) ([]mutantResult, error) {

	resultsChan := make(chan mutantResult, sumMutantIDs(pkgToMutantIDs))
	
	var compErrsMu sync.Mutex
	var compErrors = make(map[string]error)

	var compileGroup, compileCtx = errgroup.WithContext(ctx)
	compileGroup.SetLimit(concurrent)

	pkgDirs := make([]string, 0, len(pkgToMutantIDs))
	for pkgDir := range pkgToMutantIDs {
		pkgDirs = append(pkgDirs, pkgDir)
	}
	sort.Strings(pkgDirs)

	for _, pkgDir := range pkgDirs {
		mutantIDsForPkg := pkgToMutantIDs[pkgDir]
		sort.Ints(mutantIDsForPkg)

		compileGroup.Go(func() error {
			pkgDir := pkgDir
			mutantIDsForPkg := mutantIDsForPkg
			executor := newTestExecutor(tempDir, pkgDir, tests)
			err := executor.compileWithDebug(compileCtx, false)
			if err != nil {
				compErrsMu.Lock()
				compErrors[pkgDir] = err
				compErrsMu.Unlock()
				
				for _, mutantID := range mutantIDsForPkg {
					resultsChan <- mutantResult{id: mutantID, status: "error", err: err}
				}
				return nil
			}

			baseline := executor.measureBaseline(compileCtx)
			executor.timeoutFor(baseline)

			
			executor.runMutantsConcurrent(compileCtx, mutantIDsForPkg, concurrent, resultsChan)
			
			return nil
		})
	}

	_ = compileGroup.Wait()
	close(resultsChan)

	var allResults []mutantResult
	for result := range resultsChan {
		allResults = append(allResults, result)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].id < allResults[j].id
	})

	if len(compErrors) > 0 {
		var errs []string
		for pkgDir, err := range compErrors {
			errs = append(errs, fmt.Sprintf("%s: %v", pkgDir, err))
		}
		return nil, fmt.Errorf("compilation failures: %s", strings.Join(errs, "; "))
	}

	return allResults, nil
}



func runStandalonePackage(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, debug bool) error {

	tempDir, err := os.MkdirTemp("", "gorgon-standalone-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)


	pkgName := detectPackageName(pkgDir)


	goMod := fmt.Sprintf("module %s\n\ngo %s\n", pkgName, goVersion)
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}


	if err := copyDir(pkgDir, tempDir); err != nil {
		return err
	}


	tempFileToMutants := mapFilesToMutants(pkgMutants, tempDir, pkgDir)


	
	tempFiles := make([]string, 0, len(tempFileToMutants))
	for tempFile := range tempFileToMutants {
		tempFiles = append(tempFiles, tempFile)
	}
	sort.Strings(tempFiles)


	
	astToFileMutants := make(map[*ast.File][]*Mutant)
	for _, tempFile := range tempFiles {
		mutants := tempFileToMutants[tempFile]
		for _, m := range mutants {
			if m.Site.FileAST != nil {
				astToFileMutants[m.Site.FileAST] = append(astToFileMutants[m.Site.FileAST], m)
			}
		}
	}


	
	type astEntry struct {
		astFile *ast.File
		mutants []*Mutant
	}
	sortedASTs := make([]astEntry, 0, len(astToFileMutants))
	for astFile, mutants := range astToFileMutants {
		sortedASTs = append(sortedASTs, astEntry{astFile, mutants})
	}
	sort.Slice(sortedASTs, func(i, j int) bool {
		if len(sortedASTs[i].mutants) == 0 || len(sortedASTs[j].mutants) == 0 {
			return false
		}
		return sortedASTs[i].mutants[0].Site.File.Name() < sortedASTs[j].mutants[0].Site.File.Name()
	})

	for _, entry := range sortedASTs {
		astFile := entry.astFile
		mutants := entry.mutants
		if len(mutants) == 0 || mutants[0].Site.File == nil {
			continue
		}
		origPath := mutants[0].Site.File.Name()
		rel, _ := filepath.Rel(pkgDir, origPath)
		tempFile := filepath.Join(tempDir, rel)
		src, _ := os.ReadFile(origPath)
		if err := ApplySchemataToAST(astFile, mutants[0].Site.Fset, tempFile, src, mutants); err != nil {
			return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
	}


	
	for _, tempFile := range tempFiles {
		mutants := tempFileToMutants[tempFile]
		hasAST := false
		for _, m := range mutants {
			if m.Site.FileAST != nil {
				hasAST = true
				break
			}
		}
		if !hasAST {
			if err := ApplySchemataToFile(tempFile, mutants); err != nil {
				return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
			}
		}
	}

	if err := InjectSchemataHelpers(tempDir, tempFileToMutants); err != nil {
		return err
	}


	executor := newTestExecutor(tempDir, tempDir, tests)
	if err := executor.compileWithDebug(context.Background(), debug); err != nil {
		for _, m := range pkgMutants {
			m.Status = "error"
			m.Error = err
			m.TempDir = tempDir
		}
		return nil
	}


	baseline := executor.measureBaseline(context.Background())
	_, _ = executor.timeoutFor(baseline)

	resultsChan := make(chan mutantResult, len(pkgMutants))
	
	ids := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		ids[i] = m.ID
	}
	sort.Ints(ids)


	executor.runMutantsConcurrent(context.Background(), ids, concurrent, resultsChan)


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




func (e *testExecutor) runMutantsConcurrent(ctx context.Context, mutantIDs []int, concurrent int, results chan mutantResult) {
	defer close(results)

	
	sort.Ints(mutantIDs)

	
	
	workChan := make(chan int, len(mutantIDs))
	
	
	for _, id := range mutantIDs {
		workChan <- id
	}
	close(workChan)

	var wg sync.WaitGroup
	
	
	for i := 0; i < concurrent && i < len(mutantIDs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for mutantID := range workChan {
				select {
				case <-ctx.Done():
					results <- mutantResult{id: mutantID, status: "error", err: ctx.Err()}
					return
				default:
				}

				status, err := e.runMutant(ctx, mutantID)
				results <- mutantResult{id: mutantID, status: status, err: err}
			}
		}()
	}

	wg.Wait()
}



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

func mapFilesToMutants(pkgMutants []*Mutant, tempDir string, pkgDir string) map[string][]*Mutant {
	result := make(map[string][]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		
		origPath := m.Site.File.Name()
		rel, err := filepath.Rel(pkgDir, origPath)
		if err != nil {
			rel = filepath.Base(origPath)
		}
		tempFile := filepath.Join(tempDir, rel)
		result[tempFile] = append(result[tempFile], m)
	}
	return result
}

type mutantResult struct {
	id     int
	status string
	err    error
}
