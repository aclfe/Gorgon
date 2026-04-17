package runner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/cli"
	"github.com/aclfe/gorgon/internal/core"
	"github.com/aclfe/gorgon/internal/diff"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/internal/suppressions"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

func Run(flags *cli.Flags, cfg *config.Config, targets []string, configPath string) error {
	if len(targets) == 0 {
		cli.PrintUsage()
	}

	ops, err := cli.ParseOperators(cfg)
	if err != nil {
		return err
	}

	concurrent := cli.ParseConcurrent(cfg.Concurrent)

	eng := engine.NewEngine(flags.PrintAST)
	eng.SetOperators(ops)

	projectRoot := findProjectRoot(targets[0], cfg.Base)
	eng.SetProjectRoot(projectRoot)
	eng.SetSuppressEntries(cfg.Suppress)

	// Discover sub-configs
	resolver, err := subconfig.Discover(projectRoot, configPath)
	if err != nil {
		return fmt.Errorf("failed to discover sub-configs: %w", err)
	}

	if cfg.ProgBar {
		var mu sync.Mutex
		var lastPct int
		var lastFile string
		eng.FileProgressFunc = func(filename string) {
			mu.Lock()
			lastFile = filepath.Base(filename)
			mu.Unlock()
		}
		eng.ProgressFunc = func(current, total int) {
			pct := (current * 100) / total
			mu.Lock()
			lp := lastPct
			f := lastFile
			mu.Unlock()
			if pct != lp {
				mu.Lock()
				lastPct = pct
				mu.Unlock()
				fmt.Fprintf(os.Stderr, "Scanning [%d/%d %d%%] %s\n", current, total, pct, f)
			}
		}
	}

	for _, target := range targets {
		if err := eng.Traverse(target, nil); err != nil {
			return err
		}
	}

	if cfg.ProgBar {
		fmt.Fprintf(os.Stderr, "\n")
	}

	if flags.PrintAST {
		return nil
	}

	sites := eng.Sites()
	sites = FilterSites(sites, targets, cfg, resolver)

	if cfg.ProgBar {
		fmt.Fprintf(os.Stderr, "Found %d mutation sites\n", len(sites))
	}

	baseDir := targets[0]
	if info, err := os.Stat(targets[0]); err == nil && !info.IsDir() {
		baseDir = filepath.Dir(targets[0])
	}

	ctx := context.Background()

	var c *cache.Cache
	if cfg.Cache {
		c, err = cache.Load(baseDir)
		if err != nil {
			return err
		}
	}

	debugFilePath := ""
	if cfg.Debug {
		if cfg.Output != "" {
			ext := filepath.Ext(cfg.Output)
			base := strings.TrimSuffix(cfg.Output, ext)
			debugFilePath = base + ".debug" + ext
		} else {
			debugFilePath = "gorgon-debug.txt"
		}
	}

	tests, testPaths, err := extractTests(cfg.Tests)

	if err != nil {
		return err
	}

	log := logger.New(cfg.Debug)
	if debugFilePath != "" {
		f, err := os.Create(debugFilePath)
		if err == nil {
			log.SetDebugFile(f)
			defer f.Close()
		}
	}

	if resolver.HasAnyOverrides() {
		log.Info("Loaded sub-configs from %d directories", resolver.Entries())
	}

	if cfg.Diff != "" {
		changedLines, err := diff.Resolve(cfg.Diff)
		if err != nil {
			log.Warn("failed to resolve diff %q: %v", cfg.Diff, err)
			return fmt.Errorf("failed to resolve diff %q: %w", cfg.Diff, err)
		}
		if changedLines != nil {
			sites = FilterSitesByDiff(sites, changedLines)
			log.Info("Diff filter: %d files with changes, %d mutation sites after filtering", len(changedLines), len(sites))
			if cfg.ProgBar {
				fmt.Fprintf(os.Stderr, "Diff filter: %d mutation sites after filtering\n", len(sites))
			}
		}
	}

	if cfg.DryRun {
		allOps := mutator.ListAll()
		mutants := testing.GenerateMutants(sites, ops, allOps, projectRoot, cfg.DirRules, resolver, log)
		fmt.Printf("Total mutants: %d\n\n", len(mutants))
		for _, m := range mutants {
			fmt.Printf("#%d %s:%d:%d (%s)\n", m.ID, m.Site.File.Name(), m.Site.Line, m.Site.Column, m.Operator.Name())
		}
		return nil
	}

	allOps := mutator.ListAll()
	if cfg.ExternalSuites.Enabled {
		log.Debug("External suites enabled with %d suites", len(cfg.ExternalSuites.Suites))
	}
	mutants, err := testing.GenerateAndRunSchemata(ctx, sites, ops, allOps, baseDir, projectRoot, cfg.DirRules, resolver, concurrent, c, tests, testPaths, log, cfg.ProgBar, cfg.UnitTestsEnabled, cfg.ExternalSuites)
	totalMutants := testing.GetTotalMutants()

	if len(mutants) > 0 {
		if reportErr := reporter.Report(mutants, totalMutants, cfg.Threshold, resolver, cfg.Debug, cfg.ShowKilled, cfg.ShowSurvived, cfg.Output, debugFilePath, cfg.Format); reportErr != nil {
			if cfg.Cache {
				path, pathErr := cache.Path(baseDir)
				if pathErr == nil {
					fmt.Printf("\nCache stored at: %s\n", path)
				}
			}
			return reportErr
		}
	}

	if err != nil {
		return err
	}

	if cfg.Cache {
		path, err := cache.Path(baseDir)
		if err == nil {
			fmt.Printf("\nCache stored at: %s\n", path)
		}
	}

	suppressions.SyncSuppressions(configPath, eng)
	return nil
}

