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

func (ConditionNegation) Mutate(n ast.Node) string {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return ""
	}
	//nolint:exhaustive
	switch be.Op {
	case token.EQL:
		return "!="
	case token.NEQ:
		return "=="
	case token.LSS:
		return ">="
	case token.LEQ:
		return ">"
	case token.GTR:
		return "<="
	case token.GEQ:
		return "<"
	}
	return ""
}
