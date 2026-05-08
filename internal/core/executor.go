package testing

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
)

// removeDirWithPermissions removes a directory, handling permission errors
func removeDirWithPermissions(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Remove read-only files and directories by adding write permission
		if info.IsDir() {
			_ = os.Chmod(path, 0755)
		} else {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})
}

type ProgressTracker struct {
	total       int
	lastPrinted int
	mu          sync.Mutex
	done        int
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

func rebuildMutantSites(mutants []*Mutant) map[int]MutantSite {
	sites := make(map[int]MutantSite, len(mutants))
	for _, m := range mutants {
		sites[m.ID] = MutantSite{
			File: m.Site.File.Name(),
			Line: m.Site.Line,
			Col:  m.Site.Column,
		}
	}
	return sites
}

type compileResultWithAttribution struct {
	compilerOutput string
	perMutant      map[int]error
	// attributed[id] is true iff perMutant[id] is a compile error pinned to the
	// mutant's own schemata block in the transformed source. False entries are
	// generic package-level failures that any of the mutants might be guilty
	// of (or none — could be infrastructure-level).
	attributed    map[int]bool
	compileFailed bool
}

// mutationCompileError is the typed error placed in perMutant for mutants
// whose schemata block was definitively the source of a compile error. Callers
// check for this type instead of substring-matching the message.
type mutationCompileError struct {
	lines []string
}

func (e *mutationCompileError) Error() string {
	return "compilation failed (mutation detected):\n" + strings.Join(e.lines, "\n")
}

// schemataSpan describes one schemata-block IfStmt in a transformed temp file:
// the line range its Body covers (closing brace inclusive). Compile errors at
// any line in [start, end] are attributable to mutantID.
type schemataSpan struct {
	start    int
	end      int
	mutantID int
}

// findSchemataSpans parses the schemata-transformed temp file and returns the
// exact line span for every `if activeMutantID == N { … }` IfStmt found,
// keyed by N. The parser handles strings, comments, and braces inside struct
// literals correctly — no manual brace counting.
func findSchemataSpans(tempFile string) map[int]schemataSpan {
	src, err := os.ReadFile(tempFile)
	if err != nil {
		return nil
	}
	fset := token.NewFileSet()
	// Mode 0 with errors ignored: the parser still produces a partial AST that
	// covers the schemata blocks, even if a downstream mutation introduced a
	// type error (which doesn't affect parsing).
	file, _ := parser.ParseFile(fset, tempFile, src, 0)
	if file == nil {
		return nil
	}
	spans := make(map[int]schemataSpan)
	ast.Inspect(file, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || ifStmt.Body == nil {
			return true
		}
		bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op != token.EQL {
			return true
		}
		ident, ok := bin.X.(*ast.Ident)
		if !ok || ident.Name != "activeMutantID" {
			return true
		}
		lit, ok := bin.Y.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return true
		}
		id, err := strconv.Atoi(lit.Value)
		if err != nil {
			return true
		}
		spans[id] = schemataSpan{
			start:    fset.Position(ifStmt.Pos()).Line,
			end:      fset.Position(ifStmt.Body.End()).Line,
			mutantID: id,
		}
		return true
	})
	return spans
}

