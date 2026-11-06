package variable_replacement

import (
	"go/ast"
	"go/types"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/analysis"
)

type VariableReplacement struct{}

func (VariableReplacement) Name() string             { return "variable_replacement" }
func (VariableReplacement) CanApply(ast.Node) bool   { return false }
func (VariableReplacement) Mutate(ast.Node) ast.Node { return nil }

func (VariableReplacement) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ident, ok := n.(*ast.Ident)
	if !ok || ident.Name == "_" || ctx.EnclosingFunc == nil {
		return false
	}
	if ctx.Info != nil {
		return typedReplacement(ident, ctx) != ""
	}
	if isExcludedParent(ident, ctx.Parent) {
		return false
	}
	typeMap := analysis.BuildTypeMap(ctx.EnclosingFunc)
	identType, ok := typeMap[ident.Name]
	if !ok || identType == "" {
		return false
	}
	return analysis.HasCompatibleVar(analysis.CollectVars(ctx.EnclosingFunc), ident.Name, identType)
}

func (VariableReplacement) MutationHint(n ast.Node, ctx mutator.Context) string {
	ident, ok := n.(*ast.Ident)
	if !ok || ctx.Info == nil {
		return ""
	}
	return typedReplacement(ident, ctx)
}

func (VariableReplacement) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ident, ok := n.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	if ctx.Hint != "" {
		if ctx.Hint == ident.Name {
			return nil
		}
		return &ast.Ident{NamePos: ident.NamePos, Name: ctx.Hint}
	}
	if ctx.Info != nil {
		if repl := typedReplacement(ident, ctx); repl != "" {
			return &ast.Ident{NamePos: ident.NamePos, Name: repl}
		}
		return nil
	}
	if ctx.EnclosingFunc == nil {
		return nil
	}
	if isExcludedParent(ident, ctx.Parent) {
		return nil
	}
	typeMap := analysis.BuildTypeMap(ctx.EnclosingFunc)
	identType, ok := typeMap[ident.Name]
	if !ok || identType == "" {
		return nil
	}
	repl := analysis.FindCompatibleVar(analysis.CollectVars(ctx.EnclosingFunc), ident.Name, identType)
	if repl.Name == "" {
		return nil
	}
	return &ast.Ident{NamePos: ident.NamePos, Name: repl.Name}
}

func isExcludedParent(ident *ast.Ident, parent ast.Node) bool {
	switch parent.(type) {
	case *ast.IncDecStmt:
		return true
	}
	switch p := parent.(type) {
	case *ast.SelectorExpr:
		return p.Sel == ident
	case *ast.KeyValueExpr:
		return p.Key == ident
	case *ast.AssignStmt:
		for _, lhs := range p.Lhs {
			if lhs == ident {
				return true
			}
		}
	case *ast.RangeStmt:
		return p.Key == ident || p.Value == ident
	case *ast.TypeAssertExpr:
		return p.Type == ident
	case *ast.CallExpr:
		return p.Fun == ident
	case *ast.CompositeLit:
		return p.Type == ident
	}
	return false
}

func init() { mutator.Register(VariableReplacement{}) }

var _ mutator.Operator = VariableReplacement{}
var _ mutator.ContextualOperator = VariableReplacement{}

func (VariableReplacement) RequiresTypeCheck() bool {
	return true
}

func typedReplacement(ident *ast.Ident, ctx mutator.Context) string {
	if ctx.Info == nil || ctx.Pkg == nil {
		return ""
	}
	obj, ok := ctx.Info.Uses[ident].(*types.Var)
	if !ok || obj.IsField() || obj.Pkg() != ctx.Pkg {
		return ""
	}
	pkgScope := ctx.Pkg.Scope()
	scope := pkgScope.Innermost(ident.Pos())
	if scope == nil || scope == pkgScope {
		return ""
	}
	for s := scope; s != nil && s != pkgScope && s.Parent() != pkgScope && s != types.Universe; s = s.Parent() {
		for _, name := range s.Names() {
			if name == ident.Name || name == "_" {
				continue
			}
			v, ok := s.Lookup(name).(*types.Var)
			if !ok || v.IsField() || v == obj {
				continue
			}
			if !(v.Pos() < ident.Pos()) {
				continue
			}
			if _, found := scope.LookupParent(name, ident.Pos()); found != v {
				continue
			}
			if !types.Identical(v.Type(), obj.Type()) {
				continue
			}
			return name
		}
	}
	return ""
}

var _ mutator.HintingOperator = VariableReplacement{}
