package defer_panic_recover

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type PanicRemoval struct{}

func (PanicRemoval) Name() string {
	return "panic_removal"
}

func (PanicRemoval) CanApply(n ast.Node) bool {
	return false
}

func (PanicRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	exprStmt, ok := n.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := exprStmt.X.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "panic"
}

func (PanicRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (PanicRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&PanicRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func init() {
	mutator.Register(PanicRemoval{})
}

var _ mutator.Operator = PanicRemoval{}
var _ mutator.ContextualOperator = PanicRemoval{}