func attributeCompileErrors(tempDir string, projectRoot string, mutantIDs []int, sites map[int]MutantSite, output string) compileResultWithAttribution {
	result := compileResultWithAttribution{
		compilerOutput: output,
		perMutant:      make(map[int]error, len(mutantIDs)),
		attributed:     make(map[int]bool, len(mutantIDs)),
	}

	parsed := ParseCompilerErrors(output)
	if len(parsed) == 0 {
		result.compileFailed = true
		compileErr := fmt.Errorf("compilation failed: %s", output)
		for _, id := range mutantIDs {
			result.perMutant[id] = compileErr
		}
		return result
	}

	// Build the set of schemata-transformed temp files referenced by the
	// mutants in this batch, then parse each once to extract precise IfStmt
	// spans for every mutant ID it contains.
	tempFiles := make(map[string]bool)
	mutantTempFile := make(map[int]string, len(mutantIDs))
	for _, id := range mutantIDs {
		site, ok := sites[id]
		if !ok {
			continue
		}
		relPath, _ := filepath.Rel(projectRoot, site.File)
		tempFile := filepath.Clean(filepath.Join(tempDir, relPath))
		mutantTempFile[id] = tempFile
		tempFiles[tempFile] = true
	}

	fileSpans := make(map[string][]schemataSpan, len(tempFiles))
	for f := range tempFiles {
		for _, span := range findSchemataSpans(f) {
			fileSpans[f] = append(fileSpans[f], span)
		}
	}

	mutantErrLines := make(map[int][]string, len(mutantIDs))
	var unattributed []string
	for _, ce := range parsed {
		errFile := filepath.Clean(ce.File)
		if !filepath.IsAbs(errFile) {
			errFile = filepath.Clean(filepath.Join(tempDir, errFile))
		}
		spans, ok := fileSpans[errFile]
		formatted := fmt.Sprintf("%s:%d:%d: %s", ce.File, ce.Line, ce.Col, ce.Message)
		if !ok {
			unattributed = append(unattributed, formatted)
			continue
		}
		// Innermost match: the schemata block whose [start,end] contains the
		// error line and whose start is greatest. Handles nested if-else-if
		// chains where multiple mutants share a site.
		bestID := -1
		bestStart := -1
		for _, sp := range spans {
			if sp.start <= ce.Line && ce.Line <= sp.end && sp.start > bestStart {
				bestStart = sp.start
				bestID = sp.mutantID
			}
		}
		if bestID >= 0 {
			mutantErrLines[bestID] = append(mutantErrLines[bestID], formatted)
		} else {
			unattributed = append(unattributed, formatted)
		}
	}

	for id, lines := range mutantErrLines {
		result.perMutant[id] = &mutationCompileError{lines: lines}
		result.attributed[id] = true
	}

	if len(unattributed) > 0 {
		result.compileFailed = true
		genericErr := fmt.Errorf("compilation failed (unattributed):\n%s", strings.Join(unattributed, "\n"))
		for _, id := range mutantIDs {
			if result.perMutant[id] == nil {
				result.perMutant[id] = genericErr
			}
		}
	}

	return result
}

type testExecutor struct {
	tempDir     string
	testBinary  string
	pkgDir      string
	tests       []string
	timeout     time.Duration
	baseEnv     []string
	mutantEnv   []string
	log         *logger.Logger
	projectRoot string
	buildTags   []string
}

func newTestExecutor(tempDir, pkgDir, projectRoot string, tests []string, log *logger.Logger) *testExecutor {
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
		log:         log,
		projectRoot: projectRoot,
	}
}

