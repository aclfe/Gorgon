package schemata_nodes

import (
	"go/ast"
	"go/token"
)

// GetNodePosition returns the position of an AST node.
// For BinaryExpr, it returns the operator position (OpPos).
// For IncDecStmt, it returns the token position (TokPos).
// For all other nodes, it returns the node's starting position.
func GetNodePosition(node ast.Node, fset *token.FileSet) token.Position {
	if be, ok := node.(*ast.BinaryExpr); ok {
		return fset.Position(be.OpPos)
	}
	if ids, ok := node.(*ast.IncDecStmt); ok {
		return fset.Position(ids.TokPos)
	}
	return fset.Position(node.Pos())
}
