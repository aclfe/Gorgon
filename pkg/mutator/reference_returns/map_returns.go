package reference_returns

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type MapReturns struct{}

func (MapReturns) Name() string {
	return "map_returns"
}

func (MapReturns) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	expr := ret.Results[0]
	cl, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	_, ok = cl.Type.(*ast.MapType)
	return ok
}

func (MapReturns) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func init() {
	mutator.Register(MapReturns{})
}

var _ mutator.Operator = MapReturns{}
