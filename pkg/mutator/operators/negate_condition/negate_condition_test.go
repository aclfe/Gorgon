package negate_condition

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestNegateCondition_Name(t *testing.T) {
	op := NegateCondition{}
	if op.Name() != "negate_condition" {
		t.Errorf("expected name 'negate_condition', got '%s'", op.Name())
	}
}

func TestNegateCondition_CanApply(t *testing.T) {
	op := NegateCondition{}
	if op.CanApply(nil) {
		t.Error("expected CanApply to return false (needs context)")
	}
}

func TestNegateCondition_Registration(t *testing.T) {
	_, ok := mutator.Get("negate_condition")
	if !ok {
		t.Error("expected negate_condition to be registered")
	}
}

var _ mutator.Operator = NegateCondition{}
var _ mutator.ContextualOperator = NegateCondition{}

func TestNegateCondition_PreservesInit(t *testing.T) {
	op := NegateCondition{}

	initStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.Ident{Name: "v"}, &ast.Ident{Name: "ok"}},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.IndexExpr{
			X:     &ast.Ident{Name: "m"},
			Index: &ast.Ident{Name: "k"},
		}},
	}
	ifStmt := &ast.IfStmt{
		Init: initStmt,
		Cond: &ast.Ident{Name: "ok"},
		Body: &ast.BlockStmt{},
	}

	ctx := mutator.Context{Parent: &ast.BlockStmt{}}
	if !op.CanApplyWithContext(ifStmt, ctx) {
		t.Fatal("expected CanApplyWithContext to return true")
	}

	result := op.MutateWithContext(ifStmt, ctx)
	mutated, ok := result.(*ast.IfStmt)
	if !ok {
		t.Fatalf("expected *ast.IfStmt, got %T", result)
	}
	if mutated.Init == nil {
		t.Error("Init was dropped: mutated IfStmt has nil Init, variables declared in Init will be undefined")
	}
}
