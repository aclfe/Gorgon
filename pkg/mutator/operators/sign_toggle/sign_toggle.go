package sign_toggle

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/tokens"
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
	_, ok = tokens.SignToggleTokens[ue.Op]
	return ok
}

func (SignToggle) Mutate(n ast.Node) ast.Node {
	ue, ok := n.(*ast.UnaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := tokens.SwapBinaryToken(ue.Op, tokens.SignTogglePairs)
	if !ok {
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
