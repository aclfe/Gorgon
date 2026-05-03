package testing

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

var lastTotalMutants int

func GetTotalMutants() int { return lastTotalMutants }

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) (result []Mutant, retErr error) {

	log.Debug("GenerateAndRunSchemata called with externalCfg.Enabled=%v, suites=%d", externalCfg.Enabled, len(externalCfg.Suites))

	mutants := GenerateMutants(sites, operators, allOps, projectRoot, dirRules, resolver, log)
	if len(mutants) == 0 {
		return nil, nil
	}

	if progbar {
		log.Print("Generated %d mutants from sites", len(mutants))
	}

	if len(testPaths) > 0 {
		filterMutantsByTestPackages(mutants, testPaths)
	} else if unitTestsEnabled {
		// Skip when unit tests are disabled — the "untested" label would be
		// misleading since external suites will cover all mutants.
		filterMutantsWithoutTests(mutants, baseDir)
	}

	// Run preflight validation: Level 1 (static) + Level 2 (type-check each mutant)
	validMutants, allInvalid := RunPreflight(mutants, log)

	// Collect invalid mutants with their status set, for inclusion in the final result.
	var invalidMutants []Mutant
	for _, r := range allInvalid {
		for i := range mutants {
			if mutants[i].ID == r.MutantID {
				mutants[i].Status = r.Status
				mutants[i].Error = r.Error
				mutants[i].KillOutput = r.ErrorReason
				invalidMutants = append(invalidMutants, mutants[i])
				break
			}
		}
	}

	// Total includes all mutants (valid + invalid) so reporter math always balances.
	lastTotalMutants = len(mutants)
	mutants = validMutants

	// Ensure invalid mutants are always appended to the returned slice so
	// computeStats can count them and Total == sum of all categories.
	defer func() {
		if len(invalidMutants) > 0 {
			result = append(result, invalidMutants...)
		}
	}()

	if len(mutants) == 0 {
		return nil, nil
	}

	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("cache resolution failed: %w", err))
		return mutants, err
	}
	log.Debug("After cache check: uncachedIndices nil=%v", uncachedIndices == nil)

	// If all cached and no external suites, return early
	if uncachedIndices == nil && !externalCfg.Enabled {
		log.Debug("All mutants cached and no external suites, returning early")
		return mutants, nil
	}

	// If all cached but external suites enabled, still need to run external phase
	if uncachedIndices == nil && externalCfg.Enabled {
		log.Debug("All mutants cached but external suites enabled, running external phase only")
		// Need workspace for external tests - find module root from baseDir
		baseDirAbs, _ := filepath.Abs(baseDir)
		goModDir := FindGoModDir(baseDirAbs)
		if goModDir == "" {
			log.Warn("[EXTERNAL] External suites require go.mod, skipping")
			return mutants, nil
		}

		ws, err := NewModuleWorkspace()
		if err != nil {
			return mutants, fmt.Errorf("workspace creation failed: %w", err)
		}
		defer ws.Cleanup()

		if err := ws.Setup(baseDir, mutants); err != nil {
			return mutants, fmt.Errorf("workspace setup failed: %w", err)
		}

		_ = MakeSelfContained(ws.TempDir)

		if _, _, err := ws.applySchemata(mutants, log); err != nil {
			return mutants, fmt.Errorf("schemata application failed: %w", err)
		}

		// Run external phase
		log.Debug("Before external phase check: enabled=%v, suites=%d", externalCfg.Enabled, len(externalCfg.Suites))
		if len(externalCfg.Suites) > 0 {
			log.Info("[EXTERNAL] Starting external suite phase with %d suites", len(externalCfg.Suites))
			allSuitePaths := collectAllSuitePaths(externalCfg.Suites)
			log.Info("[EXTERNAL] Collected %d suite paths", len(allSuitePaths))
			if copyErr := ws.copyExternalSuites(ws.absModule, allSuitePaths, log); copyErr != nil {
				log.Warn("external suite copy failed: %v", copyErr)
			} else {
				if err := runExternalPhase(ctx, ws, mutants, externalCfg, concurrent, log); err != nil {
					log.Warn("external suite phase failed: %v", err)
				}
			}
		}

		return mutants, nil
	}

	projectRootAbs, _ := filepath.Abs(projectRoot)

	// A go.work file at projectRoot is sufficient — each member module
	// has its own go.mod, but the workspace root itself may not.
	hasGoWork := fileExists(filepath.Join(projectRootAbs, "go.work"))
	hasGoMod := fileExists(filepath.Join(projectRootAbs, "go.mod"))

	log.Debug("Checking module layout at %s: go.work=%v go.mod=%v", projectRootAbs, hasGoWork, hasGoMod)

	if !hasGoWork && !hasGoMod {
		log.Debug("Neither go.work nor go.mod found, using standalone mode")
		return runStandalone(ctx, mutants, uncachedIndices, concurrent, cache, baseDir, tests, progbar, fileHashes, log)
	}
	log.Debug("Module layout detected, using workspace mode")

	ws, err := NewModuleWorkspace()
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("workspace creation failed: %w", err))
		return mutants, err
	}
	defer ws.Cleanup()

	if err := ws.Setup(projectRoot, mutants); err != nil {
		setMutantErrors(mutants, fmt.Errorf("workspace setup failed: %w", err))
		return mutants, err
	}

	_ = MakeSelfContained(ws.TempDir)

	_, hasNonStdlib, err := ws.applySchemata(mutants, log)
	if err != nil {
		log.Warn("CRITICAL: Schemata application failed: %v", err)
		setMutantErrors(mutants, fmt.Errorf("schemata application failed: %w", err))
		return mutants, fmt.Errorf("FATAL: schemata transformation produced invalid code: %w", err)
	}
	log.Debug("Schemata application completed successfully")

	//Verify the transformed code compiles with L4 retry logic
	log.Debug("Verifying schemata-transformed code compiles...")
	var removedByVerify []Mutant
	mutants, removedByVerify, err = verifyAndCleanSchemata(ctx, ws, mutants, log)
	// Track removed mutants so they appear in final counts (compile-error status already set).
	invalidMutants = append(invalidMutants, removedByVerify...)
	if err != nil {
		log.Warn("Schemata compilation failed, marking all mutants as errors to prevent OOM")
		setMutantErrors(mutants, err)
		// Force cleanup and return immediately to prevent retry loops
		ws.Cleanup()
		runtime.GC()
		return mutants, err
	}
	log.Debug("Schemata-transformed code compiles successfully")

	ws.simplifyGoMod(hasNonStdlib || externalCfg.Enabled)

	// Build external test binaries AFTER schemata is applied
	var suiteBinaries map[string]map[string]string
	if externalCfg.Enabled && len(externalCfg.Suites) > 0 {
		log.Info("[EXTERNAL] Copying external test suites to workspace")
		allSuitePaths := collectAllSuitePaths(externalCfg.Suites)
		if copyErr := ws.copyExternalSuites(ws.absModule, allSuitePaths, log); copyErr != nil {
			log.Warn("external suite copy failed: %v", copyErr)
		} else {
			var buildErr error
			suiteBinaries, buildErr = buildAllExternalSuiteBinaries(ctx, ws, externalCfg, log)
			if buildErr != nil {
				log.Warn("external suite binary build failed: %v", buildErr)
			}
		}
	}

	pkgToMutantIDs, mutantIDToIndex, err := ws.buildPkgMap(mutants)
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("build package map failed: %w", err))
		return mutants, err
	}

	// DEBUG: expose key mismatch between the two package maps
	log.Debug("[DEBUG-PKGMAP] buildPkgMap produced %d package keys:", len(pkgToMutantIDs))
	for k, ids := range pkgToMutantIDs {
		log.Debug("[DEBUG-PKGMAP] pkgToMutantIDs[%q] = %v", k, ids)
	}
	log.Debug("[DEBUG-PKGMAP] mutantIDToIndex: %v", mutantIDToIndex)

	// DEBUG: verify every mutant ID appears in mutantIDToIndex
	log.Debug("[DEBUG-INDEX] Checking mutantIDToIndex coverage:")
	for _, m := range mutants {
		if idx, ok := mutantIDToIndex[m.ID]; ok {
			log.Debug("[DEBUG-INDEX] mutant %d → index %d ✓", m.ID, idx)
		} else {
			log.Debug("[DEBUG-INDEX] mutant %d → NOT IN INDEX ✗ (will never be classified)", m.ID)
		}
	}

	mutantSites := make(map[int]MutantSite, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.File != nil {
			line := m.TempLine
			col := m.TempCol
			if line == 0 {
				line = m.Site.Line
			}
			if col == 0 {
				col = m.Site.Column
			}
			mutantSites[m.ID] = MutantSite{
				File: m.Site.File.Name(),
				Line: line,
				Col:  col,
			}
		}
	}

	pkgToMutants := make(map[string][]*Mutant, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.File == nil {
			continue
		}
		rel, err := ws.relPath(m.Site.File.Name())
		if err != nil {
			continue
		}
		pkgDir := filepath.Join(ws.TempDir, filepath.Dir(rel))
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], m)
	}

	// DEBUG: compare keys against pkgToMutantIDs above
	log.Debug("[DEBUG-PKGMAP] pkgToMutants has %d package keys:", len(pkgToMutants))
	for k, ms := range pkgToMutants {
		ids := make([]int, len(ms))
		for i, m := range ms {
			ids[i] = m.ID
		}
		log.Debug("[DEBUG-PKGMAP] pkgToMutants[%q] = mutant IDs %v", k, ids)
	}

	var prog *ProgressTracker
	if progbar {
		prog = NewProgressTracker(len(mutants))
	}

	runUnitTests := unitTestsEnabled

	// ── Phase 1 (before_unit only): External runs BEFORE unit tests ──────────
	var preExternalDone bool
	if externalCfg.Enabled && externalCfg.RunMode == "before_unit" && len(suiteBinaries) > 0 {
		log.Info("[EXTERNAL] Running external suites before unit tests (%d suites)", len(externalCfg.Suites))
		if err := runExternalPhaseWithBinaries(ctx, ws, mutants, externalCfg, suiteBinaries, concurrent, log); err != nil {
			log.Warn("external suite phase (before_unit) failed: %v", err)
		}
		preExternalDone = true
		pkgToMutantIDs = filterKilledFromPkgMap(pkgToMutantIDs, mutants, mutantIDToIndex)
	}

	var results []mutantResult
	if runUnitTests {
		var err error
		results, err = compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, pkgToMutants, mutantSites, concurrent, tests, prog, log)

		if len(results) > 0 {
			collectResults(mutants, results, mutantIDToIndex, ws.TempDir)
		}

		// DEBUG: show what collectResults actually matched
		log.Debug("[DEBUG-COLLECT] After collectResults, raw result IDs and statuses:")
		for _, r := range results {
			log.Debug("[DEBUG-COLLECT] result id=%d status=%q", r.id, r.status)
		}
		log.Debug("[DEBUG-COLLECT] Mutant statuses after collection:")
		for _, m := range mutants {
			log.Debug("[DEBUG-COLLECT] mutant id=%d status=%q", m.ID, m.Status)
		}

		if err != nil {
			SaveCache(mutants, baseDir, cache, fileHashes)
			return mutants, err
		}
	}

	log.Debug("After unit tests, about to check external phase")
	// ── Phase 2: External Suites (default: run after unit tests) ─────────────
	if !preExternalDone && externalCfg.Enabled && len(externalCfg.Suites) > 0 && len(suiteBinaries) > 0 {
		log.Info("[EXTERNAL] Running external suite phase with %d suites", len(externalCfg.Suites))
		if err := runExternalPhaseWithBinaries(ctx, ws, mutants, externalCfg, suiteBinaries, concurrent, log); err != nil {
			log.Warn("external suite phase failed: %v", err)
		}
	} else {
		log.Debug("[EXTERNAL] Skipping external phase: enabled=%v, preRun=%v, suites=%d, binaries=%d",
			externalCfg.Enabled, preExternalDone, len(externalCfg.Suites), len(suiteBinaries))
	}

	// Any mutant that passed preflight and schemata verification but never
	// received an execution result (e.g. its package path couldn't be resolved)
	// is marked untested so Total always equals the sum of all categories.
	for i := range mutants {
		if mutants[i].Status == "" {
			mutants[i].Status = StatusUntested
		}
	}

	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

