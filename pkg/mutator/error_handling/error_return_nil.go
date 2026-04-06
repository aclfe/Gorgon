package error_handling

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type ErrorReturnNil struct{}

func (ErrorReturnNil) Name() string {
	return "error_return_nil"
}

func (ErrorReturnNil) CanApply(n ast.Node) bool {
	return false
}

func (ErrorReturnNil) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok {
		return false
	}
	if len(ret.Results) < 2 {
		return false
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if isErrorNil(lastResult) {
		return false
	}
	if !isErrorExpr(lastResult) {
		return false
	}
	if ctx.File != nil && isInsideCaseClause(ret, ctx.File) {
		return false
	}
	return true
}

func (ErrorReturnNil) Mutate(n ast.Node) ast.Node {
	ret, ok := n.(*ast.ReturnStmt)
	if !ok || len(ret.Results) < 2 {
		return nil
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if isErrorNil(lastResult) || !isErrorExpr(lastResult) {
		return nil
	}

	newResults := make([]ast.Expr, len(ret.Results))
	copy(newResults, ret.Results)
	newResults[len(newResults)-1] = &ast.Ident{Name: "nil"}

	return &ast.ReturnStmt{
		Return:  ret.Return,
		Results: newResults,
	}
}

func (ErrorReturnNil) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	if !(&ErrorReturnNil{}).CanApplyWithContext(n, ctx) {
		return nil
	}
	ret := n.(*ast.ReturnStmt)

	newResults := make([]ast.Expr, len(ret.Results))
	copy(newResults, ret.Results)
	newResults[len(newResults)-1] = &ast.Ident{Name: "nil"}

	return &ast.ReturnStmt{
		Return:  ret.Return,
		Results: newResults,
	}
}

func isErrorNil(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "nil"
}

func isErrorExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "fmt" && (sel.Sel.Name == "Errorf" || sel.Sel.Name == "Errorw") {
					return true
				}
				if ident.Name == "errors" && (sel.Sel.Name == "New" || sel.Sel.Name == "As" || sel.Sel.Name == "Is" || sel.Sel.Name == "Unwrap") {
					return true
				}
			}
		}
		if ident, ok := e.Fun.(*ast.Ident); ok {
			if ident.Name == "New" || ident.Name == "Errorf" || ident.Name == "Wrap" || ident.Name == "Wrapf" {
				return true
			}
		}
		return false
	case *ast.CompositeLit:
		if sel, ok := e.Type.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				if ident.Name == "errors" || strings.HasSuffix(sel.Sel.Name, "Error") {
					return true
				}
			}
		}
		return false
	case *ast.UnaryExpr:
		return e.Op == token.AND
	case *ast.Ident:
		if e.Name == "nil" || e.Name == "true" || e.Name == "false" {
			return false
		}
		if isNumericIdent(e.Name) {
			return false
		}
		return true
	case *ast.SelectorExpr:
		return true
	case *ast.IndexExpr:
		return isErrorExpr(e.X)
	case *ast.IndexListExpr:
		return isErrorExpr(e.X)
	default:
		return false
	}
}

func isNumericIdent(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= '0' && name[0] <= '9'
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

func init() {
	mutator.Register(ErrorReturnNil{})
}

var _ mutator.Operator = ErrorReturnNil{}
var _ mutator.ContextualOperator = ErrorReturnNil{}
