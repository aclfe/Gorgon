package runner

import (
	"context"
	"encoding/json"
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
	"github.com/aclfe/gorgon/internal/gowork"
	"github.com/aclfe/gorgon/internal/logger"
	"github.com/aclfe/gorgon/internal/orgpolicy"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/subconfig"
	"github.com/aclfe/gorgon/internal/suppressions"
	"github.com/aclfe/gorgon/internal/badge"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

func Run(flags *cli.Flags, cfg *config.Config, targets []string, configPath string) error {
	if len(targets) == 0 {
		cli.PrintUsage()
	}

	// Apply Go version override if specified in config
	if cfg.GoVersion != "" {
		testing.SetGoVersion(cfg.GoVersion)
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

	// Load org policy — search from project root upward
	policy, err := findAndLoadOrgPolicy(projectRoot)
	if err != nil {
		return fmt.Errorf("failed to load org policy: %w", err)
	}

	// Discover sub-configs with policy if present
	var resolver *subconfig.Resolver
	if policy != nil && !policy.IsZero() {
		resolver, err = subconfig.DiscoverWithPolicy(projectRoot, configPath, policy)
	} else {
		resolver, err = subconfig.Discover(projectRoot, configPath)
	}
	if err != nil {
		return fmt.Errorf("failed to discover sub-configs: %w", err)
	}

	// Apply org policy to root config
	allOps := mutator.ListAll()
	if policy != nil && !policy.IsZero() {
		result := orgpolicy.Apply(cfg, policy, allOps)
		cfg = result.Config
		if len(result.Violations) > 0 && cfg.ViolationMode != config.ViolationSilent {
			fmt.Fprintf(os.Stderr, "Org policy applied %d constraint(s):\n", len(result.Violations))
			for _, v := range result.Violations {
				fmt.Fprintf(os.Stderr, "  %s\n", v.Error())
			}
			if cfg.ViolationMode == config.ViolationFail {
				// Violations are logged but not fatal by default
				// Only fail if explicitly configured
			}
		}
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
		// Extract output from first outputs entry
		output := ""
		if len(cfg.Outputs) > 0 {
			parts := strings.SplitN(cfg.Outputs[0], ":", 2)
			if len(parts) == 2 {
				output = strings.TrimSpace(parts[1])
			}
		}
		if output != "" {
			ext := filepath.Ext(output)
			base := strings.TrimSuffix(output, ext)
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
		mutants := testing.GenerateMutants(sites, ops, allOps, projectRoot, cfg.DirRules, resolver, log)
		fmt.Printf("Total mutants: %d\n\n", len(mutants))
		for _, m := range mutants {
			fmt.Printf("#%d %s:%d:%d (%s)\n", m.ID, m.Site.File.Name(), m.Site.Line, m.Site.Column, m.Operator.Name())
		}
		return nil
	}

	if cfg.ExternalSuites.Enabled {
		log.Debug("External suites enabled with %d suites", len(cfg.ExternalSuites.Suites))
	}
	mutants, err := testing.GenerateAndRunSchemata(ctx, sites, ops, allOps, baseDir, projectRoot, cfg.DirRules, resolver, concurrent, c, tests, testPaths, log, cfg.ProgBar, cfg.UnitTestsEnabled, cfg.ExternalSuites)
	totalMutants := testing.GetTotalMutants()

	if len(mutants) > 0 {
		blOpts := reporter.BaselineOptions{
			Save:         cfg.Baseline.Save,
			NoRegression: flags.NoRegression || cfg.Baseline.NoRegression,
			Tolerance:    flags.BaselineTolerance,
			Dir:          baseDir,
			File:         flags.BaselineFile,
			MultiOutputs: cfg.Outputs,
		}
		// Config fills in when CLI flags weren't explicitly set
		if blOpts.Tolerance == 0 && cfg.Baseline.Tolerance > 0 {
			blOpts.Tolerance = cfg.Baseline.Tolerance
		}
		if blOpts.File == "" && cfg.Baseline.File != "" {
			blOpts.File = cfg.Baseline.File
		}

		// Extract format and output from first outputs entry for backward compatibility
		format := "textfile"
		output := ""
		if len(cfg.Outputs) > 0 {
			parts := strings.SplitN(cfg.Outputs[0], ":", 2)
			if len(parts) == 2 {
				format = strings.TrimSpace(parts[0])
				output = strings.TrimSpace(parts[1])
			}
		}

		// Always write textfile to stdout when using multi-outputs
		writeTextToStdout := len(cfg.Outputs) > 0 && format != "textfile"

		reportErr := reporter.Report(mutants, totalMutants, cfg.Threshold, resolver, cfg.Debug, cfg.ShowKilled, cfg.ShowSurvived, output, debugFilePath, format, blOpts, writeTextToStdout)
		
		// Generate badge even if report had errors (e.g., threshold failure)
		if cfg.Badge != "" {
			if err := generateBadge(cfg.Badge, baseDir); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to generate badge: %v\n", err)
			}
		}
		
		if reportErr != nil {
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

	// Prefer go.work root so the workspace is the authoritative boundary.
	if ws := gowork.Find(target); ws != nil {
		return ws.Root
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

// findAndLoadOrgPolicy walks up from projectRoot looking for gorgon-org.yml.
// Also checks GORGON_ORG_POLICY env var for org-wide installation.
func findAndLoadOrgPolicy(projectRoot string) (*config.OrgPolicy, error) {
	// 1. Explicit env override — highest priority
	if envPath := os.Getenv("GORGON_ORG_POLICY"); envPath != "" {
		return config.LoadOrgPolicy(envPath)
	}

	// 2. Walk up from project root
	dir := projectRoot
	for {
		candidate := filepath.Join(dir, config.OrgPolicyFilename)
		if _, err := os.Stat(candidate); err == nil {
			return config.LoadOrgPolicy(candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 3. XDG config dir (Linux/Mac standard location)
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		candidate := filepath.Join(xdg, "gorgon", config.OrgPolicyFilename)
		if _, err := os.Stat(candidate); err == nil {
			return config.LoadOrgPolicy(candidate)
		}
	}

	// Not found — return zero policy, not an error
	return &config.OrgPolicy{}, nil
}

// GetLastMutationScore reads the last mutation score from baseline file
func GetLastMutationScore(baseDir string) (float64, error) {
	baselinePath := filepath.Join(baseDir, ".gorgon-baseline.json")
	data, err := os.ReadFile(baselinePath)
	if err != nil {
		return 0, fmt.Errorf("baseline file not found: %w", err)
	}
	
	var baseline struct {
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal(data, &baseline); err != nil {
		return 0, fmt.Errorf("failed to parse baseline: %w", err)
	}
	
	return baseline.Score, nil
}

// generateBadge creates a badge file based on the mutation score
func generateBadge(format, baseDir string) error {
	score, err := GetLastMutationScore(baseDir)
	if err != nil {
		return err
	}

	var output string
	var filename string

	switch format {
	case "json":
		output, err = badge.GenerateJSON(score)
		if err != nil {
			return err
		}
		filename = "mutation-badge.json"
	case "svg":
		output = badge.GenerateSVG(score)
		filename = "mutation-badge.svg"
	default:
		return fmt.Errorf("invalid badge format: %s (use 'json' or 'svg')", format)
	}

	outputPath := filepath.Join(baseDir, filename)
	if err := os.WriteFile(outputPath, []byte(output), 0644); err != nil {
		return fmt.Errorf("failed to write badge file: %w", err)
	}

	fmt.Printf("Badge generated: %s\n", outputPath)
	return nil
}