func runStandalone(ctx context.Context, mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, progbar bool, fileHashes map[string]string, log *logger.Logger) ([]Mutant, error) {

	pkgToMutants := make(map[string][]*Mutant, len(uncachedIndices))
	for _, idx := range uncachedIndices {
		m := &mutants[idx]
		pkgDir := filepath.Dir(m.Site.File.Name())
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], m)
	}

	totalMutants := len(uncachedIndices)
	var prog *ProgressTracker
	if progbar {
		prog = NewProgressTracker(totalMutants)
	}

	pkgDirs := make([]string, 0, len(pkgToMutants))
	for pkgDir := range pkgToMutants {
		pkgDirs = append(pkgDirs, pkgDir)
	}
	sort.Strings(pkgDirs)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrent)

	parentTempDir, err := os.MkdirTemp("", "gorgon-standalone-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create parent temp dir: %w", err)
	}
	defer os.RemoveAll(parentTempDir)

	for i, pkgDir := range pkgDirs {
		pkgMutants := pkgToMutants[pkgDir]
		pkgDir := pkgDir

		workerTempDir := filepath.Join(parentTempDir, fmt.Sprintf("pkg-%d", i))
		if err := os.MkdirAll(workerTempDir, 0o755); err != nil {
			return nil, fmt.Errorf("failed to create package temp dir: %w", err)
		}

		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return runStandalonePackage(ctx, pkgDir, pkgMutants, concurrent, tests, workerTempDir, progbar, prog, log)
		})
	}

	if err := g.Wait(); err != nil {
		if prog != nil {
			prog.Finish()
		}
		return nil, err
	}

	if prog != nil {
		prog.Finish()
	}

	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

