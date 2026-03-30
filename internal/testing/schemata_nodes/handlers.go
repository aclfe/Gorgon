package schemata_nodes

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/aclfe/gorgon/pkg/mutator"
)

type SchemataHandler func(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node

type MutantForSite struct {
	ID         int
	Op         mutator.Operator
	ReturnType string
}

var Handlers = make(map[string]SchemataHandler)

func init() {
	Handlers["*ast.BinaryExpr"] = HandleBinaryExpr
	Handlers["*ast.UnaryExpr"] = HandleUnaryExpr
	Handlers["*ast.CallExpr"] = HandleCallExpr
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
}

func GetHandler(node ast.Node) SchemataHandler {
	typeName := fmt.Sprintf("%T", node)
	return Handlers[typeName]
}

func HandleBinaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return wrapExpression(original, mutants, returnType, file, inferResultType(original))
}

func HandleUnaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return wrapExpression(original, mutants, returnType, file, inferResultType(original))
}

func HandleCallExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return wrapExpression(original, mutants, returnType, file, inferResultType(original))
}

func wrapExpression(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File, resultType string) ast.Node {
	if len(mutants) == 0 {
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

	var originalExpr ast.Expr
	switch n := original.(type) {
	case ast.Expr:
		originalExpr = n
	default:
		originalExpr = &ast.Ident{Name: "nil"}
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
	if len(mutants) == 0 {
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
	return WrapStatement(original, mutants, returnType, file)
}

func HandleForStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleRangeStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleAssignStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleIncDecStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleDeferStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleGoStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleSendStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleSwitchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleTypeSwitchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleReturnStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleBranchStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleSelectStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleCommClause(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleLabeledStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleExprStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleDeclStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleEmptyStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func HandleBlockStmt(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	return WrapStatement(original, mutants, returnType, file)
}

func WrapStatement(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
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

	originalStmt, ok := original.(ast.Stmt)
	if !ok {
		return original
	}
	stmts = append(stmts, originalStmt)

	if len(stmts) == 1 {
		return original
	}

	return &ast.BlockStmt{List: stmts}
}
