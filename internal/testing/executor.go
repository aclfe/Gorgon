package testing

import (
	"context"
	"fmt"
	"go/ast"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)


type ProgressTracker struct {
	total      int
	lastPrinted int
	mu         sync.Mutex
	done       int
}

func NewProgressTracker(total int) *ProgressTracker {
	return &ProgressTracker{total: total}
}


func (p *ProgressTracker) Record() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.done++
	pct := (p.done * 100) / p.total
	for p.lastPrinted < pct && pct > 0 {
		p.lastPrinted += 2
		fmt.Fprintf(os.Stderr, "Mutating [%d/%d %d%%]\n", p.done, p.total, p.lastPrinted)
	}
}

func (p *ProgressTracker) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done < p.total {
		pct := (p.done * 100) / p.total
		if pct > p.lastPrinted {
			p.lastPrinted = pct
		}
		fmt.Fprintf(os.Stderr, "Mutating [%d/%d %d%%]\n", p.done, p.total, pct)
	}
	fmt.Fprintln(os.Stderr)
}

type MutantSite struct {
	File string 
	Line int
	Col  int
}

type compileResultWithAttribution struct {
	compilerOutput string
	perMutant      map[int]error
}

func attributeCompileErrors(tempDir string, projectRoot string, mutantIDs []int, sites map[int]MutantSite, output string) compileResultWithAttribution {
	result := compileResultWithAttribution{
		compilerOutput: output,
		perMutant:      make(map[int]error, len(mutantIDs)),
	}

	errors := ParseCompilerErrors(output)
	if len(errors) == 0 {
		
		
		for _, id := range mutantIDs {
			result.perMutant[id] = fmt.Errorf("compilation failed (unparseable errors):\n%s", output)
		}
		return result
	}

	type pos struct {
		file string
		line int
		id   int
	}
	positions := make([]pos, 0, len(mutantIDs))
	for _, id := range mutantIDs {
		site, ok := sites[id]
		if !ok {
			continue
		}
		
		relPath, _ := filepath.Rel(projectRoot, site.File)
		tempFile := filepath.Join(tempDir, relPath)
		positions = append(positions, pos{file: tempFile, line: site.Line, id: id})
	}

	mutantErrors := make(map[int][]string, len(mutantIDs))

	
	
	// Collect all files that have compilation errors (for fallback attribution)
	// Store both the cleaned absolute path AND basenames for matching
	errorFiles := make(map[string]bool)
	errorFileBasenames := make(map[string]bool)
	for _, ce := range errors {
		errFile := filepath.Clean(ce.File)
		if !filepath.IsAbs(errFile) {
			errFile = filepath.Join(tempDir, errFile)
		}
		errorFiles[errFile] = true
		errorFileBasenames[filepath.Base(ce.File)] = true
	}

	for _, ce := range errors {
		errFile := filepath.Clean(ce.File)
		if !filepath.IsAbs(errFile) {
			errFile = filepath.Join(tempDir, errFile)
		}

		bestID := -1
		bestDist := math.MaxInt32
		for _, p := range positions {
			if filepath.Clean(p.file) == errFile {
				dist := absInt(p.line - ce.Line)
				if dist < bestDist {
					bestDist = dist
					bestID = p.id
				}
			}
		}
		if bestID >= 0 && bestDist <= 50 {
			line := fmt.Sprintf("%s:%d:%d: %s", ce.File, ce.Line, ce.Col, ce.Message)
			mutantErrors[bestID] = append(mutantErrors[bestID], line)
		}
	}



	if len(mutantErrors) == 0 {
		// No errors could be matched to specific mutants.
		// Only mark mutants as having errors if their file is one of the files
		// with compilation errors. Mutants in other files should proceed to testing.
		for _, id := range mutantIDs {
			site, ok := sites[id]
			if !ok {
				continue
			}
			relPath, _ := filepath.Rel(projectRoot, site.File)
			tempFile := filepath.Join(tempDir, relPath)
			cleanTempFile := filepath.Clean(tempFile)

			// Check if this mutant's file has errors using multiple matching strategies
			hasErrors := errorFiles[cleanTempFile] || errorFileBasenames[filepath.Base(site.File)]

			if hasErrors {
				result.perMutant[id] = fmt.Errorf("compilation failed in package:\n%s", output)
			}
			// Else: mutant is in a different file with no errors - leave as nil so it can be tested
		}
		return result
	}

	for _, id := range mutantIDs {
		if errs, ok := mutantErrors[id]; ok && len(errs) > 0 {
			// Errors were matched to this mutant - it likely caused the compilation failure
			result.perMutant[id] = fmt.Errorf("compilation failed (mutation detected):\n%s", strings.Join(errs, "\n"))
		} else {
			// This mutant exists but no errors were matched to it.
			// Only mark as error if its file has compilation errors.
			site, ok := sites[id]
			if !ok {
				continue
			}
			relPath, _ := filepath.Rel(projectRoot, site.File)
			tempFile := filepath.Join(tempDir, relPath)
			cleanTempFile := filepath.Clean(tempFile)

			hasErrors := errorFiles[cleanTempFile] || errorFileBasenames[filepath.Base(site.File)]
			if hasErrors {
				// Mutant is in a file with errors, but errors didn't match closely enough
				result.perMutant[id] = fmt.Errorf("compilation failed (mutation detected):\n%s", output)
			}
			// Else: mutant is in a different file - compilation failed elsewhere, proceed to testing
		}
	}

	return result
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

type testExecutor struct {
	tempDir    string
	testBinary string
	pkgDir     string
	tests      []string
	timeout    time.Duration
	baseEnv    []string
	mutantEnv  []string
	debug      bool
	projectRoot string
}

func newTestExecutor(tempDir, pkgDir, projectRoot string, tests []string, debug bool) *testExecutor {
	baseEnv := os.Environ()

	mutantEnv := make([]string, len(baseEnv)+1)
	copy(mutantEnv, baseEnv)
	return &testExecutor{
		tempDir:     tempDir,
		pkgDir:      pkgDir,
		tests:       tests,
		baseEnv:     baseEnv,
		mutantEnv:   mutantEnv,
		timeout:     30 * time.Second, 
		debug:       debug,
		projectRoot: projectRoot,
	}
}




func (e *testExecutor) compileWithAttribution(ctx context.Context, mutantIDs []int, sites map[int]MutantSite) compileResultWithAttribution {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-vet=off", "-o", e.testBinary, relPkg)
	cmd.Dir = e.tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return attributeCompileErrors(e.tempDir, e.projectRoot, mutantIDs, sites, string(out))
	}
	
	result := compileResultWithAttribution{
		perMutant: make(map[int]error, len(mutantIDs)),
	}
	for _, id := range mutantIDs {
		result.perMutant[id] = nil
	}
	return result
}

