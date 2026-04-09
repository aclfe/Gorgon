
package testing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)



func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string, debug bool) ([]Mutant, error) {
	
	mutants := GenerateMutants(sites, operators)
	if len(mutants) == 0 {
		return nil, nil
	}

	
	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {
		return nil, err
	}
	if uncachedIndices == nil {
		return mutants, nil 
	}

	
	baseDirAbs, _ := filepath.Abs(baseDir)
	if !fileExists(filepath.Join(baseDirAbs, "go.mod")) {
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, debug, fileHashes)
	}

	
	ws, err := NewModuleWorkspace()
	if err != nil {
		return nil, err
	}
	defer ws.Cleanup()

	if err := ws.setup(baseDir, mutants); err != nil {
		return nil, err
	}

	_ = MakeSelfContained(ws.TempDir)

	fileToMutants, err := ws.applySchemata(mutants)
	if err != nil {
		return nil, err
	}

	
	ws.simplifyGoMod(fileToMutants)

	
	pkgToMutantIDs, mutantIDToIndex, err := ws.buildPkgMap(mutants)
	if err != nil {
		return nil, err
	}

	
	results, err := compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, concurrent, tests)
	if err != nil {
		return nil, err
	}

	
	collectResults(mutants, results, mutantIDToIndex, ws.TempDir)

	
	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}



func runStandalone(mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, debug bool, fileHashes map[string]string) ([]Mutant, error) {

	pkgToMutants := make(map[string][]*Mutant, len(uncachedIndices))
	for _, idx := range uncachedIndices {
		m := &mutants[idx]
		pkgDir := filepath.Dir(m.Site.File.Name())
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], m)
	}

	// Sort package directories for deterministic processing
	pkgDirs := make([]string, 0, len(pkgToMutants))
	for pkgDir := range pkgToMutants {
		pkgDirs = append(pkgDirs, pkgDir)
	}
	sort.Strings(pkgDirs)

	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrent)

	for _, pkgDir := range pkgDirs {
		pkgMutants := pkgToMutants[pkgDir]
		pkgDir := pkgDir
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return runStandalonePackage(pkgDir, pkgMutants, concurrent, tests, debug)
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
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
