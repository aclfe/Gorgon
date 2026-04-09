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
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/go/packages"

	"github.com/aclfe/gorgon/internal/testing/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

// bufPool provides reusable bytes.Buffer instances for AST formatting operations,
// reducing GC pressure during repeated format.Node calls.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type Visitor func(n ast.Node) bool

type Engine struct {
	PrintAST    bool
	sites       []Site
	operators   []mutator.Operator
	mu          sync.Mutex
	projectRoot string
	// ignoreDirectives maps filepath → line → operator → column → true.
	// Empty operator key means "all operators on this line".
	// Zero column means "all columns for this operator on this line".
	ignoreDirectives map[string]map[int]map[string]map[int]bool
}

func NewEngine(printAST bool) *Engine {
	return &Engine{
		PrintAST:         printAST,
		ignoreDirectives: make(map[string]map[int]map[string]map[int]bool),
	}
}

func (e *Engine) SetOperators(ops []mutator.Operator) {
	e.operators = ops
}

// SetProjectRoot sets the project root directory used for resolving
// relative paths in suppression entries. This should be the directory
// containing go.mod (or a user-specified base directory).
func (e *Engine) SetProjectRoot(root string) {
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			e.projectRoot = abs
		} else {
			e.projectRoot = root
		}
	}
}

// SetSuppressEntries loads suppression entries from the YAML config.
// Each entry has a Location like "path/to/file.go:6" and an optional
// Operators list. Empty operators = suppress all on that line.
// Operator names can include an optional column suffix: "arithmetic_flip:12".
// Relative paths are resolved against e.projectRoot.
func (e *Engine) SetSuppressEntries(entries []config.SuppressEntry) {
	root := e.projectRoot
	if root == "" {
		// Fallback: try to resolve from CWD if no project root set
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}

	for _, entry := range entries {
		loc := strings.TrimSpace(entry.Location)
		if loc == "" {
			continue
		}
		// Parse "path/to/file.go:6" — split on last colon to handle Windows paths
		lastColon := strings.LastIndex(loc, ":")
		if lastColon < 0 {
			continue
		}
		filePath := loc[:lastColon]
		line, err := strconv.Atoi(loc[lastColon+1:])
		if err != nil {
			continue
		}

		// Resolve relative paths against project root
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(root, filePath)
			if abs, err := filepath.Abs(filePath); err == nil {
				filePath = abs
			}
		}

		if e.ignoreDirectives[filePath] == nil {
			e.ignoreDirectives[filePath] = make(map[int]map[string]map[int]bool)
		}
		if e.ignoreDirectives[filePath][line] == nil {
			e.ignoreDirectives[filePath][line] = make(map[string]map[int]bool)
		}

		// Empty operators list = suppress all on this line
		if len(entry.Operators) == 0 {
			if e.ignoreDirectives[filePath][line][""] == nil {
				e.ignoreDirectives[filePath][line][""] = make(map[int]bool)
			}
			e.ignoreDirectives[filePath][line][""][0] = true
			continue
		}

		for _, op := range entry.Operators {
			op = strings.TrimSpace(op)
			if op == "" {
				continue
			}
			// Check for operator:column suffix
			parts := strings.SplitN(op, ":", 2)
			operator := parts[0]
			column := 0
			if len(parts) == 2 {
				col, err := strconv.Atoi(parts[1])
				if err == nil {
					column = col
				}
			}
			if e.ignoreDirectives[filePath][line][operator] == nil {
				e.ignoreDirectives[filePath][line][operator] = make(map[int]bool)
			}
			if column == 0 {
				e.ignoreDirectives[filePath][line][operator][0] = true
			} else {
				e.ignoreDirectives[filePath][line][operator][column] = true
			}
		}
	}
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

// typeDeclCache stores resolved type names for a single file, built once.
type typeDeclCache struct {
	resolved map[string]string
	built    bool
}

func (c *typeDeclCache) buildOnce(file *ast.File, fset *token.FileSet) {
	if c.built {
		return
	}
	c.resolved = make(map[string]string)
	c.built = true

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Type == nil {
				continue
			}
			// Avoid infinite recursion for self-referential types
			if _, exists := c.resolved[ts.Name.Name]; exists {
				continue
			}
			c.resolved[ts.Name.Name] = typeToString(ts.Type, file, fset, c)
		}
	}
}

