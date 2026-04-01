package zero_value_return

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ZeroValueReturnNumeric struct{}

func (ZeroValueReturnNumeric) Name() string {
	return "zero_value_return_numeric"
}

func (ZeroValueReturnNumeric) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	return isNumericLiteral(ret.Results[0])
}

func isNumericLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT || e.Kind == token.FLOAT || e.Kind == token.IMAG
	case *ast.ParenExpr:
		return isNumericLiteral(e.X)
	default:
		return false
	}
}

func isInsideCaseClause(ret *ast.ReturnStmt, file *ast.File) bool {
	var result bool
	ast.Inspect(file, func(n ast.Node) bool {
		if n == ret {
			return true
		}

		if cc, ok := n.(*ast.CaseClause); ok {
			for _, stmt := range cc.Body {
				if findNode(stmt, ret) {
					result = true
					return false
				}
			}
		}

		return true
	})

	return result
}

func findNode(node, target ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if n == target {
			found = true
			return false
		}
		return true
	})
	return found
}

func (ZeroValueReturnNumeric) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	firstResult := ret.Results[0]
	if !isNumericLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{numericZeroValue(firstResult)},
	}
}

func (ZeroValueReturnNumeric) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return false
	}
	return isNumericLiteral(ret.Results[0])
}

func (ZeroValueReturnNumeric) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return nil
	}

	firstResult := ret.Results[0]
	if !isNumericLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{numericZeroValue(firstResult)},
	}
}

func numericZeroValue(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case token.FLOAT:
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
		case token.IMAG:
			return &ast.BasicLit{Kind: token.IMAG, Value: "0i"}
		}
		return &ast.Ident{Name: "0"}
	default:
		return &ast.Ident{Name: "0"}
	}
}

func init() {
	mutator.Register(ZeroValueReturnNumeric{})
}

var _ mutator.Operator = ZeroValueReturnNumeric{}
var _ mutator.ContextualOperator = ZeroValueReturnNumeric{}

type ZeroValueReturnString struct{}

func (ZeroValueReturnString) Name() string {
	return "zero_value_return_string"
}

func (ZeroValueReturnString) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	return isStringLiteral(ret.Results[0])
}

func isStringLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.ParenExpr:
		return isStringLiteral(e.X)
	default:
		return false
	}
}

func (ZeroValueReturnString) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	firstResult := ret.Results[0]
	if !isStringLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"\""}},
	}
}

func (ZeroValueReturnString) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return false
	}
	return isStringLiteral(ret.Results[0])
}

func (ZeroValueReturnString) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return nil
	}

	firstResult := ret.Results[0]
	if !isStringLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: "\"\""}},
	}
}

func init() {
	mutator.Register(ZeroValueReturnString{})
}

var _ mutator.Operator = ZeroValueReturnString{}
var _ mutator.ContextualOperator = ZeroValueReturnString{}

type ZeroValueReturnBool struct{}

func (ZeroValueReturnBool) Name() string {
	return "zero_value_return_bool"
}

func (ZeroValueReturnBool) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	return isBoolLiteral(ret.Results[0])
}

func isBoolLiteral(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "true" || ident.Name == "false"
}

func (ZeroValueReturnBool) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	firstResult := ret.Results[0]
	if !isBoolLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "false"}},
	}
}

func (ZeroValueReturnBool) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return false
	}
	return isBoolLiteral(ret.Results[0])
}

func (ZeroValueReturnBool) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return nil
	}

	firstResult := ret.Results[0]
	if !isBoolLiteral(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "false"}},
	}
}

func init() {
	mutator.Register(ZeroValueReturnBool{})
}

var _ mutator.Operator = ZeroValueReturnBool{}
var _ mutator.ContextualOperator = ZeroValueReturnBool{}

type ZeroValueReturnError struct{}

func (ZeroValueReturnError) Name() string {
	return "zero_value_return_error"
}

func (ZeroValueReturnError) CanApply(n ast.Node) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	return isErrorCall(ret.Results[0])
}

func isErrorExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				return ident.Name == "fmt" && sel.Sel.Name == "Errorf"
			}
		}
		return false
	case *ast.Ident:
		return e.Name == "nil"
	default:
		return false
	}
}

func isErrorCall(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				return ident.Name == "fmt" && sel.Sel.Name == "Errorf"
			}
		}
		return false
	default:
		return false
	}
}

func (ZeroValueReturnError) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	firstResult := ret.Results[0]
	if !isErrorCall(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func (ZeroValueReturnError) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return false
	}
	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return false
	}
	return isErrorCall(ret.Results[0])
}

func (ZeroValueReturnError) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) == 0 {
		return nil
	}

	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return nil
	}

	firstResult := ret.Results[0]
	if !isErrorCall(firstResult) {
		return nil
	}

	return &ast.ReturnStmt{
		Results: []ast.Expr{&ast.Ident{Name: "nil"}},
	}
}

func init() {
	mutator.Register(ZeroValueReturnError{})
}

var _ mutator.Operator = ZeroValueReturnError{}
var _ mutator.ContextualOperator = ZeroValueReturnError{}