func (e *testExecutor) compileWithAttribution(ctx context.Context, mutantIDs []int, sites map[int]MutantSite) compileResultWithAttribution {
	e.testBinary = filepath.Join(e.pkgDir, "package.test")
	relPkg := e.relPath()

	args := []string{"test", "-c", "-vet=off"}
	if len(e.buildTags) > 0 {
		args = append(args, "-tags", strings.Join(e.buildTags, ","))
	}
	args = append(args, "-o", e.testBinary, relPkg)
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = e.tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.log.Debug("[COMPILE] FAILED for package %s: %v\nOutput:\n%s", relPkg, err, string(out))
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
	durations := make([]time.Duration, 0, 2)
	for i := 0; i < 2; i++ {
		start := time.Now()
		cmd := exec.CommandContext(ctx, e.testBinary, testArgs("10s", e.tests)...)
		cmd.Dir = e.tempDir
		runErr := cmd.Run() // pass/fail is irrelevant; we want elapsed time only.
		elapsed := time.Since(start)
		// Discard runs that the OS rejected outright (binary missing or
		// non-executable). A test that genuinely ran and exited — pass, fail,
		// or skip — is a valid baseline sample.
		if _, missing := runErr.(*exec.Error); missing {
			continue
		}
		durations = append(durations, elapsed)
	}

	if len(durations) == 0 {
		return defaultMutantTimeout, false
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

	cmdEnv := make([]string, len(e.mutantEnv))
	copy(cmdEnv, e.mutantEnv)
	cmdEnv[len(e.mutantEnv)-1] = "GORGON_MUTANT_ID=" + strconv.Itoa(mutantID)

	testFilter := ""
	if len(e.tests) > 0 {
		testFilter = strings.Join(e.tests, "|")
	}

	start := time.Now()
	raw, err := runTestBinary(
		hardCtx,
		e.testBinary,
		e.pkgDir,
		cmdEnv,
		testFilter,
		fmt.Sprintf("%.0fs", e.timeout.Seconds()),
	)
	duration := time.Since(start)

	r := classifyVerboseRun(raw, err, hardCtx.Err() == context.DeadlineExceeded)

	return mutantResult{
		id:           mutantID,
		status:       r.status,
		err:          err,
		killedBy:     r.killedBy,
		killDuration: duration,
		killOutput:   r.killOutput,
	}
}

func (e *testExecutor) relPath() string {
	rel, _ := filepath.Rel(e.tempDir, e.pkgDir)
	if rel == "." {
		return "."
	}
	return "./" + filepath.ToSlash(rel)
}

func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, pkgToMutants map[string][]*Mutant, mutantSites map[int]MutantSite, concurrent int, testsByPkg map[string][]string, buildTags []string, prog *ProgressTracker, log *logger.Logger) ([]mutantResult, error) {
	resultsChan := make(chan mutantResult)
	var allResults []mutantResult
	var resultsMu sync.Mutex
	var collectorDone sync.WaitGroup
	collectorDone.Add(1)
	go func() {
		defer collectorDone.Done()
		for result := range resultsChan {
			resultsMu.Lock()
			allResults = append(allResults, result)
			resultsMu.Unlock()
		}
	}()
	
	testGroup, testCtx := errgroup.WithContext(ctx)
	testGroup.SetLimit(concurrent)

	var compileGroup, compileCtx = errgroup.WithContext(ctx)
	// compileConcurrent := concurrent
	// if compileConcurrent > 2 {
	// 	compileConcurrent = 2
	// }
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
			
			// Determine tests for this package by matching original source directory
			var pkgTests []string
			if len(testsByPkg) > 0 {
				// Find the original source directory for this temp package
				// pkgToMutants contains mutants with Site.File pointing to original source
				if pkgMuts := pkgToMutants[pkgDir]; len(pkgMuts) > 0 {
					for _, m := range pkgMuts {
						if m.Site.File != nil {
							origDir := filepath.Dir(m.Site.File.Name())
							// testsByPkg is keyed by absolute paths from extractTests
							absOrigDir, err := filepath.Abs(origDir)
							if err == nil {
								if tests, ok := testsByPkg[absOrigDir]; ok {
									pkgTests = tests
									break
								}
							}
						}
					}
				}
			}
			
			executor := newTestExecutor(tempDir, pkgDir, tempDir, pkgTests, log)
			executor.buildTags = buildTags
			pkgMuts := pkgToMutants[pkgDir]

			// Authoritative test-file check via `go list`. If the package has
			// no in-package or external test files, every mutant is UNTESTED
			// and we skip compile/run entirely.
			hasTests, listErr := packageHasGoTestFiles(compileCtx, tempDir, executor.relPath(), buildTags)
			if listErr != nil {
				executor.log.Debug("go list failed for %s: %v — falling through to compile", executor.relPath(), listErr)
				hasTests = true // best effort: let compile decide
			}
			if !hasTests {
				for _, mutantID := range mutantIDsForPkg {
					resultsChan <- mutantResult{id: mutantID, status: "untested"}
					if prog != nil {
						prog.Record()
					}
				}
				return nil
			}

			currentSites := make(map[int]MutantSite, len(mutantIDsForPkg))
			for _, id := range mutantIDsForPkg {
				if site, ok := mutantSites[id]; ok {
					currentSites[id] = site
				}
			}

			result := executor.compileWithAttribution(compileCtx, mutantIDsForPkg, currentSites)

			for _, mutantID := range mutantIDsForPkg {
				err := result.perMutant[mutantID]
				if err != nil {
					killedBy := "compilation error"
					if result.attributed[mutantID] {
						killedBy = "(compiler)"
					}
					resultsChan <- mutantResult{
						id:           mutantID,
						status:       "error",
						err:          err,
						killedBy:     killedBy,
						killDuration: 0,
						killOutput:   err.Error(),
					}
					if prog != nil {
						prog.Record()
					}
				}
			}

			if _, statErr := os.Stat(executor.testBinary); os.IsNotExist(statErr) {
				// We confirmed via `go list` that the package has test files.
				// If `go test -c` succeeded yet produced no binary, treat the
				// remaining unattributed mutants as compile errors.
				count := 0
				for _, mutantID := range mutantIDsForPkg {
					if result.perMutant[mutantID] == nil {
						resultsChan <- mutantResult{
							id:         mutantID,
							status:     "error",
							killedBy:   "(compiler)",
							killOutput: result.compilerOutput,
						}
						if prog != nil {
							prog.Record()
						}
						count++
					}
				}
				if count > 0 {
					pkg := executor.relPath()
					if pkg == "" || pkg == "./" {
						pkg = filepath.Base(pkgDir)
					}
					executor.log.Debug("Test build failed for %s — %d mutant(s) marked as compile errors", pkg, count)
				}
				return nil
			}

			baseline, baselineOK := executor.measureBaseline(testCtx)

			if baselineOK {
				_, _ = executor.timeoutFor(baseline)
			} else {
				// No meaningful baseline (e.g. package has no tests, or binary exits
				// instantly). Use a generous fixed timeout so mutants aren't falsely
				// killed by a too-short deadline.
				executor.timeout = defaultMutantTimeout
			}

			for _, mutantID := range mutantIDsForPkg {
				err := result.perMutant[mutantID]
				if err == nil {

					var mutant *Mutant
					for _, m := range pkgMuts {
						if m.ID == mutantID {
							mutant = m
							break
						}
					}
					if mutant != nil && (mutant.Status == "untested" || mutant.Status == "error") {

						resultsChan <- mutantResult{
							id:           mutantID,
							status:       mutant.Status,
							err:          mutant.Error,
							killedBy:     mutant.KilledBy,
							killDuration: mutant.KillDuration,
							killOutput:   mutant.KillOutput,
						}
						if prog != nil {
							prog.Record()
						}
						continue
					}

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
		collectorDone.Wait()
		resultsMu.Lock()
		results := allResults
		resultsMu.Unlock()

		if prog != nil {
			prog.Finish()
		}

		return results, fmt.Errorf("test execution failed: %w", err)
	}
	close(resultsChan)
	collectorDone.Wait()

	resultsMu.Lock()
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].id < allResults[j].id
	})
	results := allResults
	resultsMu.Unlock()

	if prog != nil {
		prog.Finish()
	}

	return results, nil
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

