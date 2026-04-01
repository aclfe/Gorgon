package math_operators

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
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
	switch be.Op {
	case token.REM, token.AND, token.OR, token.SHL, token.SHR:
		return true
	}
	return false
}

func (BinaryMath) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}

	var newOp token.Token
	switch be.Op {
	case token.REM:
		newOp = token.MUL
	case token.AND:
		newOp = token.OR
	case token.OR:
		newOp = token.AND
	case token.SHL:
		newOp = token.SHR
	case token.SHR:
		newOp = token.SHL
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
	mutator.Register(BinaryMath{})
}
