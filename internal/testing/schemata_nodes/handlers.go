package schemata_nodes

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SchemataHandler func(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node

type MutantForSite struct {
	ID         int
	Op         mutator.Operator
	ReturnType string
	NodeType   string
}

var Handlers = make(map[string]SchemataHandler)

func init() {
	Handlers["*ast.BinaryExpr"] = HandleBinaryExpr
	Handlers["*ast.UnaryExpr"] = HandleUnaryExpr
	Handlers["*ast.CallExpr"] = HandleCallExpr
	Handlers["*ast.Ident"] = HandleIdent
	Handlers["*ast.CaseClause"] = HandleCaseClause
	Handlers["*ast.IfStmt"] = HandleIfStmt
	Handlers["*ast.ForStmt"] = HandleForStmt
	Handlers["*ast.RangeStmt"] = HandleRangeStmt
	Handlers["*ast.AssignStmt"] = HandleAssignStmt
	Handlers["*ast.IncDecStmt"] = HandleIncDecStmt
	Handlers["*ast.DeferStmt"] = HandleDeferStmt
	Handlers["*ast.GoStmt"] = HandleGoStmt
	Handlers["*ast.SendStmt"] = HandleSendStmt
	Handlers["*ast.SwitchStmt"] = HandleSwitchStmt
	Handlers["*ast.TypeSwitchStmt"] = HandleTypeSwitchStmt
	Handlers["*ast.ReturnStmt"] = HandleReturnStmt
	Handlers["*ast.BranchStmt"] = HandleBranchStmt
	Handlers["*ast.SelectStmt"] = HandleSelectStmt
	Handlers["*ast.CommClause"] = HandleCommClause
	Handlers["*ast.LabeledStmt"] = HandleLabeledStmt
	Handlers["*ast.ExprStmt"] = HandleExprStmt
	Handlers["*ast.DeclStmt"] = HandleDeclStmt
	Handlers["*ast.EmptyStmt"] = HandleEmptyStmt
	Handlers["*ast.BlockStmt"] = HandleBlockStmt
	Handlers["*ast.FuncDecl"] = HandleFuncDecl
}

func GetHandler(node ast.Node) SchemataHandler {
	typeName := fmt.Sprintf("%T", node)
	return Handlers[typeName]
}

func HandleUnaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}
	if len(mutants) > 1 {
		return original
	}
	mutant := mutants[0]
	var mutated ast.Node
	if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
		mutated = cop.MutateWithContext(original, mutator.Context{File: file})
	} else {
		mutated = mutant.Op.Mutate(original)
	}
	if mutated != nil {
		return mutated
	}
	return original
}

func HandleCallExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}
	if len(mutants) > 1 {
		return original
	}
	mutant := mutants[0]
	var mutated ast.Node
	if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
		mutated = cop.MutateWithContext(original, mutator.Context{File: file})
	} else {
		mutated = mutant.Op.Mutate(original)
	}
	if mutated != nil {
		return mutated
	}
	return original
}

func HandleIdent(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}
	if len(mutants) > 1 {
		return original
	}
	mutant := mutants[0]
	var mutated ast.Node
	if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
		mutated = cop.MutateWithContext(original, mutator.Context{File: file})
	} else {
		mutated = mutant.Op.Mutate(original)
	}
	if mutated != nil {
		return mutated
	}
	return original
}

func HandleBinaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}
	if len(mutants) > 1 {
		return original
	}
	mutant := mutants[0]
	var mutated ast.Node
	if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
		mutated = cop.MutateWithContext(original, mutator.Context{File: file})
	} else {
		mutated = mutant.Op.Mutate(original)
	}
	if mutated != nil {
		return mutated
	}
	return original
}

func handleSingleMutant(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) != 1 {
		return original
	}
	mutant := mutants[0]
	ctx := mutator.Context{ReturnType: returnType, File: file}
	var mutated ast.Node
	if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
		mutated = cop.MutateWithContext(original, ctx)
	} else {
		mutated = mutant.Op.Mutate(original)
	}
	if mutated != nil {
		return mutated
	}
	return original
}

func wrapExpression(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File, resultType string) ast.Node {
	if len(mutants) != 1 {
		return original
	}

	originalExpr, ok := original.(ast.Expr)
	if !ok {
		return original
	}

	stmts := make([]ast.Stmt, 0, len(mutants)+1)

	for _, mutant := range mutants {
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated == nil {
			continue
		}

		mutatedExpr, ok := mutated.(ast.Expr)
		if !ok {
			continue
		}

		retStmt := &ast.ReturnStmt{Results: []ast.Expr{mutatedExpr}}

		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutant.ID)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{retStmt},
			},
		})
	}

	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{originalExpr}})

	return &ast.CallExpr{
		Fun: &ast.FuncLit{
			Type: &ast.FuncType{
				Results: &ast.FieldList{
					List: []*ast.Field{
						{Type: &ast.Ident{Name: resultType}},
					},
				},
			},
			Body: &ast.BlockStmt{List: stmts},
		},
	}
}

