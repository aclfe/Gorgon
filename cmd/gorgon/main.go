// Package main provides the gorgon command-line tool for visualizing Go AST structures.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aclfe/gorgon/internal/engine"
)

func main() {
	var (
		printAST bool
		output   string
	)

	flag.BoolVar(&printAST, "print-ast", false, "Print the AST tree structure")
	flag.StringVar(&output, "o", "", "Output file path for the AST tree (default: stdout)")
	flag.StringVar(&output, "output", "", "Output file path for the AST tree (alias for -o)")

	flag.Parse()

	files := flag.Args()
	if len(files) == 0 {
		printUsageAndExit()
	}

	if !printAST {
		fmt.Fprintln(os.Stderr, "Nothing to do (use -print-ast to print AST)")
		os.Exit(0)
	}

	for i, path := range files {
		if i > 0 {
			fmt.Println("\n" + strings.Repeat("─", 60) + "\n")
		}
		processFile(path, output)
	}

	if output != "" {
		fmt.Fprintf(os.Stderr, "\nAST written to: %s\n", output)
	}
}

func printUsageAndExit() {
	fmt.Fprintln(os.Stderr, "Usage: gorgon [flags] <file.go> [file2.go ...]")
	fmt.Fprintln(os.Stderr, "Example:")
	fmt.Fprintln(os.Stderr, "  gorgon -print-ast main.go")
	fmt.Fprintln(os.Stderr, "  gorgon -print-ast -o output.txt testdata/minimal.go")
	os.Exit(1)
}

func processFile(path, output string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse %s: %v\n", path, err)
		return
	}

	writer, closeFn := getWriter(output, path)
	defer func() {
		if err := closeFn(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close output file: %v\n", err)
		}
	}()

	engine.PrintEnabled = true
	if err := engine.PrintTree(writer, fset, file); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to print AST: %v\n", err)
	}
}

func getWriter(output, path string) (io.Writer, func() error) {
	if output == "" {
		fmt.Printf("=== AST for %s ===\n", path)
		return os.Stdout, func() error { return nil }
	}

	// G304: file path is from user-provided command-line argument, which is expected
	const fileMode = 0o600
	//nolint:gosec
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

// Helper function to suppress unused import warning during refactoring
var _ ast.Node
