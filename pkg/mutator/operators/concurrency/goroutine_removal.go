package concurrency

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type GoroutineRemoval struct{}

func (GoroutineRemoval) Name() string {
	return "goroutine_removal"
}

func (GoroutineRemoval) CanApply(n ast.Node) bool {
	gr := GoroutineRemoval{}
	return gr.CanApplyWithContext(n, mutator.Context{})
}

func (GoroutineRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	_, ok := n.(*ast.GoStmt)
	return ok
}

func (GoroutineRemoval) Mutate(n ast.Node) ast.Node {
	gr := GoroutineRemoval{}
	return gr.MutateWithContext(n, mutator.Context{})
}

func (GoroutineRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	goStmt, ok := n.(*ast.GoStmt)
	if !ok {
		return nil
	}
	return &ast.ExprStmt{X: goStmt.Call}
}

func init() {
	mutator.Register(GoroutineRemoval{})
}

var _ mutator.Operator = GoroutineRemoval{}
var _ mutator.ContextualOperator = GoroutineRemoval{}