func inferResultType(node ast.Node) string {
	switch node.(type) {
	case *ast.BinaryExpr:
		be := node.(*ast.BinaryExpr)
		if isComparisonOp(be.Op) {
			return "bool"
		}
		return "int"
	case *ast.UnaryExpr:
		return "int"
	case *ast.CallExpr:
		return "interface{}"
	default:
		return "interface{}"
	}
}

func isComparisonOp(op token.Token) bool {
	switch op {
	case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
		return true
	}
	return false
}

func HandleCaseClause(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	cc, ok := original.(*ast.CaseClause)
	if !ok {
		return original
	}
	if len(mutants) != 1 {
		return cc
	}

	if returnType == "" {
		returnType = "string"
	}

	zeroVal := GetZeroValueForType(returnType)

	newBody := make([]ast.Stmt, 0, len(cc.Body)+len(mutants))

	for _, mutant := range mutants {
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(cc, ctx)
		} else {
			mutated = mutant.Op.Mutate(cc)
		}
		if mutated == nil {
			continue
		}

		mutatedCC, ok := mutated.(*ast.CaseClause)
		if !ok {
			continue
		}

		if len(mutatedCC.Body) == 0 {
			newBody = append(newBody, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  &ast.Ident{Name: "activeMutantID"},
					Op: token.EQL,
					Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mutant.ID)},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.ReturnStmt{Results: []ast.Expr{zeroVal}},
					},
				},
			})
		} else {
			newBody = append(newBody, &ast.IfStmt{
				Cond: &ast.BinaryExpr{
					X:  &ast.Ident{Name: "activeMutantID"},
					Op: token.EQL,
					Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mutant.ID)},
				},
				Body: &ast.BlockStmt{
					List: mutatedCC.Body,
				},
			})
		}
	}

	newBody = append(newBody, cc.Body...)

	return &ast.CaseClause{
		Case:  cc.Case,
		List:  cc.List,
		Colon: cc.Colon,
		Body:  newBody,
	}
}

func GetZeroValueForType(returnType string) ast.Expr {
	switch returnType {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return &ast.BasicLit{Kind: token.INT, Value: "0"}
	case "float32", "float64":
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
	case "string":
		return &ast.BasicLit{Kind: token.STRING, Value: "\"\""}
	case "bool":
		return &ast.Ident{Name: "false"}
	default:
		return &ast.Ident{Name: "nil"}
	}
}

func HandleIfStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleForStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleRangeStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleBlockStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleBranchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleAssignStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	assignStmt, ok := original.(*ast.AssignStmt)
	if !ok {
		return original
	}

	for _, lhs := range assignStmt.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
			return original
		}
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleIncDecStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleDeferStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	deferStmt, ok := original.(*ast.DeferStmt)
	if !ok {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			if _, isEmpty := mutated.(*ast.EmptyStmt); isEmpty {
				return wrapDeferRemoval(deferStmt, file)
			}
			return mutated
		}
		return original
	}

	return wrapDeferWithMutants(deferStmt, mutants, returnType, file)
}

func collectIdentsFromExpr(expr ast.Expr) []*ast.Ident {
	var idents []*ast.Ident
	
	// Helper to extract the root identifier from a selector chain
	// e.g., for "a.b.c", returns "a"
	var getRootIdent func(ast.Expr) *ast.Ident
	getRootIdent = func(e ast.Expr) *ast.Ident {
		switch v := e.(type) {
		case *ast.Ident:
			return v
		case *ast.SelectorExpr:
			return getRootIdent(v.X)
		case *ast.CallExpr:
			return getRootIdent(v.Fun)
		}
		return nil
	}
	
	// Collect root identifiers from the expression
	ast.Inspect(expr, func(n ast.Node) bool {
		if ce, ok := n.(*ast.CallExpr); ok {
			if root := getRootIdent(ce.Fun); root != nil && root.Name != "_" {
				idents = append(idents, root)
			}
			return false  // Don't descend into call arguments
		}
		return true
	})
	
	return idents
}

func wrapDeferRemoval(deferStmt *ast.DeferStmt, file *ast.File) ast.Node {
	idents := collectIdentsFromExpr(deferStmt.Call)
	if len(idents) == 0 {
		return &ast.EmptyStmt{}
	}

	if len(idents) == 1 {
		return &ast.ExprStmt{
			X: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "_"},
				Op: token.ASSIGN,
				Y:  idents[0],
			},
		}
	}

	stmts := make([]ast.Stmt, 0, len(idents))
	for _, ident := range idents {
		stmts = append(stmts, &ast.ExprStmt{
			X: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "_"},
				Op: token.ASSIGN,
				Y:  ident,
			},
		})
	}

	return &ast.BlockStmt{List: stmts}
}