func runStandalonePackage(ctx context.Context, pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, workerTempDir string, progbar bool, buildTags []string, prog *ProgressTracker, log *logger.Logger) error {

	entries, _ := os.ReadDir(workerTempDir)
	for _, e := range entries {
		_ = removeDirWithPermissions(filepath.Join(workerTempDir, e.Name()))
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
		posMap, err := ApplySchemataToAST(astFile, mutants[0].Site.Fset, tempFile, src, mutants)
		if err != nil {
			return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
		}
		for _, m := range mutants {
			if pm, ok := posMap[m.ID]; ok {
				m.TempLine = pm.TempLine
				m.TempCol = pm.TempCol
			}
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
			posMap, err := ApplySchemataToFile(tempFile, mutants)
			if err != nil {
				return fmt.Errorf("schemata failed on %s: %w", tempFile, err)
			}
			for _, m := range mutants {
				if pm, ok := posMap[m.ID]; ok {
					m.TempLine = pm.TempLine
					m.TempCol = pm.TempCol
				}
			}
		}
	}

	if err := InjectSchemataHelpers(tempFileToMutants, log); err != nil {
		return err
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tempDir
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Run()

	pkgRelPath, _ := filepath.Rel(projectRoot, pkgDir)
	pkgTempDir := filepath.Join(tempDir, pkgRelPath)

	executor := newTestExecutor(tempDir, pkgTempDir, projectRoot, tests, log)
	executor.buildTags = buildTags

	mutantIDs := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		mutantIDs[i] = m.ID
	}

	// Authoritative test-file check before compile.
	if hasTests, listErr := packageHasGoTestFiles(ctx, tempDir, executor.relPath(), buildTags); listErr == nil && !hasTests {
		for _, m := range pkgMutants {
			if m.Status == "" {
				m.Status = "untested"
			}
		}
		if prog != nil {
			for range mutantIDs {
				prog.Record()
			}
		}
		return nil
	}

	sites := rebuildMutantSites(pkgMutants)

	result := executor.compileWithAttribution(ctx, mutantIDs, sites)
	for _, m := range pkgMutants {
		err := result.perMutant[m.ID]
		if err != nil {
			m.Status = "error"
			if result.attributed[m.ID] {
				m.KilledBy = "(compiler)"
				m.KillOutput = fmt.Sprintf("compilation failed: %s", err.Error())
			}
			m.Error = err
			m.TempDir = tempDir
			if prog != nil {
				prog.Record()
			}
		}
	}

	var testableIDs []int
	for _, m := range pkgMutants {
		if m.Status == "" || m.Status == "untested" {

			if m.Status != "error" {
				testableIDs = append(testableIDs, m.ID)
			}
		}
	}
	if len(testableIDs) == 0 {

		if prog != nil {
			prog.Finish()
		}
		return nil
	}

	// Test files were confirmed via `go list` above. If `go test -c` reported
	// success but produced no binary, that is a silent build failure — every
	// remaining mutant is a compile error.
	if _, statErr := os.Stat(executor.testBinary); os.IsNotExist(statErr) {
		for _, m := range pkgMutants {
			if m.Status == "" {
				m.Status = "error"
				m.KilledBy = "(compiler)"
				m.KillOutput = "test binary not created despite test files present"
			}
		}
		if prog != nil {
			for range testableIDs {
				prog.Record()
			}
		}
		return nil
	}

	baseline, baselineOK := executor.measureBaseline(ctx)

	if baselineOK {
		_, _ = executor.timeoutFor(baseline)
	} else {
		// No meaningful baseline — package may have no test files or they
		// exit immediately. Use the default per-mutant timeout.
		executor.timeout = defaultMutantTimeout
	}

	resultsChan := make(chan mutantResult, len(testableIDs))
	sort.Ints(testableIDs)

	executor.runMutantsConcurrent(ctx, testableIDs, concurrent, resultsChan, prog)

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


func resolveSuitePaths(ctx context.Context, workspaceDir string, suite config.ExternalSuite, log *logger.Logger) ([]string, error) {
	var resolved []string
	for _, p := range suite.Paths {
		args := []string{"list", "-f", "{{.Dir}}"}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		// `go list` treats bare paths like "tests/integration/..." as import
		// paths, not directory patterns. Prefix with "./" so it's resolved
		// relative to workspaceDir.
		listArg := p
		if !strings.HasPrefix(listArg, "./") && !strings.HasPrefix(listArg, "/") &&
			!strings.HasPrefix(listArg, "../") && listArg != "." {
			listArg = "./" + listArg
		}
		args = append(args, listArg)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspaceDir
		log.Debug("[EXTERNAL] Running: go %v in %s", args, workspaceDir)
		out, err := cmd.CombinedOutput()
		log.Debug("[EXTERNAL] Output: %s, Error: %v", string(out), err)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			rel, err := filepath.Rel(workspaceDir, line)
			if err != nil {
				continue
			}
			resolved = append(resolved, "./"+filepath.ToSlash(rel))
		}
	}
	return resolved, nil
}

