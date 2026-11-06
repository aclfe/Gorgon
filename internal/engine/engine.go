package engine

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
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
	buildTags        []string
	evaluatedOps     map[string]bool
}

func (e *Engine) SetBuildTags(tags []string) {
	e.buildTags = append([]string(nil), tags...)
}

func (e *Engine) buildFlags() []string {
	if len(e.buildTags) == 0 {
		return nil
	}
	return []string{"-tags=" + strings.Join(e.buildTags, ",")}
}

func NewEngine(printAST bool) *Engine {
	return &Engine{
		PrintAST:         printAST,
		ignoreDirectives: make(map[string]map[int]map[string]map[int]bool),
	}
}

func (e *Engine) SetOperators(ops []mutator.Operator) {
	sorted := append([]mutator.Operator(nil), ops...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name() < sorted[j].Name() })
	e.operators = sorted
	e.evaluatedOps = make(map[string]bool, len(sorted))
	for _, op := range sorted {
		e.evaluatedOps[op.Name()] = true
	}
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
	e.mu.Lock()
	defer e.mu.Unlock()
	MergeDirectives(e.ignoreDirectives, BuildDirectives(entries, root))
}

type Directives = map[string]map[int]map[string]map[int]bool

func BuildDirectives(entries []config.SuppressEntry, root string) Directives {
	out := make(Directives)
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
		line, err := strconv.Atoi(strings.TrimSpace(loc[lastColon+1:]))
		if err != nil {
			continue
		}

		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(root, filePath)
		}
		if abs, err := filepath.Abs(filePath); err == nil {
			filePath = abs
		}

		if out[filePath] == nil {
			out[filePath] = make(map[int]map[string]map[int]bool)
		}
		if out[filePath][line] == nil {
			out[filePath][line] = make(map[string]map[int]bool)
		}

		if len(entry.Operators) == 0 {
			if out[filePath][line][""] == nil {
				out[filePath][line][""] = make(map[int]bool)
			}
			out[filePath][line][""][0] = true
			continue
		}

		for _, op := range entry.Operators {
			op = strings.TrimSpace(op)
			if op == "" {
				continue
			}
			parts := strings.SplitN(op, ":", 2)
			operator := strings.TrimSpace(parts[0])
			column := 0
			if len(parts) == 2 {
				if col, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					column = col
				}
			}
			if out[filePath][line][operator] == nil {
				out[filePath][line][operator] = make(map[int]bool)
			}
			out[filePath][line][operator][column] = true
		}
	}
	return out
}

func MergeDirectives(dst, src Directives) {
	for file, lines := range src {
		if dst[file] == nil {
			dst[file] = make(map[int]map[string]map[int]bool)
		}
		for line, ops := range lines {
			if dst[file][line] == nil {
				dst[file][line] = make(map[string]map[int]bool)
			}
			for op, cols := range ops {
				if dst[file][line][op] == nil {
					dst[file][line][op] = make(map[int]bool)
				}
				for col, v := range cols {
					dst[file][line][op][col] = v
				}
			}
		}
	}
}

func IsIgnored(directives map[int]map[string]map[int]bool, line int, operator string, column int) bool {
	return isIgnored(directives, line, operator, column)
}

func findEnclosingFuncFast(node ast.Node, parents map[ast.Node]ast.Node) *ast.FuncDecl {
	for p := parents[node]; p != nil; p = parents[p] {
		if fn, ok := p.(*ast.FuncDecl); ok {
			return fn
		}
	}
	return nil
}

