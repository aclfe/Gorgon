package schemata_nodes

import (
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
}

type NodeType uint8

const (
	NTUnknown NodeType = iota
	NTBinaryExpr
	NTUnaryExpr
	NTCallExpr
	NTIdent
	NTCaseClause
	NTIfStmt
	NTForStmt
	NTRangeStmt
	NTAssignStmt
	NTIncDecStmt
	NTDeferStmt
	NTGoStmt
	NTSendStmt
	NTSwitchStmt
	NTTypeSwitchStmt
	NTReturnStmt
	NTBranchStmt
	NTSelectStmt
	NTCommClause
	NTLabeledStmt
	NTExprStmt
	NTDeclStmt
	NTErrStmt
	NTBlockStmt
	NTFuncDecl
	NTBasicLit
	NTMax
)

var Handlers = make([]SchemataHandler, NTMax)

func init() {
	Handlers[NTBinaryExpr] = HandleBinaryExpr
	Handlers[NTUnaryExpr] = HandleUnaryExpr
	Handlers[NTCallExpr] = HandleCallExpr
	Handlers[NTIdent] = HandleIdent
	Handlers[NTCaseClause] = HandleCaseClause
	Handlers[NTIfStmt] = HandleIfStmt
	Handlers[NTForStmt] = HandleForStmt
	Handlers[NTRangeStmt] = HandleRangeStmt
	Handlers[NTAssignStmt] = HandleAssignStmt
	Handlers[NTIncDecStmt] = HandleIncDecStmt
	Handlers[NTDeferStmt] = HandleDeferStmt
	Handlers[NTGoStmt] = HandleGoStmt
	Handlers[NTSendStmt] = HandleSendStmt
	Handlers[NTSwitchStmt] = HandleSwitchStmt
	Handlers[NTTypeSwitchStmt] = HandleTypeSwitchStmt
	Handlers[NTReturnStmt] = HandleReturnStmt
	Handlers[NTBranchStmt] = HandleBranchStmt
	Handlers[NTSelectStmt] = HandleSelectStmt
	Handlers[NTCommClause] = HandleCommClause
	Handlers[NTLabeledStmt] = HandleLabeledStmt
	Handlers[NTExprStmt] = HandleExprStmt
	Handlers[NTDeclStmt] = HandleDeclStmt
	Handlers[NTErrStmt] = HandleEmptyStmt
	Handlers[NTBlockStmt] = HandleBlockStmt
	Handlers[NTFuncDecl] = HandleFuncDecl
	Handlers[NTBasicLit] = HandleBasicLit
}

func GetHandler(node ast.Node) SchemataHandler {
	return Handlers[NodeTypeOf(node)]
}

func NodeTypeOf(node ast.Node) NodeType {
	switch node.(type) {
	case *ast.BinaryExpr:
		return NTBinaryExpr
	case *ast.UnaryExpr:
		return NTUnaryExpr
	case *ast.CallExpr:
		return NTCallExpr
	case *ast.Ident:
		return NTIdent
	case *ast.CaseClause:
		return NTCaseClause
	case *ast.IfStmt:
		return NTIfStmt
	case *ast.ForStmt:
		return NTForStmt
	case *ast.RangeStmt:
		return NTRangeStmt
	case *ast.AssignStmt:
		return NTAssignStmt
	case *ast.IncDecStmt:
		return NTIncDecStmt
	case *ast.DeferStmt:
		return NTDeferStmt
	case *ast.GoStmt:
		return NTGoStmt
	case *ast.SendStmt:
		return NTSendStmt
	case *ast.SwitchStmt:
		return NTSwitchStmt
	case *ast.TypeSwitchStmt:
		return NTTypeSwitchStmt
	case *ast.ReturnStmt:
		return NTReturnStmt
	case *ast.BranchStmt:
		return NTBranchStmt
	case *ast.SelectStmt:
		return NTSelectStmt
	case *ast.CommClause:
		return NTCommClause
	case *ast.LabeledStmt:
		return NTLabeledStmt
	case *ast.ExprStmt:
		return NTExprStmt
	case *ast.DeclStmt:
		return NTDeclStmt
	case *ast.EmptyStmt:
		return NTErrStmt
	case *ast.BlockStmt:
		return NTBlockStmt
	case *ast.FuncDecl:
		return NTFuncDecl
	case *ast.BasicLit:
		return NTBasicLit
	default:
		return NTUnknown
	}
}

func HandleUnaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapWithSchemataMulti(original, mutants, returnType, file)
}

func HandleCallExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapWithSchemataMulti(original, mutants, returnType, file)
}

func HandleIdent(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapWithSchemataMulti(original, mutants, returnType, file)
}

