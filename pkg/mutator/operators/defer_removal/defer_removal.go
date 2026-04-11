package defer_removal

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type DeferRemoval struct{}

func (DeferRemoval) Name() string {
	return "defer_removal"
}

func (DeferRemoval) CanApply(n ast.Node) bool {
	return false
}

func (DeferRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	deferStmt, ok := n.(*ast.DeferStmt)
	if !ok {
		return false
	}
	return deferStmt.Call != nil
}

func (DeferRemoval) Mutate(n ast.Node) ast.Node {
	return nil
}

func (DeferRemoval) MutateWithContext(n ast.Node, _ mutator.Context) ast.Node {
	_, ok := n.(*ast.DeferStmt)
	if !ok {
		return nil
	}
	return &ast.EmptyStmt{}
}

func init() {
	mutator.Register(DeferRemoval{})
}

var _ mutator.Operator = DeferRemoval{}
var _ mutator.ContextualOperator = DeferRemoval{}