// findInnermostFuncResults returns the Results field list of the closest
// enclosing function — either a *ast.FuncLit or *ast.FuncDecl. This matters
// for nodes inside callbacks (e.g. ast.Inspect's visitor): the literal's
// return type, not the outer FuncDecl's, governs the type of `return ...`.
func findInnermostFuncResults(node ast.Node, parents map[ast.Node]ast.Node) *ast.FieldList {
	for p := parents[node]; p != nil; p = parents[p] {
		switch fn := p.(type) {
		case *ast.FuncLit:
			if fn.Type != nil {
				return fn.Type.Results
			}
			return nil
		case *ast.FuncDecl:
			if fn.Type != nil {
				return fn.Type.Results
			}
			return nil
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
	contexts map[ast.Node]*mutator.Context
	pool     *sync.Pool
}

func newContextCache() *contextCache {
	return &contextCache{
		contexts: make(map[ast.Node]*mutator.Context),
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
	*ctx = mutator.Context{}
	ctx.FileName = fset.File(file.Pos()).Name()
	ctx.PackageName = getPackageName(file)
	ctx.File = file
	ctx.Position = schemata_nodes.GetNodePosition(node, fset)
	ctx.Parent = parents[node]

	if needReturnType {
		if fn := findEnclosingFuncFast(node, parents); fn != nil {
			ctx.EnclosingFunc = fn
			ctx.FunctionName = fn.Name.Name
		}
		// Use the innermost enclosing func's results — a FuncLit nested
		// inside a FuncDecl has its own return type that governs `return`
		// statements within it. Without this, IIFE wrappers built around
		// expressions inside callbacks (e.g. ast.Inspect) get the outer
		// FuncDecl's return type and produce uncompilable code.
		if results := findInnermostFuncResults(node, parents); results != nil && len(results.List) > 0 {
			ctx.ReturnType = resultTypeString(results, fset)
		}
	}

	cache.contexts[node] = ctx
	return ctx
}

func resultTypeString(results *ast.FieldList, fset *token.FileSet) string {
	// Extract all return types for multi-value returns
	var types []string
	for _, field := range results.List {
		typeStr := exprToString(field.Type, fset)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			types = append(types, typeStr)
		}
	}
	return strings.Join(types, ",")
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
		if hasEnclosingModule(path) {
			return e.traverseModule(path, visitor)
		}
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

func hasEnclosingModule(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, "go.mod")); err == nil {
			return true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return false
		}
		abs = parent
	}
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

func (e *Engine) fileDirectives(absPath string, comments map[int]map[string]map[int]bool) map[int]map[string]map[int]bool {
	e.mergeIgnoreDirectives(absPath, comments)
	e.mu.Lock()
	defer e.mu.Unlock()
	src := e.ignoreDirectives[absPath]
	out := make(map[int]map[string]map[int]bool, len(src))
	for line, ops := range src {
		m := make(map[string]map[int]bool, len(ops))
		for op, cols := range ops {
			c := make(map[int]bool, len(cols))
			for col, v := range cols {
				c[col] = v
			}
			m[op] = c
		}
		out[line] = m
	}
	return out
}

func (e *Engine) processFiles(files []*ast.File, fset *token.FileSet, info *types.Info, pkg *types.Package, visitor Visitor, printAST bool) error {
	for _, file := range files {
		tfile := fset.File(file.Pos())
		if tfile == nil {
			continue
		}
		if e.FileProgressFunc != nil {
			e.FileProgressFunc(tfile.Name())
		}

		fileCache := newContextCache()
		parents := buildParentMap(file)
		noMutate := buildNoMutateSet(file)
		tc := newTypeContext(info, pkg, file)

		absPath, _ := filepath.Abs(tfile.Name())
		directives := e.fileDirectives(absPath, parseIgnoreComments(file, fset))

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
			if site, ok := e.siteFor(node, file, tfile, fset, parents, noMutate, tc, directives, fileCache); ok {
				fileSites = append(fileSites, site)
			}
			if visitor != nil {
				return visitor(node)
			}
			return true
		})

		for _, ctx := range fileCache.contexts {
			fileCache.pool.Put(ctx)
		}

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

func (e *Engine) siteFor(node ast.Node, file *ast.File, tfile *token.File, fset *token.FileSet,
	parents map[ast.Node]ast.Node, noMutate map[ast.Node]bool, tc *typeContext,
	directives map[int]map[string]map[int]bool, fileCache *contextCache) (Site, bool) {

	if noMutate[node] || !node.Pos().IsValid() || !node.End().IsValid() {
		return Site{}, false
	}
	switch node.(type) {
	case *ast.File, *ast.GenDecl, *ast.Comment, *ast.CommentGroup:
		return Site{}, false
	}

	parent := parents[node]
	if _, isStmt := node.(ast.Stmt); isStmt && !injectedInPlace(node) && !isReplaceableStmtSlot(parent, node) {
		return Site{}, false
	}
	exprType := ""
	if expr, isExpr := node.(ast.Expr); isExpr {
		if !isReplaceableExprSlot(parent, node) || isInAssignableChain(node, parents) {
			return Site{}, false
		}
		if tc != nil {
			if tc.isTypeExpr(expr) {
				return Site{}, false
			}
			if pexpr, ok := parent.(ast.Expr); ok && tc.isConstant(expr) && tc.isConstant(pexpr) {
				return Site{}, false
			}
			t, ok := tc.exprType(expr)
			if !ok {
				return Site{}, false
			}
			exprType = t
		}
	}

	mctx := buildContextLazy(node, file, fset, fileCache, parents, true)
	ctx := *mctx
	if tc != nil {
		ctx.Info = tc.info
		ctx.Pkg = tc.pkg
	}
	pos := schemata_nodes.GetNodePosition(node, fset)

	var opNames []string
	var hints map[string]string
	for _, op := range e.operators {
		if !mutator.CanApplyWithContext(op, node, ctx) {
			continue
		}
		if isIgnored(directives, pos.Line, op.Name(), pos.Column) {
			continue
		}
		if hop, ok := op.(mutator.HintingOperator); ok {
			hint := hop.MutationHint(node, ctx)
			if hint == "" && ctx.Info != nil {
				continue
			}
			if hint != "" {
				if hints == nil {
					hints = make(map[string]string)
				}
				hints[op.Name()] = hint
			}
		}
		opNames = append(opNames, op.Name())
	}
	if len(opNames) == 0 {
		return Site{}, false
	}
	sort.Strings(opNames)

	return Site{
		File:          tfile,
		FileAST:       file,
		Fset:          fset,
		Line:          pos.Line,
		Column:        pos.Column,
		Offset:        tfile.Offset(node.Pos()),
		EndOffset:     tfile.Offset(node.End()),
		Node:          node,
		Parent:        parent,
		ReturnType:    mctx.ReturnType,
		FunctionName:  mctx.FunctionName,
		EnclosingFunc: mctx.EnclosingFunc,
		ExprType:      exprType,
		Ops:           opNames,
		EvaluatedOps:  e.evaluatedOps,
		Directives:    directives,
		Hints:         hints,
	}, true
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

	return e.processFiles(files, fset, nil, nil, visitor, false)
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
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Dir:        path,
		Tests:      false,
		BuildFlags: e.buildFlags(),
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

			info, tpkg := pkg.TypesInfo, pkg.Types
			if tpkg == nil || info == nil || len(pkg.TypeErrors) > 0 {
				info, tpkg = nil, nil
			}
			if err := e.processFiles(pkg.Syntax, pkg.Fset, info, tpkg, visitor, e.PrintAST); err != nil {
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

	return e.processFiles([]*ast.File{file}, fset, nil, nil, visitor, e.PrintAST)
}

func (e *Engine) Sites() []Site {
	e.mu.Lock()
	sites := make([]Site, len(e.sites))
	copy(sites, e.sites)
	e.mu.Unlock()

	SortSites(sites)
	return sites
}

func SortSites(sites []Site) {
	sort.SliceStable(sites, func(i, j int) bool {
		ki, kj := sites[i].Key(), sites[j].Key()
		if ki.File != kj.File {
			return ki.File < kj.File
		}
		if ki.Offset != kj.Offset {
			return ki.Offset < kj.Offset
		}
		if ki.EndOffset != kj.EndOffset {
			return ki.EndOffset > kj.EndOffset
		}
		return ki.NodeType < kj.NodeType
	})
}
