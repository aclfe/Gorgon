// Package boundary_value provides boundary comparison mutation operators.
// Mutates comparison operators to their boundary variants: < ↔ <=, > ↔ >=
package boundary_value

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
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
	switch be.Op {
	case token.LSS, token.GTR, token.LEQ, token.GEQ:
		return true
	}
	return false
}

func (BoundaryValue) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	var newOp token.Token
	switch be.Op {
	case token.LSS:
		newOp = token.LEQ
	case token.GTR:
		newOp = token.GEQ
	case token.LEQ:
		newOp = token.LSS
	case token.GEQ:
		newOp = token.GTR
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
	mutator.Register(BoundaryValue{})
}