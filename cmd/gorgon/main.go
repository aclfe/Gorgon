// Package main provides the gorgon command-line tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/internal/cache"
	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
	_ "github.com/aclfe/gorgon/pkg/mutator/assignment_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/boundary_value"
	_ "github.com/aclfe/gorgon/pkg/mutator/conditional_expression"
	_ "github.com/aclfe/gorgon/pkg/mutator/constant_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/defer_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/early_return_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/empty_body"
	_ "github.com/aclfe/gorgon/pkg/mutator/inc_dec_flip"
	_ "github.com/aclfe/gorgon/pkg/mutator/logical_operator"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_body_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_first"
	_ "github.com/aclfe/gorgon/pkg/mutator/loop_break_removal"
	_ "github.com/aclfe/gorgon/pkg/mutator/math_operators"
	_ "github.com/aclfe/gorgon/pkg/mutator/negate_condition"
	_ "github.com/aclfe/gorgon/pkg/mutator/reference_returns"
	_ "github.com/aclfe/gorgon/pkg/mutator/sign_toggle"
	_ "github.com/aclfe/gorgon/pkg/mutator/switch_mutations"
	_ "github.com/aclfe/gorgon/pkg/mutator/variable_replacement"
	_ "github.com/aclfe/gorgon/pkg/mutator/zero_value_return"
)

func main() {
	fs := flag.NewFlagSet("gorgon", flag.ExitOnError)
	configFile := fs.String("config", "", "Path to YAML config file (disables all other flags)")
	printAST := fs.Bool("print-ast", false, "Print AST during traversal")
	pkgPath := fs.String("pkg", ".", "Package path to mutate")
	operatorsFlag := fs.String("operators", "all",
		"Comma-separated operators (e.g. arithmetic_flip,condition_negation)")
	concurrentFlag := fs.String("concurrent", "all", "Max concurrent mutant runners: 'all' (default), 'half', or a number")
	threshold := fs.Float64("threshold", 0, "Minimum mutation score percentage required (0-100)")
	useCache := fs.Bool("cache", false, "Cache mutation results between runs")
	dryRun := fs.Bool("dry-run", false, "Preview mutants without running tests")
	excludeFlag := fs.String("exclude", "", "Comma-separated glob patterns for files to exclude (e.g. *_test.go,vendor/*)")
	includeFlag := fs.String("include", "", "Comma-separated glob patterns for files to include (overrides exclude)")
	skipFlag := fs.String("skip", "", "Comma-separated relative file paths to skip entirely")
	skipFuncFlag := fs.String("skip-func", "", "Comma-separated file:function pairs to skip (e.g. foo/bar.go:MyFunc)")
	testsFlag := fs.String("tests", "", "Comma-separated relative paths to test files/folders")

	fs.Parse(os.Args[1:])

	targets := fs.Args()

	if *configFile != "" {
		if *printAST || *pkgPath != "." || *operatorsFlag != "all" || *concurrentFlag != "all" || *threshold != 0 || *useCache || *dryRun || *excludeFlag != "" || *includeFlag != "" || *skipFlag != "" || *skipFuncFlag != "" || *testsFlag != "" {
			fmt.Fprintln(os.Stderr, "Error: -config cannot be used with other flags")
			os.Exit(1)
		}
		runWithConfig(*configFile, targets)
		return
	}

	if len(targets) == 0 && *pkgPath == "." {
		printUsageAndExit()
	}

	if len(targets) == 0 {
		targets = []string{*pkgPath}
	}

	var ops []mutator.Operator
	if *operatorsFlag == "all" {
		ops = mutator.List()
	} else {
		opNames := strings.Split(*operatorsFlag, ",")
		for _, name := range opNames {
			name = strings.TrimSpace(name)
			if categoryOps, ok := mutator.GetCategory(name); ok {
				ops = append(ops, categoryOps...)
				continue
			}
			op, ok := mutator.Get(name)
			if !ok {
				fmt.Fprintf(os.Stderr, "Unknown operator: %s\n", name)
				os.Exit(1)
			}
			ops = append(ops, op)
		}
	}

	concurrent := parseConcurrent(*concurrentFlag)

	eng := engine.NewEngine(*printAST)
	eng.SetOperators(ops)
	for _, target := range targets {
		if err := eng.Traverse(target, nil); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}

	if *printAST {
		return
	}

	sites := eng.Sites()

	sites = filterSites(sites, targets, *excludeFlag, *includeFlag, *skipFlag, *skipFuncFlag)

	baseDir := targets[0]
	if info, err := os.Stat(targets[0]); err == nil && !info.IsDir() {
		baseDir = filepath.Dir(targets[0])
	}

	if *dryRun {
		mutants := testing.GenerateMutants(sites, ops)
		fmt.Printf("Total mutants: %d\n\n", len(mutants))
		for _, m := range mutants {
			fmt.Printf("#%d %s:%d:%d (%s)\n", m.ID, m.Site.File.Name(), m.Site.Line, m.Site.Column, m.Operator.Name())
		}
		return
	}

	ctx := context.Background()

	var mutants []testing.Mutant
	var err error

	var c *cache.Cache
	if *useCache {
		c, err = cache.Load(baseDir)
		if err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}

	var tests []string
	if *testsFlag != "" {
		for _, p := range strings.Split(*testsFlag, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			names, err := extractTestNames(p)
			if err != nil {
				_, _ = os.Stderr.WriteString(fmt.Sprintf("failed to parse tests from %s: %v\n", p, err))
				os.Exit(1)
			}
			tests = append(tests, names...)
		}
	}

	mutants, err = testing.RunMutants(ctx, sites, ops, baseDir, concurrent, c, tests)
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	if err := reporter.Report(mutants, *threshold); err != nil {
		if *useCache {
			path, err := cache.Path(baseDir)
			if err == nil {
				fmt.Printf("\nCache stored at: %s\n", path)
			}
		}
		_, _ = fmt.Fprintf(os.Stderr, "Report failed: %v\n", err)
		os.Exit(1)
	}

	if *useCache {
		path, err := cache.Path(baseDir)
		if err == nil {
			fmt.Printf("\nCache stored at: %s\n", path)
		}
	}
}