func (e *testExecutor) measureBaseline(ctx context.Context) (time.Duration, bool) {


	var durations []time.Duration
	maxAttempts := 2
	failureCount := 0

	for i := 0; i < maxAttempts && len(durations) < 2; i++ {
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
		return minBaselineDuration * time.Millisecond, false
	}

	slices.Sort(durations)
	median := durations[len(durations)/2]

	if median < minBaselineDuration*time.Millisecond {
		median = minBaselineDuration * time.Millisecond
	}
	return median, true
}

func (e *testExecutor) timeoutFor(baseline time.Duration) (string, time.Duration) {
	
	
	
	if baseline > maxBaselineCap {
		baseline = maxBaselineCap
	}
	timeout := time.Duration(float64(baseline) * timeoutMultiplier)
	if timeout > maxTimeout*time.Second {
		timeout = maxTimeout * time.Second
	}
	
	if timeout < minMutantTimeout {
		timeout = minMutantTimeout
	}
	e.timeout = timeout
	return fmt.Sprintf("%.0fs", timeout.Seconds()), timeout
}



func (e *testExecutor) hardTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, e.timeout+hardTimeoutMargin)
}

func (e *testExecutor) runMutant(ctx context.Context, mutantID int) mutantResult {
	hardCtx, cancel := e.hardTimeout(ctx)
	defer cancel()

	args := testArgs(fmt.Sprintf("%.0fs", e.timeout.Seconds()), e.tests)
	cmd := exec.CommandContext(hardCtx, e.testBinary, args...)
	cmd.Dir = e.pkgDir

	
	cmdEnv := make([]string, len(e.mutantEnv))
	copy(cmdEnv, e.mutantEnv)
	cmdEnv[len(e.baseEnv)] = "GORGON_MUTANT_ID=" + strconv.Itoa(mutantID)
	cmd.Env = cmdEnv

	
	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	status := "survived"
	killedBy := ""
	killOutput := ""

	if err != nil {
		status = "killed"
		outStr := string(out)

		
		
		killedBy = parseFailedTest(outStr)

		
		if len(outStr) > 300 {
			killOutput = outStr[:300]
		} else {
			killOutput = outStr
		}
	}

	
	if e.debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Mutant #%d %s (pkg: %s, timeout=%v, killed_by=%s, duration=%v)\n  Cmd: %s %v\n  Output: %s\n",
			mutantID, status, e.pkgDir, e.timeout, killedBy, duration, e.testBinary, args, killOutput)
	}

	return mutantResult{
		id:           mutantID,
		status:       status,
		err:          err,
		killedBy:     killedBy,
		killDuration: duration,
		killOutput:   killOutput,
	}
}