func HandleBinaryExpr(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	if len(mutants) == 1 {
		mutant := mutants[0]
		ctx := mutator.Context{File: file}
		var mutated ast.Node
		if cop, ok := mutant.Op.(mutator.ContextualOperator); ok {
			mutated = cop.MutateWithContext(original, ctx)
		} else {
			mutated = mutant.Op.Mutate(original)
		}
		if mutated != nil {
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapWithSchemataMulti(original, mutants, returnType, file)
}

func wrapWithSchemata(original, mutated ast.Node, mutantID int, returnType string) ast.Node {
	switch orig := original.(type) {
	case ast.Expr:
		mutExpr, ok := mutated.(ast.Expr)
		if !ok {
			return original
		}
		resultType := inferExprType(orig, returnType)
		if resultType == "" || resultType == "interface{}" {
			resultType = returnType
		}
		if resultType == "" {
			resultType = "interface{}"
		}
		var typeExpr ast.Expr = &ast.Ident{Name: resultType}
		if strings.HasPrefix(resultType, "*") {
			baseType := strings.TrimPrefix(resultType, "*")
			if baseType != "" {
				typeExpr = &ast.StarExpr{X: &ast.Ident{Name: baseType}}
			}
		}
		return &ast.CallExpr{
			Fun: &ast.FuncLit{
				Type: &ast.FuncType{
					Results: &ast.FieldList{
						List: []*ast.Field{{Type: typeExpr}},
					},
				},
				Body: &ast.BlockStmt{
					List: []ast.Stmt{
						&ast.IfStmt{
							Cond: &ast.BinaryExpr{
								X:  &ast.Ident{Name: "activeMutantID"},
								Op: token.EQL,
								Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutantID)},
							},
							Body: &ast.BlockStmt{
								List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mutExpr}}},
							},
						},
						&ast.ReturnStmt{Results: []ast.Expr{orig}},
					},
				},
			},
		}
	case ast.Stmt:
		mutStmt, ok := mutated.(ast.Stmt)
		if !ok {
			return original
		}
		return &ast.BlockStmt{
			List: []ast.Stmt{
				&ast.IfStmt{
					Cond: &ast.BinaryExpr{
						X:  &ast.Ident{Name: "activeMutantID"},
						Op: token.EQL,
						Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutantID)},
					},
					Body: &ast.BlockStmt{List: []ast.Stmt{mutStmt}},
					Else: &ast.BlockStmt{List: []ast.Stmt{orig}},
				},
			},
		}
	default:
		return mutated
	}
}

func wrapWithSchemataMulti(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	switch original.(type) {
	case ast.Expr:
		return wrapExpression(original, mutants, returnType, file)
	case ast.Stmt:
		return WrapStatement(original, mutants, returnType, file)
	default:
		return original
	}
}

func wrapExpression(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) != 1 {
		return original
	}

	originalExpr, ok := original.(ast.Expr)
	if !ok {
		return original
	}

	resultType := inferExprType(originalExpr, returnType)

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

	var typeExpr ast.Expr = &ast.Ident{Name: resultType}
	if strings.HasPrefix(resultType, "*") {
		baseType := strings.TrimPrefix(resultType, "*")
		if baseType != "" {
			typeExpr = &ast.StarExpr{X: &ast.Ident{Name: baseType}}
		}
	}

	return &ast.CallExpr{
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
					Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutant.ID)},
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
					Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutant.ID)},
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapDeferWithMutants(deferStmt, mutants, returnType, file)
}

func wrapDeferWithMutants(deferStmt *ast.DeferStmt, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	type mutPair struct {
		id   int
		stmt ast.Stmt
	}
	var pairs []mutPair
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
		pairs = append(pairs, mutPair{mutant.ID, mutatedStmt})
	}
	if len(pairs) == 0 {
		return deferStmt
	}

	var chain ast.Stmt = &ast.BlockStmt{List: []ast.Stmt{deferStmt}}
	for i := len(pairs) - 1; i >= 0; i-- {
		chain = &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(pairs[i].id)},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{pairs[i].stmt}},
			Else: chain,
		}
	}

	return &ast.BlockStmt{List: []ast.Stmt{chain}}
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			if retStmt, ok := mutated.(*ast.ReturnStmt); ok && len(retStmt.Results) > 0 {
				return wrapReturnWithSchemata(originalRet, retStmt.Results[0], mutant.ID, returnType)
			}
			return mutated
		}
		return original
	}

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

	if len(mutResults) == 1 {
		return wrapReturnWithSchemata(originalRet, mutResults[0].expr, mutResults[0].id, returnType)
	}

	origExpr := originalRet.Results[0]

	resultType := returnType
	if resultType == "" || resultType == "interface{}" {
		resultType = inferExprType(origExpr, returnType)
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
				Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mr.id)},
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

