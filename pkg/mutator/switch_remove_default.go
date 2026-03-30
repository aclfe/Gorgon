package mutator

import (
	"go/ast"
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

func (SwitchRemoveDefault) Mutate(n ast.Node) string {
	cc, ok := n.(*ast.CaseClause)
	if !ok || cc.List == nil {
		return ""
	}
	return ""
}