func wrapDeferWithMutants(deferStmt *ast.DeferStmt, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	idents := collectIdentsFromExpr(deferStmt.Call)

	stmts := make([]ast.Stmt, 0, len(mutants)+2)

	for _, mutant := range mutants {
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(deferStmt, ctx)
		} else {
			mutated = mutant.Op.Mutate(deferStmt)
		}
		if mutated == nil {
			continue
		}

		mutatedStmt, ok := mutated.(ast.Stmt)
		if !ok {
			continue
		}

		if _, isEmpty := mutatedStmt.(*ast.EmptyStmt); isEmpty && len(idents) > 0 {
			if len(idents) == 1 {
				mutatedStmt = &ast.ExprStmt{
					X: &ast.BinaryExpr{
						X:  &ast.Ident{Name: "_"},
						Op: token.ASSIGN,
						Y:  idents[0],
					},
				}
			} else {
				blankStmts := make([]ast.Stmt, 0, len(idents))
				for _, ident := range idents {
					blankStmts = append(blankStmts, &ast.ExprStmt{
						X: &ast.BinaryExpr{
							X:  &ast.Ident{Name: "_"},
							Op: token.ASSIGN,
							Y:  ident,
						},
					})
				}
				mutatedStmt = &ast.BlockStmt{List: blankStmts}
			}
		}

		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mutant.ID)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{mutatedStmt},
			},
		})
	}

	stmts = append(stmts, deferStmt)

	return &ast.BlockStmt{List: stmts}
}

func HandleGoStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleSendStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleSwitchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleTypeSwitchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleReturnStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	originalRet, ok := original.(*ast.ReturnStmt)
	if !ok {
		return original
	}

	if len(originalRet.Results) == 0 {
		return original
	}

	// Collect all valid mutations
	type mutantResult struct {
		id   int
		expr ast.Expr
	}
	var mutResults []mutantResult

	for _, mutant := range mutants {
		mutReturnType := mutant.ReturnType
		if mutReturnType == "" {
			mutReturnType = returnType
		}
		ctx := mutator.Context{ReturnType: mutReturnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated == nil {
			continue
		}

		mutatedRet, ok := mutated.(*ast.ReturnStmt)
		if !ok || len(mutatedRet.Results) == 0 {
			continue
		}

		mutResults = append(mutResults, mutantResult{id: mutant.ID, expr: mutatedRet.Results[0]})
	}

	if len(mutResults) == 0 {
		return original
	}

	// Single mutation - apply directly
	if len(mutResults) == 1 {
		return &ast.ReturnStmt{
			Results: []ast.Expr{mutResults[0].expr},
		}
	}

	// Multiple mutations - wrap with runtime selection
	origExpr := originalRet.Results[0]

	resultType := returnType
	if resultType == "" {
		resultType = inferExprType(origExpr)
	}

	var typeExpr ast.Expr = &ast.Ident{Name: resultType}
	if strings.HasPrefix(resultType, "*") {
		baseType := strings.TrimPrefix(resultType, "*")
		if baseType != "" {
			typeExpr = &ast.StarExpr{X: &ast.Ident{Name: baseType}}
		}
	}

	stmts := make([]ast.Stmt, 0, len(mutResults)+1)
	for _, mr := range mutResults {
		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mr.id)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mr.expr}}},
			},
		})
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{origExpr}})

	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Results: &ast.FieldList{
							List: []*ast.Field{
								{Type: typeExpr},
							},
						},
					},
					Body: &ast.BlockStmt{List: stmts},
				},
			},
		},
	}
}

func inferExprType(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "rune"
		}
	case *ast.BinaryExpr:
		if isComparisonOp(e.Op) {
			return "bool"
		}
		return "int"
	case *ast.UnaryExpr:
		return "int"
	case *ast.CallExpr:
		return "interface{}"
	case *ast.StarExpr:
		return "*" + inferExprType(e.X)
	}
	return "interface{}"
}

func HandleSelectStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleCommClause(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleLabeledStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleExprStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleDeclStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleEmptyStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleFuncDecl(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return mutated
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func WrapStatement(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	originalStmt, ok := original.(ast.Stmt)
	if !ok {
		return original
	}

	stmts := make([]ast.Stmt, 0, len(mutants)+1)

	for _, mutant := range mutants {
		ctx := mutator.Context{ReturnType: returnType, File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated == nil {
			continue
		}

		mutatedStmt, ok := mutated.(ast.Stmt)
		if !ok {
			continue
		}

		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mutant.ID)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{mutatedStmt},
			},
		})
	}

	stmts = append(stmts, originalStmt)

	return &ast.BlockStmt{List: stmts}
}

func WrapStatementSimple(original ast.Node, mutants []MutantForSite, _ string, file *ast.File) ast.Node {
	if len(mutants) != 1 {
		return original
	}

	originalStmt, ok := original.(ast.Stmt)
	if !ok {
		return original
	}

	stmts := make([]ast.Stmt, 0, len(mutants))

	for _, mutant := range mutants {
		ctx := mutator.Context{File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated == nil {
			continue
		}

		mutatedStmt, ok := mutated.(ast.Stmt)
		if !ok {
			continue
		}

		stmts = append(stmts, &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", mutant.ID)},
			},
			Body: &ast.BlockStmt{
				List: []ast.Stmt{mutatedStmt},
			},
		})
	}

	if len(stmts) == 0 {
		return &ast.BlockStmt{List: []ast.Stmt{originalStmt}}
	}

	stmts = append(stmts, originalStmt)

	return &ast.BlockStmt{List: stmts}
}
