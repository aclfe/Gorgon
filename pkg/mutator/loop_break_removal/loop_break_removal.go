package loop_break_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
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
	return isInsideLoop(n, ctx.File)
}

func isInsideLoop(n ast.Node, file *ast.File) bool {
	if file == nil {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if node == n {
			found = true
			return false
		}
		return true
	})
	if !found {
		return false
	}
	var parentLoop ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == n {
			return false
		}
		switch node.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			if containsNode(node, n) {
				parentLoop = node
				return false
			}
		}
		return true
	})
	return parentLoop != nil
}

func containsNode(container ast.Node, target ast.Node) bool {
	found := false
	ast.Inspect(container, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

func (LoopBreakRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (LoopBreakRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&LoopBreakRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func init() {
	mutator.Register(LoopBreakRemoval{})
}

var _ mutator.Operator = LoopBreakRemoval{}
var _ mutator.ContextualOperator = LoopBreakRemoval{}
