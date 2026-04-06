package mutator

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator/common"
)

// FindNode checks if target exists in the AST subtree rooted at node.
// Deprecated: use common.FindNode or common.IsInsideCaseClause for cached variant.
func FindNode(node, target ast.Node) bool {
	return common.FindNode(node, target)
}

// IsInsideCaseClause checks if a return statement is inside a case clause.
// Uses caching for performance across multiple calls.
func IsInsideCaseClause(ret *ast.ReturnStmt, file *ast.File) bool {
	return common.IsInsideCaseClause(ret, file)
}