func MakeSelfContained(tempDir string) error {
	goModPath := filepath.Join(tempDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil && !os.IsNotExist(err) {
		return errors.New("read go.mod")
	}

	content := string(data)
	if os.IsNotExist(err) {
		content = "module " + benchModuleName + "\ngo " + goVersion + "\n"
	} else if !strings.Contains(content, "replace github.com/aclfe/gorgon =>") {
		content = strings.TrimSpace(content) + "\n\nreplace github.com/aclfe/gorgon => ./\n"
	}

	if err := os.WriteFile(goModPath, []byte(content), filePermissions); err != nil {
		return errors.New("write go.mod")
	}
	return nil
}

func RewriteImports(_ string) error {
	return nil
}

func isStdlib(path string) bool {
	if path == "" || path[0] == '.' {
		return false
	}
	dot := strings.IndexByte(path, '.')
	slash := strings.IndexByte(path, '/')
	return dot < 0 || (slash >= 0 && slash < dot)
}

func setMutantErrors(mutants []Mutant, err error) {
	for i := range mutants {
		mutants[i].Status = "error"
		mutants[i].Error = err
	}
}

func collectResults(mutants []Mutant, results []mutantResult, mutantIDToIndex map[int]int, tempDir string) {
	for _, result := range results {
		idx, ok := mutantIDToIndex[result.id]
		if !ok {
			continue
		}
		if mutants[idx].Status == "survived" {
			continue
		}
		mutants[idx].Status = result.status
		mutants[idx].Error = result.err
		mutants[idx].TempDir = tempDir
		mutants[idx].KilledBy = result.killedBy
		mutants[idx].KillDuration = result.killDuration
		mutants[idx].KillOutput = result.killOutput
	}
}

func filterMutantsByTestPackages(mutants []Mutant, testPaths []string) {
	coveredPackages := make(map[string]bool, len(testPaths))
	for _, testPath := range testPaths {
		absPath, err := filepath.Abs(testPath)
		if err != nil {
			continue
		}
		coveredPackages[filepath.Dir(absPath)] = true
	}
	if len(coveredPackages) == 0 {
		return
	}

	for i := range mutants {
		m := &mutants[i]
		if m.Site.File == nil {
			continue
		}
		absMutantPath, err := filepath.Abs(m.Site.File.Name())
		if err != nil {
			continue
		}
		mutantPkg := filepath.Dir(absMutantPath)
		if !coveredPackages[mutantPkg] {
			m.Status = "untested"
		}
	}
}

func filterMutantsWithoutTests(mutants []Mutant, baseDir string) {
	absBase, _ := filepath.Abs(baseDir)
	testPackages := collectPackagesWithTests(absBase)

	if len(testPackages) == 0 {
		return
	}

	for i := range mutants {
		m := &mutants[i]
		if m.Site.File == nil {
			continue
		}

		mutantFile := m.Site.File.Name()
		absMutantPath, err := filepath.Abs(mutantFile)
		if err != nil {
			continue
		}

		mutantDir := filepath.Dir(absMutantPath)
		relDir, err := filepath.Rel(absBase, mutantDir)
		if err != nil {
			continue
		}

		if !testPackages[relDir] {
			m.Status = "untested"
		}
	}
}

// collectPackagesWithTests collects all packages that have test files
func collectPackagesWithTests(absModule string) map[string]bool {
	pkgs := make(map[string]bool)

	filepath.Walk(absModule, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == ".git" || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, "_test.go") {
			dir := filepath.Dir(path)
			relDir, err := filepath.Rel(absModule, dir)
			if err != nil {
				return nil
			}
			pkgs[relDir] = true
		}
		return nil
	})

	return pkgs
}