func runWithConfig(configPath string, targets []string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		_, _ = os.Stderr.WriteString(fmt.Sprintf("failed to load config: %v\n", err))
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		_, _ = os.Stderr.WriteString(fmt.Sprintf("invalid config: %v\n", err))
		os.Exit(1)
	}

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "Error: config mode requires a target path")
		os.Exit(1)
	}

	var ops []mutator.Operator
	if len(cfg.Operators) == 0 || (len(cfg.Operators) == 1 && cfg.Operators[0] == "all") {
		ops = mutator.List()
	} else {
		for _, name := range cfg.Operators {
			name = strings.TrimSpace(name)
			if categoryOps, ok := mutator.GetCategory(name); ok {
				ops = append(ops, categoryOps...)
				continue
			}
			op, ok := mutator.Get(name)
			if !ok {
				fmt.Fprintf(os.Stderr, "Unknown operator: %s\n", name)
				os.Exit(1)
			}
			ops = append(ops, op)
		}
	}

	concurrent := parseConcurrent(cfg.Concurrent)

	eng := engine.NewEngine(false)
	eng.SetOperators(ops)
	for _, target := range targets {
		if err := eng.Traverse(target, nil); err != nil {
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}

	sites := eng.Sites()

	exclude := strings.Join(cfg.Exclude, ",")
	include := strings.Join(cfg.Include, ",")
	skip := strings.Join(cfg.Skip, ",")
	skipFunc := strings.Join(cfg.SkipFunc, ",")
	sites = filterSites(sites, targets, exclude, include, skip, skipFunc)

	baseDir := targets[0]
	if info, err := os.Stat(targets[0]); err == nil && !info.IsDir() {
		baseDir = filepath.Dir(targets[0])
	}

	if cfg.DryRun {
		mutants := testing.GenerateMutants(sites, ops)
		fmt.Printf("Total mutants: %d\n\n", len(mutants))
		for _, m := range mutants {
			fmt.Printf("#%d %s:%d:%d (%s)\n", m.ID, m.Site.File.Name(), m.Site.Line, m.Site.Column, m.Operator.Name())
		}
		return
	}

	ctx := context.Background()

	var mutants []testing.Mutant
	var runErr error

	var c *cache.Cache
	if cfg.Cache {
		c, runErr = cache.Load(baseDir)
		if runErr != nil {
			_, _ = os.Stderr.WriteString(runErr.Error() + "\n")
			os.Exit(1)
		}
	}

	var tests []string
	for _, p := range cfg.Tests {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		names, runErr := extractTestNames(p)
		if runErr != nil {
			_, _ = os.Stderr.WriteString(fmt.Sprintf("failed to parse tests from %s: %v\n", p, runErr))
			os.Exit(1)
		}
		tests = append(tests, names...)
	}

	mutants, runErr = testing.RunMutants(ctx, sites, ops, baseDir, concurrent, c, tests)
	if runErr != nil {
		_, _ = os.Stderr.WriteString(runErr.Error() + "\n")
		os.Exit(1)
	}

	if err := reporter.Report(mutants, cfg.Threshold); err != nil {
		if cfg.Cache {
			path, err := cache.Path(baseDir)
			if err == nil {
				fmt.Printf("\nCache stored at: %s\n", path)
			}
		}
		_, _ = fmt.Fprintf(os.Stderr, "Report failed: %v\n", err)
		os.Exit(1)
	}

	if cfg.Cache {
		path, err := cache.Path(baseDir)
		if err == nil {
			fmt.Printf("\nCache stored at: %s\n", path)
		}
	}
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
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Test") {
				names = append(names, fn.Name.Name)
			}
		}
	}
	return names, nil
}

func parseConcurrent(flag string) int {
	switch flag {
	case "all":
		return runtime.NumCPU()
	case "half":
		n := runtime.NumCPU() / 2
		if n < 1 {
			n = 1
		}
		return n
	default:
		n, err := strconv.Atoi(flag)
		if err != nil || n < 1 {
			return runtime.NumCPU()
		}
		return n
	}
}

