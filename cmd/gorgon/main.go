// Package main provides the gorgon command-line tool.
package main

import (
	"context"
	"flag"
	"fmt"
	"go/ast"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aclfe/gorgon/internal/engine"
	"github.com/aclfe/gorgon/internal/reporter"
	"github.com/aclfe/gorgon/internal/testing"
	"github.com/aclfe/gorgon/pkg/mutator"
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

	eng := engine.NewEngine(*printAST)
	if err := eng.Traverse(target, nil); err != nil {
		//nolint:errcheck
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}

	if *printAST {
		return
	}

	sites := eng.Sites()

	allOps := map[string]mutator.Operator{
		"arithmetic_flip":    mutator.ArithmeticFlip{},
		"condition_negation": mutator.ConditionNegation{},
		// add more here later
	}

	var ops []mutator.Operator
	if *operatorsFlag == "all" {
		for _, op := range allOps {
			ops = append(ops, op)
		}
	} else {
		opNames := strings.Split(*operatorsFlag, ",")
		for _, name := range opNames {
			name = strings.TrimSpace(name)
			op, ok := allOps[name]
			if !ok {
				fmt.Fprintf(os.Stderr, "Unknown operator: %s\n", name)
				os.Exit(1)
			}
			ops = append(ops, op)
		}
	}

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

func getWriter(output, path string) (io.Writer, func() error) {
	if output == "" {
		fmt.Printf("=== AST for %s ===\n", path)
		return os.Stdout, func() error { return nil }
	}

	const fileMode = 0o600
	//nolint:gosec // Writing to user-provided output file
	fileOut, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, fileMode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open output file %s: %v\n", output, err)
		os.Exit(1)
	}

	if _, err := fmt.Fprintf(fileOut, "=== AST for %s ===\n", filepath.Base(path)); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write header: %v\n", err)
	}

	return fileOut, fileOut.Close
}

var _ ast.Node
