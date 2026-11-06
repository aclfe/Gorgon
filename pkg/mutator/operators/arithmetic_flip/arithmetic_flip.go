// Package arithmetic_flip provides arithmetic operator flip mutations.
package arithmetic_flip

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/tokens"
)

type ArithmeticFlip struct{}

func (ArithmeticFlip) Name() string {
	return "arithmetic_flip"
}

func (ArithmeticFlip) CanApply(n ast.Node) bool {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	// Only apply to numeric operators, not string concatenation
	// The + operator can be used for both, so we need to be careful
	if be.Op == token.ADD {
		// Skip if either operand looks like a string literal
		if isStringLiteral(be.X) || isStringLiteral(be.Y) {
			return false
		}
	}
	_, ok = tokens.ArithmeticFlipTokens[be.Op]
	return ok
}

func isStringLiteral(n ast.Expr) bool {
	lit, ok := n.(*ast.BasicLit)
	return ok && lit.Kind == token.STRING
}

func (ArithmeticFlip) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := tokens.SwapBinaryToken(be.Op, tokens.ArithmeticFlipPairs)
	if !ok {
		return nil
	}
	return &ast.BinaryExpr{
		X:  be.X,
		Op: newOp,
		Y:  be.Y,
	}
}

func (a ArithmeticFlip) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	if !a.CanApply(n) {
		return false
	}
	if ctx.Info != nil {
		be := n.(*ast.BinaryExpr)
		if t := ctx.Info.TypeOf(be); t != nil {
			if b, ok := t.Underlying().(*types.Basic); ok && b.Info()&types.IsString != 0 {
				return false
			}
		}
	}
	return true
}

func (a ArithmeticFlip) MutateWithContext(n ast.Node, _ mutator.Context) ast.Node {
	return a.Mutate(n)
}

func init() {
	mutator.Register(ArithmeticFlip{})
}

var _ mutator.ContextualOperator = ArithmeticFlip{}
