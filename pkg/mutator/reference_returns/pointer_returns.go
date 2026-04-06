package reference_returns

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type PointerReturns struct{}

func (PointerReturns) Name() string {
	return "pointer_returns"
}

func (PointerReturns) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	expr := ret.Results[0]
	ue, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return false
	}
	return ue.Op == token.AND
}

func (PointerReturns) Mutate(n ast.Node) ast.Node {
	return returnNilMutate(n)
}

func init() {
	mutator.Register(PointerReturns{})
}

var _ mutator.Operator = PointerReturns{}
