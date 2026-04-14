package testing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string, testPaths []string, debug bool, progbar bool) ([]Mutant, error) {

	mutants := GenerateMutants(sites, operators)
	if len(mutants) == 0 {
		return nil, nil
	}

	if len(testPaths) > 0 {
		filterMutantsByTestPackages(mutants, testPaths)
	} else {
		filterMutantsWithoutTests(mutants, baseDir)
	}

	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {

		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("cache resolution failed: %w", err)
		}
		return mutants, err
	}
	if uncachedIndices == nil {
		return mutants, nil
	}

	baseDirAbs, _ := filepath.Abs(baseDir)
	if !fileExists(filepath.Join(baseDirAbs, "go.mod")) {
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, testPaths, progbar, fileHashes, debug)
	}

	ws, err := NewModuleWorkspace()
	if err != nil {

		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("workspace creation failed: %w", err)
		}
		return mutants, err
	}
	defer ws.Cleanup()

	if err := ws.setup(baseDir, mutants); err != nil {

		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("workspace setup failed: %w", err)
		}
		return mutants, err
	}

	_ = MakeSelfContained(ws.TempDir)

	_, hasNonStdlib, err := ws.applySchemata(mutants)
	if err != nil {

		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("schemata application failed: %w", err)
		}
		return mutants, err
	}

	ws.simplifyGoMod(hasNonStdlib)

	pkgToMutantIDs, mutantIDToIndex, err := ws.buildPkgMap(mutants)
	if err != nil {

		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("build package map failed: %w", err)
		}
		return mutants, err
	}

	mutantSites := make(map[int]MutantSite, len(mutants))
	for i := range mutants {
		m := &mutants[i]
		if m.Site.File != nil {
			mutantSites[m.ID] = MutantSite{
				File: m.Site.File.Name(),
				Line: m.Site.Line,
				Col:  m.Site.Column,
			}
		}
	}

	var prog *ProgressTracker
	if progbar {
		prog = NewProgressTracker(len(mutants))
	}
	results, err := compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, mutantSites, concurrent, tests, prog, debug)

	if len(results) > 0 {
		collectResults(mutants, results, mutantIDToIndex, ws.TempDir)
	}

	if err != nil {

		SaveCache(mutants, baseDir, cache, fileHashes)
		return mutants, err
	}

	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

func runStandalone(mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, testPaths []string, progbar bool, fileHashes map[string]string, debug bool) ([]Mutant, error) {

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
			return runStandalonePackage(pkgDir, pkgMutants, concurrent, tests, testPaths, workerTempDir, progbar, prog, debug)
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

			m.Status = "survived"
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
			m.Status = "survived"
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
