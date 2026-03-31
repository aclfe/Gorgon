package loop_break_first

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type LoopBreakFirst struct{}

func (LoopBreakFirst) Name() string {
	return "loop_break_first"
}

func (LoopBreakFirst) CanApply(n ast.Node) bool {
	return false
}

func (LoopBreakFirst) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	if n == nil {
		return false
	}
	switch n.(type) {
	case *ast.ForStmt, *ast.RangeStmt:
		return true
	}
	return false
}

func (LoopBreakFirst) Mutate(n ast.Node) ast.Node {
	return nil
}

func (LoopBreakFirst) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&LoopBreakFirst{}).CanApplyWithContext(n, ctx) {
		return nil
	}

	switch stmt := n.(type) {
	case *ast.ForStmt:
		if stmt.Body == nil || len(stmt.Body.List) == 0 {
			return nil
		}
		newBody := make([]ast.Stmt, len(stmt.Body.List)+1)
		copy(newBody, stmt.Body.List)
		newBody[len(newBody)-1] = &ast.BranchStmt{
			TokPos: stmt.Body.End(),
			Tok:    token.BREAK,
		}
		return &ast.ForStmt{
			For:  stmt.For,
			Init: stmt.Init,
			Cond: stmt.Cond,
			Post: stmt.Post,
			Body: &ast.BlockStmt{List: newBody},
		}
	case *ast.RangeStmt:
		if stmt.Body == nil || len(stmt.Body.List) == 0 {
			return nil
		}
		newBody := make([]ast.Stmt, len(stmt.Body.List)+1)
		copy(newBody, stmt.Body.List)
		newBody[len(newBody)-1] = &ast.BranchStmt{
			TokPos: stmt.Body.End(),
			Tok:    token.BREAK,
		}
		return &ast.RangeStmt{
			For:    stmt.For,
			Key:    stmt.Key,
			Value:  stmt.Value,
			TokPos: stmt.TokPos,
			Tok:    stmt.Tok,
			Range:  stmt.Range,
			X:      stmt.X,
			Body:   &ast.BlockStmt{List: newBody},
		}
	}
	return nil
}

func init() {
	mutator.Register(LoopBreakFirst{})
}

var _ mutator.Operator = LoopBreakFirst{}
var _ mutator.ContextualOperator = LoopBreakFirst{}
