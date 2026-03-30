package mutator

import (
	"go/ast"
	"go/token"
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
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func init() {
	Register(PointerReturns{})
}

var _ Operator = PointerReturns{}
