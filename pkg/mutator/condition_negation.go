// Package mutator provides mutation operators for the gorgon project.
package mutator

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator/common"
)

type ConditionNegation struct{}

func (ConditionNegation) Name() string {
	return "condition_negation"
}

func (ConditionNegation) CanApply(n ast.Node) bool {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	_, ok = common.ComparisonNegationTokens[be.Op]
	return ok
}

func (ConditionNegation) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := common.SwapBinaryToken(be.Op, common.ComparisonNegationPairs)
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
	Register(ConditionNegation{})
}