func typeToString(t ast.Expr, file *ast.File, fset *token.FileSet, typeCache *typeDeclCache) string {
	if t == nil {
		return ""
	}
	switch expr := t.(type) {
	case *ast.Ident:
		if typeCache != nil {
			if resolved, ok := typeCache.resolved[expr.Name]; ok {
				return resolved
			}
		}
		return resolveTypeName(expr.Name, file, fset, typeCache)
	case *ast.StarExpr:
		return "*" + typeToString(expr.X, file, fset, typeCache)
	case *ast.ArrayType:
		if expr.Len == nil {
			return "[]" + typeToString(expr.Elt, file, fset, typeCache)
		}
		return "[" + exprToString(expr.Len, fset) + "]" + typeToString(expr.Elt, file, fset, typeCache)
	case *ast.MapType:
		return "map[" + typeToString(expr.Key, file, fset, typeCache) + "]" + typeToString(expr.Value, file, fset, typeCache)
	case *ast.ChanType:
		return "chan " + typeToString(expr.Value, file, fset, typeCache)
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
		return typeToString(expr.X, file, fset, typeCache)
	case *ast.Ellipsis:
		return "..." + typeToString(expr.Elt, file, fset, typeCache)
	default:
		return ""
	}
}

func resolveTypeName(typeName string, file *ast.File, fset *token.FileSet, typeCache *typeDeclCache) string {
	if file == nil {
		return typeName
	}
	// Use the pre-built cache if available
	if typeCache != nil {
		if resolved, ok := typeCache.resolved[typeName]; ok {
			return resolved
		}
	}
	// Fallback: scan the file once for this type name
	resolved := ""
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Type == nil || ts.Name.Name != typeName {
				continue
			}
			resolved = typeToString(ts.Type, file, fset, typeCache)
			return resolved
		}
	}
	if resolved != "" {
		return resolved
	}
	return typeName
}

func exprToString(expr ast.Expr, fset *token.FileSet) string {
	if expr == nil {
		return ""
	}
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	if err := format.Node(buf, fset, expr); err != nil {
		return "?"
	}
	return buf.String()
}

// parseIgnoreComments scans a file's comment groups for //gorgon:ignore directives.
// Returns a map: line → operator → column → true.
// Empty operator means "all operators on this line". Zero column means "all columns".
func parseIgnoreComments(file *ast.File, fset *token.FileSet) map[int]map[string]map[int]bool {
	directives := make(map[int]map[string]map[int]bool)

	for _, cg := range file.Comments {
		for _, c := range cg.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, "gorgon:ignore") {
				continue
			}

			rest := strings.TrimSpace(strings.TrimPrefix(text, "gorgon:ignore"))
			pos := fset.Position(c.Pos())
			// The directive applies to the line *below* the comment.
			targetLine := pos.Line + 1

			if rest == "" {
				// //gorgon:ignore — all operators, all columns
				if directives[targetLine] == nil {
					directives[targetLine] = make(map[string]map[int]bool)
				}
				directives[targetLine][""] = make(map[int]bool)
				directives[targetLine][""][0] = true
				continue
			}

			// Parse operator and optional column: "operator" or "operator:col"
			parts := strings.SplitN(rest, ":", 2)
			operator := strings.TrimSpace(parts[0])
			if operator == "" {
				continue
			}

			if directives[targetLine] == nil {
				directives[targetLine] = make(map[string]map[int]bool)
			}

			if len(parts) == 2 {
				// operator:column
				col, err := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err != nil {
					// Invalid column number, treat as operator-only (all columns)
					directives[targetLine][operator] = make(map[int]bool)
					directives[targetLine][operator][0] = true
					continue
				}
				directives[targetLine][operator] = make(map[int]bool)
				directives[targetLine][operator][col] = true
			} else {
				// operator only — all columns
				directives[targetLine][operator] = make(map[int]bool)
				directives[targetLine][operator][0] = true
			}
		}
	}

	return directives
}

// isIgnored checks if a mutation site should be suppressed by an inline directive.
// Returns true if the site should be skipped.
func isIgnored(directives map[int]map[string]map[int]bool, line int, operator string, column int) bool {
	lineMap, ok := directives[line]
	if !ok {
		return false
	}

	// Check for blanket ignore: //gorgon:ignore (all operators)
	if colMap, ok := lineMap[""]; ok {
		if colMap[0] {
			return true
		}
	}

	// Check for operator-specific ignore: //gorgon:ignore operator
	if colMap, ok := lineMap[operator]; ok {
		// All columns ignored for this operator
		if colMap[0] {
			return true
		}
		// Specific column ignored
		if colMap[column] {
			return true
		}
	}

	return false
}

