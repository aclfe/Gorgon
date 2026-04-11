package reference_returns

import "go/ast"

// returnNilMutate replaces the first return expression with nil.
// Used by all reference-return mutation operators.
func returnNilMutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}
	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}
