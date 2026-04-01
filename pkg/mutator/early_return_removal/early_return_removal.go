package early_return_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type EarlyReturnRemoval struct{}

func (EarlyReturnRemoval) Name() string {
	return "early_return_removal"
}

func (EarlyReturnRemoval) CanApply(n ast.Node) bool {
	return false
}

func (EarlyReturnRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok {
		return false
	}
	if ret.Results == nil || len(ret.Results) == 0 {
		return false
	}
	return isInsideIfBlock(n, ctx.File)
}

func isInsideIfBlock(n ast.Node, file *ast.File) bool {
	if file == nil {
		return false
	}
	var parentIf *ast.IfStmt
	ast.Inspect(file, func(node ast.Node) bool {
		if node == n {
			return false
		}
		if ifStmt, ok := node.(*ast.IfStmt); ok {
			if containsNode(ifStmt.Body, n) {
				parentIf = ifStmt
				return false
			}
		}
		return true
	})
	return parentIf != nil
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

func (EarlyReturnRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (EarlyReturnRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&EarlyReturnRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	ret := n.(*ast.ReturnStmt)
	return &ast.ReturnStmt{
		Return:  ret.Return,
		Results: nil,
	}
}

func init() {
	mutator.Register(EarlyReturnRemoval{})
}

var _ mutator.Operator = EarlyReturnRemoval{}
var _ mutator.ContextualOperator = EarlyReturnRemoval{}
