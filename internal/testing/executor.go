package testing

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

func attributeCompileErrors(tempDir string, mutantIDs []int, sites map[int]MutantSite, output string) compileResultWithAttribution {
	result := compileResultWithAttribution{
		compilerOutput: output,
		perMutant:      make(map[int]error, len(mutantIDs)),
	}

	errors := ParseCompilerErrors(output)
	if len(errors) == 0 {
		for _, id := range mutantIDs {
			result.perMutant[id] = nil
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
		tempFile := filepath.Join(tempDir, relPathFromOriginal(site.File))
		positions = append(positions, pos{file: tempFile, line: site.Line, id: id})
	}

	mutantErrors := make(map[int][]string, len(mutantIDs))
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
		if bestID >= 0 && bestDist <= 20 {
			line := fmt.Sprintf("%s:%d:%d: %s", ce.File, ce.Line, ce.Col, ce.Message)
			mutantErrors[bestID] = append(mutantErrors[bestID], line)
		}
	}

	for _, id := range mutantIDs {
		if errs, ok := mutantErrors[id]; ok && len(errs) > 0 {
			result.perMutant[id] = fmt.Errorf("compilation failed:\n%s", strings.Join(errs, "\n"))
		} else {
			result.perMutant[id] = nil
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

func relPathFromOriginal(origPath string) string {
	dir := filepath.Dir(origPath)
	base := filepath.Base(origPath)
	pkg := filepath.Base(dir)
	return filepath.Join(pkg, base)
}

type testExecutor struct {
	tempDir    string
	testBinary string
	pkgDir     string
	tests      []string
	timeout    time.Duration
	baseEnv    []string
	mutantEnv  []string
}

func newTestExecutor(tempDir, pkgDir string, tests []string) *testExecutor {
	baseEnv := os.Environ()
	
	mutantEnv := make([]string, len(baseEnv)+1)
	copy(mutantEnv, baseEnv)
	return &testExecutor{
		tempDir:    tempDir,
		pkgDir:     pkgDir,
		tests:      tests,
		baseEnv:    baseEnv,
		mutantEnv:  mutantEnv,
	}
}


func (e *testExecutor) compile(ctx context.Context) error {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", e.testBinary, relPkg)
	cmd.Dir = e.tempDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("test compilation failed for %s:\n%s", relPkg, out)
	}
	return nil
}

// compileWithAttribution compiles the package and returns structured attribution
// for each mutant if compilation fails.
func (e *testExecutor) compileWithAttribution(ctx context.Context, mutantIDs []int, sites map[int]MutantSite) compileResultWithAttribution {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", e.testBinary, relPkg)
	cmd.Dir = e.tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return attributeCompileErrors(e.tempDir, mutantIDs, sites, string(out))
	}
	// Compilation succeeded — all mutants are fine
	result := compileResultWithAttribution{
		perMutant: make(map[int]error, len(mutantIDs)),
	}
	for _, id := range mutantIDs {
		result.perMutant[id] = nil
	}
	return result
}

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

func (e *testExecutor) runMutant(ctx context.Context, mutantID int) (string, error) {
	hardCtx, cancel := e.hardTimeout(ctx)
	defer cancel()

	args := testArgs(fmt.Sprintf("%.0fs", e.timeout.Seconds()), e.tests)
	cmd := exec.CommandContext(hardCtx, e.testBinary, args...)
	cmd.Dir = e.pkgDir

	// Create a copy of mutantEnv to avoid race condition when running concurrently
	cmdEnv := make([]string, len(e.mutantEnv))
	copy(cmdEnv, e.mutantEnv)
	cmdEnv[len(e.baseEnv)] = "GORGON_MUTANT_ID=" + strconv.Itoa(mutantID)
	cmd.Env = cmdEnv

	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return "killed", err
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

func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, mutantSites map[int]MutantSite, concurrent int, tests []string, prog *ProgressTracker) ([]mutantResult, error) {

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
			executor := newTestExecutor(tempDir, pkgDir, tests)
			result := executor.compileWithAttribution(compileCtx, mutantIDsForPkg, mutantSites)
			hasError := false
			for _, err := range result.perMutant {
				if err != nil {
					hasError = true
					break
				}
			}
			if hasError {
				compErrsMu.Lock()
				compErrors[pkgDir] = fmt.Errorf("compilation failed: %s", result.compilerOutput)
				compErrsMu.Unlock()

				for _, mutantID := range mutantIDsForPkg {
					err := result.perMutant[mutantID]
					// When package compilation fails, all mutants should be marked as "error"
					// because none of them could be tested
					if err == nil {
						err = fmt.Errorf("compilation failed in package: %s", result.compilerOutput)
					}
					resultsChan <- mutantResult{id: mutantID, status: "error", err: err}
					if prog != nil {
						prog.Record()
					}
				}
				return nil
			}

			baseline, baselineOK := executor.measureBaseline(testCtx)
			_, _ = executor.timeoutFor(baseline)

			if !baselineOK {
				// Baseline failed (tests don't exist or fail to run)
				// All mutants should be marked as "survived" since we can't test them
				for _, mutantID := range mutantIDsForPkg {
					resultsChan <- mutantResult{id: mutantID, status: "survived", err: nil}
					if prog != nil {
						prog.Record()
					}
				}
				return nil
			}

			for _, mutantID := range mutantIDsForPkg {
				mutantID := mutantID
				testGroup.Go(func() error {
					status, err := executor.runMutant(testCtx, mutantID)
					resultsChan <- mutantResult{id: mutantID, status: status, err: err}
					if prog != nil {
						prog.Record()
					}
					return nil
				})
			}
			return nil
		})
	}


	_ = compileGroup.Wait()


	if err := testGroup.Wait(); err != nil {
		// Collect any partial results before returning error
		close(resultsChan)
		var allResults []mutantResult
		for result := range resultsChan {
			allResults = append(allResults, result)
		}
		
		if prog != nil {
			prog.Finish()
		}
		
		// Return partial results with the error
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



func runStandalonePackage(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, workerTempDir string, progbar bool, prog *ProgressTracker) error {

	
	entries, _ := os.ReadDir(workerTempDir)
	for _, e := range entries {
		os.RemoveAll(filepath.Join(workerTempDir, e.Name()))
	}

	tempDir := workerTempDir


	pkgName := detectPackageName(pkgDir)


	// When no go.mod exists, infer the module path from imports.
	// Walk up from pkgDir to find the project root, then scan for imports.
	projectRoot := pkgDir
	for {
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			break
		}
		// If parent has a go.mod, use it as root
		if _, err := os.Stat(filepath.Join(parent, "go.mod")); err == nil {
			projectRoot = parent
			break
		}
		// If parent has no Go files in non-subdir .go files, stop
		hasGo := false
		entries, _ := os.ReadDir(parent)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
				hasGo = true
				break
			}
		}
		if !hasGo {
			break
		}
		projectRoot = parent
	}

	modulePath := detectModulePath(projectRoot, pkgDir)
	if modulePath == "" {
		modulePath = pkgName
	}
	goMod := fmt.Sprintf("module %s\n\ngo %s\n", modulePath, goVersion)
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), filePermissions); err != nil {
		return fmt.Errorf("failed to write go.mod: %w", err)
	}


	
	mutatedOrigPaths := make(map[string]bool, len(pkgMutants))
	for _, m := range pkgMutants {
		if m.Site.File != nil {
			mutatedOrigPaths[m.Site.File.Name()] = true
		}
	}


	if err := linkOrCopyDir(pkgDir, tempDir, mutatedOrigPaths); err != nil {
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

	if err := InjectSchemataHelpers(tempFileToMutants); err != nil {
		return err
	}

	// Resolve dependencies automatically. This handles external imports
	// for GOPATH-style projects that have no go.mod/go.sum.
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tempDir
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run() // Best effort: if tidy fails, compilation will report the real error


	executor := newTestExecutor(tempDir, tempDir, tests)

	// Build sites map for attribution
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
	hasCompileError := false
	for _, err := range result.perMutant {
		if err != nil {
			hasCompileError = true
			break
		}
	}
	if hasCompileError {
		for _, m := range pkgMutants {
			err := result.perMutant[m.ID]
			// When package compilation fails, all mutants should be marked as "error"
			// because none of them could be tested
			if err == nil {
				err = fmt.Errorf("compilation failed in package")
			}
			m.Status = "error"
			m.Error = err
			m.TempDir = tempDir
		}
		return nil
	}


	baseline, baselineOK := executor.measureBaseline(context.Background())
	_, _ = executor.timeoutFor(baseline)

	if !baselineOK {
		// Baseline failed (tests don't exist or fail to run)
		// All mutants should be marked as "survived" since we can't test them
		for _, m := range pkgMutants {
			m.Status = "survived"
			m.Error = nil
			m.TempDir = tempDir
		}
		return nil
	}

	resultsChan := make(chan mutantResult, len(pkgMutants))
	
	ids := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		ids[i] = m.ID
	}
	sort.Ints(ids)


	executor.runMutantsConcurrent(context.Background(), ids, concurrent, resultsChan, prog)


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

			status, err := e.runMutant(ctx, mutantID)
			results <- mutantResult{id: mutantID, status: status, err: err}
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

// detectModulePath infers the Go module path when no go.mod exists.
// It scans imports in .go files for paths that match local subdirectories,
// then extracts the module prefix (e.g. "github.com/hlandau/acmetool" from
// import "github.com/hlandau/acmetool/cli").
func detectModulePath(projectRoot string, _ string) string {
	var imports []string

	filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if !strings.HasPrefix(p, ".") && !isStdlib(p) {
				imports = append(imports, p)
			}
		}
		return nil
	})

	for _, imp := range imports {
		lastSlash := strings.LastIndex(imp, "/")
		if lastSlash < 0 {
			continue
		}
		modulePrefix := imp[:lastSlash]

		// Check if the import's last component matches a subdirectory of projectRoot
		subDir := imp[lastSlash+1:]
		if _, err := os.Stat(filepath.Join(projectRoot, subDir)); err == nil {
			return modulePrefix
		}
	}

	return ""
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