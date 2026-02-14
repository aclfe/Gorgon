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
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"
)

// Visitor is a function that visits AST nodes. Return true to continue traversal, false to stop.
// Linter forcing these comments.
type Visitor func(n ast.Node) bool

type Engine struct {
	PrintAST bool
	sites    []Site
	mu       sync.Mutex
}

func NewEngine(printAST bool) *Engine {
	return &Engine{PrintAST: printAST}
}

//nolint:gocognit,cyclop
func (e *Engine) Traverse(path string, visitor Visitor) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path %q: %w", path, err)
	}

	if !info.IsDir() {
		if filepath.Ext(path) != ".go" {
			return nil
		}
		return e.traverseSingleFile(path, visitor)
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
				for _, syntax := range pkg.Syntax {
					tfile := pkg.Fset.File(syntax.Pos())
					if e.PrintAST {
						PrintEnabled = true
						fmt.Printf("\n=== AST for %s ===\n", tfile.Name())
						if err := PrintTree(os.Stdout, pkg.Fset, syntax); err != nil {
							return err
						}
						fmt.Println("=====================================")
					}
					ast.Inspect(syntax, func(node ast.Node) bool {
						if binaryExpr, ok := node.(*ast.BinaryExpr); ok {
							//nolint:exhaustive
							switch binaryExpr.Op {
							case token.ADD, token.SUB, token.MUL, token.QUO,
								token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
								e.mu.Lock()
								opLen := len(binaryExpr.Op.String())
								e.sites = append(e.sites, Site{
									File: tfile,
									Pos:  binaryExpr.OpPos,
									End:  binaryExpr.OpPos + token.Pos(opLen),
									Node: binaryExpr,
								})
								e.mu.Unlock()
							default:
								// no-op
							}
						}
						if visitor != nil {
							return visitor(node)
						}
						return true
					})
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

func (e *Engine) traverseSingleFile(path string, visitor Visitor) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file %q: %w", path, err)
	}

	tfile := fset.File(file.Pos())
	if e.PrintAST {
		PrintEnabled = true
		fmt.Printf("\n=== AST for %s ===\n", path)
		if err := PrintTree(os.Stdout, fset, file); err != nil {
			return err
		}
		fmt.Println("=====================================")
	}

	ast.Inspect(file, func(node ast.Node) bool {
		//nolint:exhaustive
		if binaryExpr, ok := node.(*ast.BinaryExpr); ok {
			switch binaryExpr.Op {
			case token.ADD, token.SUB, token.MUL, token.QUO,
				token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
				e.mu.Lock()
				opLen := len(binaryExpr.Op.String())
				e.sites = append(e.sites, Site{
					File: tfile,
					Pos:  binaryExpr.OpPos,
					End:  binaryExpr.OpPos + token.Pos(opLen),
					Node: binaryExpr,
				})
				e.mu.Unlock()
			default:
				// no-op
			}
		}
		// I'll add more node types here later (e.g., if statements, loops)
		if visitor != nil {
			return visitor(node)
		}
		return true
	})
	return nil
}

func (e *Engine) Sites() []Site {
	return e.sites
}
