package early_return_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type EarlyReturnRemoval struct{}

func (EarlyReturnRemoval) Name() string {
	return "early_return_removal"
}

func (EarlyReturnRemoval) CanApply(n ast.Node) bool {
	return false
}

func (EarlyReturnRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok {
		return false
	}
	if ret.Results == nil || len(ret.Results) == 0 {
		return false
	}
	return isInsideIfBlockFast(n, ctx.Parent)
}

func isInsideIfBlockFast(n ast.Node, parent ast.Node) bool {
	_, ok := parent.(*ast.IfStmt)
	return ok
}

func (EarlyReturnRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (EarlyReturnRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || ret.Results == nil || len(ret.Results) == 0 {
		return nil
	}
	if !isInsideIfBlockFast(n, ctx.Parent) {
		return nil
	}
	return &ast.ReturnStmt{
		Return:  ret.Return,
		Results: nil,
	}
}

func init() {
	mutator.Register(EarlyReturnRemoval{})
}

var _ mutator.Operator = EarlyReturnRemoval{}
var _ mutator.ContextualOperator = EarlyReturnRemoval{}
