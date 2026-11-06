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
	if len(ret.Results) == 0 {
		return false
	}
	return isInsideIfBlockFast(n, ctx.Parent, ctx.File)
}

func isInsideIfBlockFast(n ast.Node, parent ast.Node, file *ast.File) bool {
	block, ok := parent.(*ast.BlockStmt)
	if !ok || file == nil {
		return false
	}
	found := false
	ast.Inspect(file, func(x ast.Node) bool {
		if found {
			return false
		}
		if ifs, ok := x.(*ast.IfStmt); ok {
			if ifs.Body == block || ifs.Else == block {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func (EarlyReturnRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (EarlyReturnRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}
	if !isInsideIfBlockFast(n, ctx.Parent, ctx.File) {
		return nil
	}
	return &ast.EmptyStmt{Semicolon: ret.Return, Implicit: true}
}

func init() {
	mutator.Register(EarlyReturnRemoval{})
}

var _ mutator.Operator = EarlyReturnRemoval{}
var _ mutator.ContextualOperator = EarlyReturnRemoval{}
