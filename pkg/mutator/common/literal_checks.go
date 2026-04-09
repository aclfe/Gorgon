package common

import (
	"go/ast"
	"go/token"
	"strings"
)



func IsNumericLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.INT || e.Kind == token.FLOAT || e.Kind == token.IMAG
	case *ast.ParenExpr:
		return IsNumericLiteral(e.X)
	default:
		return false
	}
}



func IsStringLiteral(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Kind == token.STRING
	case *ast.ParenExpr:
		return IsStringLiteral(e.X)
	default:
		return false
	}
}


func IsBoolLiteral(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "true" || ident.Name == "false"
}




func IsErrorExpr(expr ast.Expr) bool {
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
		if IsNumericIdent(e.Name) {
			return false
		}
		return true
	case *ast.SelectorExpr:
		return true
	case *ast.IndexExpr:
		return IsErrorExpr(e.X)
	case *ast.IndexListExpr:
		return IsErrorExpr(e.X)
	default:
		return false
	}
}


func IsErrorCall(expr ast.Expr) bool {
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


func IsErrorNil(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "nil"
}


func IsNumericIdent(name string) bool {
	if len(name) == 0 {
		return false
	}
	return name[0] >= '0' && name[0] <= '9'
}
