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

// finalizeMutants ensures all mutants have a status before returning
func finalizeMutants(mutants []Mutant) {
	for i := range mutants {
		if mutants[i].Status == "" {
			mutants[i].Status = StatusUntested
		}
	}
}

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, testsByPkg map[string][]string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) (result []Mutant, retErr error) {

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
		filterMutantsWithoutTests(mutants, projectRoot)
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
				if r.Status == StatusCompileError {
					mutants[i].KilledBy = "(compiler)"
				}
				invalidMutants = append(invalidMutants, mutants[i])
				break
			}
		}
	}

	// Total includes all mutants (valid + invalid) so reporter math always balances.
	lastTotalMutants = len(mutants)
	mutants = validMutants

	if len(mutants) == 0 {
		return invalidMutants, nil
	}

	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("cache resolution failed: %w", err))
		return append(mutants, invalidMutants...), err
	}
	log.Debug("After cache check: uncachedIndices nil=%v", uncachedIndices == nil)

	// If all cached and no external suites, return early
	if uncachedIndices == nil && !externalCfg.Enabled {
		log.Debug("All mutants cached and no external suites, returning early")
		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), nil
	}

	// If all cached but external suites enabled, still need to run external phase
	if uncachedIndices == nil && externalCfg.Enabled {
		log.Debug("All mutants cached but external suites enabled, running external phase only")
		// Need workspace for external tests - find module root from baseDir
		baseDirAbs, _ := filepath.Abs(baseDir)
		goModDir := FindGoModDir(baseDirAbs)
		if goModDir == "" {
			log.Warn("[EXTERNAL] External suites require go.mod, skipping")
			finalizeMutants(mutants)
			return append(mutants, invalidMutants...), nil
		}

		ws, err := NewModuleWorkspace()
		if err != nil {
			finalizeMutants(mutants)
			return append(mutants, invalidMutants...), fmt.Errorf("workspace creation failed: %w", err)
		}
		defer ws.Cleanup()

		if err := ws.Setup(baseDir, mutants); err != nil {
			finalizeMutants(mutants)
			return append(mutants, invalidMutants...), fmt.Errorf("workspace setup failed: %w", err)
		}

		_ = MakeSelfContained(ws.TempDir)

		if _, _, err := ws.applySchemata(mutants, log); err != nil {
			finalizeMutants(mutants)
			return append(mutants, invalidMutants...), fmt.Errorf("schemata application failed: %w", err)
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

		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), nil
	}

	projectRootAbs, _ := filepath.Abs(projectRoot)

	// A go.work file at projectRoot is sufficient — each member module
	// has its own go.mod, but the workspace root itself may not.
	hasGoWork := fileExists(filepath.Join(projectRootAbs, "go.work"))
	hasGoMod := fileExists(filepath.Join(projectRootAbs, "go.mod"))

	log.Debug("Checking module layout at %s: go.work=%v go.mod=%v", projectRootAbs, hasGoWork, hasGoMod)

	if !hasGoWork && !hasGoMod {
		log.Debug("Neither go.work nor go.mod found, using standalone mode")
		var bt []string
		if cfg != nil {
			bt = cfg.BuildTags
		}
		return runStandalone(ctx, mutants, uncachedIndices, concurrent, cache, baseDir, testsByPkg, progbar, bt, fileHashes, log)
	}
	log.Debug("Module layout detected, using workspace mode")

	ws, err := NewModuleWorkspace()
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("workspace creation failed: %w", err))
		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), err
	}
	defer ws.Cleanup()

	if err := ws.Setup(projectRoot, mutants); err != nil {
		setMutantErrors(mutants, fmt.Errorf("workspace setup failed: %w", err))
		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), err
	}

	_ = MakeSelfContained(ws.TempDir)

	_, hasNonStdlib, err := ws.applySchemata(mutants, log)
	if err != nil {
		log.Warn("CRITICAL: Schemata application failed: %v", err)
		setMutantErrors(mutants, fmt.Errorf("schemata application failed: %w", err))
		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), fmt.Errorf("FATAL: schemata transformation produced invalid code: %w", err)
	}
	log.Debug("Schemata application completed successfully")

	//Verify the transformed code compiles with L4 retry logic
	log.Debug("Verifying schemata-transformed code compiles...")
	var removedByVerify []Mutant
	mutants, removedByVerify, err = verifyAndCleanSchemata(ctx, ws, mutants, log)
	// Track removed mutants so they appear in final counts (compile-error status already set).
	invalidMutants = append(invalidMutants, removedByVerify...)
	if err != nil {
		if len(mutants) == 0 {
			log.Warn("Schemata compilation failed with no recoverable mutants")
			ws.Cleanup()
			runtime.GC()
			return invalidMutants, err
		}
		// Partial failure: bad mutants were removed but survivors are valid — continue.
		log.Warn("Schemata verification partially failed: %v — continuing with %d remaining mutant(s)", err, len(mutants))
	} else {
		log.Debug("Schemata-transformed code compiles successfully")
	}

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
		finalizeMutants(mutants)
		return append(mutants, invalidMutants...), err
	}

	// DEBUG: expose key mismatch between the two package maps
	log.Debug("[DEBUG-PKGMAP] buildPkgMap produced %d package keys:", len(pkgToMutantIDs))
	for k, ids := range pkgToMutantIDs {
		log.Debug("[DEBUG-PKGMAP] pkgToMutantIDs[%q] = %v", k, ids)
	}
	log.Debug("[DEBUG-PKGMAP] mutantIDToIndex: %v", mutantIDToIndex)

	// // DEBUG: verify every mutant ID appears in mutantIDToIndex
	// log.Debug("[DEBUG-INDEX] Checking mutantIDToIndex coverage:")
	// for _, m := range mutants {
	// 	if idx, ok := mutantIDToIndex[m.ID]; ok {
	// 		log.Debug("[DEBUG-INDEX] mutant %d → index %d ✓", m.ID, idx)
	// 	} else {
	// 		log.Debug("[DEBUG-INDEX] mutant %d → NOT IN INDEX ✗ (will never be classified)", m.ID)
	// 	}
	// }

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
		var bt []string
		if cfg != nil {
			bt = cfg.BuildTags
		}
		log.Info("[UNIT] Running unit tests against %d mutant(s) across %d package(s)", len(mutants), len(pkgToMutantIDs))
		results, err = compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, pkgToMutants, mutantSites, concurrent, testsByPkg, bt, prog, log)

		if len(results) > 0 {
			collectResults(mutants, results, mutantIDToIndex, ws.TempDir)
		}

		// DEBUG: show what collectResults actually matched
		//log.Debug("[DEBUG-COLLECT] After collectResults, raw result IDs and statuses:")
		// for _, r := range results {
		// 	log.Debug("[DEBUG-COLLECT] result id=%d status=%q", r.id, r.status)
		// }
		// log.Debug("[DEBUG-COLLECT] Mutant statuses after collection:")
		// for _, m := range mutants {
		// 	log.Debug("[DEBUG-COLLECT] mutant id=%d status=%q", m.ID, m.Status)
		// }

		if err != nil {
			SaveCache(mutants, baseDir, cache, fileHashes)
			finalizeMutants(mutants)
			return append(mutants, invalidMutants...), err
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
	finalizeMutants(mutants)

	SaveCache(mutants, baseDir, cache, fileHashes)

	return append(mutants, invalidMutants...), nil
}

