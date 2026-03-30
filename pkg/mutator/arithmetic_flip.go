// Package mutator provides mutation operators for the gorgon project.
package mutator

import (
	"go/ast"
	"go/token"
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
	//nolint:exhaustive
	switch be.Op {
	case token.ADD, token.SUB, token.MUL, token.QUO:
		return true
	}
	return false
}

func (ArithmeticFlip) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	var newOp token.Token
	//nolint:exhaustive
	switch be.Op {
	case token.ADD:
		newOp = token.SUB
	case token.SUB:
		newOp = token.ADD
	case token.MUL:
		newOp = token.QUO
	case token.QUO:
		newOp = token.MUL
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
	Register(ArithmeticFlip{})
}
