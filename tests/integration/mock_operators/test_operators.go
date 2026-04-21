package mock_operators

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type TypeErrorToStringOperator struct{}

func (TypeErrorToStringOperator) Name() string { return "test_type_error_to_string" }

func (TypeErrorToStringOperator) CanApply(n ast.Node) bool {
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return false
	}
	switch bl.Kind {
	case token.INT:
		return bl.Value != "0"
	case token.FLOAT:
		return bl.Value != "0.0"
	}
	return false
}

func (TypeErrorToStringOperator) Mutate(n ast.Node) ast.Node {
	return &ast.BasicLit{
		Kind:  token.STRING,
		Value: `"type_error_injection"`,
	}
}

var _ mutator.Operator = TypeErrorToStringOperator{}

type ValidIntFlipOperator struct{}

func (ValidIntFlipOperator) Name() string { return "test_valid_int_flip" }

func (ValidIntFlipOperator) CanApply(n ast.Node) bool {
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return false
	}
	return bl.Kind == token.INT
}

func (ValidIntFlipOperator) Mutate(n ast.Node) ast.Node {
	bl, ok := n.(*ast.BasicLit)
	if !ok {
		return nil
	}
	if bl.Value == "0" {
		return &ast.BasicLit{Kind: token.INT, Value: "1"}
	}
	return &ast.BasicLit{Kind: token.INT, Value: "0"}
}

var _ mutator.Operator = ValidIntFlipOperator{}

type BoolToIntOperator struct{}

func (BoolToIntOperator) Name() string { return "test_bool_to_int" }

func (BoolToIntOperator) CanApply(n ast.Node) bool {
	_, ok := n.(*ast.Ident)
	return ok
}

func (BoolToIntOperator) Mutate(n ast.Node) ast.Node {
	return &ast.BasicLit{
		Kind:  token.INT,
		Value: "999",
	}
}

var _ mutator.Operator = BoolToIntOperator{}

type MalformedBinaryExprOperator struct{}

func (MalformedBinaryExprOperator) Name() string { return "test_malformed_binary" }

func (MalformedBinaryExprOperator) CanApply(n ast.Node) bool {
	_, ok := n.(*ast.BinaryExpr)
	return ok
}

func (MalformedBinaryExprOperator) Mutate(n ast.Node) ast.Node {
	be, ok := n.(*ast.BinaryExpr)
	if !ok {
		return nil
	}
	return &ast.BinaryExpr{
		X:  be.X,
		Op: be.Op,
	}
}

var _ mutator.Operator = MalformedBinaryExprOperator{}