// IgnoreDirectives returns all collected inline ignore directives, keyed by
// absolute file path → line → operator → column. This is used by the CLI
// to sync inline comments back to the YAML config.
func (e *Engine) IgnoreDirectives() map[string]map[int]map[string]map[int]bool {
	return e.ignoreDirectives
}

// ProjectRoot returns the project root directory used for path resolution.
func (e *Engine) ProjectRoot() string {
	return e.projectRoot
}

type contextCache struct {
	contexts map[ast.Node]*mutator.Context
}

func newContextCache() *contextCache {
	return &contextCache{
		contexts:  make(map[ast.Node]*mutator.Context),
		typeCache: typeDeclCache{},
	}
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

func buildContextLazy(node ast.Node, file *ast.File, fset *token.FileSet, cache *contextCache, parents map[ast.Node]ast.Node, needReturnType bool) mutator.Context {
	if cached, ok := cache.contexts[node]; ok {
		return *cached
	}

	ctx := mutator.Context{
		FileName:    fset.File(file.Pos()).Name(),
		PackageName: getPackageName(file),
		File:        file,
		Position:    schemata_nodes.GetNodePosition(node, fset),
		Parent:      parents[node],
	}

	if needReturnType {
		fn := findEnclosingFuncFast(node, parents)
		ctx.EnclosingFunc = fn
		if fn != nil {
			ctx.FunctionName = fn.Name.Name
			if fn.Type.Results != nil {
				for _, field := range fn.Type.Results.List {
					ctx.ReturnType = typeToString(field.Type, file, fset, &cache.typeCache)
					break
				}
			}
		}
	}

	cache.contexts[node] = &ctx

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

func (e *Engine) mergeIgnoreDirectives(absPath string, ignoreMap map[int]map[string]map[int]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.ignoreDirectives[absPath] == nil {
		e.ignoreDirectives[absPath] = make(map[int]map[string]map[int]bool)
	}
	for line, opMap := range ignoreMap {
		if e.ignoreDirectives[absPath][line] == nil {
			e.ignoreDirectives[absPath][line] = make(map[string]map[int]bool)
		}
		for op, colMap := range opMap {
			if e.ignoreDirectives[absPath][line][op] == nil {
				e.ignoreDirectives[absPath][line][op] = make(map[int]bool)
			}
			for col, val := range colMap {
				e.ignoreDirectives[absPath][line][op][col] = val
			}
		}
	}
}

func (e *Engine) processFiles(files []*ast.File, fset *token.FileSet, visitor Visitor, printAST bool) error {
	fileCache := newContextCache()

	for _, file := range files {
		tfile := fset.File(file.Pos())
		parents := buildParentMap(file)
		ignoreMap := parseIgnoreComments(file, fset)

		// Build type declaration cache once per file
		fileCache.typeCache.buildOnce(file, fset)

		absPath, _ := filepath.Abs(tfile.Name())
		e.mergeIgnoreDirectives(absPath, ignoreMap)

		if printAST {
			PrintEnabled.Store(true)
			fmt.Printf("\n=== AST for %s ===\n", tfile.Name())
			if err := PrintTree(os.Stdout, fset, file); err != nil {
				return err
			}
			fmt.Println("=====================================")
		}

		// Collect sites locally, then batch-append once after inspection
		var fileSites []Site

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
					pos := schemata_nodes.GetNodePosition(node, fset)
					if isIgnored(ignoreMap, pos.Line, op.Name(), pos.Column) {
						continue
					}
					localSites = append(localSites, Site{
						File:          tfile,
						FileAST:       file,
						Fset:          fset,
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
				fileSites = append(fileSites, localSites...)
			}
			if visitor != nil {
				return visitor(node)
			}
			return true
		})

		// Batch-append all sites for this file under a single lock
		if len(fileSites) > 0 {
			e.mu.Lock()
			e.sites = append(e.sites, fileSites...)
			e.mu.Unlock()
		}
	}
	return nil
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

	return e.processFiles(files, fset, visitor, false)
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
	grp.SetLimit(runtime.NumCPU())

	for _, pkg := range pkgs {
		grp.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			return e.processFiles(pkg.Syntax, pkg.Fset, visitor, e.PrintAST)
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

	return e.processFiles([]*ast.File{file}, fset, visitor, e.PrintAST)
}

func (e *Engine) Sites() []Site {
	return e.sites
}
