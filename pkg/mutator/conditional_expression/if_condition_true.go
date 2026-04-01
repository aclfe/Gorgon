package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type IfConditionTrue struct{}

func (IfConditionTrue) Name() string {
	return "if_condition_true"
}

func (IfConditionTrue) CanApply(n ast.Node) bool {
	ie, ok := n.(*ast.IfStmt)
	if !ok {
		return false
	}
	return ie.Cond != nil
}

func (IfConditionTrue) Mutate(n ast.Node) ast.Node {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return nil
	}

	return &ast.IfStmt{
		Cond: &ast.Ident{Name: "true"},
		Body: ie.Body,
		Else: ie.Else,
	}
}

func init() {
	mutator.Register(IfConditionTrue{})
}

var _ mutator.Operator = IfConditionTrue{}