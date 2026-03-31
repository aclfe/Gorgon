package assignment_operator

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/aclfe/gorgon/pkg/mutator"
)

func TestAssignmentOperator_Name(t *testing.T) {
	ao := AssignmentOperator{}
	if ao.Name() != "assignment_operator" {
		t.Errorf("expected name 'assignment_operator', got '%s'", ao.Name())
	}
}

func TestAssignmentOperator_CanApply(t *testing.T) {
	ao := AssignmentOperator{}

	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"simple assign", "x = y", true},
		{"add assign", "x += y", true},
		{"sub assign", "x -= y", true},
		{"mul assign", "x *= y", true},
		{"quo assign", "x /= y", true},
		{"define", "x := y", false},
		{"multi assign", "x, y = a, b", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseStmt(t, tt.code)
			result := ao.CanApply(node)
			if result != tt.expected {
				t.Errorf("CanApply(%s) = %v, want %v", tt.code, result, tt.expected)
			}
		})
	}
}

func TestAssignmentOperator_Mutate(t *testing.T) {
	ao := AssignmentOperator{}

	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{"assign to add assign", "x = y", "x += y"},
		{"add assign to sub assign", "x += y", "x -= y"},
		{"sub assign to add assign", "x -= y", "x += y"},
		{"mul assign to quo assign", "x *= y", "x /= y"},
		{"quo assign to mul assign", "x /= y", "x *= y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := parseStmt(t, tt.code)
			mutated := ao.Mutate(node)
			if mutated == nil {
				t.Fatal("Mutate returned nil")
			}
			as, ok := mutated.(*ast.AssignStmt)
			if !ok {
				t.Fatal("Mutate did not return *ast.AssignStmt")
			}
			result := assignStmtToString(as)
			if result != tt.expected {
				t.Errorf("Mutate(%s) = %s, want %s", tt.code, result, tt.expected)
			}
		})
	}
}

func TestAssignmentOperator_Registration(t *testing.T) {
	op, ok := mutator.Get("assignment_operator")
	if !ok {
		t.Fatal("assignment_operator not registered")
	}
	if op.Name() != "assignment_operator" {
		t.Errorf("expected name 'assignment_operator', got '%s'", op.Name())
	}
}

func parseStmt(t *testing.T, stmt string) ast.Node {
	t.Helper()
	fset := token.NewFileSet()
	src := "package test; func f() { " + stmt + " }"
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", stmt, err)
	}
	var result ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if as, ok := n.(*ast.AssignStmt); ok {
			result = as
			return false
		}
		return true
	})
	if result == nil {
		t.Fatalf("no assign stmt found in %s", stmt)
	}
	return result
}

func assignStmtToString(as *ast.AssignStmt) string {
	lhs := identToString(as.Lhs[0])
	tok := as.Tok.String()
	rhs := identToString(as.Rhs[0])
	return lhs + " " + tok + " " + rhs
}

func identToString(n ast.Node) string {
	if ident, ok := n.(*ast.Ident); ok {
		return ident.Name
	}
	return "?"
}