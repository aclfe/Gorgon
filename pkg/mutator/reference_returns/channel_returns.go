package reference_returns

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ChannelReturns struct{}

func (ChannelReturns) Name() string {
	return "channel_returns"
}

func (ChannelReturns) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	expr := ret.Results[0]
	ce, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if ident, ok := ce.Fun.(*ast.Ident); ok && ident.Name == "make" {
		if len(ce.Args) > 0 {
			_, ok = ce.Args[0].(*ast.ChanType)
			return ok
		}
	}
	return false
}

func (ChannelReturns) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func init() {
	mutator.Register(ChannelReturns{})
}

var _ mutator.Operator = ChannelReturns{}
