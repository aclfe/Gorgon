package negate_condition

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type NegateCondition struct{}

func (NegateCondition) Name() string {
	return "negate_condition"
}

func (NegateCondition) CanApply(n ast.Node) bool {
	return false
}

func (NegateCondition) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return false
	}
	_, isUnaryNot := ie.Cond.(*ast.UnaryExpr)
	return !isUnaryNot
}

func (NegateCondition) Mutate(n ast.Node) ast.Node {
	return nil
}

func (NegateCondition) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ie, ok := n.(*ast.IfStmt)
	if !ok || ie.Cond == nil {
		return nil
	}
	if _, isUnaryNot := ie.Cond.(*ast.UnaryExpr); isUnaryNot {
		return nil
	}
	return &ast.IfStmt{
		Cond: &ast.UnaryExpr{
			Op: token.NOT,
			X:  ie.Cond,
		},
		Body: cloneBlockStmt(ie.Body),
		Else: cloneStmt(ie.Else),
	}
}

// cloneBlockStmt creates a shallow copy of a BlockStmt to avoid AST sharing issues.
// The individual statements are shared (read-only), but the BlockStmt wrapper is new.
func cloneBlockStmt(stmt *ast.BlockStmt) *ast.BlockStmt {
	if stmt == nil {
		return nil
	}
	// Create a new slice with the same statements (statements themselves are read-only)
	newList := make([]ast.Stmt, len(stmt.List))
	copy(newList, stmt.List)
	return &ast.BlockStmt{
		Lbrace: stmt.Lbrace,
		List:   newList,
		Rbrace: stmt.Rbrace,
	}
}

// cloneStmt creates a shallow copy of a statement. For if-else chains, this prevents
// the mutated AST from sharing references with the original AST.
func cloneStmt(stmt ast.Stmt) ast.Stmt {
	if stmt == nil {
		return nil
	}
	// Handle else-if chains (nested IfStmt)
	if ifStmt, ok := stmt.(*ast.IfStmt); ok {
		return &ast.IfStmt{
			If:   ifStmt.If,
			Init: ifStmt.Init,
			Cond: ifStmt.Cond,
			Body: cloneBlockStmt(ifStmt.Body),
			Else: cloneStmt(ifStmt.Else),
		}
	}
	// Handle else block (BlockStmt)
	if block, ok := stmt.(*ast.BlockStmt); ok {
		return cloneBlockStmt(block)
	}
	// For other statement types, return as-is (they're read-only)
	return stmt
}

func init() {
	mutator.Register(NegateCondition{})
}

var _ mutator.Operator = NegateCondition{}
var _ mutator.ContextualOperator = NegateCondition{}
