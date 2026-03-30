// Package mutator provides mutation operators for the gorgon project.
package mutator

import (
	"go/ast"
	"go/token"
)

type ZeroValueReturn struct{}

func (ZeroValueReturn) Name() string {
	return "zero_value_return"
}

func (ZeroValueReturn) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	return isLiteral(ret.Results[0])
}

func isLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind != token.STRING && e.Kind != token.CHAR
	case *ast.CompositeLit:
		return true
	case *ast.ParenExpr:
		if pe, ok := expr.(*ast.ParenExpr); ok {
			return isLiteral(pe.X)
		}
		return false
	default:
		return false
	}
}

func (ZeroValueReturn) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	firstResult := ret.Results[0]
	if !isLiteral(firstResult) {
		return nil
	}

	zeroVal := zeroValueForType(firstResult)

	return &ast.ReturnStmt{
		Results: []ast.Expr{zeroVal},
	}
}

func zeroValueForType(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.CompositeLit:
		return &ast.Ident{Name: "nil"}
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case token.FLOAT:
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
		case token.IMAG:
			return &ast.BasicLit{Kind: token.IMAG, Value: "0i"}
		case token.STRING:
			return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}
		case token.CHAR:
			return &ast.BasicLit{Kind: token.CHAR, Value: "''"}
		}
		return &ast.Ident{Name: "nil"}
	default:
		return &ast.Ident{Name: "nil"}
	}
}

func init() {
	Register(ZeroValueReturn{})
}


