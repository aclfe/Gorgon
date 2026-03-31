package sign_toggle

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SignToggle struct{}

func (SignToggle) Name() string {
	return "sign_toggle"
}

func (SignToggle) CanApply(n ast.Node) bool {
	ue, ok := n.(*ast.UnaryExpr)
	if !ok {
		return false
	}
	return ue.Op == token.SUB || ue.Op == token.ADD
}

func (SignToggle) Mutate(n ast.Node) ast.Node {
	ue, ok := n.(*ast.UnaryExpr)
	if !ok {
		return nil
	}
	var newOp token.Token
	switch ue.Op {
	case token.SUB:
		newOp = token.ADD
	case token.ADD:
		newOp = token.SUB
	default:
		return nil
	}
	return &ast.UnaryExpr{
		Op: newOp,
		X:  ue.X,
	}
}

func init() {
	mutator.Register(SignToggle{})
}
