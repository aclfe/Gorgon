package testing

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

var (
	lastTotalMu      sync.Mutex
	lastTotalMutants int
)

func GetTotalMutants() int {
	lastTotalMu.Lock()
	defer lastTotalMu.Unlock()
	return lastTotalMutants
}

func setTotalMutants(n int) {
	lastTotalMu.Lock()
	lastTotalMutants = n
	lastTotalMu.Unlock()
}

var statusRank = map[string]int{
	"":             0,
	StatusUntested: 1,
	StatusError:    2,
	StatusSurvived: 3,
	StatusTimeout:  4,
	StatusKilled:   5,
}

func mergeOutcome(m *Mutant, r mutantResult) bool {
	if IsTerminalPreRun(m.Status) {
		return false
	}
	if statusRank[r.status] <= statusRank[m.Status] {
		return false
	}
	m.Status = r.status
	m.Error = r.err
	m.KilledBy = r.killedBy
	m.KillDuration = r.killDuration
	m.KillOutput = r.killOutput
	if r.reason != "" {
		m.ErrorReason = r.reason
	}
	return true
}

func setStatus(m *Mutant, status, reason string) {
	m.Status = status
	m.ErrorReason = reason
	if m.KillOutput == "" {
		m.KillOutput = reason
	}
}

type run struct {
	all        []Mutant
	byID       map[int]*Mutant
	ws         *ModuleWorkspace
	buildTags  []string
	testsByPkg map[string][]string
	concurrent int
	log        *logger.Logger
	prog       *ProgressTracker

	fileMutants map[string][]*Mutant
	writeMu     sync.Mutex
}

func (r *run) pending() []*Mutant {
	var out []*Mutant
	for i := range r.all {
		if r.all[i].Status == "" {
			out = append(out, &r.all[i])
		}
	}
	return out
}

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, c *cache.Cache, testsByPkg map[string][]string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) ([]Mutant, error) {
	if log == nil {
		log = logger.New(false)
	}
	if concurrent < 1 {
		concurrent = 1
	}
	all := GenerateMutants(sites, operators, allOps, projectRoot, dirRules, resolver, log)
	setTotalMutants(len(all))
	if len(all) == 0 {
		return nil, nil
	}
	if progbar {
		log.Print("Generated %d mutants from sites", len(all))
	}

	var buildTags []string
	if cfg != nil {
		buildTags = cfg.BuildTags
	}

	r := &run{
		all:         all,
		byID:        make(map[int]*Mutant, len(all)),
		buildTags:   buildTags,
		testsByPkg:  testsByPkg,
		concurrent:  concurrent,
		log:         log,
		fileMutants: make(map[string][]*Mutant),
	}
	for i := range r.all {
		r.byID[r.all[i].ID] = &r.all[i]
	}

	if len(testPaths) > 0 {
		filterMutantsByTestPackages(r.all, testPaths)
	}

	r.preflight()

	if !unitTestsEnabled && !(externalCfg.Enabled && len(externalCfg.Suites) > 0) {
		for _, m := range r.pending() {
			setStatus(m, StatusUntested, "unit tests are disabled and no external suite is configured")
		}
		return r.all, nil
	}

	fp := newFingerprinter(projectRoot, buildTags, cacheConfigFingerprint(testsByPkg, unitTestsEnabled, externalCfg))
	if c != nil {
		r.applyCache(c, fp)
	}

	pending := r.pending()
	if len(pending) == 0 {
		r.saveCache(c, fp, baseDir)
		return r.all, nil
	}

	ws, err := NewModuleWorkspace()
	if err != nil {
		for _, m := range pending {
			setStatus(m, StatusError, "workspace creation failed: "+err.Error())
		}
		return r.all, fmt.Errorf("workspace creation failed: %w", err)
	}
	defer ws.Cleanup()
	r.ws = ws

	setupRoot := projectRoot
	if setupRoot == "" {
		setupRoot = baseDir
	}
	if err := ws.Setup(setupRoot, nil); err != nil {
		for _, m := range pending {
			setStatus(m, StatusError, "workspace setup failed: "+err.Error())
		}
		return r.all, fmt.Errorf("workspace setup failed: %w", err)
	}

	r.injectAll(pending)
	r.verifyBuild(ctx)

	if progbar {
		r.prog = NewProgressTracker(len(r.pending()))
	}

	beforeUnit := externalCfg.RunMode == "before_unit"
	runExternal := externalCfg.Enabled && len(externalCfg.Suites) > 0

	if runExternal && beforeUnit {
		r.runExternalSuites(ctx, externalCfg)
	}
	if unitTestsEnabled {
		log.Info("[UNIT] Running unit tests against %d mutant(s)", len(r.pending()))
		r.runUnitTests(ctx, beforeUnit && runExternal)
	}
	if runExternal && !beforeUnit {
		r.runExternalSuites(ctx, externalCfg)
	}
	if r.prog != nil {
		r.prog.Finish()
	}

	for _, m := range r.pending() {
		setStatus(m, StatusUntested, "no test run evaluated this mutant")
	}

	r.saveCache(c, fp, baseDir)
	return r.all, nil
}

