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

func (ArithmeticFlip) Mutate(n ast.Node) string {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return ""
	}
	//nolint:exhaustive
	switch be.Op {
	case token.ADD:
		return "-"
	case token.SUB:
		return "+"
	case token.MUL:
		return "/"
	case token.QUO:
		return "*"
	}
	return ""
}
