package empty_body

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type EmptyBody struct{}

func (EmptyBody) Name() string {
	return "empty_body"
}

func (EmptyBody) CanApply(n ast.Node) bool {
	return false
}

func (EmptyBody) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	fn, ok := n.(*ast.FuncDecl)
	if !ok {
		return false
	}
	return fn.Type.Results == nil || len(fn.Type.Results.List) == 0
}

func (EmptyBody) Mutate(n ast.Node) ast.Node {
	return nil
}

func (EmptyBody) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	fn, ok := n.(*ast.FuncDecl)
	if !ok {
		return nil
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		return nil
	}
	return &ast.FuncDecl{
		Name: fn.Name,
		Type: fn.Type,
		Body: &ast.BlockStmt{
			List: []ast.Stmt{},
		},
	}
}

func init() {
	mutator.Register(EmptyBody{})
}

var _ mutator.Operator = EmptyBody{}
var _ mutator.ContextualOperator = EmptyBody{}
