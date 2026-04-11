package reference_returns

import (
	"go/ast"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SliceReturns struct{}

func (SliceReturns) Name() string {
	return "slice_returns"
}

func (SliceReturns) CanApply(n ast.Node) bool {
	return false
}

func (SliceReturns) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if !strings.HasPrefix(ctx.ReturnType, "[]") {
		return false
	}
	expr := ret.Results[0]
	_, ok = expr.(*ast.CompositeLit)
	return ok
}

func (SliceReturns) Mutate(n ast.Node) ast.Node {
	return returnNilMutate(n)
}

func (SliceReturns) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	return returnNilMutate(n)
}

func init() {
	mutator.Register(SliceReturns{})
}

var _ mutator.Operator = SliceReturns{}
var _ mutator.ContextualOperator = SliceReturns{}
