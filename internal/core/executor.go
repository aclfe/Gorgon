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

	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/pkg/config"
)

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
	compileFailed  bool
}

func attributeCompileErrors(tempDir string, projectRoot string, mutantIDs []int, sites map[int]MutantSite, output string) compileResultWithAttribution {
	result := compileResultWithAttribution{
		compilerOutput: output,
		perMutant:      make(map[int]error, len(mutantIDs)),
	}

	errors := ParseCompilerErrors(output)
	if len(errors) == 0 {

		result.compileFailed = true
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

		relPath, _ := filepath.Rel(projectRoot, site.File)
		tempFile := filepath.Join(tempDir, relPath)
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

		if bestID >= 0 && bestDist <= 5 {
			line := fmt.Sprintf("%s:%d:%d: %s", ce.File, ce.Line, ce.Col, ce.Message)
			mutantErrors[bestID] = append(mutantErrors[bestID], line)
		}

	}

	for _, id := range mutantIDs {
		if errs, ok := mutantErrors[id]; ok && len(errs) > 0 {
			result.perMutant[id] = fmt.Errorf("compilation failed (mutation detected):\n%s", strings.Join(errs, "\n"))
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
	tempDir     string
	testBinary  string
	pkgDir      string
	tests       []string
	timeout     time.Duration
	baseEnv     []string
	mutantEnv   []string
	log         *logger.Logger
	projectRoot string
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

	for i := 0; i < 2 && len(durations) < 2; i++ {
		start := time.Now()
		cmd := exec.CommandContext(ctx, e.testBinary, testArgs("10s", e.tests)...)
		cmd.Dir = e.tempDir
		_ = cmd.Run() // ignore pass/fail — we only need the elapsed time

		elapsed := time.Since(start)
		// A run that completes in <50ms is an immediate exit (binary missing,
		// crashed, or no tests at all) — not useful for timeout estimation.
		if elapsed >= 50*time.Millisecond {
			durations = append(durations, elapsed)
		}
	}

	if len(durations) == 0 {
		// Binary exited too fast (no tests to run, or binary doesn't exist).
		// Return false so callers fall back to defaultMutantTimeout.
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

	if _, err := os.Stat(e.testBinary); os.IsNotExist(err) {
		// Binary not produced after a successful compile — package has no test files.
		return mutantResult{
			id:     mutantID,
			status: "untested",
		}
	}

	args := testArgs(fmt.Sprintf("%.0fs", e.timeout.Seconds()), e.tests)
	cmd := exec.CommandContext(hardCtx, e.testBinary, args...)
	cmd.Dir = e.pkgDir

	cmdEnv := make([]string, len(e.mutantEnv))
	copy(cmdEnv, e.mutantEnv)
	cmdEnv[len(e.mutantEnv)-1] = "GORGON_MUTANT_ID=" + strconv.Itoa(mutantID)
	cmd.Env = cmdEnv

	start := time.Now()
	out, err := cmd.CombinedOutput()
	duration := time.Since(start)

	status := "survived"
	killedBy := ""
	killOutput := ""

	outStr := string(out)

	if exitErr, ok := err.(*exec.ExitError); ok {
		outStr += string(exitErr.Stderr)
	}

	isCompErr := isCompilationError(outStr)

	if isCompErr {
		status = "error"
		killedBy = extractErrorType(outStr)
		killOutput = outStr
	} else if err != nil && outStr == "" {
		status = "error"
		killedBy = "runtime error"
		killOutput = err.Error()
	} else if hasNoTestsToRun(outStr, err) {
		status = "survived"
		killedBy = ""
		killOutput = ""
	} else if err != nil {
		status = "killed"
		killedBy = parseFailedTest(outStr)
		if len(outStr) > 300 {
			killOutput = outStr[:300]
		} else {
			killOutput = outStr
		}
	}

	if e.log.IsDebug() {
		e.log.Debug("Mutant #%d %s (pkg: %s, timeout=%v, killed_by=%s, duration=%v)\n  Cmd: %s %v\n  Output: %s",
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
		if isCompilationError(output) {
			return extractErrorType(output)
		}
		return "(test output non-empty)"
	}
	return "(compilation/runtime error)"
}

func isCompilationError(output string) bool {
	outputLower := strings.ToLower(output)
	compPatterns := []string{
		"compilation failed",
		"build failed",
		"syntax error",
		"undefined:",
		"undefined (name",
		"cannot range over",
		"invalid operation:",
		"type *ast.",
		"has no field or method",
		"mismatched types",
		"cannot assign",
		"undefined label",
		"cannot use",
		"not declared",
		"redeclared",
		"no function",
		"non-type",
		"panic:",
		"runtime error:",
		"index out of range",
		"nil pointer",
		"invalid memory address",
	}
	for _, pattern := range compPatterns {
		if strings.Contains(outputLower, strings.ToLower(pattern)) {
			return true
		}
	}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "=== ") && !strings.HasPrefix(line, "---") {
			return true
		}
	}
	return false
}

func extractErrorType(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "compilation failed") {
			return extractFirstError(line)
		}
		if strings.Contains(line, "syntax error") {
			return "syntax error: " + extractFirstError(line)
		}
		if strings.Contains(line, "undefined:") {
			return "undefined: " + extractFirstError(line)
		}
		if strings.Contains(line, "invalid operation:") {
			return "invalid operation: " + extractFirstError(line)
		}
		if strings.Contains(line, "has no field or method") {
			return "field/method not found: " + extractFirstError(line)
		}
		if strings.Contains(line, "mismatched types") {
			return "type mismatch: " + extractFirstError(line)
		}
		if strings.Contains(line, "panic:") {
			return "panic: " + extractFirstError(line)
		}
		if strings.Contains(line, "runtime error:") {
			return "runtime error: " + extractFirstError(line)
		}
	}
	return extractFirstError(output)
}

