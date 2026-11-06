package conditional_expression

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ConditionMutation struct {
	name        string
	nodeType    ast.Node
	replaceWith string
}

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

func (m *conditionMutator) ConditionKind() mutator.ConditionKind {
	if m.replaceWith == "true" {
		return mutator.ConditionForceTrue
	}
	return mutator.ConditionForceFalse
}

func (m *conditionMutator) CanApply(n ast.Node) bool {
	switch m.nodeType.(type) {
	case *ast.IfStmt:
		ie, ok := n.(*ast.IfStmt)
		if !ok || ie.Cond == nil {
			return false
		}
		if ie.Init != nil {
			assign, ok := ie.Init.(*ast.AssignStmt)
			if !ok {
				return false
			}
			// Short variable declarations (:=) in Init declare vars that are
			// typically only used in Cond. Replacing Cond with true/false would
			// make those vars unused, causing a compile error.
			if assign.Tok == token.DEFINE {
				return false
			}
		}
		return isSafeCondition(ie.Cond) && !isLiteral(ie.Cond, m.replaceWith)
	case *ast.ForStmt:
		fs, ok := n.(*ast.ForStmt)
		return ok && fs.Cond != nil && isSafeCondition(fs.Cond) && !isLiteral(fs.Cond, m.replaceWith)
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
			If:   ie.If,
			Init: ie.Init,
			Cond: &ast.Ident{
				NamePos: ie.Cond.Pos(),
				Name:    m.replaceWith,
			},
			Body: ie.Body,
			Else: ie.Else,
		}
	case *ast.ForStmt:
		fs, ok := n.(*ast.ForStmt)
		if !ok || fs.Cond == nil {
			return nil
		}
		return &ast.ForStmt{
			For:  fs.For,
			Init: fs.Init,
			Cond: &ast.Ident{
				NamePos: fs.Cond.Pos(),
				Name:    m.replaceWith,
			},
			Post: fs.Post,
			Body: fs.Body,
		}
	default:
		return nil
	}
}

func (m *conditionMutator) CanApplyWithContext(n ast.Node, _ mutator.Context) bool {
	return m.CanApply(n)
}

func (m *conditionMutator) MutateWithContext(n ast.Node, _ mutator.Context) ast.Node {
	return m.Mutate(n)
}

func isLiteral(cond ast.Expr, lit string) bool {
	for {
		p, ok := cond.(*ast.ParenExpr)
		if !ok {
			break
		}
		cond = p.X
	}
	id, ok := cond.(*ast.Ident)
	return ok && id.Name == lit
}

func isSafeCondition(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return true
	case *ast.BasicLit:
		return true
	case *ast.ParenExpr:
		return isSafeCondition(e.X)
	case *ast.UnaryExpr:
		return e.Op == token.NOT && isSafeCondition(e.X)
	case *ast.BinaryExpr:
		switch e.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
			token.LAND, token.LOR:
			return isSafeCondition(e.X) && isSafeCondition(e.Y)
		}
		return false
	case *ast.SelectorExpr:
		return true
	case *ast.CallExpr:
		return true
	default:
		return false
	}
}