func (r *run) preflight() {
	pending := r.pending()
	if len(pending) == 0 {
		return
	}
	vals := make([]Mutant, len(pending))
	for i, m := range pending {
		vals[i] = *m
	}
	valid, rejected := RunPreflightWithTags(vals, r.buildTags, r.log)
	for _, res := range rejected {
		m := r.byID[res.MutantID]
		if m == nil {
			continue
		}
		m.Status = res.Status
		m.Error = res.Error
		m.ErrorReason = res.ErrorReason
		m.KillOutput = res.ErrorReason
		if res.Status == StatusCompileError {
			m.KilledBy = "(compiler)"
		}
	}
	for _, v := range valid {
		if m := r.byID[v.ID]; m != nil {
			m.TempLine, m.TempCol = v.TempLine, v.TempCol
		}
	}
}

func (r *run) injectAll(pending []*Mutant) {
	byFile := make(map[string][]*Mutant)
	for _, m := range pending {
		if _, err := r.ws.relPath(m.Site.File.Name()); err != nil {
			setStatus(m, StatusError, "not run: "+err.Error())
			continue
		}
		byFile[m.Site.File.Name()] = append(byFile[m.Site.File.Name()], m)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	pkgDirs := make(map[string]bool)
	for _, f := range files {
		r.writeFile(f, byFile[f])
		if dir, err := r.ws.tempPkgDir(f); err == nil {
			pkgDirs[dir] = true
		}
	}
	for dir := range pkgDirs {
		if err := writeSchemataHelper(dir); err != nil {
			r.log.Warn("failed to write schemata helper in %s: %v", dir, err)
		}
	}
}

func (r *run) writeFile(srcFile string, mutants []*Mutant) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	dst, err := r.ws.tempPath(srcFile)
	if err != nil {
		for _, m := range mutants {
			setStatus(m, StatusError, "not run: "+err.Error())
		}
		return
	}
	var live []*Mutant
	for _, m := range mutants {
		if m.Status == "" {
			live = append(live, m)
		}
	}
	if len(live) == 0 {
		_ = r.ws.restoreOriginal(srcFile)
		r.fileMutants[srcFile] = nil
		return
	}
	res, err := WriteSchemataFile(srcFile, dst, live)
	if err != nil {
		for _, m := range live {
			setStatus(m, StatusCompileError, "schemata for this file could not be generated: "+err.Error())
			m.KilledBy = "(compiler)"
		}
		_ = r.ws.restoreOriginal(srcFile)
		r.fileMutants[srcFile] = nil
		return
	}
	var injected []*Mutant
	for _, m := range live {
		if reason, bad := res.NotApplied[m.ID]; bad || !res.Applied[m.ID] {
			if reason == "" {
				reason = "mutation was not injected"
			}
			setStatus(m, StatusInvalid, "not injectable: "+reason)
			continue
		}
		if pm, ok := res.Positions[m.ID]; ok {
			m.TempLine, m.TempCol = pm.TempLine, pm.TempCol
		}
		injected = append(injected, m)
	}
	r.fileMutants[srcFile] = injected
}

