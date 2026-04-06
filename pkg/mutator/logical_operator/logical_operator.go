// Package logical_operator provides logical operator replacement mutations.
// Mutates logical operators: && ↔ ||
package logical_operator

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/common"
)

type LogicalOperator struct{}

func (LogicalOperator) Name() string {
	return "logical_operator"
}

func (LogicalOperator) CanApply(n ast.Node) bool {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	_, ok = common.LogicalOperatorTokens[be.Op]
	return ok
}

func (LogicalOperator) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := common.SwapBinaryToken(be.Op, common.LogicalOperatorPairs)
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
	mutator.Register(LogicalOperator{})
}