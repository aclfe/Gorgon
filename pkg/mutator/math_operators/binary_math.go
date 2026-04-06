package math_operators

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/common"
)

type BinaryMath struct{}

func (BinaryMath) Name() string {
	return "binary_math"
}

func (BinaryMath) CanApply(n ast.Node) bool {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	_, ok = common.BinaryMathTokens[be.Op]
	return ok
}

func (BinaryMath) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := common.SwapBinaryToken(be.Op, common.BinaryMathPairs)
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
	mutator.Register(BinaryMath{})
}
