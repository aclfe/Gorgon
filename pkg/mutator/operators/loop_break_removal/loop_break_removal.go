package loop_break_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/analysis"
)

type LoopBreakRemoval struct{}

func (LoopBreakRemoval) Name() string {
	return "loop_break_removal"
}

func (LoopBreakRemoval) CanApply(n ast.Node) bool {
	return false
}

func (LoopBreakRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	branch, ok := n.(*ast.BranchStmt)
	if !ok {
		return false
	}
	if branch.Tok != 0 {
		return false
	}
	return analysis.IsInsideLoop(n, ctx.File)
}

func (LoopBreakRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (LoopBreakRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	branch, ok := n.(*ast.BranchStmt)
	if !ok || branch.Tok != 0 {
		return nil
	}
	if !analysis.IsInsideLoop(n, ctx.File) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func init() {
	mutator.Register(LoopBreakRemoval{})
}

var _ mutator.Operator = LoopBreakRemoval{}
var _ mutator.ContextualOperator = LoopBreakRemoval{}
