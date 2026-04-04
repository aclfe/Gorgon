package engine

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"

	"github.com/aclfe/gorgon/pkg/mutator"
)

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

func findEnclosingFuncFast(node ast.Node, parents map[ast.Node]ast.Node) *ast.FuncDecl {
	for p := parents[node]; p != nil; p = parents[p] {
		if fn, ok := p.(*ast.FuncDecl); ok {
			return fn
		}
	}
	return nil
}

func getPackageName(file *ast.File) string {
	if file.Name != nil {
		return file.Name.Name
	}
	return ""
}

func typeToString(t ast.Expr, file *ast.File, fset *token.FileSet) string {
	if t == nil {
		return ""
	}
	switch expr := t.(type) {
	case *ast.Ident:
		return resolveTypeName(expr.Name, file, fset)
	case *ast.StarExpr:
		return "*" + typeToString(expr.X, file, fset)
	case *ast.ArrayType:
		if expr.Len == nil {
			return "[]" + typeToString(expr.Elt, file, fset)
		}
		return "[" + exprToString(expr.Len, fset) + "]" + typeToString(expr.Elt, file, fset)
	case *ast.MapType:
		return "map[" + typeToString(expr.Key, file, fset) + "]" + typeToString(expr.Value, file, fset)
	case *ast.ChanType:
		return "chan " + typeToString(expr.Value, file, fset)
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
		return typeToString(expr.X, file, fset)
	case *ast.Ellipsis:
		return "..." + typeToString(expr.Elt, file, fset)
	default:
		return ""
	}
}

func exprToString(expr ast.Expr, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "?"
	}
	return buf.String()
}

func resolveTypeName(typeName string, file *ast.File, fset *token.FileSet) string {
	if file == nil {
		return typeName
	}
	var resolved string
	ast.Inspect(file, func(n ast.Node) bool {
		if decl, ok := n.(*ast.GenDecl); ok && decl.Tok == token.TYPE {
			for _, spec := range decl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					if typeSpec.Name.Name == typeName {
						if typeSpec.Type != nil {
							resolved = typeToString(typeSpec.Type, file, fset)
							return false
						}
					}
				}
			}
		}
		return true
	})
	if resolved != "" {
		return resolved
	}
	return typeName
}

func getNodePosition(node ast.Node, fset *token.FileSet) token.Position {
	if be, ok := node.(*ast.BinaryExpr); ok {
		return fset.Position(be.OpPos)
	}
	if ids, ok := node.(*ast.IncDecStmt); ok {
		return fset.Position(ids.TokPos)
	}
	return fset.Position(node.Pos())
}

type contextCache struct {
	contexts map[ast.Node]*mutator.Context
}

func newContextCache() *contextCache {
	return &contextCache{
		contexts: make(map[ast.Node]*mutator.Context),
	}
}

func (c *contextCache) get(node ast.Node) (*mutator.Context, bool) {
	ctx, ok := c.contexts[node]
	return ctx, ok
}

func (c *contextCache) set(node ast.Node, ctx *mutator.Context) {
	c.contexts[node] = ctx
}

func buildParentMap(file *ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	stack = append(stack, nil)

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			return true
		}
		parent := stack[len(stack)-1]
		parents[node] = parent
		stack = append(stack, node)
		return true
	})

	return parents
}

func getAncestorOfType(node ast.Node, targetType string, parents map[ast.Node]ast.Node) ast.Node {
	for p := parents[node]; p != nil; p = parents[p] {
		if typeOf(p) == targetType {
			return p
		}
	}
	return nil
}

