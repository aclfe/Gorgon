// Package assignment_operator provides assignment operator mutation operators.
// Mutates assignment operators: = → +=, -=, *=, /= and vice versa
package assignment_operator

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type AssignmentOperator struct{}

func (AssignmentOperator) Name() string {
	return "assignment_operator"
}

func (AssignmentOperator) CanApply(n ast.Node) bool {
	as, ok := n.(*ast.AssignStmt)
	if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return false
	}
	switch as.Tok {
	case token.ASSIGN, token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN:
		if as.Tok == token.ASSIGN {
			switch expr := as.Rhs[0].(type) {
			case *ast.BasicLit:
				switch expr.Kind {
				case token.INT, token.FLOAT:
					return true
				}
				return false
			case *ast.Ident:
				if expr.Name == "true" || expr.Name == "false" || expr.Name == "nil" {
					return false
				}
				return true
			case *ast.BinaryExpr, *ast.CallExpr, *ast.UnaryExpr:
				return true
			default:
				return false
			}
		}
		return true
	}
	return false
}

func (AssignmentOperator) Mutate(n ast.Node) ast.Node {
	as, ok := n.(*ast.AssignStmt)
	if !ok {
		return nil
	}
	var newTok token.Token
	switch as.Tok {
	case token.ASSIGN:
		newTok = token.ADD_ASSIGN
	case token.ADD_ASSIGN:
		newTok = token.SUB_ASSIGN
	case token.SUB_ASSIGN:
		newTok = token.ADD_ASSIGN
	case token.MUL_ASSIGN:
		newTok = token.QUO_ASSIGN
	case token.QUO_ASSIGN:
		newTok = token.MUL_ASSIGN
	default:
		return nil
	}
	return &ast.AssignStmt{
		Lhs: as.Lhs,
		Tok: newTok,
		Rhs: as.Rhs,
	}
}

func (a AssignmentOperator) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	if !a.CanApply(n) {
		return false
	}
	if ctx.Info == nil {
		return true
	}
	as := n.(*ast.AssignStmt)
	t := ctx.Info.TypeOf(as.Lhs[0])
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}
	numeric := b.Info()&types.IsNumeric != 0
	switch as.Tok {
	case token.ASSIGN:
		return numeric || b.Info()&types.IsString != 0
	default:
		return numeric
	}
}

func (a AssignmentOperator) MutateWithContext(n ast.Node, _ mutator.Context) ast.Node {
	return a.Mutate(n)
}

func init() {
	mutator.Register(AssignmentOperator{})
}

var _ mutator.ContextualOperator = AssignmentOperator{}