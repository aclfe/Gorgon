package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func init() {
	mutator.Register(buildConditionMutator(ConditionMutation{
		name:        "for_condition_true",
		nodeType:    &ast.ForStmt{},
		replaceWith: "true",
	}))
}