func typeOf(n ast.Node) string {
	switch n.(type) {
	case *ast.BinaryExpr:
		return "*ast.BinaryExpr"
	case *ast.UnaryExpr:
		return "*ast.UnaryExpr"
	case *ast.CallExpr:
		return "*ast.CallExpr"
	case *ast.Ident:
		return "*ast.Ident"
	case *ast.CaseClause:
		return "*ast.CaseClause"
	case *ast.IfStmt:
		return "*ast.IfStmt"
	case *ast.ForStmt:
		return "*ast.ForStmt"
	case *ast.RangeStmt:
		return "*ast.RangeStmt"
	case *ast.AssignStmt:
		return "*ast.AssignStmt"
	case *ast.IncDecStmt:
		return "*ast.IncDecStmt"
	case *ast.DeferStmt:
		return "*ast.DeferStmt"
	case *ast.GoStmt:
		return "*ast.GoStmt"
	case *ast.SendStmt:
		return "*ast.SendStmt"
	case *ast.SwitchStmt:
		return "*ast.SwitchStmt"
	case *ast.TypeSwitchStmt:
		return "*ast.TypeSwitchStmt"
	case *ast.ReturnStmt:
		return "*ast.ReturnStmt"
	case *ast.BranchStmt:
		return "*ast.BranchStmt"
	case *ast.SelectStmt:
		return "*ast.SelectStmt"
	case *ast.CommClause:
		return "*ast.CommClause"
	case *ast.LabeledStmt:
		return "*ast.LabeledStmt"
	case *ast.ExprStmt:
		return "*ast.ExprStmt"
	case *ast.DeclStmt:
		return "*ast.DeclStmt"
	case *ast.EmptyStmt:
		return "*ast.EmptyStmt"
	case *ast.BlockStmt:
		return "*ast.BlockStmt"
	case *ast.FuncDecl:
		return "*ast.FuncDecl"
	case *ast.BasicLit:
		return "*ast.BasicLit"
	case *ast.File:
		return "*ast.File"
	case *ast.FuncType:
		return "*ast.FuncType"
	case *ast.Field:
		return "*ast.Field"
	case *ast.GenDecl:
		return "*ast.GenDecl"
	case *ast.ValueSpec:
		return "*ast.ValueSpec"
	case *ast.TypeSpec:
		return "*ast.TypeSpec"
	case *ast.CommentGroup:
		return "*ast.CommentGroup"
	case *ast.Comment:
		return "*ast.Comment"
	case *ast.ImportSpec:
		return "*ast.ImportSpec"
	default:
		return fmt.Sprintf("%T", n)
	}
}

func buildContextLazy(node ast.Node, file *ast.File, fset *token.FileSet, cache *contextCache, parents map[ast.Node]ast.Node, needReturnType bool) mutator.Context {
	if cached, ok := cache.get(node); ok {
		return *cached
	}

	ctx := mutator.Context{
		FileName:    fset.File(file.Pos()).Name(),
		PackageName: getPackageName(file),
		File:        file,
		Position:    getNodePosition(node, fset),
		Parent:      parents[node],
	}

	if needReturnType {
		fn := findEnclosingFuncFast(node, parents)
		ctx.EnclosingFunc = fn
		if fn != nil {
			ctx.FunctionName = fn.Name.Name
			if fn.Type.Results != nil {
				for _, field := range fn.Type.Results.List {
					ctx.ReturnType = typeToString(field.Type, file, fset)
					break
				}
			}
		}
	}

	cache.set(node, &ctx)

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

	// Find all go.mod files to detect multiple modules
	modFiles, err := findGoModFiles(path)
	if err != nil {
		return fmt.Errorf("failed to find go.mod files: %w", err)
	}

	// If multiple go.mod files found, traverse each module separately
	if len(modFiles) > 1 {
		for _, modFile := range modFiles {
			modDir := filepath.Dir(modFile)
			if err := e.traverseModule(modDir, visitor); err != nil {
				return fmt.Errorf("failed to traverse module %s: %w", modDir, err)
			}
		}
		return nil
	}

	// No go.mod found — treat each subdirectory with .go files as a separate
	// standalone package. This handles cases like examples/ where each
	// subdirectory is an independent package without its own go.mod.
	if len(modFiles) == 0 {
		pkgDirs, err := findGoPackages(path)
		if err != nil {
			return fmt.Errorf("failed to find Go packages: %w", err)
		}
		if len(pkgDirs) == 0 {
			return nil
		}
		for _, pkgDir := range pkgDirs {
			if err := e.traverseSinglePkgDir(pkgDir, visitor); err != nil {
				return fmt.Errorf("failed to traverse package %s: %w", pkgDir, err)
			}
		}
		return nil
	}

	// Single go.mod - use original behavior
	return e.traverseModule(path, visitor)
}

func findGoPackages(root string) ([]string, error) {
	var pkgDirs []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		// Skip hidden dirs and vendor
		if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
			return filepath.SkipDir
		}
		// Check if this directory has .go files
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		hasGo := false
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".go") && !entry.IsDir() {
				hasGo = true
				break
			}
		}
		if hasGo {
			pkgDirs = append(pkgDirs, path)
			// Don't recurse into subdirectories that are already packages
			return filepath.SkipDir
		}
		return nil
	})
	return pkgDirs, err
}

