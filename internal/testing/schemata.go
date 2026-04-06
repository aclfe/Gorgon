// Package testing provides mutation testing execution logic.
package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// GenerateAndRunSchemata is the main entry point: generates mutants, applies mutations,
// compiles test binaries, runs tests, and collects results.
func GenerateAndRunSchemata(ctx context.Context, sites []engine.Site, operators []mutator.Operator, baseDir string, concurrent int, cache *cache.Cache, tests []string, debug bool) ([]Mutant, error) {
	// Step 1: Generate mutants (single-pass dedup + generation)
	mutants := GenerateMutants(sites, operators)
	if len(mutants) == 0 {
		return nil, nil
	}

	// Step 2: Resolve cache - get indices of mutants that still need to run
	uncachedIndices, fileHashes, err := ResolveCache(mutants, baseDir, cache)
	if err != nil {
		return nil, err
	}
	if uncachedIndices == nil {
		return mutants, nil // All cached
	}

	// Step 3: Determine execution path
	baseDirAbs, _ := filepath.Abs(baseDir)
	if !fileExists(filepath.Join(baseDirAbs, "go.mod")) {
		return runStandalone(mutants, uncachedIndices, concurrent, cache, baseDir, tests, debug, fileHashes)
	}

	// Step 4: Setup module workspace
	ws, err := NewModuleWorkspace()
	if err != nil {
		return nil, err
	}
	defer ws.Cleanup()

	if err := ws.setup(baseDir, mutants); err != nil {
		return nil, err
	}

	// Step 5: Apply schemata
	if err := RewriteImports(ws.TempDir); err != nil {
		return nil, fmt.Errorf("rewrite imports: %w", err)
	}
	_ = MakeSelfContained(ws.TempDir)

	fileToMutants, err := ws.applySchemata(mutants)
	if err != nil {
		return nil, err
	}

	// Step 6: Simplify go.mod if only stdlib used
	ws.simplifyGoMod(fileToMutants)

	// Step 7: Build package mapping
	pkgToMutantIDs, mutantIDToIndex, err := ws.buildPkgMap(mutants)
	if err != nil {
		return nil, err
	}

	// Step 8: Compile and run tests with max concurrency
	results, err := compileAndRunPackages(ctx, ws.TempDir, pkgToMutantIDs, concurrent, tests)
	if err != nil {
		return nil, err
	}

	// Step 9: Collect results
	collectResults(mutants, results, mutantIDToIndex, ws.TempDir)

	// Step 10: Save cache (reuse hashes from ResolveCache)
	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

// runStandalone handles packages without go.mod.
// All packages run concurrently, each with its own temp dir.
func runStandalone(mutants []Mutant, uncachedIndices []int, concurrent int, cache *cache.Cache, baseDir string, tests []string, debug bool, fileHashes map[string]string) ([]Mutant, error) {
	// Group uncached mutants by package
	pkgToMutants := make(map[string][]*Mutant, len(uncachedIndices))
	for _, idx := range uncachedIndices {
		m := &mutants[idx]
		pkgDir := filepath.Dir(m.Site.File.Name())
		pkgToMutants[pkgDir] = append(pkgToMutants[pkgDir], m)
	}

	// Run all packages concurrently, limited by the concurrent setting.
	// Each package has its own temp dir and compiles/runs independently.
	g, ctx := errgroup.WithContext(context.Background())
	g.SetLimit(concurrent)

	for pkgDir, pkgMutants := range pkgToMutants {
		pkgDir := pkgDir
		pkgMutants := pkgMutants
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

	// Save cache (reuse hashes from ResolveCache)
	SaveCache(mutants, baseDir, cache, fileHashes)

	return mutants, nil
}

// RewriteImports is a placeholder for future import rewriting logic.
func RewriteImports(_ string) error {
	return nil
}

// MakeSelfContained ensures the temp module is self-contained.
func MakeSelfContained(tempDir string) error {
	goModPath := filepath.Join(tempDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read go.mod: %w", err)
	}

	content := string(data)
	if os.IsNotExist(err) {
		content = "module " + benchModuleName + "\ngo " + goVersion + "\n"
	} else if !strings.Contains(content, "replace github.com/aclfe/gorgon =>") {
		content = strings.TrimSpace(content) + "\n\nreplace github.com/aclfe/gorgon => ./\n"
	}

	if err := os.WriteFile(goModPath, []byte(content), filePermissions); err != nil {
		return fmt.Errorf("write go.mod: %w", err)
	}
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
