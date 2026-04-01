package loop_body_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type LoopBodyRemoval struct{}

func (LoopBodyRemoval) Name() string {
	return "loop_body_removal"
}

func (LoopBodyRemoval) CanApply(n ast.Node) bool {
	return false
}

func (LoopBodyRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	if n == nil {
		return false
	}
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

func (LoopBodyRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (LoopBodyRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&LoopBodyRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}

	switch stmt := n.(type) {
	case *ast.ForStmt:
		return &ast.ForStmt{
			For:  stmt.For,
			Init: stmt.Init,
			Cond: stmt.Cond,
			Post: stmt.Post,
			Body: &ast.BlockStmt{},
		}
	case *ast.RangeStmt:
		return &ast.RangeStmt{
			For:    stmt.For,
			Key:    stmt.Key,
			Value:  stmt.Value,
			TokPos: stmt.TokPos,
			Tok:    stmt.Tok,
			Range:  stmt.Range,
			X:      stmt.X,
			Body:   &ast.BlockStmt{},
		}
	}
	return nil
}

func init() {
	mutator.Register(LoopBodyRemoval{})
}

var _ mutator.Operator = LoopBodyRemoval{}
var _ mutator.ContextualOperator = LoopBodyRemoval{}