func runStandalone(ctx context.Context, mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, testsByPkg map[string][]string, progbar bool, buildTags []string, fileHashes map[string]string, log *logger.Logger) ([]Mutant, error) {

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
			// Determine tests for this package
			var pkgTests []string
			if len(testsByPkg) > 0 && len(pkgMutants) > 0 {
				if tests, ok := testsByPkg[pkgDir]; ok {
					pkgTests = tests
				}
			}
			return runStandalonePackage(ctx, pkgDir, pkgMutants, concurrent, pkgTests, workerTempDir, progbar, buildTags, prog, log)
		})
	}

	if err := g.Wait(); err != nil {
		if prog != nil {
			prog.Finish()
		}
		finalizeMutants(mutants)
		return mutants, err
	}

	if prog != nil {
		prog.Finish()
	}

	finalizeMutants(mutants)
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

var statusRank = map[string]int{
	"":          0,
	"untested":  1,
	"survived":  2,
	"error":     3,
	"timeout":   4,
	"killed":    5,
	"invalid":   6, // terminal — never overwrite
}

func shouldUpdate(current, incoming string) bool {
	if current == "invalid" || current == "killed" {
		return false // terminal states
	}
	return statusRank[incoming] >= statusRank[current]
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
		if !shouldUpdate(mutants[idx].Status, result.status) {
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

func filterMutantsWithoutTests(mutants []Mutant, projectRoot string) {
	absRoot, _ := filepath.Abs(projectRoot)
	testPackages := collectPackagesWithTestsAbs(absRoot)

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
		if !testPackages[mutantDir] {
			m.Status = "untested"
		}
	}
}

// collectPackagesWithTestsAbs collects all packages that have test files, using absolute paths as keys
func collectPackagesWithTestsAbs(absModule string) map[string]bool {
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
			absDir, err := filepath.Abs(dir)
			if err == nil {
				pkgs[absDir] = true
			}
		}
		return nil
	})

	return pkgs
}

