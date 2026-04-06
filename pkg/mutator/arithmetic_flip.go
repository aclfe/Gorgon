// Package mutator provides mutation operators for the gorgon project.
package mutator

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator/common"
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
	_, ok = common.ArithmeticFlipTokens[be.Op]
	return ok
}

func (ArithmeticFlip) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := common.SwapBinaryToken(be.Op, common.ArithmeticFlipPairs)
	if !ok {
		return nil
	}
	return &ast.BinaryExpr{
		X:  be.X,
		Op: newOp,
		Y:  be.Y,
	}
}

func init() {
	Register(ArithmeticFlip{})
}
