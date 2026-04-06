package error_handling

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type NilCheckRemoval struct{}

func (NilCheckRemoval) Name() string {
	return "nil_check_removal"
}

func (NilCheckRemoval) CanApply(n ast.Node) bool {
	return false
}

func (NilCheckRemoval) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ifStmt, ok := n.(*ast.IfStmt)
	if !ok {
		return false
	}
	if !isSimpleNilCheck(ifStmt) {
		return false
	}
	return true
}

func (NilCheckRemoval) Mutate(n ast.Node) ast.Node {
	ifStmt, ok := n.(*ast.IfStmt)
	if !ok {
		return nil
	}
	if !isSimpleNilCheck(ifStmt) {
		return nil
	}
	return &ast.BlockStmt{List: ifStmt.Body.List}
}

func (NilCheckRemoval) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&NilCheckRemoval{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	ifStmt := n.(*ast.IfStmt)
	return &ast.BlockStmt{List: ifStmt.Body.List}
}

func isSimpleNilCheck(ifStmt *ast.IfStmt) bool {
	if ifStmt.Init != nil {
		return false
	}
	if ifStmt.Else != nil {
		return false
	}
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if bin.Op != token.NEQ && bin.Op != token.EQL {
		return false
	}
	ident, ok := bin.X.(*ast.Ident)
	if !ok {
		return false
	}
	nilIdent, ok := bin.Y.(*ast.Ident)
	if !ok || nilIdent.Name != "nil" {
		return false
	}
	if ident.Name == "err" {
		return false
	}
	if len(ifStmt.Body.List) == 0 {
		return false
	}
	return true
}

func init() {
	mutator.Register(NilCheckRemoval{})
}

var _ mutator.Operator = NilCheckRemoval{}
var _ mutator.ContextualOperator = NilCheckRemoval{}
