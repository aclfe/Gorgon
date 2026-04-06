package error_handling

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ErrorCheckRemoval struct{}

func (ErrorCheckRemoval) Name() string {
	return "error_check_removal"
}

func (ErrorCheckRemoval) CanApply(n ast.Node) bool {
	return false
}

func (ErrorCheckRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ifStmt, ok := n.(*ast.IfStmt)
	if !ok {
		return false
	}
	if !isErrNotNilCheck(ifStmt) {
		return false
	}
	return true
}

func (ErrorCheckRemoval) Mutate(n ast.Node) ast.Node {
	ifStmt, ok := n.(*ast.IfStmt)
	if !ok {
		return nil
	}
	if !isErrNotNilCheck(ifStmt) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func (ErrorCheckRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&ErrorCheckRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	return &ast.EmptyStmt{}
}

func isErrNotNilCheck(ifStmt *ast.IfStmt) bool {
	if ifStmt.Else != nil {
		return false
	}
	if ifStmt.Init != nil {
		return false
	}
	binExpr, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || binExpr.Op != token.NEQ {
		return false
	}
	ident, ok := binExpr.X.(*ast.Ident)
	if !ok || ident.Name != "err" {
		return false
	}
	nilIdent, ok := binExpr.Y.(*ast.Ident)
	if !ok || nilIdent.Name != "nil" {
		return false
	}
	if len(ifStmt.Body.List) != 1 {
		return false
	}
	_, isReturn := ifStmt.Body.List[0].(*ast.ReturnStmt)
	return isReturn
}

func init() {
	mutator.Register(ErrorCheckRemoval{})
}

var _ mutator.Operator = ErrorCheckRemoval{}
var _ mutator.ContextualOperator = ErrorCheckRemoval{}
