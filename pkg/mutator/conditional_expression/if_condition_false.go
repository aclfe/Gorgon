package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type IfConditionFalse struct{}

func (IfConditionFalse) Name() string {
	return "if_condition_false"
}

func (IfConditionFalse) CanApply(n ast.Node) bool {
	ie, ok := n.(*ast.IfStmt)
	if !ok {
		return false
	}
	return ie.Cond != nil
}

func (IfConditionFalse) Mutate(n ast.Node) ast.Node {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return nil
	}

	return &ast.IfStmt{
		Cond: &ast.Ident{Name: "false"},
		Body: ie.Body,
		Else: ie.Else,
	}
}

func init() {
	mutator.Register(IfConditionFalse{})
}

var _ mutator.Operator = IfConditionFalse{}