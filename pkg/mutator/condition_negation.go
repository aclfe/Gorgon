// Package mutator provides mutation operators for the gorgon project.
package mutator

import (
	"go/ast"
	"go/token"
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
	//nolint:exhaustive
	switch be.Op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

func (ConditionNegation) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	var newOp token.Token
	//nolint:exhaustive
	switch be.Op {
	case token.EQL:
		newOp = token.NEQ
	case token.NEQ:
		newOp = token.EQL
	case token.LSS:
		newOp = token.GEQ
	case token.LEQ:
		newOp = token.GTR
	case token.GTR:
		newOp = token.LEQ
	case token.GEQ:
		newOp = token.LSS
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
	Register(ConditionNegation{})
}