// buildExternalBinariesFromSource builds test binaries from the original source directory
func buildExternalBinariesFromSource(ctx context.Context, sourceRoot string, cfg config.ExternalSuitesConfig, log *logger.Logger) (map[string]map[string]string, error) {
	allBinaries := make(map[string]map[string]string)

	for _, suite := range cfg.Suites {
		resolvedPaths, err := resolveSuitePaths(ctx, sourceRoot, suite, log)
		if err != nil || len(resolvedPaths) == 0 {
			log.Warn("[EXTERNAL] No packages found for suite %q: %v", suite.Name, err)
			continue
		}

		binaries, err := buildExternalSuiteBinaries(ctx, sourceRoot, suite, resolvedPaths, log)
		if err != nil {
			log.Warn("[EXTERNAL] Build failed for suite %q: %v", suite.Name, err)
			continue
		}

		if len(binaries) > 0 {
			allBinaries[suite.Name] = binaries
			log.Info("[EXTERNAL] Built %d binaries for suite %q", len(binaries), suite.Name)
		}
	}

	return allBinaries, nil
}

// buildAllExternalSuiteBinaries builds test binaries for all external suites upfront
// before any mutations are applied. Returns a map of suite name -> binaries.
func buildAllExternalSuiteBinaries(ctx context.Context, ws *ModuleWorkspace, cfg config.ExternalSuitesConfig, log *logger.Logger) (map[string]map[string]string, error) {
	allBinaries := make(map[string]map[string]string)

	for _, suite := range cfg.Suites {
		resolvedPaths, err := resolveSuitePaths(ctx, ws.absModule, suite, log)
		if err != nil || len(resolvedPaths) == 0 {
			log.Warn("[EXTERNAL] No packages found for suite %q: %v", suite.Name, err)
			continue
		}

		binaries, err := buildExternalSuiteBinaries(ctx, ws.absModule, suite, resolvedPaths, log)
		if err != nil {
			log.Warn("[EXTERNAL] Build failed for suite %q: %v", suite.Name, err)
			continue
		}

		if len(binaries) > 0 {
			allBinaries[suite.Name] = binaries
			log.Info("[EXTERNAL] Built %d binaries for suite %q", len(binaries), suite.Name)
		}
	}

	return allBinaries, nil
}