func FilterSites(sites []engine.Site, targets []string, cfg *config.Config, resolver *subconfig.Resolver) []engine.Site {
	var filtered []engine.Site
	for _, site := range sites {
		filePath := site.File.Name()

		// Get effective filters for this specific file
		var exclude, include, skip, skipFunc []string
		if resolver != nil && resolver.HasAnyOverrides() {
			exclude, include, skip, skipFunc = resolver.EffectiveFilters(filePath, cfg)
		} else {
			exclude, include, skip, skipFunc = cfg.Exclude, cfg.Include, cfg.Skip, cfg.SkipFunc
		}

		if len(exclude) == 0 && len(include) == 0 && len(skip) == 0 && len(skipFunc) == 0 {
			filtered = append(filtered, site)
			continue
		}

		relPath := filePath
		cwdRel := filePath

		for _, target := range targets {
			if abs, err := filepath.Abs(target); err == nil {
				if r, err := filepath.Rel(abs, filePath); err == nil {
					relPath = r
					break
				}
			}
		}

		if cwd, err := os.Getwd(); err == nil {
			if r, err := filepath.Rel(cwd, filePath); err == nil {
				cwdRel = r
			}
		}

		if shouldSkip(relPath, cwdRel, skip) {
			continue
		}

		skipFuncMap := make(map[string]map[string]bool)
		for _, sf := range skipFunc {
			parts := strings.SplitN(sf, ":", 2)
			if len(parts) == 2 {
				file, name := parts[0], parts[1]
				if skipFuncMap[name] == nil {
					skipFuncMap[name] = make(map[string]bool)
				}
				skipFuncMap[name][file] = true
			}
		}

		if shouldSkipFunc(site.FunctionName, relPath, cwdRel, skipFuncMap) {
			continue
		}

		if len(include) > 0 && !matchesAny(relPath, include) {
			continue
		}

		if matchesAny(relPath, exclude) {
			continue
		}

		filtered = append(filtered, site)
	}
	return filtered
}

func FilterSitesByDiff(sites []engine.Site, changedLines diff.FileLines) []engine.Site {
	if changedLines == nil {
		return sites
	}
	filtered := make([]engine.Site, 0, len(sites))
	for _, site := range sites {
		absPath, err := filepath.Abs(site.File.Name())
		if err != nil {
			absPath = site.File.Name()
		}
		if lines, ok := changedLines[absPath]; ok && lines[site.Line] {
			filtered = append(filtered, site)
		}
	}
	return filtered
}

func shouldSkip(relPath, cwdRel string, skipPatterns []string) bool {
	for _, s := range skipPatterns {
		if relPath == s || cwdRel == s {
			return true
		}
		if ok, _ := filepath.Match(s, relPath); ok {
			return true
		}
		if ok, _ := filepath.Match(s, cwdRel); ok {
			return true
		}

		if matchParentDirs(relPath, s) || matchParentDirs(cwdRel, s) {
			return true
		}
	}
	return false
}

func matchParentDirs(path, pattern string) bool {
	dir := path
	for dir != "." && dir != "/" {
		if dir == pattern {
			return true
		}
		dir = filepath.Dir(dir)
	}
	return false
}

func shouldSkipFunc(funcName, relPath, cwdRel string, skipFuncMap map[string]map[string]bool) bool {
	if funcName == "" {
		return false
	}

	files, exists := skipFuncMap[funcName]
	if !exists {
		return false
	}

	if files[""] {
		return true
	}

	for file := range files {
		if relPath == file || cwdRel == file || filepath.Base(relPath) == file {
			return true
		}
	}
	return false
}

func matchesAny(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if ok, _ := filepath.Match(pattern, filepath.Base(path)); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, path); ok {
			return true
		}
	}
	return false
}

func extractTests(testPaths []string) ([]string, []string, error) {
	var tests []string
	var paths []string
	for _, p := range testPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		names, err := extractTestNames(p)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse tests from %s: %w", p, err)
		}
		tests = append(tests, names...)
		paths = append(paths, p)
	}
	return tests, paths, nil
}

func extractTestNames(path string) ([]string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}

	var testFiles []string
	if info.IsDir() {
		entries, err := os.ReadDir(abs)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
				testFiles = append(testFiles, filepath.Join(abs, e.Name()))
			}
		}
	} else {
		testFiles = append(testFiles, abs)
	}

	var names []string
	for _, f := range testFiles {
		names = append(names, parseTestNamesFromFile(f)...)
	}
	return names, nil
}

func parseTestNamesFromFile(filePath string) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, 0)
	if err != nil {
		return nil
	}
	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if strings.HasPrefix(fn.Name.Name, "Test") {
			names = append(names, fn.Name.Name)
		}
	}
	return names
}

func findProjectRoot(target string, configBase string) string {
	if configBase != "" {
		if abs, err := filepath.Abs(configBase); err == nil {
			return abs
		}
		return configBase
	}

	if dir := testing.FindGoModDir(target); dir != "" {
		return dir
	}

	startPath, err := filepath.Abs(target)
	if err != nil {
		return target
	}
	info, err := os.Stat(startPath)
	if err != nil {
		return target
	}
	if !info.IsDir() {
		return filepath.Dir(startPath)
	}
	return startPath
}

func ExitWithError(err error) {
	_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
	os.Exit(1)
}
