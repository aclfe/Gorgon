package engine

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
)

func buildNoMutateSet(file *ast.File) map[ast.Node]bool {
	no := make(map[ast.Node]bool)
	markTree := func(n ast.Node) {
		if n == nil {
			return
		}
		ast.Inspect(n, func(c ast.Node) bool {
			if c != nil {
				no[c] = true
			}
			return true
		})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GenDecl:
			switch x.Tok {
			case token.CONST:
				markTree(x)
				return false
			case token.IMPORT:
				markTree(x)
				return false
			}
		case *ast.Field:
			markTree(x.Type)
			if x.Tag != nil {
				markTree(x.Tag)
			}
		case *ast.TypeSpec:
			markTree(x)
			return false
		case *ast.ValueSpec:
			markTree(x.Type)
		case *ast.CompositeLit:
			markTree(x.Type)
		case *ast.TypeAssertExpr:
			markTree(x.Type)
		case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType,
			*ast.StructType, *ast.InterfaceType:
			markTree(x)
			return false
		case *ast.FuncLit:
			markTree(x.Type)
		case *ast.FuncDecl:
			if x.Recv != nil {
				markTree(x.Recv)
			}
			markTree(x.Type)
			if x.Name != nil {
				no[x.Name] = true
			}
		case *ast.IndexListExpr:
			for _, idx := range x.Indices {
				markTree(idx)
			}
		}
		return true
	})
	return no
}

func isReplaceableExprSlot(parent, node ast.Node) bool {
	switch p := parent.(type) {
	case nil:
		return false
	case *ast.Field, *ast.TypeSpec, *ast.ImportSpec, *ast.BranchStmt,
		*ast.Ellipsis, *ast.ArrayType, *ast.MapType, *ast.ChanType,
		*ast.FuncType, *ast.StructType, *ast.InterfaceType:
		return false
	case *ast.ValueSpec:
		for _, v := range p.Values {
			if v == node {
				return true
			}
		}
		return false
	case *ast.FuncDecl:
		return false
	case *ast.LabeledStmt:
		return p.Label != node
	case *ast.SelectorExpr:
		return p.Sel != node
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if lhs == node {
				return false
			}
		}
		return true
	case *ast.RangeStmt:
		return p.Key != node && p.Value != node
	case *ast.IncDecStmt:
		return p.X != node
	case *ast.UnaryExpr:
		return p.Op != token.AND
	case *ast.KeyValueExpr:
		return p.Key != node
	case *ast.CompositeLit:
		return p.Type != node
	case *ast.TypeAssertExpr:
		return p.Type != node
	case *ast.CallExpr:
		return p.Fun != node
	case *ast.CaseClause:
		for _, e := range p.List {
			if e == node {
				return false
			}
		}
		return true
	case *ast.IndexListExpr:
		return p.X == node
	case *ast.DeferStmt, *ast.GoStmt:
		return false
	}
	return true
}

func isInAssignableChain(node ast.Node, parents map[ast.Node]ast.Node) bool {
	cur := node
	for {
		p := parents[cur]
		switch x := p.(type) {
		case *ast.SelectorExpr:
			if x.X != cur {
				return false
			}
		case *ast.IndexExpr:
			if x.X != cur {
				return false
			}
		case *ast.IndexListExpr:
			if x.X != cur {
				return false
			}
		case *ast.ParenExpr:
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if lhs == cur {
					return cur != node
				}
			}
			return false
		case *ast.IncDecStmt:
			return x.X == cur && cur != node
		case *ast.UnaryExpr:
			return x.Op == token.AND && cur != node
		case *ast.RangeStmt:
			return (x.Key == cur || x.Value == cur) && cur != node
		default:
			return false
		}
		cur = p
	}
}

type typeContext struct {
	info    *types.Info
	pkg     *types.Package
	imports map[string]string
}

func newTypeContext(info *types.Info, pkg *types.Package, file *ast.File) *typeContext {
	if info == nil || pkg == nil {
		return nil
	}
	tc := &typeContext{info: info, pkg: pkg, imports: make(map[string]string)}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if imp.Name != nil {
			switch imp.Name.Name {
			case "_":
				continue
			case ".":
				tc.imports[path] = ""
				continue
			}
			tc.imports[path] = imp.Name.Name
			continue
		}
		name := ""
		for _, p := range pkg.Imports() {
			if p.Path() == path {
				name = p.Name()
				break
			}
		}
		if name == "" {
			continue
		}
		tc.imports[path] = name
	}
	return tc
}

