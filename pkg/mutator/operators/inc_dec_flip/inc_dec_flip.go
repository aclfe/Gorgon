package inc_dec_flip

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/tokens"
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
	_, ok = tokens.IncDecTokens[ids.Tok]
	return ok
}

func (IncDecFlip) Mutate(n ast.Node) ast.Node {
	ids, ok := n.(*ast.IncDecStmt)
	if !ok {
		return nil
	}
	newTok, ok := tokens.SwapBinaryToken(ids.Tok, tokens.IncDecPairs)
	if !ok {
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
