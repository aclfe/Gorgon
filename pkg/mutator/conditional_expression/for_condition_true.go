package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ForConditionTrue struct{}

func (ForConditionTrue) Name() string {
	return "for_condition_true"
}

func (ForConditionTrue) CanApply(n ast.Node) bool {
	fs, ok := n.(*ast.ForStmt)
	if !ok {
		return false
	}
	return fs.Cond != nil
}

func (ForConditionTrue) Mutate(n ast.Node) ast.Node {
	fs, ok := n.(*ast.ForStmt)
	if !ok || fs.Cond == nil {
		return nil
	}

	return &ast.ForStmt{
		Init: fs.Init,
		Cond: &ast.Ident{Name: "true"},
		Post: fs.Post,
		Body: fs.Body,
	}
}

func init() {
	mutator.Register(ForConditionTrue{})
}

var _ mutator.Operator = ForConditionTrue{}