func printUsageAndExit() {
	fmt.Fprintln(os.Stderr, "Usage: gorgon [flags] <path>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  <path>   File or directory to mutate (e.g. examples/mutations)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -config string        path to YAML config file (disables all other flags)")
	fmt.Fprintln(os.Stderr, "  -concurrent string    max parallel test runs: 'all' (default), 'half', or a number")
	fmt.Fprintln(os.Stderr, "  -operators string     all (default) or comma-separated list")
	fmt.Fprintln(os.Stderr, "  -print-ast            only print AST")
	fmt.Fprintln(os.Stderr, "  -threshold float      minimum mutation score percentage (0-100)")
	fmt.Fprintln(os.Stderr, "  -cache                cache mutation results between runs")
	fmt.Fprintln(os.Stderr, "  -dry-run              preview mutants without running tests")
	fmt.Fprintln(os.Stderr, "  -exclude string       comma-separated glob patterns for files to exclude")
	fmt.Fprintln(os.Stderr, "  -include string       comma-separated glob patterns for files to include")
	fmt.Fprintln(os.Stderr, "  -skip string          comma-separated relative file paths to skip entirely")
	fmt.Fprintln(os.Stderr, "  -skip-func string     comma-separated file:function pairs to skip (e.g. foo/bar.go:MyFunc)")
	fmt.Fprintln(os.Stderr, "  -tests string         comma-separated relative paths to test files/folders")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  gorgon examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -concurrent=half examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -concurrent=2 examples/mutations/arithmetic_flip")
	fmt.Fprintln(os.Stderr, "  gorgon -print-ast main.go")
	fmt.Fprintln(os.Stderr, "  gorgon -exclude=\"*_test.go,vendor/*\" ./path")
	fmt.Fprintln(os.Stderr, "  gorgon -config=gorgon.yml ./path")
	os.Exit(1)
}

func filterSites(sites []engine.Site, targets []string, excludePatterns, includePatterns, skipFiles, skipFuncs string) []engine.Site {
	if excludePatterns == "" && includePatterns == "" && skipFiles == "" && skipFuncs == "" {
		return sites
	}

	var exclude []string
	if excludePatterns != "" {
		exclude = strings.Split(excludePatterns, ",")
		for i, p := range exclude {
			exclude[i] = strings.TrimSpace(p)
		}
	}

	var include []string
	if includePatterns != "" {
		include = strings.Split(includePatterns, ",")
		for i, p := range include {
			include[i] = strings.TrimSpace(p)
		}
	}

	var skip []string
	if skipFiles != "" {
		skip = strings.Split(skipFiles, ",")
		for i, p := range skip {
			skip[i] = strings.TrimSpace(p)
		}
	}

	type skipFunc struct {
		file string
		name string
	}
	var skipFuncList []skipFunc
	if skipFuncs != "" {
		for _, part := range strings.Split(skipFuncs, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			parts := strings.SplitN(part, ":", 2)
			if len(parts) == 2 {
				skipFuncList = append(skipFuncList, skipFunc{file: parts[0], name: parts[1]})
			}
		}
	}

	var filtered []engine.Site
	for _, site := range sites {
		filePath := site.File.Name()
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

		skipThis := false
		for _, s := range skip {
			if relPath == s || cwdRel == s {
				skipThis = true
				break
			}
			if ok, _ := filepath.Match(s, relPath); ok {
				skipThis = true
				break
			}
			if ok, _ := filepath.Match(s, cwdRel); ok {
				skipThis = true
				break
			}
			dir := relPath
			for dir != "." && dir != "/" {
				if dir == s {
					skipThis = true
					break
				}
				dir = filepath.Dir(dir)
			}
			if !skipThis {
				dir = cwdRel
				for dir != "." && dir != "/" {
					if dir == s {
						skipThis = true
						break
					}
					dir = filepath.Dir(dir)
				}
			}
			if skipThis {
				break
			}
		}
		if skipThis {
			continue
		}

		if len(skipFuncList) > 0 && site.FunctionName != "" {
			for _, sf := range skipFuncList {
				if site.FunctionName == sf.name {
					if sf.file == "" || relPath == sf.file || cwdRel == sf.file || filepath.Base(relPath) == sf.file {
						skipThis = true
						break
					}
				}
			}
			if skipThis {
				continue
			}
		}

		if len(include) > 0 {
			matched := false
			for _, pattern := range include {
				if ok, _ := filepath.Match(pattern, filepath.Base(relPath)); ok {
					matched = true
					break
				}
				if ok, _ := filepath.Match(pattern, relPath); ok {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		if len(exclude) > 0 {
			skip := false
			for _, pattern := range exclude {
				if ok, _ := filepath.Match(pattern, filepath.Base(relPath)); ok {
					skip = true
					break
				}
				if ok, _ := filepath.Match(pattern, relPath); ok {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
		}

		filtered = append(filtered, site)
	}
	return filtered
}
