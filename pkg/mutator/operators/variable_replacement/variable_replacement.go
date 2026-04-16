package variable_replacement

import (
	"go/ast"

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

func (VariableReplacement) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ident, ok := n.(*ast.Ident)
	if !ok || ident.Name == "_" || ctx.EnclosingFunc == nil {
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
	case *ast.FuncDecl, *ast.File, *ast.Field,
		*ast.StarExpr, *ast.ArrayType, *ast.MapType, *ast.ChanType,
		*ast.InterfaceType, *ast.StructType, *ast.FuncType,
		*ast.IncDecStmt:
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
