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

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) ([]Mutant, error) {

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
	} else {
		filterMutantsWithoutTests(mutants, baseDir)
	}

	// Run preflight validation: Level 1 (static) + Level 2 (type-check each mutant)
	validMutants, allInvalid := RunPreflight(mutants, log)

	// Mark the bad ones on the original list
	for _, r := range allInvalid {
		for i := range mutants {
			if mutants[i].ID == r.MutantID {
				mutants[i].Status = r.Status
				mutants[i].Error = r.Error
				mutants[i].KillOutput = r.ErrorReason
				break
			}
		}
	}

	mutants = validMutants
	lastTotalMutants = len(mutants)

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
		// Need workspace for external tests
		baseDirAbs, _ := filepath.Abs(baseDir)
		if !fileExists(filepath.Join(baseDirAbs, "go.mod")) {
			log.Warn("[EXTERNAL] External suites require go.mod, skipping")
			return mutants, nil
		}

		ws, err := NewModuleWorkspace()
		if err != nil {
			return mutants, fmt.Errorf("workspace creation failed: %w", err)
		}
		defer ws.Cleanup()

		if err := ws.setup(baseDir, mutants); err != nil {
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
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, progbar, fileHashes, log)
	}
	log.Debug("Module layout detected, using workspace mode")

	ws, err := NewModuleWorkspace()
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("workspace creation failed: %w", err))
		return mutants, err
	}
	defer ws.Cleanup()

	if err := ws.setup(projectRoot, mutants); err != nil {
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
	mutants, err = verifyAndCleanSchemata(ctx, ws, mutants, log)
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
		rel, err := filepath.Rel(ws.absModule, m.Site.File.Name())
		if err != nil {
			continue
		}
		pkgDir := filepath.Join(ws.TempDir, filepath.Dir(rel))
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], m)
	}

	var prog *ProgressTracker
	if progbar {
		prog = NewProgressTracker(len(mutants))
	}

	runUnitTests := unitTestsEnabled && (!externalCfg.Enabled || externalCfg.RunMode != "only")

	var results []mutantResult
	if runUnitTests {
		var err error
		results, err = compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, pkgToMutants, mutantSites, concurrent, tests, prog, log)

		if len(results) > 0 {
			collectResults(mutants, results, mutantIDToIndex, ws.TempDir)
		}

		if err != nil {
			SaveCache(mutants, baseDir, cache, fileHashes)
			return mutants, err
		}
	}

	log.Debug("After unit tests, about to check external phase")
	// ── Phase 2: External Suites ──────────────────────────────────────────────
	if externalCfg.Enabled && len(externalCfg.Suites) > 0 && len(suiteBinaries) > 0 {
		log.Info("[EXTERNAL] Running external suite phase with %d suites", len(externalCfg.Suites))
		if err := runExternalPhaseWithBinaries(ctx, ws, mutants, externalCfg, suiteBinaries, concurrent, log); err != nil {
			log.Warn("external suite phase failed: %v", err)
		}
	} else {
		log.Debug("[EXTERNAL] Skipping external phase: enabled=%v, suites=%d, binaries=%d", externalCfg.Enabled, len(externalCfg.Suites), len(suiteBinaries))
	}

	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

