package mutator

import (
	"go/ast"
	"strings"
)

type SliceReturns struct{}

func (SliceReturns) Name() string {
	return "slice_returns"
}

func (SliceReturns) CanApply(n ast.Node) bool {
	return false
}

func (SliceReturns) CanApplyWithContext(n ast.Node, ctx Context) bool {
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
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func (SliceReturns) MutateWithContext(n ast.Node, ctx Context) ast.Node {
	return SliceReturns{}.Mutate(n)
}

func init() {
	Register(SliceReturns{})
}

var _ Operator = SliceReturns{}
var _ ContextualOperator = SliceReturns{}
