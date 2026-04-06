package function_call_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type FunctionCallRemoval struct{}

func (FunctionCallRemoval) Name() string {
	return "function_call_removal"
}

func (FunctionCallRemoval) CanApply(n ast.Node) bool {
	_, ok := n.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := n.(*ast.ExprStmt)
	if !ok {
		return false
	}
	_, ok = call.X.(*ast.CallExpr)
	return ok
}

func (FunctionCallRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	exprStmt, ok := n.(*ast.ExprStmt)
	if !ok {
		return false
	}
	_, ok = exprStmt.X.(*ast.CallExpr)
	return ok
}

func (FunctionCallRemoval) Mutate(n ast.Node) ast.Node {
	if !(&FunctionCallRemoval{}).CanApply(n) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func (FunctionCallRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&FunctionCallRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func init() {
	mutator.Register(FunctionCallRemoval{})
}

var _ mutator.Operator = FunctionCallRemoval{}
var _ mutator.ContextualOperator = FunctionCallRemoval{}