func writeSchemataHelper(dir string) error {
	pkgName := packageNameInDir(dir)
	helper := fmt.Sprintf(schemataHelperSrc, pkgName)
	return os.WriteFile(filepath.Join(dir, "gorgon_schemata.go"), []byte(helper), filePermissions)
}

func (r *run) verifyBuild(ctx context.Context) {
	const maxRounds = 12
	for round := 1; round <= maxRounds; round++ {
		pkgs := r.injectedPackages()
		if len(pkgs) == 0 {
			return
		}
		out, err := r.buildTestBinaries(ctx, pkgs)
		if err == nil {
			if round > 1 {
				r.log.Debug("[VERIFY] build clean after %d round(s)", round)
			}
			return
		}
		if ctx.Err() != nil {
			return
		}

		errs := ParseCompilerErrors(out)
		tempToSrc := make(map[string]string)
		for src := range r.fileMutants {
			if p, err := r.ws.tempPath(src); err == nil {
				tempToSrc[filepath.Clean(p)] = src
			}
		}

		bad := attributeErrors(r.ws.TempDir, errs, tempToSrc, r.fileMutants)
		if len(bad) > 0 {
			r.log.Debug("[VERIFY] round %d: %d mutant(s) attributed to compile errors", round, len(bad))
			r.rejectCompileErrors(bad)
			continue
		}

		failingSrc := make(map[string]bool)
		for _, ce := range errs {
			if src, ok := tempToSrc[absInWorkspace(r.ws.TempDir, ce.File)]; ok {
				failingSrc[src] = true
			}
		}
		if len(failingSrc) > 0 {
			found := 0
			for _, src := range sortedKeys(failingSrc) {
				culprits := r.bisectFile(ctx, src, r.fileMutants[src])
				for _, m := range culprits {
					bad[m.ID] = "compile error (identified by bisection)"
				}
				found += len(culprits)
			}
			if found > 0 {
				r.rejectCompileErrors(bad)
				continue
			}
		}

		failingPkgs := make(map[string]bool)
		for _, ce := range errs {
			failingPkgs[filepath.Dir(absInWorkspace(r.ws.TempDir, ce.File))] = true
		}
		reason := "not run: the package's tests do not build in the workspace even without the failing mutation: " + firstLines(out, 5)
		removed := 0
		for src, ms := range r.fileMutants {
			dir, err := r.ws.tempPkgDir(src)
			if err != nil {
				continue
			}
			if len(failingPkgs) > 0 && !failingPkgs[dir] {
				continue
			}
			for _, m := range ms {
				if m.Status == "" {
					setStatus(m, StatusError, reason)
					removed++
				}
			}
			r.writeFile(src, nil)
		}
		r.log.Warn("[VERIFY] build failure not attributable to any mutant; %d mutant(s) not run:\n%s", removed, firstLines(out, 10))
		if removed == 0 {
			return
		}
	}
	pkgs := r.injectedPackages()
	if out, err := r.buildTestBinaries(ctx, pkgs); err != nil {
		for src, ms := range r.fileMutants {
			for _, m := range ms {
				if m.Status == "" {
					setStatus(m, StatusError, "not run: package build did not converge: "+firstLines(out, 3))
				}
			}
			r.writeFile(src, nil)
		}
	}
}

func (r *run) rejectCompileErrors(bad map[int]string) {
	touched := make(map[string]bool)
	for id, msg := range bad {
		m := r.byID[id]
		if m == nil || m.Status != "" {
			continue
		}
		setStatus(m, StatusCompileError, msg)
		m.KilledBy = "(compiler)"
		m.Error = fmt.Errorf("compilation failed (mutation detected):\n%s", msg)
		touched[m.Site.File.Name()] = true
	}
	for src := range touched {
		r.writeFile(src, r.fileMutants[src])
	}
}

