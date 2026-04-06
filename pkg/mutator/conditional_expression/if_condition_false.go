package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func init() {
	mutator.Register(buildConditionMutator(ConditionMutation{
		name:        "if_condition_false",
		nodeType:    &ast.IfStmt{},
		replaceWith: "false",
	}))
}