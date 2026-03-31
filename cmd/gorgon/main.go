// Package main provides the gorgon command-line tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	printAST := flag.Bool("print-ast", false, "Print AST during traversal")
	pkgPath := flag.String("pkg", ".", "Package path to mutate")
	operatorsFlag := flag.String("operators", "all",
		"Comma-separated operators (e.g. arithmetic_flip,condition_negation)")
	concurrent := flag.Int("concurrent", 0, "Max concurrent mutant runners (default: CPU/2)")

	flag.Parse()

	if flag.NArg() == 0 && *pkgPath == "." {
		printUsageAndExit()
	}

	target := *pkgPath
	if flag.NArg() > 0 {
		target = flag.Arg(0)
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

	eng := engine.NewEngine(*printAST)
	eng.SetOperators(ops)
	if err := eng.Traverse(target, nil); err != nil {
		//nolint:errcheck
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	if *printAST {
		return
	}

	sites := eng.Sites()

	baseDir := target
	if info, err := os.Stat(target); err == nil && !info.IsDir() {
		baseDir = filepath.Dir(target)
	}

	ctx := context.Background()

	var mutants []testing.Mutant
	var err error

	mutants, err = testing.RunMutants(ctx, sites, ops, baseDir, *concurrent)
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

func printUsageAndExit() {
	fmt.Fprintln(os.Stderr, "Usage: gorgon [flags] <path>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  <path>   File or directory to mutate (e.g. examples/mutations)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  -mode string          schemata (default, fast) or classic")
	fmt.Fprintln(os.Stderr, "  -operators string     all (default) or comma-separated list")
	fmt.Fprintln(os.Stderr, "  -concurrent int       max parallel test runs")
	fmt.Fprintln(os.Stderr, "  -print-ast            only print AST")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  gorgon examples/mutations")
	fmt.Fprintln(os.Stderr, "  gorgon -mode=classic examples/mutations/arithmetic_flip")
	fmt.Fprintln(os.Stderr, "  gorgon -print-ast main.go")
	os.Exit(1)
}