func (tc *typeContext) qualifier(p *types.Package) string {
	if p == nil || p == tc.pkg {
		return ""
	}
	return tc.imports[p.Path()]
}

func (tc *typeContext) expressible(t types.Type) bool {
	seen := make(map[types.Type]bool)
	var ok func(types.Type) bool
	ok = func(t types.Type) bool {
		if t == nil {
			return false
		}
		if seen[t] {
			return true
		}
		seen[t] = true
		switch x := t.(type) {
		case *types.Basic:
			return x.Kind() != types.Invalid && x.Info()&types.IsUntyped == 0
		case *types.Pointer:
			return ok(x.Elem())
		case *types.Slice:
			return ok(x.Elem())
		case *types.Array:
			return ok(x.Elem())
		case *types.Map:
			return ok(x.Key()) && ok(x.Elem())
		case *types.Chan:
			return ok(x.Elem())
		case *types.Signature:
			for i := 0; i < x.Params().Len(); i++ {
				if !ok(x.Params().At(i).Type()) {
					return false
				}
			}
			for i := 0; i < x.Results().Len(); i++ {
				if !ok(x.Results().At(i).Type()) {
					return false
				}
			}
			return true
		case *types.Struct:
			for i := 0; i < x.NumFields(); i++ {
				f := x.Field(i)
				if !f.Exported() && f.Pkg() != tc.pkg {
					return false
				}
				if !ok(f.Type()) {
					return false
				}
			}
			return true
		case *types.Interface:
			for i := 0; i < x.NumExplicitMethods(); i++ {
				m := x.ExplicitMethod(i)
				if !m.Exported() && m.Pkg() != tc.pkg {
					return false
				}
				if !ok(m.Type()) {
					return false
				}
			}
			for i := 0; i < x.NumEmbeddeds(); i++ {
				if !ok(x.EmbeddedType(i)) {
					return false
				}
			}
			return true
		case *types.Named:
			if !tc.objectNameable(x.Obj()) {
				return false
			}
			if args := x.TypeArgs(); args != nil {
				for i := 0; i < args.Len(); i++ {
					if !ok(args.At(i)) {
						return false
					}
				}
			}
			return true
		case *types.Alias:
			if !tc.objectNameable(x.Obj()) {
				return false
			}
			return true
		case *types.TypeParam:
			return true
		case *types.Tuple:
			return false
		}
		return false
	}
	return ok(t)
}

func (tc *typeContext) objectNameable(obj *types.TypeName) bool {
	if obj == nil {
		return false
	}
	p := obj.Pkg()
	if p == nil {
		return true
	}
	if p == tc.pkg {
		return true
	}
	if !obj.Exported() {
		return false
	}
	_, imported := tc.imports[p.Path()]
	return imported
}

func (tc *typeContext) exprType(expr ast.Expr) (string, bool) {
	tv, found := tc.info.Types[expr]
	if !found || tv.Type == nil || !tv.IsValue() {
		return "", false
	}
	t := tv.Type
	if b, isBasic := t.(*types.Basic); isBasic && b.Info()&types.IsUntyped != 0 {
		if b.Kind() == types.UntypedNil {
			return "", false
		}
		t = types.Default(t)
	}
	if !tc.expressible(t) {
		return "", false
	}
	return types.TypeString(t, tc.qualifier), true
}

func (tc *typeContext) isConstant(expr ast.Expr) bool {
	tv, found := tc.info.Types[expr]
	return found && tv.Value != nil
}

func (tc *typeContext) isTypeExpr(expr ast.Expr) bool {
	tv, found := tc.info.Types[expr]
	return found && tv.IsType()
}

func isReplaceableStmtSlot(parent, node ast.Node) bool {
	switch p := parent.(type) {
	case *ast.BlockStmt, *ast.LabeledStmt, *ast.CaseClause:
		return true
	case *ast.CommClause:
		return p.Comm != node
	case *ast.IfStmt:
		return p.Else == node
	}
	return false
}

func injectedInPlace(node ast.Node) bool {
	switch node.(type) {
	case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.FuncDecl:
		return true
	}
	return false
}