func buildExternalSuiteBinaries(ctx context.Context, workspaceDir string, suite config.ExternalSuite, resolvedPaths []string, log *logger.Logger) (map[string]string, error) {
	binaries := make(map[string]string, len(resolvedPaths))
	
	// Create temp dir for binaries (outside workspace to avoid conflicts)
	binDir, err := os.MkdirTemp("", "gorgon-external-bins-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create binary temp dir: %w", err)
	}
	
	log.Debug("[EXTERNAL] Building binaries for %d resolved paths from workspace %s", len(resolvedPaths), workspaceDir)
	for _, relPkg := range resolvedPaths {
		safeName := strings.NewReplacer("/", "_", ".", "_").Replace(relPkg)
		binPath := filepath.Join(binDir, safeName+".test")

		args := []string{"test", "-c", "-vet=off", "-o", binPath}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		args = append(args, relPkg)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspaceDir
		log.Debug("[EXTERNAL] Building: go %v in %s", args, workspaceDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Debug("[EXTERNAL] Build failed: %s", string(out))
			continue
		}

		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			log.Debug("[EXTERNAL] Binary not created at %s", binPath)
			continue
		}
		log.Debug("[EXTERNAL] Binary created at %s", binPath)
		binaries[relPkg] = binPath
	}
	log.Debug("[EXTERNAL] Built %d binaries in %s", len(binaries), binDir)
	return binaries, nil
}

