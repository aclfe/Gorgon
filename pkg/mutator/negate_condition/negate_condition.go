package negate_condition

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type NegateCondition struct{}

func (NegateCondition) Name() string {
	return "negate_condition"
}

func (NegateCondition) CanApply(n ast.Node) bool {
	return false
}

func (NegateCondition) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return false
	}
	_, isUnaryNot := ie.Cond.(*ast.UnaryExpr)
	return !isUnaryNot
}

func (NegateCondition) Mutate(n ast.Node) ast.Node {
	return nil
}

func (NegateCondition) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return nil
	}
	if _, isUnaryNot := ie.Cond.(*ast.UnaryExpr); isUnaryNot {
		return nil
	}
	return &ast.IfStmt{
		Cond: &ast.UnaryExpr{
			Op: token.NOT,
			X:  ie.Cond,
		},
		Body: ie.Body,
		Else: ie.Else,
	}
}

func init() {
	mutator.Register(NegateCondition{})
}

var _ mutator.Operator = NegateCondition{}
var _ mutator.ContextualOperator = NegateCondition{}
