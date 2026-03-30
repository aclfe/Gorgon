package switch_mutations

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SwitchRemoveDefault struct{}

func (SwitchRemoveDefault) Name() string {
	return "switch_remove_default"
}

func (SwitchRemoveDefault) CanApply(n ast.Node) bool {
	cc, ok := n.(*ast.CaseClause)
	if !ok {
		return false
	}
	return cc.List == nil
}

func (SwitchRemoveDefault) Mutate(n ast.Node) ast.Node {
	cc, ok := n.(*ast.CaseClause)
	if !ok {
		return nil
	}
	return &ast.CaseClause{
		Case:  cc.Case,
		List:  cc.List,
		Colon: cc.Colon,
		Body:  []ast.Stmt{},
	}
}

func (SwitchRemoveDefault) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	return SwitchRemoveDefault{}.CanApply(n)
}

func (SwitchRemoveDefault) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	cc, ok := n.(*ast.CaseClause)
	if !ok {
		return nil
	}

	return &ast.CaseClause{
		Case:  cc.Case,
		List:  cc.List,
		Colon: cc.Colon,
		Body:  []ast.Stmt{},
	}
}

func init() {
	mutator.Register(SwitchRemoveDefault{})
}
