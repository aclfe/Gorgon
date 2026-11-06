package testing

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"golang.org/x/tools/go/ast/astutil"

	"github.com/aclfe/gorgon/internal/core/schemata_nodes"
)

type PositionMapping struct {
	OriginalLine int
	OriginalCol  int
	TempLine     int
	TempCol      int
}

type ApplyResult struct {
	Applied    map[int]bool
	NotApplied map[int]string
	Positions  map[int]PositionMapping
}

func newApplyResult() *ApplyResult {
	return &ApplyResult{
		Applied:    make(map[int]bool),
		NotApplied: make(map[int]string),
		Positions:  make(map[int]PositionMapping),
	}
}

type siteKey struct {
	Offset int
	End    int
	Type   uint8
}

func buildSiteMap(mutants []*Mutant) map[siteKey][]schemata_nodes.MutantForSite {
	sites := make(map[siteKey][]schemata_nodes.MutantForSite, len(mutants))
	for _, m := range mutants {
		if m.Site.Node == nil {
			continue
		}
		key := siteKey{Offset: m.Site.Offset, End: m.Site.EndOffset, Type: schemata_nodes.NodeTypeToUint8(m.Site.Node)}
		hint := ""
		if m.Site.Hints != nil && m.Operator != nil {
			hint = m.Site.Hints[m.Operator.Name()]
		}
		sites[key] = append(sites[key], schemata_nodes.MutantForSite{
			ID:            m.ID,
			Op:            m.Operator,
			ReturnType:    m.Site.ReturnType,
			EnclosingFunc: m.Site.EnclosingFunc,
			ExprType:      m.Site.ExprType,
			Hint:          hint,
		})
	}
	for k := range sites {
		ms := sites[k]
		sort.Slice(ms, func(i, j int) bool { return ms[i].ID < ms[j].ID })
	}
	return sites
}

//

func applySchemataToFileAST(file *ast.File, fset *token.FileSet, mutants []*Mutant) *ApplyResult {
	res := newApplyResult()
	tfile := fset.File(file.Pos())
	sites := buildSiteMap(mutants)
	if tfile == nil {
		for _, m := range mutants {
			res.NotApplied[m.ID] = "source file has no position information"
		}
		return res
	}

	plans := make(map[ast.Node]schemata_nodes.SitePlan)
	var stack []ast.Node
	var funcStack []*ast.FuncDecl
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			if top := stack[len(stack)-1]; top != nil {
				if fd, ok := top.(*ast.FuncDecl); ok && len(funcStack) > 0 && funcStack[len(funcStack)-1] == fd {
					funcStack = funcStack[:len(funcStack)-1]
				}
			}
			stack = stack[:len(stack)-1]
			return true
		}
		var parent ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		stack = append(stack, n)
		if fd, ok := n.(*ast.FuncDecl); ok {
			funcStack = append(funcStack, fd)
		}
		if !n.Pos().IsValid() || !n.End().IsValid() {
			return true
		}
		key := siteKey{Offset: tfile.Offset(n.Pos()), End: tfile.Offset(n.End()), Type: schemata_nodes.NodeTypeToUint8(n)}
		ms, ok := sites[key]
		if !ok {
			return true
		}
		delete(sites, key)
		var encl *ast.FuncDecl
		if len(funcStack) > 0 {
			encl = funcStack[len(funcStack)-1]
		}
		plans[n] = schemata_nodes.BuildPlan(n, ms, parent, file, encl)
		return true
	})

	astutil.Apply(file, nil, func(c *astutil.Cursor) bool {
		n := c.Node()
		plan, ok := plans[n]
		if !ok {
			return true
		}
		repl, applied, notApplied := schemata_nodes.Inject(n, plan)
		for id, reason := range notApplied {
			res.NotApplied[id] = reason
		}
		if repl != n {
			if err := replaceNode(c, repl); err != nil {
				for _, id := range applied {
					res.NotApplied[id] = "mutation cannot be placed here: " + err.Error()
				}
				applied = nil
			}
		}
		for _, id := range applied {
			res.Applied[id] = true
		}
		return true
	})

	for _, ms := range sites {
		for _, m := range ms {
			res.NotApplied[m.ID] = "mutation site not found in source"
		}
	}
	for _, m := range mutants {
		if !res.Applied[m.ID] {
			if _, ok := res.NotApplied[m.ID]; !ok {
				res.NotApplied[m.ID] = "mutation was not injected"
			}
		} else {
			delete(res.NotApplied, m.ID)
		}
	}
	return res
}

func replaceNode(c *astutil.Cursor, repl ast.Node) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()
	c.Replace(repl)
	return nil
}

func ApplySchemataToSource(src []byte, filePath string, mutants []*Mutant) ([]byte, *ApplyResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	res := applySchemataToFileAST(file, fset, mutants)

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, res, fmt.Errorf("format %s: %w", filePath, err)
	}
	out := buf.Bytes()

	found := findGuardLines(out)
	for _, m := range mutants {
		if !res.Applied[m.ID] {
			continue
		}
		pos, ok := found[m.ID]
		if !ok {
			delete(res.Applied, m.ID)
			res.NotApplied[m.ID] = "guard missing from generated code"
			continue
		}
		res.Positions[m.ID] = PositionMapping{
			OriginalLine: m.Site.Line,
			OriginalCol:  m.Site.Column,
			TempLine:     pos.TempLine,
			TempCol:      pos.TempCol,
		}
	}
	return out, res, nil
}

func WriteSchemataFile(srcPath, outPath string, mutants []*Mutant) (*ApplyResult, error) {
	src, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	out, res, err := ApplySchemataToSource(src, srcPath, mutants)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return res, fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(outPath, out, filePermissions); err != nil {
		return res, fmt.Errorf("write failed: %w", err)
	}
	return res, nil
}

// ApplySchemataInMemory applies schemata to a fresh parse of src and returns
// the mutated AST without any disk I/O. Used by preflight type-checking.
func ApplySchemataInMemory(src []byte, filePath string, fset *token.FileSet, fileMutants []*Mutant) (*ast.File, error) {
	file, _, err := ApplySchemataInMemoryResult(src, filePath, fset, fileMutants)
	return file, err
}

func ApplySchemataInMemoryResult(src []byte, filePath string, fset *token.FileSet, fileMutants []*Mutant) (*ast.File, *ApplyResult, error) {
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	res := applySchemataToFileAST(file, fset, fileMutants)
	return file, res, nil
}

func findGuardLines(content []byte) map[int]PositionMapping {
	result := make(map[int]PositionMapping)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.SkipObjectResolution)
	if file == nil {
		_ = err
		return result
	}
	ast.Inspect(file, func(n ast.Node) bool {
		be, ok := n.(*ast.BinaryExpr)
		if !ok || (be.Op != token.EQL && be.Op != token.NEQ) {
			return true
		}
		id, ok := be.X.(*ast.Ident)
		if !ok || id.Name != "activeMutantID" {
			return true
		}
		lit, ok := be.Y.(*ast.BasicLit)
		if !ok || lit.Kind != token.INT {
			return true
		}
		v, err := strconv.Atoi(lit.Value)
		if err != nil {
			return true
		}
		if _, seen := result[v]; !seen {
			p := fset.Position(be.Pos())
			result[v] = PositionMapping{TempLine: p.Line, TempCol: p.Column}
		}
		return true
	})
	return result
}
