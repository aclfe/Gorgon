package reference_returns

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type InterfaceReturns struct{}

func (InterfaceReturns) Name() string {
	return "interface_returns"
}

func (InterfaceReturns) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	expr := ret.Results[0]
	return isInterfaceLiteral(expr)
}

func (InterfaceReturns) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if ctx.ReturnType != "interface{}" {
		return false
	}
	expr := ret.Results[0]
	return isInterfaceLiteral(expr)
}

func (InterfaceReturns) Mutate(n ast.Node) ast.Node {
	return nil
}

func (InterfaceReturns) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func isInterfaceLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING || e.Kind == token.CHAR || e.Kind == token.INT
	case *ast.Ident:
		return false
	case *ast.ParenExpr:
		return isInterfaceLiteral(e.X)
	default:
		return false
	}
}

func init() {
	mutator.Register(InterfaceReturns{})
}

var _ mutator.Operator = InterfaceReturns{}
var _ mutator.ContextualOperator = InterfaceReturns{}