func runStandalone(mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, progbar bool, fileHashes map[string]string, log *logger.Logger) ([]Mutant, error) {

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

	g, ctx := errgroup.WithContext(context.Background())
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
			return runStandalonePackage(pkgDir, pkgMutants, concurrent, tests, workerTempDir, progbar, prog, log)
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
		resolvedPaths, err := resolveSuitePaths(ctx, ws.TempDir, suite, log)
		if err != nil || len(resolvedPaths) == 0 {
			log.Warn("[EXTERNAL] No packages found for suite %q: %v", suite.Name, err)
			continue
		}

		binaries, err := buildExternalSuiteBinaries(ctx, ws.TempDir, suite, resolvedPaths, log)
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

	var targets []*Mutant
	switch cfg.RunMode {
	case "only", "alongside":
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	default: // "after_unit"
		for i := range mutants {
			if mutants[i].Status == "survived" || mutants[i].Status == "" {
				targets = append(targets, &mutants[i])
			}
		}
	}

	if len(targets) == 0 {
		log.Info("[EXTERNAL] No survivors to test against external suites")
		return nil
	}

	log.Info("[EXTERNAL] Running %d survivors against external suites", len(targets))

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
				if r.status == "killed" {
					if idx, ok := idToIdx[r.id]; ok {
						mutants[idx].Status = "killed"
						mutants[idx].KilledBy = r.killedBy
						mutants[idx].KillOutput = r.killOutput
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
	switch cfg.RunMode {
	case "only", "alongside":
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	default: // "after_unit"
		for i := range mutants {
			if mutants[i].Status == "survived" || mutants[i].Status == "" {
				targets = append(targets, &mutants[i])
			}
		}
	}

	if len(targets) == 0 {
		log.Info("[EXTERNAL] No survivors to test against external suites")
		return nil
	}

	log.Info("[EXTERNAL] Running %d survivors against external suites", len(targets))

	for _, suite := range cfg.Suites {
		resolvedPaths, err := resolveSuitePaths(ctx, ws.TempDir, suite, log)
		if err != nil || len(resolvedPaths) == 0 {
			log.Warn("[EXTERNAL] No packages found for suite %q: %v", suite.Name, err)
			continue
		}

		binaries, err := buildExternalSuiteBinaries(ctx, ws.TempDir, suite, resolvedPaths, log)
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
				if r.status == "killed" {
					if idx, ok := idToIdx[r.id]; ok {
						mutants[idx].Status = "killed"
						mutants[idx].KilledBy = r.killedBy
						mutants[idx].KillOutput = r.killOutput
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
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = tempDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("build failed")
	}
	return "", nil
}

// verifyAndCleanSchemata runs build verification with retry loop that removes bad mutants.
func verifyAndCleanSchemata(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, log *logger.Logger) ([]Mutant, error) {
	const maxRounds = 5

	for round := 0; round < maxRounds; round++ {
		buildOut, buildErr := verifyBuildSequential(ctx, ws.TempDir, log)
		if buildErr == nil {
			if round > 0 {
				log.Debug("[VERIFY] Build clean after %d removal round(s)", round)
			}
			return mutants, nil
		}

		log.Debug("[VERIFY] Round %d: build failed, scanning for bad mutant IDs", round+1)

		badIDs := extractMutantIDsFromBuildErrors(ws.TempDir, buildOut)
		if len(badIDs) == 0 {
			return nil, fmt.Errorf("schemata build failed (round %d), bad mutants unidentifiable:\n%s", round+1, buildOut)
		}

		log.Debug("[VERIFY] Round %d: removing %d bad mutant(s): %v", round+1, len(badIDs), badIDs)

		badSet := make(map[int]bool, len(badIDs))
		for _, id := range badIDs {
			badSet[id] = true
		}

		var kept []Mutant
		for i := range mutants {
			if badSet[mutants[i].ID] {
				mutants[i].Status = StatusCompileError
				mutants[i].KillOutput = fmt.Sprintf("build verification: removed in round %d", round+1)
			} else {
				kept = append(kept, mutants[i])
			}
		}

		if len(kept) == len(mutants) {
			return nil, fmt.Errorf("build verification: bad mutants identified but none removed, aborting")
		}

		// Re-apply schemata to the affected files.
		if err := reapplyAffectedFiles(ws, badSet, kept, log); err != nil {
			return nil, fmt.Errorf("re-apply after round %d: %w", round+1, err)
		}
		mutants = kept
	}

	if _, finalErr := verifyBuildSequential(ctx, ws.TempDir, log); finalErr != nil {
		log.Warn("[VERIFY] Still failing after %d rounds — proceeding with remaining mutants", maxRounds)
	}
	return mutants, nil
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

		// Scan a ±15 line window around the error.
		lo := ce.Line - 16
		if lo < 0 {
			lo = 0
		}
		hi := ce.Line + 15
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
func reapplyAffectedFiles(ws *ModuleWorkspace, removed map[int]bool, kept []Mutant, log *logger.Logger) error {
	// Find which source files contained removed mutants.
	affected := make(map[string][]*Mutant) // origPath → kept mutants for that file

	for i := range kept {
		m := &kept[i]
		if m.Site.File == nil {
			continue
		}
		p := m.Site.File.Name()
		affected[p] = append(affected[p], m)
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

// Test helper for integration tests - calls GenerateAndRunSchemata with proper types
func TestGenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig, cfg *config.Config) ([]Mutant, error) {
	return GenerateAndRunSchemata(ctx, sites, operators, allOps, baseDir, projectRoot, dirRules, resolver, concurrent, cache, tests, testPaths, log, progbar, unitTestsEnabled, externalCfg, cfg)
}

// Test helper for extractMutantIDsFromBuildErrors
func TestExtractMutantIDsFromBuildErrors(tempDir, buildOutput string) []int {
	return extractMutantIDsFromBuildErrors(tempDir, buildOutput)
}
