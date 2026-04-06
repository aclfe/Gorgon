package error_handling

import (
	"go/ast"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/common"
)

type ErrorReturnNil struct{}

func (ErrorReturnNil) Name() string {
	return "error_return_nil"
}

func (ErrorReturnNil) CanApply(n ast.Node) bool {
	ret, ok := common.IsReturnStmtWithResults(n, 2)
	if !ok {
		return false
	}
	lastResult := ret.Results[len(ret.Results)-1]
	return !common.IsErrorNil(lastResult) && common.IsErrorExpr(lastResult)
}

func (ErrorReturnNil) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := common.IsReturnStmtWithResults(n, 2)
	if !ok {
		return false
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if common.IsErrorNil(lastResult) || !common.IsErrorExpr(lastResult) {
		return false
	}
	if ctx.File != nil && common.IsInsideCaseClause(ret, ctx.File) {
		return false
	}
	return true
}

func (ErrorReturnNil) Mutate(n ast.Node) ast.Node {
	ret, ok := common.IsReturnStmtWithResults(n, 2)
	if !ok {
		return nil
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if common.IsErrorNil(lastResult) || !common.IsErrorExpr(lastResult) {
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
	ret, ok := common.IsReturnStmtWithResults(n, 2)
	if !ok {
		return nil
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if common.IsErrorNil(lastResult) || !common.IsErrorExpr(lastResult) {
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

func init() {
	mutator.Register(ErrorReturnNil{})
}

var _ mutator.Operator = ErrorReturnNil{}
var _ mutator.ContextualOperator = ErrorReturnNil{}
