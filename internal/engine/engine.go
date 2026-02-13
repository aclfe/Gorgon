// Package engine provides Go AST traversal and visualization functionality.
// Linter forcing these comments.
package engine

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
)

// Visitor is a function that visits AST nodes. Return true to continue traversal, false to stop.
// Linter forcing these comments.
type Visitor func(n ast.Node) bool

// Traverse walks through Go source files and visits each AST node using the provided visitor function.
// Linter forcing these comments.
// If path is a file, it processes that single file. If path is a directory, it recursively processes all Go files.
func Traverse(path string, visit Visitor) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path %q: %w", path, err)
	}

	if !info.IsDir() {
		if filepath.Ext(path) != ".go" {
			return nil
		}
		return traverseSingleFile(path, visit)
	}

	cfg := &packages.Config{
		Mode:  packages.NeedFiles | packages.NeedSyntax,
		Dir:   path,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("failed to load packages from %q: %w", path, err)
	}

	grp, ctx := errgroup.WithContext(context.Background())
	grp.SetLimit(runtime.NumCPU() - 1)

	for _, pkg := range pkgs {
		grp.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				for i := range pkg.Syntax {
					ast.Inspect(pkg.Syntax[i], visit)
				}
				return nil
			}
		})
	}

	if err := grp.Wait(); err != nil {
		return fmt.Errorf("error during traversal: %w", err)
	}
	return nil
}

func traverseSingleFile(path string, visit Visitor) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file %q: %w", path, err)
	}

	if PrintEnabled {
		fmt.Fprintf(os.Stderr, "\n=== AST for %s ===\n", path)
		if err := PrintTree(os.Stderr, fset, file); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "=====================================")
	}

	ast.Inspect(file, visit)
	return nil
}
