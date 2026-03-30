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

	"github.com/aclfe/gorgon/pkg/mutator"
)

// Visitor is a function that visits AST nodes. Return true to continue traversal, false to stop.
// Linter forcing these comments.
type Visitor func(n ast.Node) bool

type Engine struct {
	PrintAST  bool
	sites     []Site
	operators []mutator.Operator
	mu        sync.Mutex
}

func NewEngine(printAST bool) *Engine {
	return &Engine{PrintAST: printAST}
}

func (e *Engine) SetOperators(ops []mutator.Operator) {
	e.operators = ops
}

func getReturnType(node ast.Node, file *ast.File) string {
	retStmt, ok := node.(*ast.ReturnStmt)
	if !ok {
		return ""
	}

	targetFunc := findEnclosingFunc(file, retStmt)
	if targetFunc != nil && targetFunc.Type.Results != nil {
		for _, field := range targetFunc.Type.Results.List {
			return typeToString(field.Type, file)
		}
	}
	return ""
}

func findEnclosingFunc(file *ast.File, node ast.Node) *ast.FuncDecl {
	var targetFunc *ast.FuncDecl

	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && fn.Body != nil {
			for _, stmt := range fn.Body.List {
				if containsNode(stmt, node) {
					targetFunc = fn
					return false
				}
			}
		}
		return true
	})

	return targetFunc
}

func getPackageName(file *ast.File) string {
	if file.Name != nil {
		return file.Name.Name
	}
	return ""
}

func containsNode(parent, target ast.Node) bool {
	found := false
	ast.Inspect(parent, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

func typeToString(t ast.Expr, file *ast.File) string {
	if t == nil {
		return ""
	}
	switch expr := t.(type) {
	case *ast.Ident:
		return resolveTypeName(expr.Name, file)
	case *ast.StarExpr:
		return "*" + typeToString(expr.X, file)
	case *ast.ArrayType:
		if expr.Len == nil {
			return "[]" + typeToString(expr.Elt, file)
		}
		return "[" + expr.Len.(*ast.BasicLit).Value + "]" + typeToString(expr.Elt, file)
	case *ast.MapType:
		return "map[" + typeToString(expr.Key, file) + "]" + typeToString(expr.Value, file)
	case *ast.ChanType:
		return "chan " + typeToString(expr.Value, file)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.SelectorExpr:
		if ident, ok := expr.X.(*ast.Ident); ok {
			return ident.Name + "." + expr.Sel.Name
		}
		return expr.Sel.Name
	case *ast.ParenExpr:
		return typeToString(expr.X, file)
	case *ast.Ellipsis:
		return "..." + typeToString(expr.Elt, file)
	default:
		return ""
	}
}

func resolveTypeName(typeName string, file *ast.File) string {
	for _, decl := range file.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
			for _, spec := range genDecl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == typeName {
						if typeSpec.Type != nil {
							return typeToString(typeSpec.Type, file)
						}
					}
				}
			}
		}
	}
	return typeName
}

func getNodePosition(node ast.Node, fset *token.FileSet) token.Position {
	if be, ok := node.(*ast.BinaryExpr); ok {
		return fset.Position(be.OpPos)
	}
	return fset.Position(node.Pos())
}

func buildContext(node ast.Node, file *ast.File, fset *token.FileSet) mutator.Context {
	ctx := mutator.Context{
		FileName: fset.File(file.Pos()).Name(),
		PackageName: getPackageName(file),
	}

	if retStmt, ok := node.(*ast.ReturnStmt); ok {
		ctx.ReturnType = getReturnType(retStmt, file)
		ctx.EnclosingFunc = findEnclosingFunc(file, retStmt)
		if ctx.EnclosingFunc != nil {
			ctx.FunctionName = ctx.EnclosingFunc.Name.Name
		}
	}

	ctx.Position = getNodePosition(node, fset)

	return ctx
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
						if node == nil {
							return true
						}
						mctx := buildContext(node, syntax, pkg.Fset)
						for _, op := range e.operators {
							apply := false
							if cop, ok := op.(mutator.ContextualOperator); ok {
								apply = cop.CanApplyWithContext(node, mctx)
							} else {
								apply = op.CanApply(node)
							}
							if apply {
								pos := getNodePosition(node, pkg.Fset)
								e.mu.Lock()
								e.sites = append(e.sites, Site{
									File:       tfile,
									Line:       pos.Line,
									Column:     pos.Column,
									Node:       node,
									ReturnType: mctx.ReturnType,
								})
								e.mu.Unlock()
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
		if node == nil {
			return true
		}
		mctx := buildContext(node, file, fset)
		for _, op := range e.operators {
			apply := false
			if cop, ok := op.(mutator.ContextualOperator); ok {
				apply = cop.CanApplyWithContext(node, mctx)
			} else {
				apply = op.CanApply(node)
			}
			if apply {
				pos := getNodePosition(node, fset)
				e.mu.Lock()
				e.sites = append(e.sites, Site{
					File:       tfile,
					Line:       pos.Line,
					Column:     pos.Column,
					Node:       node,
					ReturnType: mctx.ReturnType,
				})
				e.mu.Unlock()
			}
		}
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