func (r *run) bisectFile(ctx context.Context, srcFile string, cands []*Mutant) []*Mutant {
	defer r.writeFile(srcFile, r.fileMutants[srcFile])
	pkgDir, err := r.ws.tempPkgDir(srcFile)
	if err != nil {
		return nil
	}
	tempFile, _ := r.ws.tempPath(srcFile)
	var rec func([]*Mutant) []*Mutant
	rec = func(set []*Mutant) []*Mutant {
		if len(set) == 0 || ctx.Err() != nil {
			return nil
		}
		dst := tempFile
		if _, err := WriteSchemataFile(srcFile, dst, set); err != nil {
			if len(set) == 1 {
				return set
			}
		} else {
			out, berr := r.buildTestBinaries(ctx, []string{pkgDir})
			if berr == nil || !mentionsFile(r.ws.TempDir, out, dst) {
				return nil
			}
			if len(set) == 1 {
				return set
			}
		}
		mid := len(set) / 2
		left := rec(set[:mid])
		right := rec(set[mid:])
		return append(left, right...)
	}
	return rec(cands)
}

func (r *run) injectedPackages() []string {
	set := make(map[string]bool)
	for src, ms := range r.fileMutants {
		live := false
		for _, m := range ms {
			if m.Status == "" {
				live = true
				break
			}
		}
		if !live {
			continue
		}
		if dir, err := r.ws.tempPkgDir(src); err == nil {
			set[dir] = true
		}
	}
	return sortedKeys(set)
}

func (r *run) buildTestBinaries(ctx context.Context, pkgDirs []string) (string, error) {
	args := []string{"test", "-count=1", "-run=^$", "-vet=off", "-gcflags=-e"}
	if truePath, err := exec.LookPath("true"); err == nil {
		args = append(args, "-exec="+truePath)
	}
	if len(r.buildTags) > 0 {
		args = append(args, "-tags", strings.Join(r.buildTags, ","))
	}
	for _, d := range pkgDirs {
		rel, err := filepath.Rel(r.ws.TempDir, d)
		if err != nil {
			continue
		}
		args = append(args, "./"+filepath.ToSlash(rel))
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = r.ws.TempDir
	cmd.Env = envWithMutant(0)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func mentionsFile(tempDir, buildOut, file string) bool {
	want := filepath.Clean(file)
	for _, ce := range ParseCompilerErrors(buildOut) {
		if absInWorkspace(tempDir, ce.File) == want {
			return true
		}
	}
	return false
}

func absInWorkspace(tempDir, f string) string {
	if !filepath.IsAbs(f) {
		f = filepath.Join(tempDir, f)
	}
	return filepath.Clean(f)
}

func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func filterMutantsByTestPackages(mutants []Mutant, testPaths []string) {
	coveredPackages := make(map[string]bool, len(testPaths))
	for _, testPath := range testPaths {
		absPath, err := filepath.Abs(testPath)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			coveredPackages[absPath] = true
			continue
		}
		coveredPackages[filepath.Dir(absPath)] = true
	}
	if len(coveredPackages) == 0 {
		return
	}

	for i := range mutants {
		m := &mutants[i]
		if m.Site.File == nil || m.Status != "" {
			continue
		}
		absMutantPath, err := filepath.Abs(m.Site.File.Name())
		if err != nil {
			continue
		}
		if !coveredPackages[filepath.Dir(absMutantPath)] {
			setStatus(m, StatusUntested, "package not covered by the configured tests")
		}
	}
}

// TestResolveSuitePaths exposes resolveSuitePaths for integration tests.
func TestResolveSuitePaths(ctx context.Context, workspaceDir string, suite config.ExternalSuite, log *logger.Logger) ([]string, error) {
	return resolveSuitePaths(ctx, workspaceDir, suite, log)
}

// Test helper for integration tests - calls GenerateAndRunSchemata with proper types
func TestGenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, c *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) ([]Mutant, error) {
	testsByPkg := make(map[string][]string)
	if len(tests) > 0 {
		testsByPkg[""] = tests
	}
	return GenerateAndRunSchemata(ctx, sites, operators, allOps, baseDir, projectRoot, dirRules, resolver, concurrent, c, testsByPkg, testPaths, log, progbar, unitTestsEnabled, externalCfg, cfg)
}
