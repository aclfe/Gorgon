package variable_replacement

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type VariableReplacement struct{}

func (VariableReplacement) Name() string {
	return "variable_replacement"
}

func (VariableReplacement) CanApply(n ast.Node) bool {
	return false
}

func (VariableReplacement) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ident, ok := n.(*ast.Ident)
	if !ok {
		return false
	}
	if ident.Name == "_" {
		return false
	}
	return ctx.EnclosingFunc != nil
}

func (VariableReplacement) Mutate(n ast.Node) ast.Node {
	return nil
}

func (VariableReplacement) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ident, ok := n.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	if ctx.EnclosingFunc == nil {
		return nil
	}
	replacement := findReplacementVar(ctx.EnclosingFunc, ident.Name)
	if replacement == "" {
		return nil
	}
	return &ast.Ident{NamePos: ident.NamePos, Name: replacement}
}

func findReplacementVar(fn *ast.FuncDecl, exclude string) string {
	var candidates []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name != exclude && ident.Name != "_" {
						candidates = append(candidates, ident.Name)
					}
				}
			}
		}
		return true
	})
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func init() {
	mutator.Register(VariableReplacement{})
}

var _ mutator.Operator = VariableReplacement{}
var _ mutator.ContextualOperator = VariableReplacement{}
