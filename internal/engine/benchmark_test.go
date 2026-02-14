// Package engine_test provides benchmarks for the engine package.
package engine_test

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"testing"

	"github.com/aclfe/gorgon/internal/engine"
)

func BenchmarkTraverseTreeGo(b *testing.B) {
	// Benchmark the Traverse function on tree.go (a realistic workload)
	// We use the file path directly.
	path := "tree.go"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e := engine.NewEngine(false)
		err := e.Traverse(path, func(_ ast.Node) bool {
			return true
		})
		if err != nil {
			b.Fatalf("Traverse failed: %v", err)
		}
	}
}

//nolint:varnamelen // short variable names are idiomatic in benchmarks
func BenchmarkPrintTreePerformance(b *testing.B) {
	// Benchmark the PrintTree function.
	// We parse the file once, then benchmark just the printing.
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "tree.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	engine.PrintEnabled = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := engine.PrintTree(io.Discard, fset, f); err != nil {
			b.Fatalf("PrintTree failed: %v", err)
		}
	}
}

//nolint:varnamelen // short variable names are idiomatic in benchmarks
func BenchmarkGoPrinter(b *testing.B) {
	// Benchmark the standard library's printer.Fprint
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "tree.go", nil, parser.ParseComments)
	if err != nil {
		b.Fatalf("ParseFile failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := printer.Fprint(io.Discard, fset, f); err != nil {
			b.Fatalf("Fprint failed: %v", err)
		}
	}
}

func BenchmarkGoParseInspect(b *testing.B) {
	// Benchmark raw parser.ParseFile + ast.Inspect (Baseline)
	path := "tree.go"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			b.Fatalf("ParseFile failed: %v", err)
		}
		ast.Inspect(f, func(_ ast.Node) bool {
			return true
		})
	}
}
