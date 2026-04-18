package engine

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"

	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
	"github.com/aclfe/gorgon/pkg/config"
	"github.com/aclfe/gorgon/pkg/mutator"
)

var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

type Visitor func(n ast.Node) bool

type Engine struct {
	PrintAST         bool
	sites            []Site
	operators        []mutator.Operator
	mu               sync.Mutex
	projectRoot      string
	ignoreDirectives map[string]map[int]map[string]map[int]bool
	ProgressFunc     func(current, total int)
	FileProgressFunc func(filename string)
	totalFiles       int
	filesProcessed   int
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

func (e *Engine) SetProjectRoot(root string) {
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			e.projectRoot = abs
		} else {
			e.projectRoot = root
		}
	}
}

func (e *Engine) SetSuppressEntries(entries []config.SuppressEntry) {
	root := e.projectRoot
	if root == "" {
		if cwd, err := os.Getwd(); err == nil {
			root = cwd
		}
	}

	for _, entry := range entries {
		loc := strings.TrimSpace(entry.Location)
		if loc == "" {
			continue
		}
		lastColon := strings.LastIndex(loc, ":")
		if lastColon < 0 {
			continue
		}
		filePath := loc[:lastColon]
		line, err := strconv.Atoi(loc[lastColon+1:])
		if err != nil {
			continue
		}

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
			parts := strings.SplitN(op, ":", 2)
			operator := parts[0]
			column := 0
			if len(parts) == 2 {
				if col, err := strconv.Atoi(parts[1]); err == nil {
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
	if typeCache != nil {
		if resolved, ok := typeCache.resolved[typeName]; ok {
			return resolved
		}
	}
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
			resolved := typeToString(ts.Type, file, fset, typeCache)
			if typeCache != nil {
				typeCache.resolved[typeName] = resolved
			}
			return resolved
		}
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
			targetLine := pos.Line + 1

			if directives[targetLine] == nil {
				directives[targetLine] = make(map[string]map[int]bool)
			}

			if rest == "" {
				directives[targetLine][""] = map[int]bool{0: true}
				continue
			}

			parts := strings.SplitN(rest, ":", 2)
			operator := strings.TrimSpace(parts[0])
			if operator == "" {
				continue
			}

			if len(parts) == 2 {
				if col, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					if directives[targetLine][operator] == nil {
						directives[targetLine][operator] = make(map[int]bool)
					}
					directives[targetLine][operator][col] = true
					continue
				}
			}

			if directives[targetLine][operator] == nil {
				directives[targetLine][operator] = make(map[int]bool)
			}
			directives[targetLine][operator][0] = true
		}
	}
	return directives
}

func isIgnored(directives map[int]map[string]map[int]bool, line int, operator string, column int) bool {
	lineMap, ok := directives[line]
	if !ok {
		return false
	}

	if colMap, ok := lineMap[""]; ok && colMap[0] {
		return true
	}
	if colMap, ok := lineMap[operator]; ok {
		if colMap[0] || colMap[column] {
			return true
		}
	}
	return false
}

func (e *Engine) IgnoreDirectives() map[string]map[int]map[string]map[int]bool {
	return e.ignoreDirectives
}

func (e *Engine) ProjectRoot() string {
	return e.projectRoot
}

type contextCache struct {
	contexts  map[ast.Node]*mutator.Context
	typeCache typeDeclCache
	pool      *sync.Pool
}

func newContextCache() *contextCache {
	return &contextCache{
		contexts:  make(map[ast.Node]*mutator.Context),
		typeCache: typeDeclCache{},
		pool: &sync.Pool{
			New: func() any { return &mutator.Context{} },
		},
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
		parents[node] = stack[len(stack)-1]
		stack = append(stack, node)
		return true
	})
	return parents
}

