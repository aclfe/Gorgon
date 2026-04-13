
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



func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string, debug bool, progbar bool) ([]Mutant, error) {

	mutants := GenerateMutants(sites, operators)
	if len(mutants) == 0 {
		return nil, nil
	}


	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {
		// Set error status on all mutants when cache resolution fails
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
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, progbar, fileHashes)
	}


	ws, err := NewModuleWorkspace()
	if err != nil {
		// Set error status on all mutants when workspace creation fails
		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("workspace creation failed: %w", err)
		}
		return mutants, err
	}
	defer ws.Cleanup()

	if err := ws.setup(baseDir, mutants); err != nil {
		// Set error status on all mutants when workspace setup fails
		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("workspace setup failed: %w", err)
		}
		return mutants, err
	}

	_ = MakeSelfContained(ws.TempDir)

	_, hasNonStdlib, err := ws.applySchemata(mutants)
	if err != nil {
		// Set error status on all mutants when schemata application fails
		for i := range mutants {
			mutants[i].Status = "error"
			mutants[i].Error = fmt.Errorf("schemata application failed: %w", err)
		}
		return mutants, err
	}


	ws.simplifyGoMod(hasNonStdlib)

	
	pkgToMutantIDs, mutantIDToIndex, err := ws.buildPkgMap(mutants)
	if err != nil {
		// Set error status on all mutants when package map building fails
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
	results, err := compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, mutantSites, concurrent, tests, prog)
	
	// Collect results even if there's an error
	if len(results) > 0 {
		collectResults(mutants, results, mutantIDToIndex, ws.TempDir)
	}
	
	if err != nil {
		// Save any cached results before returning
		SaveCache(mutants, baseDir, cache, fileHashes)
		return mutants, err
	}


	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}



func runStandalone(mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, progbar bool, fileHashes map[string]string) ([]Mutant, error) {

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

	// Create worker-scoped temp dirs (one per concurrent slot) to avoid
	// create/destroy syscalls per package.
	workerTempDirs := make([]string, concurrent)
	for i := 0; i < concurrent; i++ {
		d, err := os.MkdirTemp("", "gorgon-worker-*")
		if err != nil {
			// Clean up any already created dirs
			for j := 0; j < i; j++ {
				os.RemoveAll(workerTempDirs[j])
			}
			return nil, fmt.Errorf("failed to create worker temp dir: %w", err)
		}
		workerTempDirs[i] = d
	}
	defer func() {
		for _, d := range workerTempDirs {
			os.RemoveAll(d)
		}
	}()

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrent)

	for i, pkgDir := range pkgDirs {
		pkgMutants := pkgToMutants[pkgDir]
		pkgDir := pkgDir
		workerTempDir := workerTempDirs[i%concurrent]
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return runStandalonePackage(pkgDir, pkgMutants, concurrent, tests, workerTempDir, progbar, prog)
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
		mutants[idx].Status = result.status
		mutants[idx].Error = result.err
		mutants[idx].TempDir = tempDir
	}
}
