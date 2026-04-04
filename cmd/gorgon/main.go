// Package main provides the gorgon command-line tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/testing"
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
	printAST := fs.Bool("print-ast", false, "Print AST during traversal")
	pkgPath := fs.String("pkg", ".", "Package path to mutate")
	operatorsFlag := fs.String("operators", "all",
		"Comma-separated operators (e.g. arithmetic_flip,condition_negation)")

	concurrentFlag := fs.String("concurrent", "all", "Max concurrent mutant runners: 'all' (default), 'half', or a number")

	fs.Parse(os.Args[1:])

	targets := fs.Args()
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
			//nolint:errcheck
			_, _ = os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
	}

	if *printAST {
		return
	}

	sites := eng.Sites()

	baseDir := targets[0]
	if info, err := os.Stat(targets[0]); err == nil && !info.IsDir() {
		baseDir = filepath.Dir(targets[0])
	}

	ctx := context.Background()

	var mutants []testing.Mutant
	var err error

	mutants, err = testing.RunMutants(ctx, sites, ops, baseDir, concurrent)
	if err != nil {
		//nolint:errcheck
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	if err := reporter.Report(mutants); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Report failed: %v\n", err)
		os.Exit(1)
	}
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
	fmt.Fprintln(os.Stderr, "  -concurrent string    max parallel test runs: 'all' (default), 'half', or a number")
	fmt.Fprintln(os.Stderr, "  -operators string     all (default) or comma-separated list")
	fmt.Fprintln(os.Stderr, "  -print-ast            only print AST")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  gorgon examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -concurrent=half examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -concurrent=2 examples/mutations/arithmetic_flip")
	fmt.Fprintln(os.Stderr, "  gorgon -print-ast main.go")
	os.Exit(1)
}