// runExternalPhaseWithBinaries runs mutations against pre-built external test binaries
func runExternalPhaseWithBinaries(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, cfg config.ExternalSuitesConfig, suiteBinaries map[string]map[string]string, concurrent int, log *logger.Logger) error {
	// Build ID→index map once
	idToIdx := make(map[int]int, len(mutants))
	for i := range mutants {
		idToIdx[mutants[i].ID] = i
	}

	// before_unit: this is the pre-unit pass — test all mutants.
	// default: run after unit tests — only test what wasn't killed.
	var targets []*Mutant
	if cfg.RunMode == "before_unit" {
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	} else {
		for i := range mutants {
			s := mutants[i].Status
			if s == "survived" || s == "" || s == StatusUntested {
				targets = append(targets, &mutants[i])
			}
		}
	}

	if len(targets) == 0 {
		log.Info("[EXTERNAL] No mutants left to test against external suites")
		return nil
	}

	log.Info("[EXTERNAL] Running %d mutants against external suites", len(targets))

	for _, suite := range cfg.Suites {
		binaries, ok := suiteBinaries[suite.Name]
		if !ok || len(binaries) == 0 {
			log.Warn("[EXTERNAL] No binaries available for suite %q", suite.Name)
			continue
		}

		for _, binPath := range binaries {
			var stillAlive []*Mutant
			for _, m := range targets {
				if m.Status != "killed" {
					stillAlive = append(stillAlive, m)
				}
			}
			if len(stillAlive) == 0 {
				break
			}

			results := runMutantsAgainstBinary(ctx, binPath, ws.TempDir, stillAlive, 30*time.Second, concurrent, suite.Name)
			for _, r := range results {
				idx, ok := idToIdx[r.id]
				if !ok {
					continue
				}
				switch r.status {
				case StatusKilled:
					mutants[idx].Status = StatusKilled
					mutants[idx].KilledBy = r.killedBy
					mutants[idx].KillOutput = r.killOutput
				case StatusSurvived:
					// Mark as survived if not already conclusively classified so
					// mutants don't fall through to "untested" in the final sweep.
					if s := mutants[idx].Status; s == "" || s == StatusUntested {
						mutants[idx].Status = StatusSurvived
					}
				}
			}
		}
	}

	return nil
}

