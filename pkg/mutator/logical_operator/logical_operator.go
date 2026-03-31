// Package logical_operator provides logical operator replacement mutations.
// Mutates logical operators: && ↔ ||
package logical_operator

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
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
	switch be.Op {
	case token.LAND, token.LOR:
		return true
	}
	return false
}

func (LogicalOperator) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	var newOp token.Token
	switch be.Op {
	case token.LAND:
		newOp = token.LOR
	case token.LOR:
		newOp = token.LAND
	default:
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