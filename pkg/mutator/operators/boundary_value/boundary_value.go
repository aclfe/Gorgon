// Package boundary_value provides boundary comparison mutation operators.
// Mutates comparison operators to their boundary variants: < ↔ <=, > ↔ >=
package boundary_value

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/tokens"
)

type BoundaryValue struct{}

func (BoundaryValue) Name() string {
	return "boundary_value"
}

func (BoundaryValue) CanApply(n ast.Node) bool {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	_, ok = tokens.BoundaryValueTokens[be.Op]
	return ok
}

func (BoundaryValue) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	newOp, ok := tokens.SwapBinaryToken(be.Op, tokens.BoundaryValuePairs)
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
	mutator.Register(BoundaryValue{})
}