func runExternalPhase(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, cfg config.ExternalSuitesConfig, concurrent int, log *logger.Logger) error {
	idToIdx := make(map[int]int, len(mutants))
	for i := range mutants {
		idToIdx[mutants[i].ID] = i
	}

	var targets []*Mutant
	if cfg.RunMode == "before_unit" {
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	} else {
		for i := range mutants {
			s := mutants[i].Status
			if s == "survived" || s == "" || s == StatusUntested {
				targets = append(targets, &mutants[i])
			}
		}
	}

	if len(targets) == 0 {
		log.Info("[EXTERNAL] No mutants left to test against external suites")
		return nil
	}

	log.Info("[EXTERNAL] Running %d mutants against external suites", len(targets))

	for _, suite := range cfg.Suites {
		resolvedPaths, err := resolveSuitePaths(ctx, ws.absModule, suite, log)
		if err != nil || len(resolvedPaths) == 0 {
			log.Warn("[EXTERNAL] No packages found for suite %q: %v", suite.Name, err)
			continue
		}

		binaries, err := buildExternalSuiteBinaries(ctx, ws.absModule, suite, resolvedPaths, log)
		if err != nil {
			log.Warn("[EXTERNAL] Build failed for suite %q: %v", suite.Name, err)
			continue
		}

		for _, binPath := range binaries {
			var stillAlive []*Mutant
			for _, m := range targets {
				if m.Status != "killed" {
					stillAlive = append(stillAlive, m)
				}
			}
			if len(stillAlive) == 0 {
				break
			}

			results := runMutantsAgainstBinary(ctx, binPath, ws.TempDir, stillAlive, 30*time.Second, concurrent, suite.Name)
			for _, r := range results {
				idx, ok := idToIdx[r.id]
				if !ok {
					continue
				}
				switch r.status {
				case StatusKilled:
					mutants[idx].Status = StatusKilled
					mutants[idx].KilledBy = r.killedBy
					mutants[idx].KillOutput = r.killOutput
				case StatusSurvived:
					if s := mutants[idx].Status; s == "" || s == StatusUntested {
						mutants[idx].Status = StatusSurvived
					}
				}
			}
		}
	}
	return nil
}

// verifyBuildSequential builds all packages at once.
// Returns combined error output and error if any package fails.
func verifyBuildSequential(ctx context.Context, tempDir string, log *logger.Logger) (string, error) {
	// -gcflags=all=-e disables the default 10-error-per-package truncation so
	// all bad mutant IDs can be extracted in a single round.
	cmd := exec.CommandContext(ctx, "go", "build", "-gcflags=all=-e", "./...")
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("build failed")
	}
	return "", nil
}

// TestVerifyBuildSequential exposes verifyBuildSequential for integration tests.
func TestVerifyBuildSequential(ctx context.Context, tempDir string, log *logger.Logger) (string, error) {
	return verifyBuildSequential(ctx, tempDir, log)
}

// verifyAndCleanSchemata runs build verification with retry loop that removes bad mutants.
// Returns kept mutants, removed mutants (with compile-error status set), and any error.
// Removed mutants are always returned so callers can include them in final counts.
func verifyAndCleanSchemata(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, log *logger.Logger) (kept []Mutant, removed []Mutant, err error) {
	const maxRounds = 5

	verifyCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for round := 0; round < maxRounds; round++ {
		buildOut, buildErr := verifyBuildSequential(verifyCtx, ws.TempDir, log)
		if buildErr == nil {
			if round > 0 {
				log.Debug("[VERIFY] Build clean after %d removal round(s)", round)
			}
			return mutants, removed, nil
		}

		log.Debug("[VERIFY] Round %d: build failed, scanning for bad mutant IDs", round+1)

		badIDs := extractMutantIDsFromBuildErrors(ws.TempDir, buildOut)
		if len(badIDs) == 0 {
			for i := range mutants {
				if mutants[i].Status == "" {
					mutants[i].Status = StatusError
					mutants[i].KilledBy = "(compiler)"
					mutants[i].KillOutput = fmt.Sprintf("build verification failed (round %d): unidentifiable", round+1)
				}
			}
			removed = append(removed, mutants...)
			return nil, removed, fmt.Errorf("schemata build failed (round %d), bad mutants unidentifiable:\n%s", round+1, buildOut)
		}

		log.Debug("[VERIFY] Round %d: removing %d bad mutant(s): %v", round+1, len(badIDs), badIDs)

		badSet := make(map[int]bool, len(badIDs))
		for _, id := range badIDs {
			badSet[id] = true
		}

		log.Debug("[DEBUG-VERIFY] badSet IDs to remove: %v", badIDs)
		log.Debug("[DEBUG-VERIFY] mutant IDs currently in slice:")
		for _, m := range mutants {
			log.Debug("[DEBUG-VERIFY] mutant ID=%d status=%q — in badSet: %v", m.ID, m.Status, badSet[m.ID])
		}

		var keptThisRound []Mutant
		for i := range mutants {
			if badSet[mutants[i].ID] {
				mutants[i].Status = StatusError
				mutants[i].KilledBy = "(compiler)"
				mutants[i].KillOutput = fmt.Sprintf("build verification: removed in round %d", round+1)
				removed = append(removed, mutants[i])
			} else {
				keptThisRound = append(keptThisRound, mutants[i])
			}
		}

		if len(keptThisRound) == len(mutants) {
			for i := range keptThisRound {
				if keptThisRound[i].Status == "" {
					keptThisRound[i].Status = StatusError
					keptThisRound[i].KilledBy = "(compiler)"
					keptThisRound[i].KillOutput = fmt.Sprintf("build verification (round %d): bad IDs identified but none matched", round+1)
				}
			}
			removed = append(removed, keptThisRound...)
			return nil, removed, fmt.Errorf("build verification: bad mutants identified but none removed, aborting")
		}

		// Re-apply schemata to the affected files.
		if err := reapplyAffectedFiles(ws, badSet, mutants, keptThisRound, log); err != nil {
			for i := range keptThisRound {
				if keptThisRound[i].Status == "" {
					keptThisRound[i].Status = StatusError
					keptThisRound[i].KilledBy = "(compiler)"
					keptThisRound[i].KillOutput = fmt.Sprintf("build verification (round %d): re-apply failed", round+1)
				}
			}
			removed = append(removed, keptThisRound...)
			return nil, removed, fmt.Errorf("re-apply after round %d: %w", round+1, err)
		}
		mutants = keptThisRound
	}

	if _, finalErr := verifyBuildSequential(verifyCtx, ws.TempDir, log); finalErr != nil {
		log.Warn("[VERIFY] Still failing after %d rounds — proceeding with remaining mutants", maxRounds)
	}
	return mutants, removed, nil
}

