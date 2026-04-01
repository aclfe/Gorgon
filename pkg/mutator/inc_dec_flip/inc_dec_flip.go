package inc_dec_flip

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type IncDecFlip struct{}

func (IncDecFlip) Name() string {
	return "inc_dec_flip"
}

func (IncDecFlip) CanApply(n ast.Node) bool {
	ids, ok := n.(*ast.IncDecStmt)
	if !ok {
		return false
	}
	return ids.Tok == token.INC || ids.Tok == token.DEC
}

func (IncDecFlip) Mutate(n ast.Node) ast.Node {
	ids, ok := n.(*ast.IncDecStmt)
	if !ok {
		return nil
	}
	var newTok token.Token
	switch ids.Tok {
	case token.INC:
		newTok = token.DEC
	case token.DEC:
		newTok = token.INC
	default:
		return nil
	}
	return &ast.IncDecStmt{
		X:   ids.X,
		Tok: newTok,
	}
}

func init() {
	mutator.Register(IncDecFlip{})
}
