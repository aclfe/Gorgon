package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ForConditionFalse struct{}

func (ForConditionFalse) Name() string {
	return "for_condition_false"
}

func (ForConditionFalse) CanApply(n ast.Node) bool {
	fs, ok := n.(*ast.ForStmt)
	if !ok {
		return false
	}
	return fs.Cond != nil
}

func (ForConditionFalse) Mutate(n ast.Node) ast.Node {
	fs, ok := n.(*ast.ForStmt)
	if !ok || fs.Cond == nil {
		return nil
	}

	return &ast.ForStmt{
		Init: fs.Init,
		Cond: &ast.Ident{Name: "false"},
		Post: fs.Post,
		Body: fs.Body,
	}
}

func init() {
	mutator.Register(ForConditionFalse{})
}

var _ mutator.Operator = ForConditionFalse{}