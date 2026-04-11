package conditional_expression

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
)

// ConditionMutation defines a condition mutation operation.
type ConditionMutation struct {
	name        string
	nodeType    ast.Node
	replaceWith string
}

// buildConditionMutator creates a mutator for a specific condition mutation.
func buildConditionMutator(m ConditionMutation) mutator.Operator {
	return &conditionMutator{
		name:        m.name,
		nodeType:    m.nodeType,
		replaceWith: m.replaceWith,
	}
}

type conditionMutator struct {
	name        string
	nodeType    ast.Node
	replaceWith string
}

func (m *conditionMutator) Name() string {
	return m.name
}

func (m *conditionMutator) CanApply(n ast.Node) bool {
	switch m.nodeType.(type) {
	case *ast.IfStmt:
		ie, ok := n.(*ast.IfStmt)
		return ok && ie.Cond != nil
	case *ast.ForStmt:
		fs, ok := n.(*ast.ForStmt)
		return ok && fs.Cond != nil
	default:
		return false
	}
}

func (m *conditionMutator) Mutate(n ast.Node) ast.Node {
	switch m.nodeType.(type) {
	case *ast.IfStmt:
		ie, ok := n.(*ast.IfStmt)
		if !ok || ie.Cond == nil {
			return nil
		}
		return &ast.IfStmt{
			Cond: &ast.Ident{Name: m.replaceWith},
			Body: ie.Body,
			Else: ie.Else,
		}
	case *ast.ForStmt:
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return nil
		}
		return &ast.ForStmt{
			Init: fs.Init,
			Cond: &ast.Ident{Name: m.replaceWith},
			Post: fs.Post,
			Body: fs.Body,
		}
	default:
		return nil
	}
}

func (m *conditionMutator) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	return m.CanApply(n)
}

func (m *conditionMutator) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	return m.Mutate(n)
}