func extractFirstError(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "---") && !strings.HasPrefix(line, "===") {
			if len(line) > 100 {
				return line[:100]
			}
			return line
		}
	}
	return "unknown error"
}

func hasNoTestsToRun(output string, runErr error) bool {
	if runErr == nil {
		return false
	}

	noTestPatterns := []string{
		"no test files",
		"no tests to run",
	}

	if exitErr, ok := runErr.(*exec.ExitError); ok {
		stderrLower := strings.ToLower(string(exitErr.Stderr))
		for _, pattern := range noTestPatterns {
			if strings.Contains(stderrLower, strings.ToLower(pattern)) {
				return true
			}
		}
	}

	return false
}

func (e *testExecutor) relPath() string {
	rel, _ := filepath.Rel(e.tempDir, e.pkgDir)
	if rel == "." {
		return ""
	}
	return "./" + filepath.ToSlash(rel)
}

func compileAndRunPackages(ctx context.Context, tempDir string, pkgToMutantIDs map[string][]int, pkgToMutants map[string][]*Mutant, mutantSites map[int]MutantSite, concurrent int, tests []string, prog *ProgressTracker, log *logger.Logger) ([]mutantResult, error) {
	resultsChan := make(chan mutantResult, sumMutantIDs(pkgToMutantIDs))
	testGroup, testCtx := errgroup.WithContext(ctx)
	testGroup.SetLimit(concurrent)

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
			executor := newTestExecutor(tempDir, pkgDir, tempDir, tests, log)
			pkgMuts := pkgToMutants[pkgDir]

			currentSites := rebuildMutantSites(pkgMuts)

			result := executor.compileWithAttribution(compileCtx, mutantIDsForPkg, currentSites)

			for _, mutantID := range mutantIDsForPkg {
				err := result.perMutant[mutantID]
				if err != nil {
					killedBy := "compilation error"
					if strings.Contains(err.Error(), "mutation detected") {
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
				untestedCount := 0
				for _, mutantID := range mutantIDsForPkg {
					if result.perMutant[mutantID] == nil {
						resultsChan <- mutantResult{id: mutantID, status: "untested"}
						if prog != nil {
							prog.Record()
						}
						untestedCount++
					}
				}
				if untestedCount > 0 {
					pkg := executor.relPath()
					if pkg == "" || pkg == "./" {
						pkg = filepath.Base(pkgDir)
					}
					executor.log.Info("No test binary for %s — package has no test files, %d mutant(s) marked untested", pkg, untestedCount)
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

func runStandalonePackage(pkgDir string, pkgMutants []*Mutant, concurrent int, tests []string, workerTempDir string, progbar bool, prog *ProgressTracker, log *logger.Logger) error {

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

	executor := newTestExecutor(tempDir, pkgTempDir, projectRoot, tests, log)

	mutantIDs := make([]int, len(pkgMutants))
	for i, m := range pkgMutants {
		mutantIDs[i] = m.ID
	}

	sites := rebuildMutantSites(pkgMutants)

	result := executor.compileWithAttribution(context.Background(), mutantIDs, sites)
	for _, m := range pkgMutants {
		err := result.perMutant[m.ID]
		if err != nil {
			m.Status = "error"
			if strings.Contains(err.Error(), "mutation detected") {
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

	// Check whether the test binary was actually produced. go test -c succeeds
	// silently for packages with no test files but writes no binary. Mark all
	// remaining (non-error) mutants as "untested" and log once per package.
	if _, statErr := os.Stat(executor.testBinary); os.IsNotExist(statErr) {
		pkgRelPath, _ := filepath.Rel(projectRoot, pkgDir)
		if pkgRelPath == "" || pkgRelPath == "." {
			pkgRelPath = filepath.Base(pkgDir)
		}
		fmt.Fprintf(os.Stderr, "[INFO] No test binary for ./%s — package has no test files, %d mutant(s) marked untested\n", pkgRelPath, len(testableIDs))
		for _, m := range pkgMutants {
			if m.Status == "" {
				m.Status = "untested"
			}
		}
		if prog != nil {
			for range testableIDs {
				prog.Record()
			}
		}
		return nil
	}

	baseline, baselineOK := executor.measureBaseline(context.Background())

	if baselineOK {
		_, _ = executor.timeoutFor(baseline)
	} else {
		// No meaningful baseline — package may have no test files or they
		// exit immediately. Use the default per-mutant timeout.
		executor.timeout = defaultMutantTimeout
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


func resolveSuitePaths(ctx context.Context, workspaceDir string, suite config.ExternalSuite, log *logger.Logger) ([]string, error) {
	var resolved []string
	for _, p := range suite.Paths {
		args := []string{"list", "-f", "{{.Dir}}"}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		args = append(args, p)

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
	log.Debug("[EXTERNAL] Building binaries for %d resolved paths", len(resolvedPaths))
	for _, relPkg := range resolvedPaths {
		safeName := strings.NewReplacer("/", "_", ".", "_").Replace(relPkg)
		binPath := filepath.Join(workspaceDir, safeName+".test")

		args := []string{"test", "-c", "-vet=off", "-o", binPath}
		if len(suite.Tags) > 0 {
			args = append(args, "-tags", strings.Join(suite.Tags, ","))
		}
		args = append(args, relPkg)

		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = workspaceDir
		log.Debug("[EXTERNAL] Building: go %v", args)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Debug("[EXTERNAL] Build failed: %s, error: %v", string(out), err)
			continue
		}

		if _, err := os.Stat(binPath); os.IsNotExist(err) {
			log.Debug("[EXTERNAL] Binary not created at %s", binPath)
			continue
		}
		log.Debug("[EXTERNAL] Binary created at %s", binPath)
		binaries[relPkg] = binPath
	}
	log.Debug("[EXTERNAL] Built %d binaries", len(binaries))
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

			cmd := exec.CommandContext(ctx, binPath,
				fmt.Sprintf("-test.timeout=%.0fs", timeout.Seconds()))
			cmd.Dir = workspaceDir
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("GORGON_MUTANT_ID=%d", m.ID))

			out, err := cmd.CombinedOutput()
			if err != nil {
				killedBy := parseFailedTest(string(out))
				if killedBy == "" {
					killedBy = suiteName
				} else {
					killedBy = killedBy + " [" + suiteName + "]"
				}
				resultsChan <- mutantResult{
					id:       m.ID,
					status:   "killed",
					killedBy: killedBy,
					killOutput: string(out),
				}
			} else {
				resultsChan <- mutantResult{id: m.ID, status: "survived"}
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