func parseFailedTest(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "--- FAIL: ") {
			
			parts := strings.SplitN(line, " ", 3)
			if len(parts) >= 3 {
				
				testName := parts[2]
				if idx := strings.Index(testName, " ("); idx > 0 {
					testName = testName[:idx]
				}
				return testName
			}
		}
	}
	
	if output != "" {
		return "(test output non-empty)"
	}
	return "(compilation/runtime error)"
}

func (e *testExecutor) relPath() string {
	rel, _ := filepath.Rel(e.tempDir, e.pkgDir)
	if rel == "." {
		return ""
	}
	return "./" + filepath.ToSlash(rel)
}

func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, mutantSites map[int]MutantSite, concurrent int, tests []string, prog *ProgressTracker, debug bool) ([]mutantResult, error) {

	type compileResult struct {
		pkgDir string
		err    error
	}

	resultsChan := make(chan mutantResult, sumMutantIDs(pkgToMutantIDs))
	testGroup, testCtx := errgroup.WithContext(ctx)
	testGroup.SetLimit(concurrent)

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
			executor := newTestExecutor(tempDir, pkgDir, tempDir, tests, debug)
			result := executor.compileWithAttribution(compileCtx, mutantIDsForPkg, mutantSites)

			// Check if ALL mutants have compilation errors (nil means no error, can proceed to testing)
			hasAnyMutationDetected := false
			hasNilError := false
			for _, err := range result.perMutant {
				if err != nil {
					if strings.Contains(err.Error(), "mutation detected") {
						hasAnyMutationDetected = true
					}
				} else {
					hasNilError = true
				}
			}

			if hasAnyMutationDetected || (!hasNilError && len(result.perMutant) > 0) {
				// All mutants have compilation errors - set status for all
				hasUnrelatedError := false
				for _, err := range result.perMutant {
					if err != nil && !strings.Contains(err.Error(), "mutation detected") {
						hasUnrelatedError = true
						break
					}
				}

				if hasUnrelatedError {
					compErrsMu.Lock()
					compErrors[pkgDir] = fmt.Errorf("compilation failed: %s", result.compilerOutput)
					compErrsMu.Unlock()
				}

				for _, mutantID := range mutantIDsForPkg {
					err := result.perMutant[mutantID]
					status := "killed"
					killedBy := ""
					killOutput := ""
					if err != nil && !strings.Contains(err.Error(), "mutation detected") {
						status = "error"
					} else if err != nil {
						killedBy = "(compiler)"
						killOutput = fmt.Sprintf("compilation failed: %s", err.Error()[:min(200, len(err.Error()))])
					}
					resultsChan <- mutantResult{
						id:           mutantID,
						status:       status,
						err:          err,
						killedBy:     killedBy,
						killDuration: 0,
						killOutput:   killOutput,
					}
					if prog != nil {
						prog.Record()
					}
				}
				return nil
			}

			// Some mutants have no compilation errors - they can proceed to testing
			// First, emit results for mutants that DO have errors
			for _, mutantID := range mutantIDsForPkg {
				err := result.perMutant[mutantID]
				if err != nil {
					status := "error"
					if strings.Contains(err.Error(), "mutation detected") {
						status = "killed"
					}
					resultsChan <- mutantResult{
						id:           mutantID,
						status:       status,
						err:          err,
						killedBy:     "",
						killDuration: 0,
						killOutput:   "",
					}
					if prog != nil {
						prog.Record()
					}
				}
			}

			// Then run tests for mutants without errors
			baseline, baselineOK := executor.measureBaseline(testCtx)


			if baselineOK {
				_, _ = executor.timeoutFor(baseline)
			} else {

				executor.timeout = 30 * time.Second
			}

			for _, mutantID := range mutantIDsForPkg {
				err := result.perMutant[mutantID]
				if err == nil {
					// No compilation error for this mutant - run tests
					mutantID := mutantID
					testGroup.Go(func() error {
						result := executor.runMutant(testCtx, mutantID)
						resultsChan <- result
						if prog != nil {
							prog.Record()
						}
						return nil
					})
				}
			}
			return nil
		})
	}


	_ = compileGroup.Wait()


	if err := testGroup.Wait(); err != nil {
		
		close(resultsChan)
		var allResults []mutantResult
		for result := range resultsChan {
			allResults = append(allResults, result)
		}
		
		if prog != nil {
			prog.Finish()
		}
		
		
		return allResults, fmt.Errorf("test execution failed: %w", err)
	}
	close(resultsChan)

	var allResults []mutantResult
	for result := range resultsChan {
		allResults = append(allResults, result)
	}


	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].id < allResults[j].id
	})

	if prog != nil {
		prog.Finish()
	}

	if len(compErrors) > 0 {
		var errs []string
		for pkgDir, err := range compErrors {
			errs = append(errs, fmt.Sprintf("%s: %v", pkgDir, err))
		}
		return allResults, fmt.Errorf("compilation failures: %s", strings.Join(errs, "; "))
	}

	return allResults, nil
}