// extractMutantIDsFromBuildErrors scans temp files near error lines for activeMutantID patterns.
func extractMutantIDsFromBuildErrors(tempDir, buildOutput string) []int {
	errors := ParseCompilerErrors(buildOutput)
	seen := make(map[int]bool)
	var ids []int

	prefix := []byte("activeMutantID == ")

	for _, ce := range errors {
		filePath := ce.File
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(tempDir, filePath)
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")

		// Scan only the exact error line (±1 for off-by-one safety).
		lo := ce.Line - 2
		if lo < 0 {
			lo = 0
		}
		hi := ce.Line + 1
		if hi > len(lines) {
			hi = len(lines)
		}

		for _, line := range lines[lo:hi] {
			idx := strings.Index(line, string(prefix))
			if idx < 0 {
				continue
			}
			rest := line[idx+len(prefix):]
			end := 0
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if end == 0 {
				continue
			}
			var id int
			if _, err := fmt.Sscanf(rest[:end], "%d", &id); err != nil || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// reapplyAffectedFiles re-applies schemata to files that had bad mutants removed.
func reapplyAffectedFiles(ws *ModuleWorkspace, removed map[int]bool, allMutants []Mutant, kept []Mutant, log *logger.Logger) error {
	// Find which source files contained removed mutants.
	affected := make(map[string][]*Mutant) // origPath → kept mutants for that file

	// Determine affected files by looking at what was REMOVED.
	for i := range allMutants {
		m := &allMutants[i]
		if !removed[m.ID] || m.Site.File == nil {
			continue
		}
		p := m.Site.File.Name()
		if _, ok := affected[p]; !ok {
			affected[p] = nil // ensure the file is in the map even if no kept mutants
		}
	}

	// Populate kept mutants per affected file.
	for i := range kept {
		m := &kept[i]
		if m.Site.File == nil {
			continue
		}
		p := m.Site.File.Name()
		if _, ok := affected[p]; ok {
			affected[p] = append(affected[p], m)
		}
	}

	for origPath, fileMutants := range affected {
		rel, err := ws.relPath(origPath)
		if err != nil {
			continue
		}
		tempPath := filepath.Join(ws.TempDir, rel)

		// Restore original source.
		src, err := os.ReadFile(origPath)
		if err != nil {
			return fmt.Errorf("re-read %s: %w", origPath, err)
		}
		if err := os.WriteFile(tempPath, src, filePermissions); err != nil {
			return fmt.Errorf("restore %s: %w", tempPath, err)
		}

		if len(fileMutants) == 0 {
			continue
		}

		// Re-parse a fresh AST from original source — never reuse Site.FileAST
		// which has already been mutated in-place by the first applySchemata pass.
		fset := token.NewFileSet()
		freshAST, err := parser.ParseFile(fset, origPath, src, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("re-parse %s: %w", origPath, err)
		}
		for _, m := range fileMutants {
			m.Site.Fset = fset
		}

		log.Debug("[VERIFY] Re-applying schemata to %s with %d mutant(s)", filepath.Base(origPath), len(fileMutants))
		posMap, err := ApplySchemataToAST(freshAST, fset, tempPath, src, fileMutants)
		if err != nil {
			return fmt.Errorf("re-apply schemata %s: %w", origPath, err)
		}

		for _, m := range fileMutants {
			if pm, ok := posMap[m.ID]; ok {
				m.TempLine = pm.TempLine
				m.TempCol = pm.TempCol
			}
		}
	}

	// Re-inject helpers only for affected packages.
	tempFileToMutants := make(map[string][]*Mutant)
	for origPath, fileMutants := range affected {
		rel, _ := ws.relPath(origPath)
		tempFileToMutants[filepath.Join(ws.TempDir, rel)] = fileMutants
	}
	return InjectSchemataHelpers(tempFileToMutants, log)
}

// filterKilledFromPkgMap returns a copy of pkgToMutantIDs with already-killed mutant IDs
// removed. Used in before_unit mode to skip unit tests for mutations caught by external.
func filterKilledFromPkgMap(pkgToMutantIDs map[string][]int, mutants []Mutant, idToIdx map[int]int) map[string][]int {
	filtered := make(map[string][]int, len(pkgToMutantIDs))
	for pkg, ids := range pkgToMutantIDs {
		var kept []int
		for _, id := range ids {
			if idx, ok := idToIdx[id]; ok && mutants[idx].Status == StatusKilled {
				continue
			}
			kept = append(kept, id)
		}
		if len(kept) > 0 {
			filtered[pkg] = kept
		}
	}
	return filtered
}

// Test helper for integration tests - calls GenerateAndRunSchemata with proper types
func TestGenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) ([]Mutant, error) {
	return GenerateAndRunSchemata(ctx, sites, operators, allOps, baseDir, projectRoot, dirRules, resolver, concurrent, cache, tests, testPaths, log, progbar, unitTestsEnabled, externalCfg, cfg)
}

// Test helper for extractMutantIDsFromBuildErrors
func TestExtractMutantIDsFromBuildErrors(tempDir, buildOutput string) []int {
	return extractMutantIDsFromBuildErrors(tempDir, buildOutput)
}

// Test helper for integration tests - calls runExternalPhase
func TestRunExternalPhase(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, cfg config.ExternalSuitesConfig, concurrent int, log *logger.Logger) error {
	return runExternalPhase(ctx, ws, mutants, cfg, concurrent, log)
}

// Test helper for integration tests - calls collectPackagesWithTests
func TestCollectPackagesWithTests(absModule string) map[string]bool {
	return collectPackagesWithTests(absModule)
}

// TestApplySchemataToWorkspace applies the schemata transformation and returns
// (tempDir, error) so integration tests can inspect the generated code structure.
func TestApplySchemataToWorkspace(ws *ModuleWorkspace, mutants []Mutant, log *logger.Logger) (string, error) {
	log.Debug("[DEBUG-IDS] IDs entering applySchemata: %v", func() []int {
		ids := make([]int, len(mutants))
		for i, m := range mutants {
			ids[i] = m.ID
		}
		return ids
	}())
	_, _, err := ws.applySchemata(mutants, log)
	log.Debug("[DEBUG-IDS] IDs after applySchemata: %v", func() []int {
		ids := make([]int, len(mutants))
		for i, m := range mutants {
			ids[i] = m.ID
		}
		return ids
	}())
	return ws.TempDir, err
}

// TestVerifyAndCleanSchemata exposes the internal build-verify/retry loop so
// integration tests can exercise it directly without going through the full pipeline.
func TestVerifyAndCleanSchemata(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, log *logger.Logger) ([]Mutant, []Mutant, error) {
	return verifyAndCleanSchemata(ctx, ws, mutants, log)
}

// TestWorkspaceRelPath exposes the internal relPath method so integration tests
// can verify that files outside the module root are correctly rejected and never
// produce "../"-escaping paths that could write outside TempDir.
func TestWorkspaceRelPath(ws *ModuleWorkspace, filePath string) (string, error) {
	return ws.relPath(filePath)
}

// TestResolveSuitePaths exposes resolveSuitePaths for integration tests.
func TestResolveSuitePaths(ctx context.Context, workspaceDir string, suite config.ExternalSuite, log *logger.Logger) ([]string, error) {
	return resolveSuitePaths(ctx, workspaceDir, suite, log)
}