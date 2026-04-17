package testing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, allOps []mutator.Operator, baseDir string, projectRoot string, dirRules []config.DirOperatorRule, resolver *subconfig.Resolver, concurrent int, cache *cache.Cache, tests []string, testPaths []string, log *logger.Logger, progbar bool, unitTestsEnabled bool, externalCfg config.ExternalSuitesConfig) ([]Mutant, error) {

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

	totalMutants := len(mutants)

	// === Level 1: Quick static filter ===
	validAfterLevel1, level1Invalid := quickStaticFilter(mutants)

	// === Level 2: Accurate schemata compile check (this is the important one) ===
	validMutants, level2Invalid := level2PackagePreflight(validAfterLevel1)

	allInvalid := append(level1Invalid, level2Invalid...)

	// Log nice stats. Invariant: level1 + level2 + validCount == totalMutants.
	LogPreflightResults(log, totalMutants, allInvalid, len(validMutants))

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
		
		if _, _, err := ws.applySchemata(mutants); err != nil {
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
	log.Debug("Checking for go.mod at projectRoot: %s", filepath.Join(projectRootAbs, "go.mod"))
	if !fileExists(filepath.Join(projectRootAbs, "go.mod")) {
		log.Debug("No go.mod found, using standalone mode")
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, progbar, fileHashes, log)
	}
	log.Debug("go.mod found, using workspace mode")

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

	_, hasNonStdlib, err := ws.applySchemata(mutants)
	if err != nil {
		setMutantErrors(mutants, fmt.Errorf("schemata application failed: %w", err))
		return mutants, err
	}

	ws.simplifyGoMod(hasNonStdlib || externalCfg.Enabled)

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
	log.Debug("Before external phase check: enabled=%v, suites=%d", externalCfg.Enabled, len(externalCfg.Suites))
	if externalCfg.Enabled && len(externalCfg.Suites) > 0 {
		log.Info("[EXTERNAL] Starting external suite phase with %d suites, enabled=%v, runMode=%s", len(externalCfg.Suites), externalCfg.Enabled, externalCfg.RunMode)
		allSuitePaths := collectAllSuitePaths(externalCfg.Suites)
		log.Info("[EXTERNAL] Collected %d suite paths", len(allSuitePaths))
		if copyErr := ws.copyExternalSuites(ws.absModule, allSuitePaths, log); copyErr != nil {
			log.Warn("external suite copy failed: %v", copyErr)
		} else {
			if err := runExternalPhase(ctx, ws, mutants, externalCfg, concurrent, log); err != nil {
				log.Warn("external suite phase failed: %v", err)
			}
		}
	} else {
		log.Info("[EXTERNAL] External suites disabled or no suites configured (enabled=%v, suites=%d)", externalCfg.Enabled, len(externalCfg.Suites))
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
		idx := mutantIDToIndex[result.id]
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

	coveredPackages := make(map[string]bool)
	for _, testPath := range testPaths {
		absPath, err := filepath.Abs(testPath)
		if err != nil {
			continue
		}

		pkgDir := filepath.Dir(absPath)
		coveredPackages[pkgDir] = true
	}

	if len(coveredPackages) == 0 {
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

		mutantPkg := filepath.Dir(absMutantPath)

		isCovered := false
		for pkgDir := range coveredPackages {

			if mutantPkg == pkgDir || strings.HasPrefix(mutantPkg+string(filepath.Separator), pkgDir+string(filepath.Separator)) {
				isCovered = true
				break
			}
		}

		if !isCovered {

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


func runExternalPhase(ctx context.Context, ws *ModuleWorkspace, mutants []Mutant, cfg config.ExternalSuitesConfig, concurrent int, log *logger.Logger) error {
	var targets []*Mutant
	switch cfg.RunMode {
	case "only":
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	case "alongside":
		for i := range mutants {
			targets = append(targets, &mutants[i])
		}
	default: // "after_unit"
		for i := range mutants {
			// Include mutants that survived OR have no status (unit tests didn't run)
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
					for i := range mutants {
						if mutants[i].ID == r.id {
							mutants[i].Status = "killed"
							mutants[i].KilledBy = r.killedBy
							mutants[i].KillOutput = r.killOutput
							break
						}
					}
				}
			}
		}
	}
	return nil
}