func (e *Engine) traverseSinglePkgDir(dir string, visitor Visitor) error {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path for %q: %w", dir, err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return fmt.Errorf("failed to read dir %q: %w", absDir, err)
	}

	var goFiles []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".go") && !entry.IsDir() && !strings.HasSuffix(entry.Name(), "_test.go") {
			goFiles = append(goFiles, filepath.Join(absDir, entry.Name()))
		}
	}
	if len(goFiles) == 0 {
		return nil
	}

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(goFiles))
	for _, gf := range goFiles {
		file, err := parser.ParseFile(fset, gf, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil
	}

	fileCache := newContextCache()

	for _, file := range files {
		tfile := fset.File(file.Pos())
		parents := buildParentMap(file)

		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				return true
			}

			var mctx mutator.Context
			contextBuilt := false
			var localSites []Site

			for _, op := range e.operators {
				apply := false
				if cop, ok := op.(mutator.ContextualOperator); ok {
					if !contextBuilt {
						mctx = buildContextLazy(node, file, fset, fileCache, parents, true)
						contextBuilt = true
					}
					apply = cop.CanApplyWithContext(node, mctx)
				} else {
					apply = op.CanApply(node)
				}
				if apply {
					if !contextBuilt {
						mctx = buildContextLazy(node, file, fset, fileCache, parents, true)
						contextBuilt = true
					}
					pos := getNodePosition(node, fset)
					localSites = append(localSites, Site{
						File:          tfile,
						Line:          pos.Line,
						Column:        pos.Column,
						Node:          node,
						ReturnType:    mctx.ReturnType,
						FunctionName:  mctx.FunctionName,
						EnclosingFunc: mctx.EnclosingFunc,
					})
				}
			}
			if len(localSites) > 0 {
				e.mu.Lock()
				e.sites = append(e.sites, localSites...)
				e.mu.Unlock()
			}
			if visitor != nil {
				return visitor(node)
			}
			return true
		})
	}
	return nil
}

func findGoModFiles(root string) ([]string, error) {
	var modFiles []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "go.mod" && !info.IsDir() {
			modFiles = append(modFiles, path)
		}
		return nil
	})
	return modFiles, err
}

func (e *Engine) traverseModule(path string, visitor Visitor) error {
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
				// Create per-package context cache and parent map to avoid redundant AST walks
				pkgCache := newContextCache()

				for _, syntax := range pkg.Syntax {
					tfile := pkg.Fset.File(syntax.Pos())
					// Build parent map once per file for O(1) parent lookup
					parents := buildParentMap(syntax)

					if e.PrintAST {
						PrintEnabled.Store(true)
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

						var mctx mutator.Context
						contextBuilt := false
						var localSites []Site

						for _, op := range e.operators {
							apply := false
							if cop, ok := op.(mutator.ContextualOperator); ok {
								if !contextBuilt {
									mctx = buildContextLazy(node, syntax, pkg.Fset, pkgCache, parents, true)
									contextBuilt = true
								}
								apply = cop.CanApplyWithContext(node, mctx)
							} else {
								apply = op.CanApply(node)
							}
							if apply {
								if !contextBuilt {
									mctx = buildContextLazy(node, syntax, pkg.Fset, pkgCache, parents, true)
									contextBuilt = true
								}
								pos := getNodePosition(node, pkg.Fset)
								localSites = append(localSites, Site{
									File:          tfile,
									Line:          pos.Line,
									Column:        pos.Column,
									Node:          node,
									ReturnType:    mctx.ReturnType,
									FunctionName:  mctx.FunctionName,
									EnclosingFunc: mctx.EnclosingFunc,
								})
							}
						}
						if len(localSites) > 0 {
							e.mu.Lock()
							e.sites = append(e.sites, localSites...)
							e.mu.Unlock()
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
		PrintEnabled.Store(true)
		fmt.Printf("\n=== AST for %s ===\n", path)
		if err := PrintTree(os.Stdout, fset, file); err != nil {
			return err
		}
		fmt.Println("=====================================")
	}

	// Use context cache and parent map for single file traversal too
	fileCache := newContextCache()
	parents := buildParentMap(file)

	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			return true
		}

		var mctx mutator.Context
		contextBuilt := false
		var localSites []Site

		for _, op := range e.operators {
			apply := false
			if cop, ok := op.(mutator.ContextualOperator); ok {
				if !contextBuilt {
					mctx = buildContextLazy(node, file, fset, fileCache, parents, true)
					contextBuilt = true
				}
				apply = cop.CanApplyWithContext(node, mctx)
			} else {
				apply = op.CanApply(node)
			}
			if apply {
				if !contextBuilt {
					mctx = buildContextLazy(node, file, fset, fileCache, parents, true)
					contextBuilt = true
				}
				pos := getNodePosition(node, fset)
				localSites = append(localSites, Site{
					File:          tfile,
					Line:          pos.Line,
					Column:        pos.Column,
					Node:          node,
					ReturnType:    mctx.ReturnType,
					FunctionName:  mctx.FunctionName,
					EnclosingFunc: mctx.EnclosingFunc,
				})
			}
		}
		if len(localSites) > 0 {
			e.mu.Lock()
			e.sites = append(e.sites, localSites...)
			e.mu.Unlock()
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