func wrapReturnWithSchemata(original *ast.ReturnStmt, mutatedExpr ast.Expr, mutantID int, returnType string) ast.Node {
	resultType := returnType
	if resultType == "" || resultType == "interface{}" {
		resultType = inferExprType(mutatedExpr, returnType)
	}
	var typeExpr ast.Expr = &ast.Ident{Name: resultType}
	if strings.HasPrefix(resultType, "*") {
		baseType := strings.TrimPrefix(resultType, "*")
		if baseType != "" {
			typeExpr = &ast.StarExpr{X: &ast.Ident{Name: baseType}}
		}
	}
	return &ast.ReturnStmt{
		Results: []ast.Expr{
			&ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Results: &ast.FieldList{
							List: []*ast.Field{{Type: typeExpr}},
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.IfStmt{
								Cond: &ast.BinaryExpr{
									X:  &ast.Ident{Name: "activeMutantID"},
									Op: token.EQL,
									Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(mutantID)},
								},
								Body: &ast.BlockStmt{
									List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{mutatedExpr}}},
								},
							},
							&ast.ReturnStmt{Results: original.Results},
						},
					},
				},
			},
		},
	}
}

func inferExprType(expr ast.Expr, siteReturnType string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		switch e.Name {
		case "true", "false":
			return "bool"
		case "nil":
			return "interface{}"
		case "int", "int8", "int16", "int32", "int64":
			return "int"
		case "uint", "uint8", "uint16", "uint32", "uint64":
			return "uint"
		case "float32", "float64":
			return "float64"
		case "string":
			return "string"
		case "bool":
			return "bool"
		case "byte":
			if siteReturnType != "" && siteReturnType != "interface{}" {
				return siteReturnType
			}
			return "byte"
		case "rune":
			if siteReturnType != "" && siteReturnType != "interface{}" {
				return siteReturnType
			}
			return "rune"
		default:
			if siteReturnType != "" && siteReturnType != "interface{}" {
				return siteReturnType
			}
			return "interface{}"
		}
	case *ast.BasicLit:
		switch e.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float64"
		case token.STRING:
			return "string"
		case token.CHAR:
			if siteReturnType == "byte" {
				return "byte"
			}
			return "rune"
		}
	case *ast.BinaryExpr:
		if isComparisonOp(e.Op) || isLogicalOp(e.Op) {
			return "bool"
		}
		return inferExprType(e.X, siteReturnType)
	case *ast.UnaryExpr:
		if e.Op == token.NOT {
			return "bool"
		}
		return inferExprType(e.X, siteReturnType)
	case *ast.CallExpr:
		if siteReturnType != "" && siteReturnType != "interface{}" {
			return siteReturnType
		}
		return "interface{}"
	case *ast.StarExpr:
		return "*" + inferExprType(e.X, siteReturnType)
	case *ast.IndexExpr:
		return inferExprType(e.X, siteReturnType)
	case *ast.SliceExpr:
		return inferExprType(e.X, siteReturnType)
	case *ast.SelectorExpr:
		return inferExprType(e.X, siteReturnType)
	case *ast.TypeAssertExpr:
		if e.Type != nil {
			return typeToString(e.Type)
		}
		return "interface{}"
	case *ast.CompositeLit:
		return typeToString(e.Type)
	case *ast.FuncLit:
		return "func"
	case *ast.ParenExpr:
		return inferExprType(e.X, siteReturnType)
	case *ast.KeyValueExpr:
		return inferExprType(e.Value, siteReturnType)
	}
	return "interface{}"
}

func typeToString(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + typeToString(e.X)
	case *ast.ArrayType:
		if e.Len == nil {
			return "[]" + typeToString(e.Elt)
		}
		return "[" + formatNode(e.Len) + "]" + typeToString(e.Elt)
	case *ast.MapType:
		return "map[" + typeToString(e.Key) + "]" + typeToString(e.Value)
	case *ast.ChanType:
		return "chan " + typeToString(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	default:
		return ""
	}
}

func formatNode(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	default:
		return ""
	}
}

func isLogicalOp(op token.Token) bool {
	switch op {
	case token.LAND, token.LOR:
		return true
	}
	return false
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return WrapStatement(original, mutants, returnType, file)
}

func HandleBasicLit(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
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
			return wrapWithSchemata(original, mutated, mutant.ID, returnType)
		}
		return original
	}

	return wrapWithSchemataMulti(original, mutants, returnType, file)
}

func WrapStatement(original ast.Node, mutants []MutantForSite, returnType string, file *ast.File) ast.Node {
	if len(mutants) == 0 {
		return original
	}

	originalStmt, ok := original.(ast.Stmt)
	if !ok {
		return original
	}

	type mutPair struct {
		id   int
		stmt ast.Stmt
	}
	var pairs []mutPair
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
		pairs = append(pairs, mutPair{mutant.ID, mutatedStmt})
	}
	if len(pairs) == 0 {
		return original
	}

	var chain ast.Stmt = &ast.BlockStmt{List: []ast.Stmt{originalStmt}}
	for i := len(pairs) - 1; i >= 0; i-- {
		chain = &ast.IfStmt{
			Cond: &ast.BinaryExpr{
				X:  &ast.Ident{Name: "activeMutantID"},
				Op: token.EQL,
				Y:  &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(pairs[i].id)},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{pairs[i].stmt}},
			Else: chain,
		}
	}

	return &ast.BlockStmt{List: []ast.Stmt{chain}}
}
