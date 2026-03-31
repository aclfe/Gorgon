package logical_operator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestLogicalOperator_Name(t *testing.T) {
	lo := LogicalOperator{}
	if lo.Name() != "logical_operator" {
		t.Errorf("expected name 'logical_operator', got '%s'", lo.Name())
	}
}

func TestLogicalOperator_CanApply(t *testing.T) {
	lo := LogicalOperator{}

	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"logical AND", "a && b", true},
		{"logical OR", "a || b", true},
		{"less than", "a < b", false},
		{"equal", "a == b", false},
		{"addition", "a + b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseExpr(t, tt.code)
			result := lo.CanApply(node)
			if result != tt.expected {
				t.Errorf("CanApply(%s) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_Mutate(t *testing.T) {
	lo := LogicalOperator{}

	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{"AND to OR", "a && b", "a || b"},
		{"OR to AND", "a || b", "a && b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseExpr(t, tt.code)
			mutated := lo.Mutate(node)
			if mutated == nil {
				t.Fatal("Mutate returned nil")
			}
			be, ok := mutated.(*ast.BinaryExpr)
			if !ok {
				t.Fatal("Mutate did not return *ast.BinaryExpr")
			}
			result := binaryExprToString(be)
			if result != tt.expected {
				t.Errorf("Mutate(%s) = %s, want %s", tt.code, result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_Registration(t *testing.T) {
	op, ok := mutator.Get("logical_operator")
	if !ok {
		t.Fatal("logical_operator not registered")
	}
	if op.Name() != "logical_operator" {
		t.Errorf("expected name 'logical_operator', got '%s'", op.Name())
	}
}

func parseExpr(t *testing.T, expr string) ast.Node {
	t.Helper()
	fset := token.NewFileSet()
	src := "package test; func f() { _ = " + expr + " }"
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", expr, err)
	}
	var result ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if be, ok := n.(*ast.BinaryExpr); ok {
			result = be
			return false
		}
		return true
	})
	if result == nil {
		t.Fatalf("no binary expr found in %s", expr)
	}
	return result
}

func binaryExprToString(be *ast.BinaryExpr) string {
	x := identToString(be.X)
	op := be.Op.String()
	y := identToString(be.Y)
	return x + " " + op + " " + y
}

func identToString(n ast.Node) string {
	if ident, ok := n.(*ast.Ident); ok {
		return ident.Name
	}
	return "?"
}