func copyAllPackages(srcRoot, dstRoot string, skipFiles map[string]bool) error {
	return filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == ".git" || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			
			relPath, err := filepath.Rel(srcRoot, path)
			if err != nil {
				return nil
			}
			dstPath := filepath.Join(dstRoot, relPath)
			os.MkdirAll(dstPath, 0o755)
			return nil
		}
		
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		
		if skipFiles[path] {
			return nil
		}
		
		relPath, err := filepath.Rel(srcRoot, path)
		if err != nil {
			return nil
		}
		dstPath := filepath.Join(dstRoot, relPath)
		return copyFileWithBuffer(path, dstPath)
	})
}

func runStandalonePackage(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, testPaths []string, workerTempDir string, progbar bool, prog *ProgressTracker, debug bool) error {

	
	entries, _ := os.ReadDir(workerTempDir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(workerTempDir, e.Name()))
	}

	tempDir := workerTempDir

	
	projectRoot := pkgDir
	for {
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			break
		}
		if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
			projectRoot = parent
			break
		}
		projectRoot = parent
	}

	
	modulePath := filepath.Base(projectRoot)
	if goModData, err := os.ReadFile(filepath.Join(projectRoot, "go.mod")); err == nil {
		for _, line := range strings.Split(string(goModData), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "module ") {
				modulePath = strings.TrimPrefix(line, "module ")
				break
			}
		}
	}
	
	if modulePath == "" {
		modulePath = filepath.Base(pkgDir)
	}
	
	
	if goModData, err := os.ReadFile(filepath.Join(projectRoot, "go.mod")); err == nil {
		if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), goModData, filePermissions); err != nil {
			return fmt.Errorf("failed to copy go.mod: %w", err)
		}
	} else {
		
		goMod := fmt.Sprintf("module %s\n\ngo %s\n", modulePath, goVersion)
		if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
			return fmt.Errorf("failed to write go.mod: %w", err)
		}
	}
	
	
	if goSumData, err := os.ReadFile(filepath.Join(projectRoot, "go.sum")); err == nil {
		if err := os.WriteFile(filepath.Join(tempDir, "go.sum"), goSumData, filePermissions); err != nil {
			return fmt.Errorf("failed to copy go.sum: %w", err)
		}
	}

	
	mutatedOrigPaths := make(map[string]bool, len(pkgMutants))
	for _, m := range pkgMutants {
		if m.Site.File != nil {
			mutatedOrigPaths[m.Site.File.Name()] = true
		}
	}

	
	
	
	if err := copyAllPackages(projectRoot, tempDir, mutatedOrigPaths); err != nil {
		return fmt.Errorf("failed to copy packages: %w", err)
	}

	
	tempFileToMutants := make(map[string][]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		if m.Site.File == nil {
			continue
		}
		origPath := m.Site.File.Name()
		rel, err := filepath.Rel(projectRoot, origPath)
		if err != nil {
			rel = filepath.Base(origPath)
		}
		tempFile := filepath.Join(tempDir, rel)
		tempFileToMutants[tempFile] = append(tempFileToMutants[tempFile], m)
	}


	
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
		rel, _ := filepath.Rel(projectRoot, origPath)  
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

	if err := InjectSchemataHelpers(tempFileToMutants); err != nil {
		return err
	}

	
	
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tempDir
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run() 


	
	pkgRelPath, _ := filepath.Rel(projectRoot, pkgDir)
	pkgTempDir := filepath.Join(tempDir, pkgRelPath)

	
	executor := newTestExecutor(tempDir, pkgTempDir, projectRoot, tests, debug)

	
	sites := make(map[int]MutantSite, len(pkgMutants))
	mutantIDs := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		mutantIDs[i] = m.ID
		if m.Site.File != nil {
			sites[m.ID] = MutantSite{
				File: m.Site.File.Name(),
				Line: m.Site.Line,
				Col:  m.Site.Column,
			}
		}
	}

	result := executor.compileWithAttribution(context.Background(), mutantIDs, sites)
	for _, m := range pkgMutants {
		err := result.perMutant[m.ID]
		if err != nil {
			if strings.Contains(err.Error(), "mutation detected") {

				m.Status = "killed"
				m.KilledBy = "(compiler)"
				m.KillOutput = fmt.Sprintf("compilation failed: %s", err.Error()[:min(200, len(err.Error()))])
			} else {

				m.Status = "error"
			}
			m.Error = err
			m.TempDir = tempDir
			if prog != nil {
				prog.Record()
			}
		}
	}

	// Collect mutant IDs that have no compilation errors for testing
	var testableIDs []int
	for _, m := range pkgMutants {
		if m.Status == "" {
			testableIDs = append(testableIDs, m.ID)
		}
	}
	if len(testableIDs) == 0 {

		if prog != nil {
			prog.Finish()
		}
		return nil
	}


	baseline, baselineOK := executor.measureBaseline(context.Background())


	if baselineOK {
		_, _ = executor.timeoutFor(baseline)
	} else {

		fmt.Fprintf(os.Stderr, "Warning: baseline measurement failed, using default timeout\n")
		executor.timeout = 30 * time.Second
	}

	resultsChan := make(chan mutantResult, len(testableIDs))
	sort.Ints(testableIDs)


	executor.runMutantsConcurrent(context.Background(), testableIDs, concurrent, resultsChan, prog)


	idToMutant := make(map[int]*Mutant, len(pkgMutants))
	for _, m := range pkgMutants {
		idToMutant[m.ID] = m
	}
	for result := range resultsChan {
		if m, ok := idToMutant[result.id]; ok {
			m.Status = result.status
			m.Error = result.err
			m.TempDir = tempDir
			m.KilledBy = result.killedBy
			m.KillDuration = result.killDuration
			m.KillOutput = result.killOutput
		}
	}

	return nil
}




func (e *testExecutor) runMutantsConcurrent(ctx context.Context, mutantIDs []int, concurrent int, results chan mutantResult, prog *ProgressTracker) {
	defer close(results)

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrent)

	for _, mutantID := range mutantIDs {
		mutantID := mutantID
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				<-sem
				if prog != nil {
					prog.Record()
				}
			}()

			select {
			case <-ctx.Done():
				results <- mutantResult{id: mutantID, status: "error", err: ctx.Err()}
				return
			default:
			}

			result := e.runMutant(ctx, mutantID)
			results <- result
		}()
	}

	wg.Wait()
}



func testArgs(timeout string, tests []string) []string {
	
	if timeout == "0s" || timeout == "" {
		timeout = "5s"
	}
	args := []string{"-test.timeout=" + timeout}
	if len(tests) > 0 {
		args = append(args, "-test.run="+strings.Join(tests, "|"))
	}
	return args
}

func sumMutantIDs(m map[string][]int) int {
	total := 0
	for _, ids := range m {
		total += len(ids)
	}
	return total
}

type mutantResult struct {
	id           int
	status       string
	err          error
	killedBy     string
	killDuration time.Duration
	killOutput   string
}