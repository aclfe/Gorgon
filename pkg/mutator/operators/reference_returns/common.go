package reference_returns

import "go/ast"

// returnNilMutate replaces the first return expression with nil.
// Used by all reference-return mutation operators.
func returnNilMutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}
	results := make([]ast.Expr, len(ret.Results))
	copy(results, ret.Results)
	results[0] = &ast.Ident{Name: "nil"}
	return &ast.ReturnStmt{Return: ret.Return, Results: results}
}