func buildContextLazy(node ast.Node, file *ast.File, fset *token.FileSet, cache *contextCache, parents map[ast.Node]ast.Node, needReturnType bool) *mutator.Context {
	if cached, ok := cache.contexts[node]; ok {
		return cached
	}

	ctx := cache.pool.Get().(*mutator.Context)
	ctx.FileName = fset.File(file.Pos()).Name()
	ctx.PackageName = getPackageName(file)
	ctx.File = file
	ctx.Position = schemata_nodes.GetNodePosition(node, fset)
	ctx.Parent = parents[node]

	if needReturnType {
		if fn := findEnclosingFuncFast(node, parents); fn != nil {
			ctx.EnclosingFunc = fn
			ctx.FunctionName = fn.Name.Name
			if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
				// Extract all return types for multi-value returns
				var types []string
				for _, field := range fn.Type.Results.List {
					typeStr := typeToString(field.Type, file, fset, &cache.typeCache)
					// Handle multiple names with same type: (a, b int) -> int, int
					if len(field.Names) > 1 {
						for range field.Names {
							types = append(types, typeStr)
						}
					} else {
						types = append(types, typeStr)
					}
				}
				if len(types) == 1 {
					ctx.ReturnType = types[0]
				} else {
					// Use comma-separated for multi-value returns
					ctx.ReturnType = strings.Join(types, ",")
				}
			}
		}
	}

	cache.contexts[node] = ctx
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

	modFiles, err := findGoModFiles(path)
	if err != nil {
		return fmt.Errorf("failed to find go.mod files: %w", err)
	}

	if len(modFiles) > 1 {
		for _, modFile := range modFiles {
			if err := e.traverseModule(filepath.Dir(modFile), visitor); err != nil {
				return err
			}
		}
		return nil
	}

	if len(modFiles) == 0 {
		pkgDirs, err := findGoPackages(path)
		if err != nil {
			return fmt.Errorf("failed to find Go packages: %w", err)
		}
		for _, pkgDir := range pkgDirs {
			if err := e.traverseSinglePkgDir(pkgDir, visitor); err != nil {
				return err
			}
		}
		return nil
	}

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
		if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
			return filepath.SkipDir
		}
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
	defer func() {
		for _, ctx := range fileCache.contexts {
			fileCache.pool.Put(ctx)
		}
	}()

	for _, file := range files {
		tfile := fset.File(file.Pos())
		if e.FileProgressFunc != nil {
			e.FileProgressFunc(tfile.Name())
		}

		parents := buildParentMap(file)
		ignoreMap := parseIgnoreComments(file, fset)

		fileCache.typeCache.buildOnce(file, fset)

		absPath, _ := filepath.Abs(tfile.Name())
		e.mergeIgnoreDirectives(absPath, ignoreMap)

		if printAST {
			fmt.Printf("\n=== AST for %s ===\n", tfile.Name())
			_ = PrintTree(os.Stdout, fset, file)
			fmt.Println("=====================================")
		}

		var fileSites []Site

		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				return true
			}

			mctx := buildContextLazy(node, file, fset, fileCache, parents, true)

			for _, op := range e.operators {
				apply := false
				if cop, ok := op.(mutator.ContextualOperator); ok {
					apply = cop.CanApplyWithContext(node, *mctx)
				} else {
					apply = op.CanApply(node)
				}
				if !apply {
					continue
				}

				pos := schemata_nodes.GetNodePosition(node, fset)
				if isIgnored(ignoreMap, pos.Line, op.Name(), pos.Column) {
					continue
				}

				fileSites = append(fileSites, Site{
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

			if visitor != nil {
				return visitor(node)
			}
			return true
		})

		if len(fileSites) > 0 {
			e.mu.Lock()
			e.sites = append(e.sites, fileSites...)
			e.mu.Unlock()
		}

		e.mu.Lock()
		e.filesProcessed++
		if e.ProgressFunc != nil && e.totalFiles > 0 {
			e.ProgressFunc(e.filesProcessed, e.totalFiles)
		}
		e.mu.Unlock()
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

	e.mu.Lock()
	e.totalFiles += len(files)
	e.mu.Unlock()

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

	totalFiles := 0
	for _, pkg := range pkgs {
		totalFiles += len(pkg.Syntax)
	}

	e.mu.Lock()
	e.totalFiles += totalFiles
	e.mu.Unlock()

	sem := make(chan struct{}, runtime.NumCPU())
	defer close(sem)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, pkg := range pkgs {
		sem <- struct{}{}
		wg.Add(1)
		go func(pkg *packages.Package) {
			defer func() {
				<-sem
				wg.Done()
			}()

			if err := e.processFiles(pkg.Syntax, pkg.Fset, visitor, e.PrintAST); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
		}(pkg)
	}

	wg.Wait()

	if firstErr != nil {
		return fmt.Errorf("error during traversal: %w", firstErr)
	}
	return nil
}

func (e *Engine) traverseSingleFile(path string, visitor Visitor) error {
	e.mu.Lock()
	e.totalFiles = 1
	e.mu.Unlock()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("failed to parse file %q: %w", path, err)
	}

	return e.processFiles([]*ast.File{file}, fset, visitor, e.PrintAST)
}

func (e *Engine) Sites() []Site {
	e.mu.Lock()
	sites := make([]Site, len(e.sites))
	copy(sites, e.sites)
	e.mu.Unlock()

	sort.Slice(sites, func(i, j int) bool {
		si, sj := &sites[i], &sites[j]
		if si.File.Name() != sj.File.Name() {
			return si.File.Name() < sj.File.Name()
		}
		if si.Line != sj.Line {
			return si.Line < sj.Line
		}
		if si.Column != sj.Column {
			return si.Column < sj.Column
		}
		return schemata_nodes.NodeTypeToUint8(si.Node) < schemata_nodes.NodeTypeToUint8(sj.Node)
	})
	return sites
}
