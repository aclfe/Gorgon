package error_handling

import (
	"go/ast"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
	"github.com/aclfe/gorgon/pkg/mutator/analysis"
)

// lastReturnTypeIsError reports whether the last component of a comma-separated
// return-type signature is exactly `error`. The engine emits ReturnType as a
// comma-joined list (`"int,error"`, `"token.Position,bool"`, …), so the check
// is a literal compare against the trailing component. An empty signature
// means the engine could not determine the type — we do not assume.
func lastReturnTypeIsError(returnType string) bool {
	if returnType == "" {
		return false
	}
	if idx := strings.LastIndex(returnType, ","); idx >= 0 {
		return returnType[idx+1:] == "error"
	}
	return returnType == "error"
}

type ErrorReturnNil struct{}

func (ErrorReturnNil) Name() string {
	return "error_return_nil"
}

func (ErrorReturnNil) CanApply(n ast.Node) bool {
	ret, ok := analysis.IsReturnStmtWithResults(n, 2)
	if !ok {
		return false
	}
	lastResult := ret.Results[len(ret.Results)-1]
	return !analysis.IsErrorNil(lastResult) && analysis.IsErrorExpr(lastResult)
}

func (ErrorReturnNil) CanApplyWithContext(n ast.Node, ctx mutator.Context) bool {
	ret, ok := analysis.IsReturnStmtWithResults(n, 2)
	if !ok {
		return false
	}
	// Authoritative gate: the enclosing function's last return type must be
	// `error`. Without this, the AST-only IsErrorExpr heuristic matches any
	// non-literal identifier (e.g. a `token.Position` value) and the mutation
	// `return x, nil` produces uncompilable code like
	// "cannot use nil as token.Position value in return statement".
	if !lastReturnTypeIsError(ctx.ReturnType) {
		return false
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if analysis.IsErrorNil(lastResult) || !analysis.IsErrorExpr(lastResult) {
		return false
	}
	if ctx.File != nil && analysis.IsInsideCaseClause(ret, ctx.File) {
		return false
	}
	return true
}

func (ErrorReturnNil) Mutate(n ast.Node) ast.Node {
	ret, ok := analysis.IsReturnStmtWithResults(n, 2)
	if !ok {
		return nil
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if analysis.IsErrorNil(lastResult) || !analysis.IsErrorExpr(lastResult) {
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
	ret, ok := analysis.IsReturnStmtWithResults(n, 2)
	if !ok {
		return nil
	}
	if !lastReturnTypeIsError(ctx.ReturnType) {
		return nil
	}
	lastResult := ret.Results[len(ret.Results)-1]
	if analysis.IsErrorNil(lastResult) || !analysis.IsErrorExpr(lastResult) {
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