func runMutantsAgainstBinary(ctx context.Context, binPath, workspaceDir string, mutants []*Mutant, timeout time.Duration, concurrent int, suiteName string) []mutantResult {
	resultsChan := make(chan mutantResult, len(mutants))
	sem := make(chan struct{}, concurrent)
	var wg sync.WaitGroup

	for _, m := range mutants {
		m := m
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			env := append(os.Environ(),
				fmt.Sprintf("GORGON_MUTANT_ID=%d", m.ID))

			runCtx, cancel := context.WithTimeout(ctx, timeout+hardTimeoutMargin)
			defer cancel()

			raw, err := runTestBinary(
				runCtx,
				binPath,
				workspaceDir,
				env,
				"",
				fmt.Sprintf("%.0fs", timeout.Seconds()),
			)
			r := classifyVerboseRun(raw, err, runCtx.Err() == context.DeadlineExceeded)

			killedBy := r.killedBy
			if r.status == "killed" {
				if killedBy == "" {
					killedBy = suiteName
				} else {
					killedBy = killedBy + " [" + suiteName + "]"
				}
			}

			resultsChan <- mutantResult{
				id:         m.ID,
				status:     r.status,
				killedBy:   killedBy,
				killOutput: r.killOutput,
			}
		}()
	}

	wg.Wait()
	close(resultsChan)

	var all []mutantResult
	for r := range resultsChan {
		all = append(all, r)
	}
	return all
}

func collectAllSuitePaths(suites []config.ExternalSuite) []string {
	var paths []string
	for _, s := range suites {
		paths = append(paths, s.Paths...)
	}
	return paths
}
