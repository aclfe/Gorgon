package schemata_nodes

import (
	"go/ast"
	"go/token"
)

func GetNodePosition(node ast.Node, fset *token.FileSet) token.Position {
	switch n := node.(type) {
	case *ast.BinaryExpr:
		return fset.Position(n.OpPos)
	case *ast.UnaryExpr:
		return fset.Position(n.OpPos)
	case *ast.IncDecStmt:
		return fset.Position(n.TokPos)
	case *ast.AssignStmt:
		return fset.Position(n.TokPos)
	case *ast.BranchStmt:
		return fset.Position(n.TokPos)
	default:
		return fset.Position(node.Pos())
	}
}
