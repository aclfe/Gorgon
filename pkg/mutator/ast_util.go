package mutator

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator/common"
)



func FindNode(node, target ast.Node) bool {
	return common.FindNode(node, target)
}



func IsInsideCaseClause(ret *ast.ReturnStmt, file *ast.File) bool {
	return common.IsInsideCaseClause(ret, file)
}
