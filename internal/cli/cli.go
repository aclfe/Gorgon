package cli

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

type Flags struct {
	ConfigFile   string
	PrintAST     bool
	PkgPath      string
	Operators    string
	Concurrent   string
	Threshold    float64
	UseCache     bool
	DryRun       bool
	Debug        bool
	ProgBar      bool
	ShowKilled   bool
	ShowSurvived bool
	Format       string
	Output       string
	CPUProfile   string
	Exclude      string
	Include      string
	Skip         string
	SkipFunc     string
	Tests        string
	Targets      []string
}

func Parse(args []string) (*Flags, error) {
	fs := flag.NewFlagSet("gorgon", flag.ContinueOnError)

	f := &Flags{}
	fs.StringVar(&f.ConfigFile, "config", "", "Path to YAML config file (disables all other flags)")
	fs.BoolVar(&f.PrintAST, "print-ast", false, "Print AST during traversal")
	fs.StringVar(&f.PkgPath, "pkg", ".", "Package path to mutate")
	fs.StringVar(&f.Operators, "operators", "all", "Comma-separated operators (e.g. arithmetic_flip,condition_negation)")
	fs.StringVar(&f.Concurrent, "concurrent", "all", "Max concurrent mutant runners: 'all' (default), 'half', or a number")
	fs.Float64Var(&f.Threshold, "threshold", 0, "Minimum mutation score percentage required (0-100)")
	fs.BoolVar(&f.UseCache, "cache", false, "Cache mutation results between runs")
	fs.BoolVar(&f.DryRun, "dry-run", false, "Preview mutants without running tests")
	fs.StringVar(&f.Exclude, "exclude", "", "Comma-separated glob patterns for files to exclude")
	fs.StringVar(&f.Include, "include", "", "Comma-separated glob patterns for files to include")
	fs.StringVar(&f.Skip, "skip", "", "Comma-separated relative file paths to skip entirely")
	fs.StringVar(&f.SkipFunc, "skip-func", "", "Comma-separated file:function pairs to skip")
	fs.StringVar(&f.Tests, "tests", "", "Comma-separated relative paths to test files/folders")
	fs.BoolVar(&f.Debug, "debug", false, "Enable full debug output (console + {output}.debug.txt or gorgon-debug.txt)")
	fs.BoolVar(&f.ProgBar, "progbar", false, "Show progress percentage during execution")
	fs.BoolVar(&f.ShowKilled, "show-killed", false, "Show killed mutants with test attribution")
	fs.BoolVar(&f.ShowSurvived, "show-survived", false, "Show survived mutants in output")
	fs.StringVar(&f.Format, "format", "textfile", "Output format for report file (textfile)")
	fs.StringVar(&f.Output, "output", "", "Write report to file (e.g. report.txt)")
	fs.StringVar(&f.CPUProfile, "cpu-profile", "", "Write CPU profile to file (analyzable with go tool pprof)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	f.Targets = fs.Args()
	return f, nil
}

func (f *Flags) ValidateChecks() error {
	if f.ConfigFile != "" && (f.PrintAST || f.PkgPath != "." || f.Operators != "all" ||
		f.Concurrent != "all" || f.Threshold != 0 || f.UseCache || f.DryRun ||
		f.ProgBar || f.CPUProfile != "" || f.Exclude != "" || f.Include != "" || f.Skip != "" || f.SkipFunc != "" || f.Tests != "") {
		return fmt.Errorf("Error: -config cannot be used with other flags")
	}
	return nil
}

func (f *Flags) LoadConfig() (*config.Config, error) {
	if f.ConfigFile != "" {
		cfg, err := config.Load(f.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
		if err := cfg.Validate(); err != nil {
			return nil, fmt.Errorf("invalid config: %w", err)
		}
		return cfg, nil
	}

	cfg := config.Default()
	if f.Operators != "all" {
		cfg.Operators = strings.Split(f.Operators, ",")
	}
	if f.Concurrent != "all" {
		cfg.Concurrent = f.Concurrent
	}
	cfg.Threshold = f.Threshold
	cfg.Cache = f.UseCache
	cfg.DryRun = f.DryRun
	if f.Exclude != "" {
		cfg.Exclude = splitAndTrim(f.Exclude)
	}
	if f.Include != "" {
		cfg.Include = splitAndTrim(f.Include)
	}
	if f.Skip != "" {
		cfg.Skip = splitAndTrim(f.Skip)
	}
	if f.SkipFunc != "" {
		cfg.SkipFunc = splitAndTrim(f.SkipFunc)
	}
	if f.Tests != "" {
		cfg.Tests = splitAndTrim(f.Tests)
	}
	cfg.Debug = f.Debug
	cfg.ProgBar = f.ProgBar
	cfg.ShowKilled = f.ShowKilled
	cfg.ShowSurvived = f.ShowSurvived
	cfg.Format = f.Format
	cfg.Output = f.Output
	cfg.CPUProfile = f.CPUProfile
	return cfg, nil
}

func ParseOperators(cfg *config.Config) ([]mutator.Operator, error) {
	if len(cfg.Operators) == 0 || (len(cfg.Operators) == 1 && cfg.Operators[0] == "all") {
		return mutator.List(), nil
	}

	var ops []mutator.Operator
	for _, name := range cfg.Operators {
		name = strings.TrimSpace(name)
		if categoryOps, ok := mutator.GetCategory(name); ok {
			ops = append(ops, categoryOps...)
			continue
		}
		op, ok := mutator.Get(name)
		if !ok {
			return nil, fmt.Errorf("unknown operator: %s", name)
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func ParseConcurrent(val string) int {
	switch val {
	case "all":
		return runtime.NumCPU()
	case "half":
		n := runtime.NumCPU() / 2
		if n < 1 {
			n = 1
		}
		return n
	default:
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			return runtime.NumCPU()
		}
		return n
	}
}

func PrintUsage() {
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
	fmt.Fprintln(os.Stderr, "  -debug                enable full debug output (console + {output}.debug.txt or gorgon-debug.txt)")
	fmt.Fprintln(os.Stderr, "  -progbar              show progress percentage during execution")
	fmt.Fprintln(os.Stderr, "  -cpu-profile string   write CPU profile to file (go tool pprof)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  gorgon examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -debug examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -output=report.txt -debug examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -concurrent=half examples/mutations")
	os.Exit(1)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