// collectPackagesWithTests collects all packages that have test files (relative paths for backward compatibility)
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
			// filepath.Rel returns "." when dir == absModule (root package).
			// Normalise to "." so filterMutantsWithoutTests matches correctly.
			if relDir == "" {
				relDir = "."
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
			if s == "survived" || s == "" || s == StatusUntested || s == StatusError {
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
				if shouldUpdate(mutants[idx].Status, r.status) {
					mutants[idx].Status = r.status
					mutants[idx].KilledBy = r.killedBy
					mutants[idx].KillOutput = r.killOutput
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
			if s == "survived" || s == "" || s == StatusUntested || s == StatusError {
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
	// Use "go test -run=^$ ./..." instead of "go build ./..." so that test
	// files are compiled together with source files — this catches errors that
	// only appear when the test binary is linked (e.g. undefined symbols that
	// are only visible when _test.go files are included in the build).
	cmd := exec.CommandContext(ctx, "go", "test", "-run=^$", "-gcflags=all=-e", "./...")
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

// applySingleFile resets srcFile in the workspace to its original source, then
// applies schemata for only `mutants` (which must all belong to srcFile).
// Other files are left untouched.
func applySingleFile(ws *ModuleWorkspace, srcFile string, mutants []*Mutant) error {
	rel, err := ws.relPath(srcFile)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(ws.TempDir, rel)

	src, err := os.ReadFile(srcFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcFile, err)
	}
	if err := os.WriteFile(tempPath, src, filePermissions); err != nil {
		return fmt.Errorf("restore %s: %w", tempPath, err)
	}
	if len(mutants) == 0 {
		return nil
	}

	fset := token.NewFileSet()
	freshAST, err := parser.ParseFile(fset, srcFile, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", srcFile, err)
	}
	for _, m := range mutants {
		m.Site.Fset = fset
	}

	posMap, err := ApplySchemataToAST(freshAST, fset, tempPath, src, mutants)
	if err != nil {
		return fmt.Errorf("apply schemata to %s: %w", srcFile, err)
	}
	for _, m := range mutants {
		if pm, ok := posMap[m.ID]; ok {
			m.TempLine, m.TempCol = pm.TempLine, pm.TempCol
		}
	}
	return nil
}

// fileHasCompileErrors returns true if any compiler error in buildOut points
// at the temp-workspace location of srcFile. This is the local oracle: it
// ignores errors in OTHER files (which are out of scope for this bisection).
func fileHasCompileErrors(buildOut string, ws *ModuleWorkspace, srcFile string) bool {
	rel, err := ws.relPath(srcFile)
	if err != nil {
		return false
	}
	want := filepath.Clean(filepath.Join(ws.TempDir, rel))
	for _, ce := range ParseCompilerErrors(buildOut) {
		ef := ce.File
		if !filepath.IsAbs(ef) {
			ef = filepath.Join(ws.TempDir, ef)
		}
		if filepath.Clean(ef) == want {
			return true
		}
	}
	return false
}

// bisectFileMutants binary-searches the mutants of one source file to identify
// which ones cause compile errors IN THAT FILE. Other files in the workspace
// are left in whatever state the caller set up; their errors are ignored.
func bisectFileMutants(ctx context.Context, ws *ModuleWorkspace, srcFile string, candidates []*Mutant, log *logger.Logger) (good, bad []*Mutant) {
	if len(candidates) == 0 {
		return nil, nil
	}

	if err := applySingleFile(ws, srcFile, candidates); err != nil {
		for _, m := range candidates {
			m.Status = StatusError
			m.KilledBy = "(compiler)"
			m.KillOutput = fmt.Sprintf("bisect: re-apply failed: %v", err)
			bad = append(bad, m)
		}
		return nil, bad
	}

	buildOut, _ := verifyBuildSequential(ctx, ws.TempDir, log)
	if !fileHasCompileErrors(buildOut, ws, srcFile) {
		// File compiles cleanly with this subset — every candidate here is good.
		return candidates, nil
	}

	if len(candidates) == 1 {
		candidates[0].Status = StatusError
		candidates[0].KilledBy = "(compiler)"
		candidates[0].KillOutput = "bisect: identified as compile-failing mutant"
		return nil, []*Mutant{candidates[0]}
	}

	mid := len(candidates) / 2
	lg, lb := bisectFileMutants(ctx, ws, srcFile, candidates[:mid], log)
	rg, rb := bisectFileMutants(ctx, ws, srcFile, candidates[mid:], log)
	return append(lg, rg...), append(lb, rb...)
}

// bisectBadMutants uses binary search to identify which mutants cause compilation failures.
// Returns good mutants and bad mutants (with error status set).
// DEPRECATED: Replaced by per-file bisection (bisectFileMutants). This function had
// structural bugs (empty removed set, wrong oracle) and is kept only for reference.
func bisectBadMutants(ctx context.Context, ws *ModuleWorkspace, candidates []Mutant, log *logger.Logger) (good []Mutant, bad []Mutant) {
	if len(candidates) == 0 {
		return nil, nil
	}

	// Reapply schemata for THIS subset only, then test-build
	badSet := make(map[int]bool)
	for _, m := range candidates {
		badSet[m.ID] = true
	}
	
	// Create a temporary copy of all mutants to reapply
	allMutants := make([]Mutant, len(candidates))
	copy(allMutants, candidates)
	
	if err := reapplyAffectedFiles(ws, make(map[int]bool), allMutants, candidates, log); err != nil {
		// Reapply itself failed — this subset is unrecoverable
		for i := range candidates {
			candidates[i].Status = StatusError
			candidates[i].KilledBy = "(compiler)"
			candidates[i].KillOutput = fmt.Sprintf("bisect: re-apply failed: %v", err)
		}
		return nil, candidates
	}
	
	if _, err := verifyBuildSequential(ctx, ws.TempDir, log); err == nil {
		return candidates, nil // all good
	}
	
	if len(candidates) == 1 {
		// Single bad mutant
		candidates[0].Status = StatusError
		candidates[0].KilledBy = "(compiler)"
		candidates[0].KillOutput = "bisect: single mutant causes compilation failure"
		return nil, candidates
	}

	mid := len(candidates) / 2
	leftGood, leftBad := bisectBadMutants(ctx, ws, candidates[:mid], log)
	rightGood, rightBad := bisectBadMutants(ctx, ws, candidates[mid:], log)
	return append(leftGood, rightGood...), append(leftBad, rightBad...)
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
			log.Debug("[VERIFY] Round %d: tight scan missed — falling back to per-file bisection", round+1)

			// Group current mutants by source file & build a temp→source path map.
			byFile := make(map[string][]*Mutant, 16)
			tempToSrc := make(map[string]string, 16)
			for i := range mutants {
				if mutants[i].Site.File == nil {
					continue
				}
				src := mutants[i].Site.File.Name()
				byFile[src] = append(byFile[src], &mutants[i])
				if _, ok := tempToSrc[src]; !ok {
					if rel, err := ws.relPath(src); err == nil {
						tempToSrc[filepath.Clean(filepath.Join(ws.TempDir, rel))] = src
					}
				}
			}

			// Map compiler errors → source files we know about.
			failingFiles := make(map[string]bool)
			for _, ce := range ParseCompilerErrors(buildOut) {
				ef := ce.File
				if !filepath.IsAbs(ef) {
					ef = filepath.Join(ws.TempDir, ef)
				}
				if src, ok := tempToSrc[filepath.Clean(ef)]; ok {
					failingFiles[src] = true
				}
			}

			if len(failingFiles) == 0 {
				// Errors don't map to any source file we mutated (e.g. they live
				// in helper files or test files we didn't touch). Don't poison
				// the run — leave mutants unstatused and let the per-package
				// compile in compileAndRunPackages classify them individually.
				log.Warn("[VERIFY] Round %d: errors map to no tracked source file — deferring %d mutant(s) to per-package compile",
					round+1, len(mutants))
				return mutants, removed, nil
			}

			// Bisect each failing file independently.
			badSet := make(map[int]bool)
			for srcFile := range failingFiles {
				fileMutants := byFile[srcFile]
				if len(fileMutants) == 0 {
					continue
				}
				log.Debug("[VERIFY] Round %d: bisecting %d mutant(s) in %s",
					round+1, len(fileMutants), filepath.Base(srcFile))
				_, bad := bisectFileMutants(ctx, ws, srcFile, fileMutants, log)
				for _, m := range bad {
					badSet[m.ID] = true
				}
			}

			if len(badSet) == 0 {
				// Per-file bisection couldn't pinpoint a culprit (typical when
				// the build error originates in a sibling file we don't track,
				// or from a cross-file interaction). Fall back to package-level
				// quarantine: mark every mutant in the failing packages as a
				// compile error, reset those source files to baseline so the
				// rest of the workspace can build, then continue.
				failingPkgs := identifyFailingPackages(ws, buildOut)
				if len(failingPkgs) == 0 {
					log.Warn("[VERIFY] Round %d: bisection found no bad mutants and no failing packages — deferring %d mutant(s) to per-package compile",
						round+1, len(mutants))
					return mutants, removed, nil
				}
				kept, failed := partitionMutantsByPackages(ws, mutants, failingPkgs, round+1)
				removed = append(removed, failed...)
				if err := resetMutatedFilesInPkgs(ws, mutants, failingPkgs); err != nil {
					log.Warn("[VERIFY] Round %d: failed to reset failing packages to source: %v", round+1, err)
				}
				log.Warn("[VERIFY] Round %d: bisection inconclusive — quarantined %d mutant(s) in %d failing package(s), keeping %d",
					round+1, len(failed), len(failingPkgs), len(kept))
				mutants = kept
				if len(mutants) == 0 {
					return nil, removed, nil
				}
				continue
			}

			var keptThisRound []Mutant
			var failedThisRound []Mutant
			for i := range mutants {
				if badSet[mutants[i].ID] {
					// bisectFileMutants already set Status/KilledBy/KillOutput on the pointer.
					failedThisRound = append(failedThisRound, mutants[i])
				} else {
					keptThisRound = append(keptThisRound, mutants[i])
				}
			}
			removed = append(removed, failedThisRound...)
			log.Debug("[VERIFY] Round %d: per-file bisection identified %d bad mutant(s), keeping %d",
				round+1, len(failedThisRound), len(keptThisRound))

			// Re-sync the workspace: bisection left each failing file in whatever
			// state its last recursive call wrote. reapplyAffectedFiles resets each
			// file containing a removed mutant and re-applies schemata for the
			// kept set only — that's the canonical post-round state.
			if err := reapplyAffectedFiles(ws, badSet, mutants, keptThisRound, log); err != nil {
				for i := range keptThisRound {
					if keptThisRound[i].Status == "" {
						keptThisRound[i].Status = StatusUntested
					}
				}
				return keptThisRound, removed, fmt.Errorf("re-apply after per-file bisection in round %d: %w", round+1, err)
			}
			mutants = keptThisRound
			continue
		}

		log.Debug("[VERIFY] Round %d: removing %d bad mutant(s): %v", round+1, len(badIDs), badIDs)

		badSet := make(map[int]bool, len(badIDs))
		for _, id := range badIDs {
			badSet[id] = true
		}

		keptThisRound, failedThisRound := partitionMutantsByIDs(mutants, badSet, round+1)
		removed = append(removed, failedThisRound...)

		if len(keptThisRound) == len(mutants) {
			for i := range keptThisRound {
				if keptThisRound[i].Status == "" {
					keptThisRound[i].Status = StatusUntested
					keptThisRound[i].KilledBy = "(compiler)"
					keptThisRound[i].KillOutput = fmt.Sprintf("build verification (round %d): bad IDs identified but none matched", round+1)
				}
			}
			removed = append(removed, keptThisRound...)
			return keptThisRound, removed, fmt.Errorf("build verification: bad mutants identified but none removed, aborting")
		}

		if err := reapplyAffectedFiles(ws, badSet, mutants, keptThisRound, log); err != nil {
			for i := range keptThisRound {
				if keptThisRound[i].Status == "" {
					keptThisRound[i].Status = StatusUntested
					keptThisRound[i].KilledBy = "(compiler)"
					keptThisRound[i].KillOutput = fmt.Sprintf("build verification (round %d): re-apply failed", round+1)
				}
			}
			removed = append(removed, keptThisRound...)
			return keptThisRound, removed, fmt.Errorf("re-apply after round %d: %w", round+1, err)
		}
		mutants = keptThisRound
	}

	// After max rounds, quarantine mutants in any still-failing packages so
	// the rest of the workspace can compile and its tests can run.
	if finalOut, finalErr := verifyBuildSequential(verifyCtx, ws.TempDir, log); finalErr != nil {
		failingPkgs := identifyFailingPackages(ws, finalOut)
		if len(failingPkgs) == 0 {
			log.Warn("[VERIFY] Still failing after %d rounds with no attributable packages — deferring %d mutant(s) to per-package compile", maxRounds, len(mutants))
			return mutants, removed, nil
		}
		kept, failed := partitionMutantsByPackages(ws, mutants, failingPkgs, maxRounds)
		log.Warn("[VERIFY] Still failing after %d rounds — quarantining %d mutant(s) across %d package(s); %d remain", maxRounds, len(failed), len(failingPkgs), len(kept))
		if err := resetMutatedFilesInPkgs(ws, mutants, failingPkgs); err != nil {
			return mutants, removed, fmt.Errorf("reset failing packages after max rounds: %w", err)
		}
		removed = append(removed, failed...)
		return kept, removed, nil
	}
	return mutants, removed, nil
}

// identifyFailingPackages extracts package directories from compiler errors
func identifyFailingPackages(ws *ModuleWorkspace, buildOutput string) map[string]bool {
	failingPkgs := make(map[string]bool)
	for _, ce := range ParseCompilerErrors(buildOutput) {
		fp := ce.File
		if !filepath.IsAbs(fp) {
			fp = filepath.Join(ws.TempDir, fp)
		}
		failingPkgs[filepath.Dir(fp)] = true
	}
	return failingPkgs
}

// resetMutatedFilesInPkgs restores every original source file whose mutated
// copy lives inside one of failingPkgs back to its unmutated content, so the
// rest of the workspace can compile after we give up bisecting that package's
// mutants. The previously-injected gorgon_schemata.go helper stays in place,
// so the activeMutantID symbol remains defined for any file that references it.
func resetMutatedFilesInPkgs(ws *ModuleWorkspace, allMutants []Mutant, failingPkgs map[string]bool) error {
	if len(failingPkgs) == 0 {
		return nil
	}
	resetPaths := make(map[string]bool)
	for i := range allMutants {
		m := &allMutants[i]
		if m.Site.File == nil {
			continue
		}
		rel, err := ws.relPath(m.Site.File.Name())
		if err != nil {
			continue
		}
		pkgDir := filepath.Join(ws.TempDir, filepath.Dir(rel))
		if !failingPkgs[pkgDir] {
			continue
		}
		tempPath := filepath.Join(ws.TempDir, rel)
		if resetPaths[tempPath] {
			continue
		}
		src, err := os.ReadFile(m.Site.File.Name())
		if err != nil {
			return fmt.Errorf("re-read %s: %w", m.Site.File.Name(), err)
		}
		if err := os.WriteFile(tempPath, src, filePermissions); err != nil {
			return fmt.Errorf("restore %s: %w", tempPath, err)
		}
		resetPaths[tempPath] = true
	}
	return nil
}

// partitionMutantsByPackages splits mutants into those in failing packages vs clean packages
func partitionMutantsByPackages(ws *ModuleWorkspace, mutants []Mutant, failingPkgs map[string]bool, round int) (kept []Mutant, failed []Mutant) {
	for i := range mutants {
		if mutants[i].Site.File == nil {
			kept = append(kept, mutants[i])
			continue
		}
		rel, _ := ws.relPath(mutants[i].Site.File.Name())
		pkgDir := filepath.Join(ws.TempDir, filepath.Dir(rel))
		if failingPkgs[pkgDir] {
			mutants[i].Status = StatusError
			mutants[i].KilledBy = "(compiler)"
			mutants[i].KillOutput = fmt.Sprintf("build verification round %d: unable to identify specific mutant", round)
			failed = append(failed, mutants[i])
		} else {
			kept = append(kept, mutants[i])
		}
	}
	return kept, failed
}

// partitionMutantsByIDs splits mutants into those with bad IDs vs good IDs
func partitionMutantsByIDs(mutants []Mutant, badSet map[int]bool, round int) (kept []Mutant, failed []Mutant) {
	for i := range mutants {
		if badSet[mutants[i].ID] {
			mutants[i].Status = StatusError
			mutants[i].KilledBy = "(compiler)"
			mutants[i].KillOutput = fmt.Sprintf("build verification: removed in round %d", round)
			failed = append(failed, mutants[i])
		} else {
			kept = append(kept, mutants[i])
		}
	}
	return kept, failed
}

// extractMutantIDsFromBuildErrors scans temp files near error lines for activeMutantID patterns.
// Uses a tight ±5 line window. If no IDs found, returns empty (bisection will take over).
func extractMutantIDsFromBuildErrors(tempDir, buildOutput string) []int {
	errors := ParseCompilerErrors(buildOutput)
	seen := make(map[int]bool)
	var ids []int

	prefix := []byte("activeMutantID == ")

	// Scan files near error lines with tight ±5 line window
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

		// Scan ±5 lines around error location to find activeMutantID checks.
		lo := ce.Line - 5
		if lo < 0 {
			lo = 0
		}
		hi := ce.Line + 5
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

	// No fallback: if we can't pinpoint within ±5 lines, return empty and let bisection take over
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
	testsByPkg := make(map[string][]string)
	if len(tests) > 0 {
		testsByPkg[""] = tests
	}
	return GenerateAndRunSchemata(ctx, sites, operators, allOps, baseDir, projectRoot, dirRules, resolver, concurrent, cache, testsByPkg, testPaths, log, progbar, unitTestsEnabled, externalCfg, cfg)
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