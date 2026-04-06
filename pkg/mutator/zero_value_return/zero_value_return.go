package zero_value_return

import (
	"go/ast"
	"go/token"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/common"
)

// zeroValueReturnBase provides shared functionality for all zero-value return mutations.
type zeroValueReturnBase struct {
	name        string
	checkExpr   func(ast.Expr) bool
	zeroValueFn func(ast.Expr) ast.Expr
}

func (b zeroValueReturnBase) Name() string {
	return b.name
}

func (b zeroValueReturnBase) CanApply(n ast.Node) bool {
	ret, ok := common.IsReturnStmtWithResults(n, 1)
	if !ok {
		return false
	}
	return b.checkExpr(ret.Results[0])
}

func (b zeroValueReturnBase) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := common.IsReturnStmtWithResults(n, 1)
	if !ok {
		return false
	}
	if ctx.File != nil && common.IsInsideCaseClause(ret, ctx.File) {
		return false
	}
	return b.checkExpr(ret.Results[0])
}

func (b zeroValueReturnBase) Mutate(n ast.Node) ast.Node {
	ret, ok := common.IsReturnStmtWithResults(n, 1)
	if !ok {
		return nil
	}
	if !b.checkExpr(ret.Results[0]) {
		return nil
	}
	return &ast.ReturnStmt{
		Results: []ast.Expr{b.zeroValueFn(ret.Results[0])},
	}
}

func (b zeroValueReturnBase) MutateWithContext(n ast.Node, ctx mutator.Context) ast.Node {
	ret, ok := common.IsReturnStmtWithResults(n, 1)
	if !ok {
		return nil
	}
	if ctx.File != nil && common.IsInsideCaseClause(ret, ctx.File) {
		return nil
	}
	if !b.checkExpr(ret.Results[0]) {
		return nil
	}
	return &ast.ReturnStmt{
		Results: []ast.Expr{b.zeroValueFn(ret.Results[0])},
	}
}

// ZeroValueReturnNumeric mutates numeric literals to their zero value.
type ZeroValueReturnNumeric struct {
	zeroValueReturnBase
}

func init() {
	mutator.Register(ZeroValueReturnNumeric{
		zeroValueReturnBase: zeroValueReturnBase{
			name:        "zero_value_return_numeric",
			checkExpr:   common.IsNumericLiteral,
			zeroValueFn: numericZeroValue,
		},
	})
}

var _ mutator.Operator = ZeroValueReturnNumeric{}
var _ mutator.ContextualOperator = ZeroValueReturnNumeric{}

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

// ZeroValueReturnString mutates string literals to empty string.
type ZeroValueReturnString struct {
	zeroValueReturnBase
}

func init() {
	mutator.Register(ZeroValueReturnString{
		zeroValueReturnBase: zeroValueReturnBase{
			name:      "zero_value_return_string",
			checkExpr: common.IsStringLiteral,
			zeroValueFn: func(ast.Expr) ast.Expr {
				return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}
			},
		},
	})
}

var _ mutator.Operator = ZeroValueReturnString{}
var _ mutator.ContextualOperator = ZeroValueReturnString{}

// ZeroValueReturnBool mutates boolean literals to false.
type ZeroValueReturnBool struct {
	zeroValueReturnBase
}

func init() {
	mutator.Register(ZeroValueReturnBool{
		zeroValueReturnBase: zeroValueReturnBase{
			name:      "zero_value_return_bool",
			checkExpr: common.IsBoolLiteral,
			zeroValueFn: func(ast.Expr) ast.Expr {
				return &ast.Ident{Name: "false"}
			},
		},
	})
}

var _ mutator.Operator = ZeroValueReturnBool{}
var _ mutator.ContextualOperator = ZeroValueReturnBool{}

// ZeroValueReturnError mutates error-producing expressions to nil.
type ZeroValueReturnError struct {
	zeroValueReturnBase
}

func init() {
	mutator.Register(ZeroValueReturnError{
		zeroValueReturnBase: zeroValueReturnBase{
			name:      "zero_value_return_error",
			checkExpr: common.IsErrorCall,
			zeroValueFn: func(ast.Expr) ast.Expr {
				return &ast.Ident{Name: "nil"}
			},
		},
	})
}

var _ mutator.Operator = ZeroValueReturnError{}
var _ mutator.ContextualOperator = ZeroValueReturnError{}
