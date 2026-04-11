package boundary_value

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestBoundaryValue_Name(t *testing.T) {
	bv := BoundaryValue{}
	if bv.Name() != "boundary_value" {
		t.Errorf("expected name 'boundary_value', got '%s'", bv.Name())
	}
}

func TestBoundaryValue_CanApply(t *testing.T) {
	bv := BoundaryValue{}

	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"less than", "a < b", true},
		{"greater than", "a > b", true},
		{"less or equal", "a <= b", true},
		{"greater or equal", "a >= b", true},
		{"equal", "a == b", false},
		{"not equal", "a != b", false},
		{"addition", "a + b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseExpr(t, tt.code)
			result := bv.CanApply(node)
			if result != tt.expected {
				t.Errorf("CanApply(%s) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestBoundaryValue_Mutate(t *testing.T) {
	bv := BoundaryValue{}

	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{"less to less or equal", "a < b", "a <= b"},
		{"greater to greater or equal", "a > b", "a >= b"},
		{"less or equal to less", "a <= b", "a < b"},
		{"greater or equal to greater", "a >= b", "a > b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseExpr(t, tt.code)
			mutated := bv.Mutate(node)
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

func TestBoundaryValue_Registration(t *testing.T) {
	op, ok := mutator.Get("boundary_value")
	if !ok {
		t.Fatal("boundary_value not registered")
	}
	if op.Name() != "boundary_value" {
		t.Errorf("expected name 'boundary_value', got '%s'", op.Name())
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