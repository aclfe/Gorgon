package schemata_nodes

import (
	"go/ast"
	"go/token"
)

// GetNodePosition returns the most precise position for a node.
// For operators (like +, -, ++, etc.), it returns the position of the operator token
// rather than the start of the expression/statement. This ensures consistent position
// matching between mutant generation and AST transformation.
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
